// Package adminserver is the `admin` role: the HTTP/JSON API the console
// calls, generated from proto/hayat/admin/v1 and served with Connect.
//
// Every request carries a Hayat access token issued to the console client.
// The token is verified offline against the published keys, then its session
// is checked live, so signing out or disabling an account cuts the console off
// at once rather than when the token expires. AccountService is for any
// signed-in user; every other service needs an instance admin.
package adminserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/rs/cors"
	zoidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/aidapedia/hayat/gen/go/hayat/admin/v1/adminv1connect"
	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/identity"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tenancy"
	"github.com/aidapedia/hayat/internal/tree"
)

// Version is set at build time with -ldflags.
var Version = "dev"

type Server struct {
	cfg      config.Config
	store    *store.Store
	ids      *identity.Service
	tenants  *tenancy.Service
	clients  *hoidc.Clients
	keys     *hoidc.KeyRing
	cat      *catalog.Service
	tree     *tree.Service
	engine   *authz.Engine
	verifier *op.AccessTokenVerifier
	log      *slog.Logger
}

// Deps is everything the admin API reads and writes through.
type Deps struct {
	Store   *store.Store
	IDs     *identity.Service
	Tenants *tenancy.Service
	Clients *hoidc.Clients
	Keys    *hoidc.KeyRing
	Storage *hoidc.Storage
	Catalog *catalog.Service
	Tree    *tree.Service
	Engine  *authz.Engine
}

func New(cfg config.Config, d Deps, log *slog.Logger) *Server {
	return &Server{
		cfg: cfg, store: d.Store, ids: d.IDs, tenants: d.Tenants, clients: d.Clients, keys: d.Keys,
		cat: d.Catalog, tree: d.Tree, engine: d.Engine, log: log,
		verifier: op.NewAccessTokenVerifier(cfg.Issuer, &op.OpenIDKeySet{Storage: d.Storage}),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	user := connect.WithInterceptors(s.authenticate(false))
	admin := connect.WithInterceptors(s.authenticate(true))

	mux.Handle(adminv1connect.NewAccountServiceHandler(&accountService{s}, user))
	mux.Handle(adminv1connect.NewOverviewServiceHandler(&overviewService{s}, admin))
	mux.Handle(adminv1connect.NewUserServiceHandler(&userService{s}, admin))
	mux.Handle(adminv1connect.NewTenantServiceHandler(&tenantService{s}, admin))
	mux.Handle(adminv1connect.NewClientServiceHandler(&clientService{s}, admin))
	mux.Handle(adminv1connect.NewKeyServiceHandler(&keyService{s}, admin))
	mux.Handle(adminv1connect.NewAuditServiceHandler(&auditService{s}, admin))
	// Policy calls are allowed by permission: each handler checks its own.
	mux.Handle(adminv1connect.NewCatalogServiceHandler(&catalogService{s}, user))
	mux.Handle(adminv1connect.NewRegistryServiceHandler(&registryService{s}, user))
	mux.Handle(adminv1connect.NewTreeAdminServiceHandler(&treeAdminService{s}, user))

	// What the console needs to start its sign-in: where Hayat is and which
	// client it is. Public on purpose; nothing here is secret.
	mux.HandleFunc("GET /config.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": s.cfg.Issuer, "clientId": hoidc.ConsoleClientID, "region": s.cfg.Region, "version": Version,
		})
	})

	// Normally the console reaches this API through its own origin (nginx or
	// the Vite proxy). CORS covers a console served from HAYAT_CONSOLE_URL
	// directly.
	c := cors.New(cors.Options{
		AllowedOrigins: []string{s.cfg.ConsoleURL},
		AllowedMethods: []string{http.MethodGet, http.MethodPost},
		AllowedHeaders: []string{"Authorization", "Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms"},
		MaxAge:         600,
	})
	return c.Handler(mux)
}

// ---------- Authentication ----------

type principal struct {
	User      identity.User
	SessionID uuid.UUID
}

type principalKey struct{}

func caller(ctx context.Context) principal { return ctx.Value(principalKey{}).(principal) }

func actor(ctx context.Context) string { return audit.UserActor(caller(ctx).User.ID) }

func (s *Server) authenticate(requireAdmin bool) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			p, err := s.principal(ctx, req.Header().Get("Authorization"))
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("sign in again: your session has ended"))
			}
			if requireAdmin && !p.User.IsAdmin() {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only an instance admin can do this"))
			}
			return next(context.WithValue(ctx, principalKey{}, p), req)
		}
	}
}

func (s *Server) principal(ctx context.Context, header string) (principal, error) {
	raw, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || raw == "" {
		return principal{}, errors.New("no bearer token")
	}
	claims, err := op.VerifyAccessToken[*zoidc.AccessTokenClaims](ctx, raw, s.verifier)
	if err != nil {
		return principal{}, err
	}
	// Only tokens issued to the console are accepted here; a token another
	// application received must not open the admin API.
	if claims.ClientID != hoidc.ConsoleClientID && !slices.Contains(claims.Audience, hoidc.ConsoleClientID) {
		return principal{}, errors.New("token was not issued to the console")
	}
	uid, err := uuid.Parse(claims.Subject)
	if err != nil {
		return principal{}, err
	}
	sidRaw, _ := claims.Claims["sid"].(string)
	sid, err := uuid.Parse(sidRaw)
	if err != nil {
		return principal{}, errors.New("token has no session")
	}
	u, err := s.ids.GetUser(ctx, uid)
	if err != nil || u.Status != identity.StatusActive {
		return principal{}, errors.New("account is not active")
	}
	sess, err := s.ids.SessionByID(ctx, u.Region, sid)
	if err != nil || !sess.Active() || sess.UserID != u.ID {
		return principal{}, errors.New("session has ended")
	}
	return principal{User: u, SessionID: sid}, nil
}

// ---------- Errors ----------

// fail turns a module error into a Connect error: user-facing messages pass
// through, a missing row is NotFound, anything else is logged and hidden.
func (s *Server) fail(err error) error {
	switch {
	case err == nil:
		return nil
	case errs.Is(err):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, store.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, errors.New("not found"))
	}
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce
	}
	s.log.Error("admin api", "err", err)
	return connect.NewError(connect.CodeInternal, errors.New("something went wrong on the server"))
}

func invalid(msg string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
}

func parseID(raw, what string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, invalid(what + " is not a valid id")
	}
	return id, nil
}
