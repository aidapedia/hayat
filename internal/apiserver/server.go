// Package apiserver is the `api` role: the gRPC API your services call to ask
// for decisions, mirror their resources into the tree, read user profiles,
// watch for policy changes and publish contracts.
//
// It is generated from proto/hayat/authz/v1 and served with Connect, which
// speaks gRPC, gRPC-Web and Connect on one port. Plain HTTP/2 (h2c) is
// accepted, for a mesh that terminates TLS in front of it.
//
// A caller is a service: it presents an access token from the client
// credentials grant, issued to the client its registration created.
package apiserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	zoidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/aidapedia/hayat/gen/go/hayat/authz/v1/authzv1connect"
	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/identity"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tree"
)

type Server struct {
	cfg      config.Config
	store    *store.Store
	cat      *catalog.Service
	tree     *tree.Service
	engine   *authz.Engine
	ids      *identity.Service
	verifier *op.AccessTokenVerifier
	log      *slog.Logger

	mu       sync.Mutex
	services map[string]cachedService
}

type cachedService struct {
	svc catalog.ServiceInfo
	at  time.Time
}

func New(cfg config.Config, s *store.Store, cat *catalog.Service, t *tree.Service, engine *authz.Engine, ids *identity.Service,
	storage *hoidc.Storage, log *slog.Logger) *Server {
	return &Server{cfg: cfg, store: s, cat: cat, tree: t, engine: engine, ids: ids, log: log,
		verifier: op.NewAccessTokenVerifier(cfg.Issuer, &op.OpenIDKeySet{Storage: storage}),
		services: map[string]cachedService{}}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	opts := connect.WithInterceptors(&serviceAuth{s})
	mux.Handle(authzv1connect.NewAuthzServiceHandler(&authzService{s}, opts))
	mux.Handle(authzv1connect.NewTreeServiceHandler(&treeService{s}, opts))
	mux.Handle(authzv1connect.NewDirectoryServiceHandler(&directoryService{s}, opts))
	mux.Handle(authzv1connect.NewWatchServiceHandler(&watchService{s}, opts))
	mux.Handle(authzv1connect.NewSchemaServiceHandler(&schemaService{s}, opts))
	return mux
}

// Protocols lets the API serve gRPC over plain HTTP/2 as well as HTTP/1.1.
func Protocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetHTTP2(true)
	p.SetUnencryptedHTTP2(true)
	return p
}

// ---------- Who is calling ----------

type serviceKey struct{}

func callerService(ctx context.Context) catalog.ServiceInfo {
	return ctx.Value(serviceKey{}).(catalog.ServiceInfo)
}

func (s *Server) authenticate(ctx context.Context, header string) (catalog.ServiceInfo, error) {
	raw, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || raw == "" {
		return catalog.ServiceInfo{}, errors.New("no bearer token")
	}
	claims, err := op.VerifyAccessToken[*zoidc.AccessTokenClaims](ctx, raw, s.verifier)
	if err != nil {
		return catalog.ServiceInfo{}, err
	}
	clientID, ok := strings.CutPrefix(claims.Subject, "client:")
	if !ok {
		return catalog.ServiceInfo{}, errors.New("the token belongs to a person, not a service")
	}
	s.mu.Lock()
	c, hit := s.services[clientID]
	s.mu.Unlock()
	if hit && time.Since(c.at) < 30*time.Second {
		return c.svc, nil
	}
	svc, err := s.cat.ServiceByClient(ctx, clientID)
	if err != nil {
		return catalog.ServiceInfo{}, errors.New("this client isn't a registered service")
	}
	s.mu.Lock()
	s.services[clientID] = cachedService{svc: svc, at: time.Now()}
	s.mu.Unlock()
	return svc, nil
}

type serviceAuth struct{ s *Server }

func (a *serviceAuth) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		svc, err := a.s.authenticate(ctx, req.Header().Get("Authorization"))
		if err != nil {
			return nil, connect.NewError(connect.CodeUnauthenticated, err)
		}
		return next(context.WithValue(ctx, serviceKey{}, svc), req)
	}
}

func (a *serviceAuth) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (a *serviceAuth) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		svc, err := a.s.authenticate(ctx, conn.RequestHeader().Get("Authorization"))
		if err != nil {
			return connect.NewError(connect.CodeUnauthenticated, err)
		}
		return next(context.WithValue(ctx, serviceKey{}, svc), conn)
	}
}

// ---------- Errors ----------

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
	s.log.Error("api", "err", err)
	return connect.NewError(connect.CodeInternal, errors.New("something went wrong on the server"))
}

func invalid(msg string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(msg)) }

func parseUser(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, invalid("user_id is not a valid id")
	}
	return id, nil
}
