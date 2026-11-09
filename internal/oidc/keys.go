package oidc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/jackc/pgx/v5"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/store"
)

// Signing keys are ES256 (ECDSA P-256), the algorithm every OIDC client
// library accepts. Each key moves next → active → retired:
//
//	next     published in JWKS before it signs, so verifiers already cache it
//	active   signs every new token
//	retired  still published for retiredOverlap, so tokens it signed stay valid
//
// Private keys are stored sealed with the master key.
const (
	keyAlg         = jose.ES256
	retiredOverlap = 48 * time.Hour
	keyCacheTTL    = 30 * time.Second
)

type Key struct {
	ID          string
	Status      string
	CreatedAt   time.Time
	ActivatedAt *time.Time
	RetiredAt   *time.Time
	private     *ecdsa.PrivateKey
}

// signingKey and publicKey adapt Key to the op package's interfaces.
type signingKey struct{ k *Key }

func (s signingKey) SignatureAlgorithm() jose.SignatureAlgorithm { return keyAlg }
func (s signingKey) Key() any                                    { return s.k.private }
func (s signingKey) ID() string                                  { return s.k.ID }

type publicKey struct{ k *Key }

func (p publicKey) ID() string                         { return p.k.ID }
func (p publicKey) Algorithm() jose.SignatureAlgorithm { return keyAlg }
func (p publicKey) Use() string                        { return "sig" }
func (p publicKey) Key() any                           { return &p.k.private.PublicKey }

// KeyRing loads keys from the database and caches them briefly, so a rotation
// on one pod reaches every pod within keyCacheTTL.
type KeyRing struct {
	store *store.Store
	box   *crypto.Box

	mu       sync.Mutex
	loadedAt time.Time
	keys     []*Key
}

func NewKeyRing(s *store.Store, box *crypto.Box) *KeyRing { return &KeyRing{store: s, box: box} }

func (r *KeyRing) load(ctx context.Context) ([]*Key, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.keys != nil && time.Since(r.loadedAt) < keyCacheTTL {
		return r.keys, nil
	}
	rows, err := r.store.Pool.Query(ctx, `SELECT kid, status, private_key_enc, created_at, activated_at, retired_at
		FROM signing_keys WHERE status <> 'retired' OR retired_at > $1 ORDER BY created_at`, time.Now().Add(-retiredOverlap))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []*Key
	for rows.Next() {
		var k Key
		var sealed []byte
		if err := rows.Scan(&k.ID, &k.Status, &sealed, &k.CreatedAt, &k.ActivatedAt, &k.RetiredAt); err != nil {
			return nil, err
		}
		der, err := r.box.Open(sealed)
		if err != nil {
			return nil, errors.New("oidc: cannot decrypt a signing key; is HAYAT_MASTER_KEY the one these keys were made with?")
		}
		if k.private, err = x509.ParseECPrivateKey(der); err != nil {
			return nil, err
		}
		keys = append(keys, &k)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	r.keys, r.loadedAt = keys, time.Now()
	return keys, nil
}

// Invalidate drops the cache, after this pod rotated keys.
func (r *KeyRing) Invalidate() {
	r.mu.Lock()
	r.keys = nil
	r.mu.Unlock()
}

func (r *KeyRing) Active(ctx context.Context) (op.SigningKey, error) {
	keys, err := r.load(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if k.Status == "active" {
			return signingKey{k}, nil
		}
	}
	return nil, errors.New("oidc: no active signing key")
}

// Published is every key verifiers should accept: next, active, and recently
// retired.
func (r *KeyRing) Published(ctx context.Context) ([]op.Key, error) {
	keys, err := r.load(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]op.Key, 0, len(keys))
	for _, k := range keys {
		out = append(out, publicKey{k})
	}
	return out, nil
}

// List returns every key, retired ones included, for the console.
func (r *KeyRing) List(ctx context.Context) ([]Key, error) {
	rows, err := r.store.Pool.Query(ctx, `SELECT kid, status, created_at, activated_at, retired_at FROM signing_keys ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Key
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.ID, &k.Status, &k.CreatedAt, &k.ActivatedAt, &k.RetiredAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *KeyRing) newKey(ctx context.Context, tx pgx.Tx, status string) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	der, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return err
	}
	var activated any
	if status == "active" {
		activated = time.Now()
	}
	_, err = tx.Exec(ctx, `INSERT INTO signing_keys (kid, alg, private_key_enc, status, activated_at) VALUES ($1, $2, $3, $4, $5)`,
		crypto.NewToken()[:16], string(keyAlg), r.box.Seal(der), status, activated)
	return err
}

// lock serialises key changes across pods with a row lock, which works the same
// on PostgreSQL and YugabyteDB.
func lock(ctx context.Context, tx pgx.Tx, name string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO instance_settings (key, value) VALUES ($1, '{}') ON CONFLICT (key) DO NOTHING`, name); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `SELECT 1 FROM instance_settings WHERE key = $1 FOR UPDATE`, name)
	return err
}

// Ensure makes sure an active and a next key exist. Every auth pod calls it at
// start-up; the lock keeps them from each creating keys.
func (r *KeyRing) Ensure(ctx context.Context) error {
	defer r.Invalidate()
	return r.store.Tx(ctx, func(tx pgx.Tx) error {
		if err := lock(ctx, tx, "lock:signing_keys"); err != nil {
			return err
		}
		for _, status := range []string{"active", "next"} {
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM signing_keys WHERE status = $1`, status).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				if err := r.newKey(ctx, tx, status); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Rotate promotes the next key to active, retires the active one and makes a
// new next key.
func (r *KeyRing) Rotate(ctx context.Context, actor string) error {
	defer r.Invalidate()
	return r.store.Tx(ctx, func(tx pgx.Tx) error {
		if err := lock(ctx, tx, "lock:signing_keys"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE signing_keys SET status = 'retired', retired_at = now() WHERE status = 'active'`); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE signing_keys SET status = 'active', activated_at = now() WHERE status = 'next'`)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			if err := r.newKey(ctx, tx, "active"); err != nil {
				return err
			}
		}
		if err := r.newKey(ctx, tx, "next"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, r.store.Region, actor, "keys.rotated", "signing_keys", nil)
	})
}

// RotateIfDue rotates when the active key is older than every. The worker calls
// it on a schedule.
func (r *KeyRing) RotateIfDue(ctx context.Context, every time.Duration) (bool, error) {
	var activated *time.Time
	err := r.store.Pool.QueryRow(ctx, `SELECT activated_at FROM signing_keys WHERE status = 'active' LIMIT 1`).Scan(&activated)
	if err != nil {
		return false, store.NotFound(err)
	}
	if activated == nil || time.Since(*activated) < every {
		return false, nil
	}
	return true, r.Rotate(ctx, audit.System)
}
