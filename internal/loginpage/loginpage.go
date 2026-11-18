// Package loginpage owns each service's login page: the draft its owners
// edit, the published design Hayat's sign-in pages render, every version ever
// published, and the logos they use.
//
// A service's owners design the page; Hayat draws it on its own sign-in
// domain. Nothing a service sets is markup or CSS (see Design), so the page
// can't be turned against the people signing in on it.
package loginpage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/media"
	"github.com/aidapedia/hayat/internal/store"
)

// MaxAssetBytes is the largest logo Hayat accepts; it is resized on the way in.
const MaxAssetBytes = media.MaxUploadBytes

// PreviewTTL is how long a preview link works.
const PreviewTTL = 30 * time.Minute

var (
	ErrNothingPublished = errs.New("this login page has never been published")
	ErrBadAsset         = media.ErrBadImage
	ErrUnknownAsset     = errs.New("that image isn't one of this service's uploads; upload it again")
	ErrPreviewExpired   = errs.New("this preview link has expired; open it again from the console")
)

// Page is one service's login page as its owners see it.
type Page struct {
	ServiceID   string
	Draft       Design
	Published   *Design
	Version     int
	PublishedBy string
	PublishedAt *time.Time
	UpdatedAt   time.Time
}

// HasChanges says whether publishing would change what people see.
func (p Page) HasChanges() bool {
	if p.Published == nil {
		return true
	}
	a, _ := json.Marshal(p.Draft)
	b, _ := json.Marshal(*p.Published)
	return string(a) != string(b)
}

// Version is one published design.
type Version struct {
	Version     int
	Design      Design
	PublishedBy string
	PublishedAt time.Time
}

type Service struct {
	store      *store.Store
	media      *media.Service
	previewKey []byte
}

// New takes the master key only to derive the key preview links are signed
// with. Logos are kept by m.
func New(s *store.Store, masterKey []byte, m *media.Service) *Service {
	mac := hmac.New(sha256.New, masterKey)
	mac.Write([]byte("hayat login page preview"))
	return &Service{store: s, media: m, previewKey: mac.Sum(nil)}
}

func owner(serviceID string) string { return media.Owner("login_page", serviceID) }

func decode(raw []byte) (Design, error) {
	var d Design
	if len(raw) == 0 || string(raw) == "null" {
		return d, nil
	}
	return d, json.Unmarshal(raw, &d)
}

func encode(d Design) string {
	b, _ := json.Marshal(d)
	return string(b)
}

func scanPage(row pgx.Row, id string) (Page, error) {
	p := Page{ServiceID: id}
	var draft, published []byte
	err := row.Scan(&draft, &published, &p.Version, &p.PublishedBy, &p.PublishedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil // never edited: an empty draft, nothing published
	}
	if err != nil {
		return Page{}, err
	}
	if p.Draft, err = decode(draft); err != nil {
		return Page{}, err
	}
	if published != nil {
		d, err := decode(published)
		if err != nil {
			return Page{}, err
		}
		p.Published = &d
	}
	return p, nil
}

const pageColumns = `draft, published, version, published_by, published_at, updated_at`

// Get is the service's page; a service that never edited one gets an empty draft.
func (s *Service) Get(ctx context.Context, serviceID string) (Page, error) {
	return scanPage(s.store.Pool.QueryRow(ctx, `SELECT `+pageColumns+` FROM login_pages WHERE service_id = $1`, serviceID), serviceID)
}

// Live is the published design people see, or nil when the service never
// published one and Hayat's own page shows instead.
func (s *Service) Live(ctx context.Context, serviceID string) (*Design, error) {
	var raw []byte
	err := s.store.Pool.QueryRow(ctx, `SELECT published FROM login_pages WHERE service_id = $1`, serviceID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && raw == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d, err := decode(raw)
	return &d, err
}

func (s *Service) checkAssets(ctx context.Context, serviceID string, d Design) error {
	for _, raw := range d.Assets() {
		id, err := uuid.Parse(raw)
		if err != nil {
			return ErrUnknownAsset
		}
		who, err := s.media.OwnerOf(ctx, id)
		if errors.Is(err, store.ErrNotFound) || (err == nil && who != owner(serviceID)) {
			return ErrUnknownAsset
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// SaveDraft replaces the draft. Nobody sees it until it is published, so it
// isn't audited; publishing is.
func (s *Service) SaveDraft(ctx context.Context, serviceID string, d Design) (Page, error) {
	d, err := d.Normalize()
	if err != nil {
		return Page{}, err
	}
	if err := s.checkAssets(ctx, serviceID, d); err != nil {
		return Page{}, err
	}
	if _, err := s.store.Pool.Exec(ctx, `INSERT INTO login_pages (service_id, draft) VALUES ($1, $2::jsonb)
		ON CONFLICT (service_id) DO UPDATE SET draft = EXCLUDED.draft, updated_at = now()`, serviceID, encode(d)); err != nil {
		return Page{}, err
	}
	return s.Get(ctx, serviceID)
}

// Publish makes the draft what people see, as a new version.
func (s *Service) Publish(ctx context.Context, serviceID, actor string) (Page, error) {
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO login_pages (service_id) VALUES ($1) ON CONFLICT (service_id) DO NOTHING`, serviceID); err != nil {
			return err
		}
		var raw []byte
		var version int
		if err := tx.QueryRow(ctx, `SELECT draft, version FROM login_pages WHERE service_id = $1 FOR UPDATE`, serviceID).Scan(&raw, &version); err != nil {
			return err
		}
		d, err := decode(raw)
		if err != nil {
			return err
		}
		// Checked again: a draft saved before a rule tightened must not go live unchecked.
		if d, err = d.Normalize(); err != nil {
			return err
		}
		if err := s.checkAssets(ctx, serviceID, d); err != nil {
			return err
		}
		version++
		if _, err := tx.Exec(ctx, `UPDATE login_pages SET published = $2::jsonb, version = $3, published_by = $4, published_at = now(), updated_at = now()
			WHERE service_id = $1`, serviceID, encode(d), version, actor); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO login_page_versions (service_id, version, design, published_by) VALUES ($1, $2, $3::jsonb, $4)`,
			serviceID, version, encode(d), actor); err != nil {
			return err
		}
		return audit.Write(ctx, tx, s.store.Region, actor, "login_page.published", "service:"+serviceID, map[string]any{"version": version})
	})
	if err != nil {
		return Page{}, err
	}
	return s.Get(ctx, serviceID)
}

// Discard throws the draft away: it goes back to what is published, or to
// Hayat's defaults when nothing is.
func (s *Service) Discard(ctx context.Context, serviceID, actor string) (Page, error) {
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE login_pages SET draft = COALESCE(published, '{}'::jsonb), updated_at = now() WHERE service_id = $1`, serviceID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, s.store.Region, actor, "login_page.draft_discarded", "service:"+serviceID, nil)
	})
	if err != nil {
		return Page{}, err
	}
	return s.Get(ctx, serviceID)
}

// Versions lists what was published, newest first.
func (s *Service) Versions(ctx context.Context, serviceID string) ([]Version, error) {
	rows, err := s.store.Pool.Query(ctx, `SELECT version, design, published_by, published_at FROM login_page_versions
		WHERE service_id = $1 ORDER BY version DESC LIMIT 100`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Version
	for rows.Next() {
		var v Version
		var raw []byte
		if err := rows.Scan(&v.Version, &raw, &v.PublishedBy, &v.PublishedAt); err != nil {
			return nil, err
		}
		if v.Design, err = decode(raw); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Restore copies an old version into the draft; publishing it is a new version.
func (s *Service) Restore(ctx context.Context, serviceID string, version int, actor string) (Page, error) {
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT design FROM login_page_versions WHERE service_id = $1 AND version = $2`, serviceID, version).Scan(&raw)
		if err != nil {
			return store.NotFound(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE login_pages SET draft = $2::jsonb, updated_at = now() WHERE service_id = $1`, serviceID, raw); err != nil {
			return err
		}
		return audit.Write(ctx, tx, s.store.Region, actor, "login_page.version_restored", "service:"+serviceID, map[string]any{"version": version})
	})
	if err != nil {
		return Page{}, err
	}
	return s.Get(ctx, serviceID)
}

// AddAsset keeps an uploaded logo, resized and re-encoded by media (so an
// SVG or anything else that could run script on the sign-in origin is refused).
func (s *Service) AddAsset(ctx context.Context, serviceID string, data []byte, actor string) (uuid.UUID, error) {
	md, err := s.media.Upload(ctx, media.Logo, owner(serviceID), data, actor)
	if err != nil {
		return uuid.Nil, err
	}
	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		return audit.Write(ctx, tx, s.store.Region, actor, "login_page.asset_uploaded", "service:"+serviceID, map[string]any{"asset": md.ID, "bytes": md.Bytes})
	})
	return md.ID, err
}

// Asset is an uploaded image and its type.
func (s *Service) Asset(ctx context.Context, id uuid.UUID) (string, []byte, error) {
	return s.media.Open(ctx, id)
}

// ---------- Preview links ----------

func (s *Service) sign(payload string) string {
	mac := hmac.New(sha256.New, s.previewKey)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// PreviewToken lets a browser see the service's draft for PreviewTTL. The
// console asks for it after checking the caller may manage the service.
func (s *Service) PreviewToken(serviceID string, now time.Time) string {
	payload := serviceID + "." + strconv.FormatInt(now.Add(PreviewTTL).Unix(), 10)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + s.sign(payload)
}

// CheckPreview answers which service a preview token is for.
func (s *Service) CheckPreview(token string, now time.Time) (string, error) {
	encoded, sig, ok := strings.Cut(token, ".")
	if !ok {
		return "", ErrPreviewExpired
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || !hmac.Equal([]byte(sig), []byte(s.sign(string(raw)))) {
		return "", ErrPreviewExpired
	}
	i := strings.LastIndexByte(string(raw), '.')
	if i < 0 {
		return "", ErrPreviewExpired
	}
	exp, err := strconv.ParseInt(string(raw[i+1:]), 10, 64)
	if err != nil || now.Unix() > exp {
		return "", ErrPreviewExpired
	}
	return string(raw[:i]), nil
}
