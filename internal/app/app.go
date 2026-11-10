// Package app wires Hayat's modules into the roles a pod can run: auth, admin
// and worker. Each role is an independent server, so a deployment runs them
// as separate pods (the default) or all in one process (development).
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aidapedia/hayat/internal/adminserver"
	"github.com/aidapedia/hayat/internal/apiserver"
	"github.com/aidapedia/hayat/internal/authserver"
	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/mail"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/ratelimit"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tenancy"
	"github.com/aidapedia/hayat/internal/tree"
	"github.com/aidapedia/hayat/internal/worker"
)

const (
	RoleAuth   = "auth"
	RoleAdmin  = "admin"
	RoleAPI    = "api"
	RoleWorker = "worker"
)

var AllRoles = []string{RoleAuth, RoleAdmin, RoleAPI, RoleWorker}

type App struct {
	Cfg     config.Config
	Store   *store.Store
	IDs     *identity.Service
	Tenants *tenancy.Service
	Clients *hoidc.Clients
	Keys    *hoidc.KeyRing
	Storage *hoidc.Storage
	Revs    *revision.Reader
	Catalog *catalog.Service
	Tree    *tree.Service
	Engine  *authz.Engine
	Log     *slog.Logger
}

// New builds the modules over an already-migrated database.
func New(cfg config.Config, s *store.Store, log *slog.Logger) (*App, error) {
	box, err := crypto.NewBox(cfg.MasterKey)
	if err != nil {
		return nil, err
	}
	ids := identity.New(s, cfg.Issuer, cfg.SessionTTL)
	clients := hoidc.NewClients(s, cfg.Issuer, cfg.Dev)
	keys := hoidc.NewKeyRing(s, box)
	// Revisions are cached for a second: within that window Watch events and
	// at_least_rev carry freshness.
	revs := revision.NewReader(s, time.Second)
	cat := catalog.New(s, revs)
	return &App{
		Cfg: cfg, Store: s, IDs: ids, Tenants: tenancy.New(s), Clients: clients, Keys: keys, Log: log,
		Storage: hoidc.NewStorage(s, ids, clients, keys, cfg.AccessTokenTTL, cfg.RefreshTokenTTL),
		Revs:    revs, Catalog: cat, Tree: tree.New(s, cat, revs), Engine: authz.New(s, cat, ids, revs),
	}, nil
}

// Bootstrap makes sure signing keys and the console client exist. Safe to run
// from every pod at once.
func (a *App) Bootstrap(ctx context.Context) error {
	if err := a.Keys.Ensure(ctx); err != nil {
		return fmt.Errorf("signing keys: %w", err)
	}
	if err := a.Catalog.Bootstrap(ctx); err != nil {
		return fmt.Errorf("platform catalog: %w", err)
	}
	return a.Clients.EnsureConsole(ctx, a.Cfg.ConsoleRedirectURI(), a.Cfg.ConsoleURL)
}

func (a *App) AuthHandler(ctx context.Context) (http.Handler, func(), error) {
	limiter, closeLimiter, err := ratelimit.New(a.Cfg.RedisURL, a.Log)
	if err != nil {
		return nil, nil, err
	}
	provider, err := hoidc.NewProvider(a.Cfg.Issuer, a.Storage, a.Cfg.MasterKey, strings.HasPrefix(a.Cfg.Issuer, "http://"), a.Log)
	if err != nil {
		closeLimiter()
		return nil, nil, err
	}
	srv, err := authserver.New(a.Cfg, a.Store, a.IDs, a.Storage, a.Clients, provider, limiter, a.Log)
	if err != nil {
		closeLimiter()
		return nil, nil, err
	}
	if link, err := authserver.SetupLink(ctx, a.Store, a.IDs, a.Cfg.Issuer, false); err != nil {
		a.Log.Error("setup link", "err", err)
	} else if link != "" {
		a.Log.Warn("No admin yet. Open this link to create the first one (valid 24 hours): " + link)
	}
	return srv.Handler(), closeLimiter, nil
}

func (a *App) AdminHandler() http.Handler {
	return adminserver.New(a.Cfg, adminserver.Deps{Store: a.Store, IDs: a.IDs, Tenants: a.Tenants, Clients: a.Clients, Keys: a.Keys,
		Storage: a.Storage, Catalog: a.Catalog, Tree: a.Tree, Engine: a.Engine}, a.Log).Handler()
}

func (a *App) APIHandler() http.Handler {
	return apiserver.New(a.Cfg, a.Store, a.Catalog, a.Tree, a.Engine, a.IDs, a.Tenants, a.Storage, a.Log).Handler()
}

func (a *App) healthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.Store.Pool.Ping(ctx); err != nil {
			http.Error(w, "database unreachable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// Serve runs the given roles until ctx is cancelled, then shuts them down
// gracefully.
func (a *App) Serve(ctx context.Context, roles []string) error {
	if err := a.Bootstrap(ctx); err != nil {
		return err
	}
	type server struct {
		name string
		srv  *http.Server
	}
	var servers []server
	var cleanups []func()
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()

	var workers sync.WaitGroup
	wctx, stopWorkers := context.WithCancel(ctx)
	defer stopWorkers()

	for _, role := range roles {
		switch role {
		case RoleAuth:
			h, cleanup, err := a.AuthHandler(ctx)
			if err != nil {
				return err
			}
			cleanups = append(cleanups, cleanup)
			servers = append(servers, server{RoleAuth, newServer(a.Cfg.AuthAddr, h)})
		case RoleAdmin:
			servers = append(servers, server{RoleAdmin, newServer(a.Cfg.AdminAddr, a.AdminHandler())})
		case RoleAPI:
			srv := newServer(a.Cfg.APIAddr, a.APIHandler())
			srv.Protocols = apiserver.Protocols()
			servers = append(servers, server{RoleAPI, srv})
		case RoleWorker:
			sender, err := mail.NewSender(a.Cfg.SMTPURL, a.Cfg.MailFrom, a.Log)
			if err != nil {
				return err
			}
			w := worker.New(a.Cfg, a.Store, sender, a.Keys, a.Log)
			workers.Add(1)
			go func() { defer workers.Done(); w.Run(wctx) }()
		default:
			return fmt.Errorf("unknown role %q (want %s)", role, strings.Join(AllRoles, ", "))
		}
	}
	servers = append(servers, server{"health", newServer(a.Cfg.HealthAddr, a.healthHandler())})

	errc := make(chan error, len(servers))
	for _, s := range servers {
		a.Log.Info("listening", "role", s.name, "addr", s.srv.Addr)
		go func() {
			if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("%s: %w", s.name, err)
			}
		}()
	}
	a.Log.Info("hayat running", "roles", roles, "issuer", a.Cfg.Issuer, "region", a.Cfg.Region)

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errc:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, s := range servers {
		_ = s.srv.Shutdown(shutdown)
	}
	stopWorkers()
	workers.Wait()
	return runErr
}

func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
}
