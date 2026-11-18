// Package media keeps uploaded images: people's avatars, and the logos of
// tenants, services and login pages. It owns the media table (what each
// image is and who it belongs to) and, through a Store, the bytes — in a
// directory (a volume the auth and admin roles mount) or an S3-compatible
// bucket (HAYAT_STORAGE). Never in the database: images would bloat its
// backups and, on YugabyteDB, be copied to every region.
//
// Every image is re-encoded on the way in (see prepare) and served by Hayat
// itself at <issuer>/media/<id>, so a private bucket works, the sign-in pages
// load nothing from another origin, and switching backends never changes a URL.
package media

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/store"
)

// Media is one kept image.
type Media struct {
	ID          uuid.UUID
	Kind        Kind
	Owner       string // user:<id> | tenant:<id> | service:<id> | login_page:<service>
	ContentType string
	Bytes       int
	Width       int
	Height      int
	CreatedAt   time.Time
}

type Service struct {
	store *store.Store
	blobs Store
	base  string
	log   *slog.Logger
}

// New keeps new images in blobs; issuer is the auth role's public URL,
// which serves them.
func New(s *store.Store, blobs Store, issuer string, log *slog.Logger) *Service {
	return &Service{store: s, blobs: blobs, base: strings.TrimRight(issuer, "/") + "/media/", log: log}
}

// Owner names who an image belongs to, for the media table.
func Owner(kind string, id string) string { return kind + ":" + id }

// Link is where the image with this id is served, or "" for no image. For
// callers that hold only an id and the issuer (converters, the service API).
func Link(issuer, id string) string {
	if id == "" {
		return ""
	}
	return strings.TrimRight(issuer, "/") + "/media/" + id
}

// URL is where an image is served.
func (m *Service) URL(id uuid.UUID) string { return m.base + id.String() }

// IDFromURL answers the image a URL of ours points at; false for anything
// else (an avatar a person's Google account brought, say).
func (m *Service) IDFromURL(u string) (uuid.UUID, bool) {
	rest, ok := strings.CutPrefix(u, m.base)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(rest)
	return id, err == nil
}

// Upload checks, resizes and keeps an image for owner.
func (m *Service) Upload(ctx context.Context, kind Kind, owner string, data []byte, actor string) (Media, error) {
	p, err := prepare(kind, data)
	if err != nil {
		return Media{}, err
	}
	id := store.NewID()
	key := id.String() + "." + p.ext
	if err := m.blobs.Put(ctx, key, p.contentType, p.data); err != nil {
		return Media{}, fmt.Errorf("keep image: %w", err)
	}
	_, err = m.store.Pool.Exec(ctx, `INSERT INTO media (id, kind, owner, backend, object_key, content_type, bytes, width, height, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, kind, owner, m.blobs.Name(), key, p.contentType, len(p.data), p.width, p.height, actor)
	if err != nil {
		_ = m.blobs.Delete(ctx, key)
		return Media{}, err
	}
	return Media{ID: id, Kind: kind, Owner: owner, ContentType: p.contentType, Bytes: len(p.data), Width: p.width, Height: p.height, CreatedAt: time.Now()}, nil
}

func (m *Service) row(ctx context.Context, id uuid.UUID) (owner, backend, key, contentType string, err error) {
	err = m.store.Pool.QueryRow(ctx, `SELECT owner, backend, object_key, content_type FROM media WHERE id = $1`, id).
		Scan(&owner, &backend, &key, &contentType)
	return owner, backend, key, contentType, store.NotFound(err)
}

// OwnerOf answers who an image belongs to.
func (m *Service) OwnerOf(ctx context.Context, id uuid.UUID) (string, error) {
	owner, _, _, _, err := m.row(ctx, id)
	return owner, err
}

func (m *Service) storeFor(backend string) (Store, error) {
	if backend == m.blobs.Name() {
		return m.blobs, nil
	}
	return nil, fmt.Errorf("image kept in %q storage, but HAYAT_STORAGE is %q", backend, m.blobs.Name())
}

// Check proves the store works; the auth and admin roles call it at boot.
func (m *Service) Check(ctx context.Context) error { return m.blobs.Check(ctx) }

// AdoptLoginPageAssets moves the login page logos 0007 kept in the database
// into the store, keeping their ids so published designs still point at
// them. The admin role runs it at boot; once they are moved it does nothing.
// It is the only reader of login_page_assets, which is dead otherwise.
func (m *Service) AdoptLoginPageAssets(ctx context.Context) (int, error) {
	rows, err := m.store.Pool.Query(ctx, `SELECT a.id, a.service_id, a.content_type, a.data, a.created_by, a.created_at
		FROM login_page_assets a WHERE NOT EXISTS (SELECT 1 FROM media m WHERE m.id = a.id)`)
	if err != nil {
		return 0, err
	}
	type legacy struct {
		id                    uuid.UUID
		service, kind, author string
		data                  []byte
		at                    time.Time
	}
	var todo []legacy
	for rows.Next() {
		var l legacy
		if err := rows.Scan(&l.id, &l.service, &l.kind, &l.data, &l.author, &l.at); err != nil {
			rows.Close()
			return 0, err
		}
		todo = append(todo, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, l := range todo {
		ext := map[string]string{"image/jpeg": "jpg", "image/webp": "webp"}[l.kind]
		if ext == "" {
			ext = "png"
		}
		key := l.id.String() + "." + ext
		if err := m.blobs.Put(ctx, key, l.kind, l.data); err != nil {
			return 0, err
		}
		if _, err := m.store.Pool.Exec(ctx, `INSERT INTO media (id, kind, owner, backend, object_key, content_type, bytes, created_by, created_at)
			VALUES ($1, 'logo', $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (id) DO NOTHING`,
			l.id, Owner("login_page", l.service), m.blobs.Name(), key, l.kind, len(l.data), l.author, l.at); err != nil {
			return 0, err
		}
	}
	return len(todo), nil
}

// Open reads an image back.
func (m *Service) Open(ctx context.Context, id uuid.UUID) (string, []byte, error) {
	_, backend, key, contentType, err := m.row(ctx, id)
	if err != nil {
		return "", nil, err
	}
	st, err := m.storeFor(backend)
	if err != nil {
		return "", nil, err
	}
	data, err := st.Get(ctx, key)
	if errors.Is(err, ErrMissing) {
		return "", nil, store.ErrNotFound
	}
	return contentType, data, err
}

// Delete forgets an image. The row goes first, so its URL stops working
// even if the backend fails to remove the bytes (which is then logged).
func (m *Service) Delete(ctx context.Context, id uuid.UUID) error {
	var backend, key string
	err := m.store.Pool.QueryRow(ctx, `DELETE FROM media WHERE id = $1 RETURNING backend, object_key`, id).Scan(&backend, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	st, err := m.storeFor(backend)
	if err == nil {
		err = st.Delete(ctx, key)
	}
	if err != nil && m.log != nil {
		m.log.Warn("media: image forgotten but its bytes remain", "id", id, "backend", backend, "key", key, "err", err)
	}
	return nil
}

// Replace keeps a new image for owner and forgets the one it replaces, when
// that one is ours. The caller stores the new id; old may be uuid.Nil.
func (m *Service) Replace(ctx context.Context, kind Kind, owner string, data []byte, old uuid.UUID, actor string) (Media, error) {
	md, err := m.Upload(ctx, kind, owner, data, actor)
	if err != nil {
		return Media{}, err
	}
	if old != uuid.Nil {
		_ = m.Delete(ctx, old)
	}
	return md, nil
}
