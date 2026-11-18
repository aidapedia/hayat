package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/aidapedia/hayat/internal/config"
)

// Store keeps the bytes of an image under a key. Keys are made by this
// package (<uuid>.<ext>), never by a caller, so no backend sees a path a
// person typed.
type Store interface {
	Name() string
	// Check proves the store can be written and read, so a role that serves
	// or keeps images fails at boot rather than at the first upload.
	Check(ctx context.Context) error
	Put(ctx context.Context, key, contentType string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

var keyRe = regexp.MustCompile(`^[0-9a-f-]{36}\.(png|jpg|webp)$`)

// ErrMissing is a key the store doesn't hold.
var ErrMissing = errors.New("media: object not found")

// NewStore builds the backend the configuration names. It touches nothing
// yet: only the roles that serve or keep images (auth, admin) Check it, so
// api and worker pods never need the volume.
func NewStore(cfg config.Storage) (Store, error) {
	switch cfg.Backend {
	case "local":
		if cfg.Dir == "" {
			return nil, errors.New("HAYAT_STORAGE=local needs HAYAT_STORAGE_DIR")
		}
		return localStore{dir: cfg.Dir}, nil
	case "s3":
		return newS3(cfg), nil
	}
	return nil, fmt.Errorf("HAYAT_STORAGE must be local or s3, got %q", cfg.Backend)
}

// ---------- local: a directory every pod mounts ----------

type localStore struct{ dir string }

func (localStore) Name() string { return "local" }

func (l localStore) Check(_ context.Context) error {
	if err := os.MkdirAll(l.dir, 0o750); err != nil {
		return fmt.Errorf("HAYAT_STORAGE_DIR %s: %w", l.dir, err)
	}
	f, err := os.CreateTemp(l.dir, ".check-*")
	if err != nil {
		return fmt.Errorf("HAYAT_STORAGE_DIR %s isn't writable by this process (a volume mounted there must be writable by uid 65532 in the image): %w", l.dir, err)
	}
	f.Close()
	return os.Remove(f.Name())
}

func (l localStore) path(key string) (string, error) {
	if !keyRe.MatchString(key) {
		return "", fmt.Errorf("media: bad key %q", key)
	}
	return filepath.Join(l.dir, key), nil
}

// Put writes to a temporary file and renames it, so a reader never sees half an image.
func (l localStore) Put(_ context.Context, key, _ string, data []byte) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(l.dir, ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func (l localStore) Get(_ context.Context, key string) ([]byte, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrMissing
	}
	return data, err
}

func (l localStore) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
