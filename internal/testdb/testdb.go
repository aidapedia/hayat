// Package testdb gives tests a migrated, empty database.
//
// It starts one embedded PostgreSQL per test binary (downloaded on first use
// and cached) and creates a fresh database per test, so tests never see each
// other's rows. Every test package must run through Main, which stops that
// server afterwards: it is a separate process and outlives the test binary
// otherwise, holding shared memory until the machine runs out.
//
// Set HAYAT_TEST_DATABASE_URL to use a server you run instead, for example a
// YugabyteDB node with HAYAT_TEST_DATABASE_ENGINE=yugabyte.
package testdb

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/migrate"
	"github.com/aidapedia/hayat/internal/store"
)

var (
	once    sync.Once
	baseURL string
	engine  config.Engine
	initErr error
	counter int
	mu      sync.Mutex
	server  *embeddedpostgres.EmbeddedPostgres
	runtime string
)

// Main runs a package's tests and stops the embedded server they used.
//
//	func TestMain(m *testing.M) { os.Exit(testdb.Main(m)) }
func Main(m *testing.M) int {
	code := m.Run()
	if server != nil {
		_ = server.Stop()
	}
	if runtime != "" {
		_ = os.RemoveAll(runtime)
	}
	return code
}

func start() {
	engine = config.Engine(os.Getenv("HAYAT_TEST_DATABASE_ENGINE"))
	if engine == "" {
		engine = config.EnginePostgres
	}
	if u := os.Getenv("HAYAT_TEST_DATABASE_URL"); u != "" {
		baseURL = u
		return
	}
	port, err := freePort()
	if err != nil {
		initErr = err
		return
	}
	cache, _ := os.UserCacheDir()
	dir, err := os.MkdirTemp("", "hayat-pg-*")
	if err != nil {
		initErr = err
		return
	}
	runtime = dir
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V17).
		Port(uint32(port)).
		Username("hayat").Password("hayat").Database("hayat").
		RuntimePath(runtime).
		CachePath(filepath.Join(cache, "hayat", "embedded-postgres")).
		Logger(io.Discard))
	if err := pg.Start(); err != nil {
		initErr = fmt.Errorf("testdb: start embedded postgres: %w", err)
		return
	}
	server = pg
	baseURL = fmt.Sprintf("postgres://hayat:hayat@127.0.0.1:%d/hayat?sslmode=disable", port)
}

// New returns a store on a fresh, migrated database, closed when the test ends.
func New(t testing.TB) *store.Store {
	t.Helper()
	once.Do(start)
	if initErr != nil {
		t.Fatal(initErr)
	}
	ctx := context.Background()

	mu.Lock()
	counter++
	name := fmt.Sprintf("t_%d_%d", os.Getpid(), counter)
	mu.Unlock()

	admin, err := pgx.Connect(ctx, baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	admin.Close(ctx)

	s, err := store.Open(ctx, withDatabase(baseURL, name), "default")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := migrate.Run(ctx, s, engine, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	return s
}

// Engine is the engine the test database runs.
func Engine() config.Engine {
	once.Do(start)
	return engine
}

func withDatabase(raw, name string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Path = "/" + strings.TrimPrefix(name, "/")
	return u.String()
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
