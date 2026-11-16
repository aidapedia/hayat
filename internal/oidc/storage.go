package oidc

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/store"
)

// Storage is what the zitadel/oidc provider reads and writes through. The
// protocol (validation, PKCE, token formats) is the library's; Storage only
// answers "who", "which client" and "is this token still good".
//
// Tokens:
//   - Access tokens are JWTs, verified offline by services. They are not
//     stored, and carry `sid` so a service can drop a revoked session.
//   - Refresh tokens are opaque "<region>.<token>", stored hashed, rotated on
//     every use. Presenting an already-rotated token revokes its whole family:
//     someone has a copy.
//   - A refresh token bound to a session dies with that session, so signing
//     out, a password reset or disabling the account ends it too.
type Storage struct {
	store    *store.Store
	ids      *identity.Service
	clients  *Clients
	keys     *KeyRing
	requests requests

	accessTTL  time.Duration
	refreshTTL time.Duration
}

var (
	_ op.Storage                        = (*Storage)(nil)
	_ op.ClientCredentialsStorage       = (*Storage)(nil)
	_ op.CanGetPrivateClaimsFromRequest = (*Storage)(nil)
)

func NewStorage(s *store.Store, ids *identity.Service, clients *Clients, keys *KeyRing, accessTTL, refreshTTL time.Duration) *Storage {
	return &Storage{store: s, ids: ids, clients: clients, keys: keys, requests: requests{store: s}, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

// ---------- Auth requests ----------

func (s *Storage) CreateAuthRequest(ctx context.Context, req *oidc.AuthRequest, _ string) (op.AuthRequest, error) {
	client, err := s.clients.Get(ctx, req.ClientID)
	if err != nil {
		return nil, oidc.ErrInvalidClient()
	}
	// PKCE is mandatory for public clients (OAuth 2.1); S256 is the only method
	// accepted from anyone.
	if req.CodeChallenge != "" && req.CodeChallengeMethod != oidc.CodeChallengeMethodS256 {
		return nil, oidc.ErrInvalidRequest().WithDescription("code_challenge_method must be S256")
	}
	if client.Kind == KindPublic && req.CodeChallenge == "" {
		return nil, oidc.ErrInvalidRequest().WithDescription("this client must use PKCE (code_challenge with S256)")
	}
	a, err := s.requests.create(ctx, req)
	if err != nil {
		return nil, err
	}
	a.ServiceID = client.ServiceID
	return a, nil
}

// withService fills in the service of the request's client, which the tokens'
// audience names. A client that is gone leaves it empty; the library refuses
// the request on its own when it looks the client up.
// Called as s.withService(ctx)(s.requests.byID(ctx, id)).
func (s *Storage) withService(ctx context.Context) func(*AuthRequest, error) (*AuthRequest, error) {
	return func(a *AuthRequest, err error) (*AuthRequest, error) {
		if err != nil {
			return nil, err
		}
		if c, cerr := s.clients.Get(ctx, a.ClientID); cerr == nil {
			a.ServiceID = c.ServiceID
		}
		return a, nil
	}
}

func (s *Storage) AuthRequestByID(ctx context.Context, id string) (op.AuthRequest, error) {
	a, err := s.withService(ctx)(s.requests.byID(ctx, id))
	if err != nil {
		return nil, oidc.ErrInvalidRequest().WithDescription("this sign-in request has expired; start again from the application")
	}
	return a, nil
}

// LoadAuthRequest is AuthRequestByID with the concrete type, for the login page.
func (s *Storage) LoadAuthRequest(ctx context.Context, id string) (*AuthRequest, error) {
	return s.withService(ctx)(s.requests.byID(ctx, id))
}

// CompleteAuthRequest records the signed-in session on the request.
func (s *Storage) CompleteAuthRequest(ctx context.Context, a *AuthRequest, sess identity.Session) error {
	return s.requests.complete(ctx, a.ID, sess.UserID, sess.ID, sess.AuthTime, sess.AMR)
}

func (s *Storage) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	a, err := s.withService(ctx)(s.requests.byCode(ctx, code))
	if err != nil {
		return nil, oidc.ErrInvalidGrant().WithDescription("the code is invalid or has expired")
	}
	return a, nil
}

func (s *Storage) SaveAuthCode(ctx context.Context, id, code string) error {
	return s.requests.saveCode(ctx, id, code)
}

func (s *Storage) DeleteAuthRequest(ctx context.Context, id string) error {
	return s.requests.delete(ctx, id)
}

// ---------- Tokens ----------

func (s *Storage) CreateAccessToken(ctx context.Context, _ op.TokenRequest) (string, time.Time, error) {
	return store.NewID().String(), time.Now().Add(s.accessTTL), nil
}

func (s *Storage) CreateAccessAndRefreshTokens(ctx context.Context, request op.TokenRequest, current string) (string, string, time.Time, error) {
	accessID := store.NewID().String()
	exp := time.Now().Add(s.accessTTL)

	if current == "" {
		a, ok := request.(*AuthRequest)
		if !ok {
			return "", "", time.Time{}, errors.New("oidc: refresh tokens are issued only from a sign-in")
		}
		token, err := s.insertRefresh(ctx, s.store.Pool, refreshRow{
			familyID:  store.NewID(),
			clientID:  a.ClientID,
			userID:    *a.UserID,
			sessionID: a.SessionID,
			scopes:    a.Scopes,
			audience:  a.GetAudience(),
			amr:       a.AMR,
			authTime:  a.GetAuthTime(),
		}, s.store.Region)
		return accessID, token, exp, err
	}

	prev, ok := request.(*RefreshTokenRequest)
	if !ok {
		return "", "", time.Time{}, errors.New("oidc: unexpected refresh request type")
	}
	var token string
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		// Rotate: the token being exchanged must still be unrotated, checked and
		// marked in one statement, so two concurrent uses can't both succeed.
		tag, err := tx.Exec(ctx, `UPDATE refresh_tokens SET rotated_at = now()
			WHERE region = $1 AND id = $2 AND rotated_at IS NULL AND revoked_at IS NULL`, prev.row.region, prev.row.id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return op.ErrInvalidRefreshToken
		}
		next := prev.row
		next.scopes = prev.scopes
		token, err = s.insertRefresh(ctx, tx, next, prev.row.region)
		return err
	})
	return accessID, token, exp, err
}

type refreshRow struct {
	region    string
	id        uuid.UUID
	familyID  uuid.UUID
	clientID  string
	userID    uuid.UUID
	sessionID *uuid.UUID
	scopes    []string
	audience  []string
	amr       []string
	authTime  time.Time
	expiresAt time.Time
	rotatedAt *time.Time
	revokedAt *time.Time
}

func (s *Storage) insertRefresh(ctx context.Context, db store.DB, r refreshRow, region string) (string, error) {
	token := crypto.NewToken()
	_, err := db.Exec(ctx, `INSERT INTO refresh_tokens
		(region, id, family_id, token_hash, client_id, user_id, session_id, scopes, audience, amr, auth_time, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		region, store.NewID(), r.familyID, crypto.HashToken(token), r.clientID, r.userID, r.sessionID,
		arr(r.scopes), arr(r.audience), arr(r.amr), r.authTime, time.Now().Add(s.refreshTTL))
	return region + "." + token, err
}

func (s *Storage) findRefresh(ctx context.Context, token string) (refreshRow, error) {
	region, raw, ok := strings.Cut(token, ".")
	if !ok || region == "" || raw == "" {
		return refreshRow{}, op.ErrInvalidRefreshToken
	}
	var r refreshRow
	err := s.store.Pool.QueryRow(ctx, `SELECT region, id, family_id, client_id, user_id, session_id, scopes, audience, amr,
		auth_time, expires_at, rotated_at, revoked_at FROM refresh_tokens WHERE region = $1 AND token_hash = $2`,
		region, crypto.HashToken(raw)).Scan(&r.region, &r.id, &r.familyID, &r.clientID, &r.userID, &r.sessionID,
		&r.scopes, &r.audience, &r.amr, &r.authTime, &r.expiresAt, &r.rotatedAt, &r.revokedAt)
	if err != nil {
		return refreshRow{}, op.ErrInvalidRefreshToken
	}
	return r, nil
}

func (s *Storage) revokeFamily(ctx context.Context, familyID uuid.UUID) error {
	_, err := s.store.Pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL`, familyID)
	return err
}

func (s *Storage) TokenRequestByRefreshToken(ctx context.Context, token string) (op.RefreshTokenRequest, error) {
	r, err := s.findRefresh(ctx, token)
	if err != nil {
		return nil, err
	}
	if r.rotatedAt != nil {
		// A rotated token came back: either the client lost track or someone
		// copied it. Either way nobody should keep this family.
		_ = s.revokeFamily(ctx, r.familyID)
		return nil, op.ErrInvalidRefreshToken
	}
	if r.revokedAt != nil || time.Now().After(r.expiresAt) {
		return nil, op.ErrInvalidRefreshToken
	}
	if client, err := s.clients.Get(ctx, r.clientID); err != nil || client.DisabledAt != nil {
		return nil, op.ErrInvalidRefreshToken
	}
	if r.sessionID != nil {
		sess, err := s.ids.SessionByID(ctx, r.region, *r.sessionID)
		if err != nil || !sess.Active() {
			return nil, op.ErrInvalidRefreshToken
		}
	}
	u, err := s.ids.GetUser(ctx, r.userID)
	if err != nil || u.Status != identity.StatusActive {
		return nil, op.ErrInvalidRefreshToken
	}
	return &RefreshTokenRequest{row: r, scopes: r.scopes}, nil
}

func (s *Storage) GetRefreshTokenInfo(ctx context.Context, clientID, token string) (string, string, error) {
	r, err := s.findRefresh(ctx, token)
	if err != nil || r.clientID != clientID {
		return "", "", op.ErrInvalidRefreshToken
	}
	return r.userID.String(), r.id.String(), nil
}

// RevokeToken handles /revoke. Refresh tokens are revoked with their family.
// Access tokens are short-lived JWTs that aren't stored; revoking one is a
// no-op, and ending its session is how a service cuts it off.
func (s *Storage) RevokeToken(ctx context.Context, tokenOrID, _ string, clientID string) *oidc.Error {
	r, err := s.findRefresh(ctx, tokenOrID)
	if err != nil {
		return nil
	}
	if r.clientID != clientID {
		return oidc.ErrInvalidClient().WithDescription("token was not issued for this client")
	}
	if err := s.revokeFamily(ctx, r.familyID); err != nil {
		return oidc.ErrServerError()
	}
	return nil
}

// TerminateSession handles RP-initiated logout: it ends the browser's Hayat
// session (the auth server puts its cookie in the context) and the client's
// refresh tokens for this user.
func (s *Storage) TerminateSession(ctx context.Context, userID, clientID string) error {
	if cookie, ok := ctx.Value(sessionCookieKey{}).(string); ok && cookie != "" {
		s.ids.EndSessionByCookie(ctx, cookie)
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil
	}
	_, err = s.store.Pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND client_id = $2 AND revoked_at IS NULL`, uid, clientID)
	return err
}

type sessionCookieKey struct{}

// WithSessionCookie lets the auth server hand the browser's session cookie to
// TerminateSession.
func WithSessionCookie(ctx context.Context, cookie string) context.Context {
	return context.WithValue(ctx, sessionCookieKey{}, cookie)
}

// ---------- Keys ----------

func (s *Storage) SigningKey(ctx context.Context) (op.SigningKey, error) { return s.keys.Active(ctx) }
func (s *Storage) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{keyAlg}, nil
}
func (s *Storage) KeySet(ctx context.Context) ([]op.Key, error) { return s.keys.Published(ctx) }

// ---------- Clients ----------

func (s *Storage) GetClientByClientID(ctx context.Context, id string) (op.Client, error) {
	c, err := s.clients.Get(ctx, id)
	if err != nil || c.DisabledAt != nil {
		return nil, oidc.ErrInvalidClient().WithDescription("unknown or disabled client")
	}
	return c, nil
}

func (s *Storage) AuthorizeClientIDSecret(ctx context.Context, id, secret string) error {
	c, err := s.clients.Get(ctx, id)
	if err != nil || c.DisabledAt != nil || !c.checkSecret(secret) {
		return oidc.ErrInvalidClient().WithDescription("client authentication failed")
	}
	return nil
}

func (s *Storage) ClientCredentials(ctx context.Context, id, secret string) (op.Client, error) {
	c, err := s.clients.Get(ctx, id)
	if err != nil || c.DisabledAt != nil || !c.checkSecret(secret) {
		return nil, oidc.ErrInvalidClient().WithDescription("client authentication failed")
	}
	if !slices.Contains(c.Grants, string(oidc.GrantTypeClientCredentials)) {
		return nil, oidc.ErrUnauthorizedClient().WithDescription("this client may not use client credentials")
	}
	return c, nil
}

func (s *Storage) ClientCredentialsTokenRequest(_ context.Context, id string, scopes []string) (op.TokenRequest, error) {
	return clientTokenRequest{clientID: id, scopes: scopes}, nil
}

type clientTokenRequest struct {
	clientID string
	scopes   []string
}

func (c clientTokenRequest) GetSubject() string    { return "client:" + c.clientID }
func (c clientTokenRequest) GetAudience() []string { return []string{c.clientID} }
func (c clientTokenRequest) GetScopes() []string   { return c.scopes }

// ---------- Claims ----------

// userClaims fills the standard claims of the scopes granted. Hayat has no
// consent screen in v1 (every client is registered by an admin), so the
// profile, email and phone scopes are honoured as requested.
func (s *Storage) userClaims(ctx context.Context, info *oidc.UserInfo, userID string, scopes []string) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return errors.New("oidc: subject is not a user")
	}
	u, err := s.ids.GetUser(ctx, uid)
	if err != nil {
		return err
	}
	if u.Status != identity.StatusActive {
		return errors.New("oidc: user is not active")
	}
	info.Subject = u.ID.String()
	for _, scope := range scopes {
		switch scope {
		case oidc.ScopeProfile:
			info.Name = u.Name
			info.Picture = u.AvatarURL
			info.Zoneinfo = u.Zoneinfo
			info.UpdatedAt = oidc.FromTime(u.UpdatedAt)
			if u.Locale != "" {
				info.Locale = oidc.NewLocale(parseLocale(u.Locale))
			}
		case oidc.ScopeEmail:
			info.Email = u.Email
			info.EmailVerified = oidc.Bool(u.EmailVerified)
		case oidc.ScopePhone:
			info.PhoneNumber = u.Phone
		}
	}
	return nil
}

func (s *Storage) SetUserinfoFromScopes(ctx context.Context, info *oidc.UserInfo, userID, _ string, scopes []string) error {
	return s.userClaims(ctx, info, userID, scopes)
}

// SetUserinfoFromToken serves /userinfo. Access tokens aren't stored, so the
// granted scopes aren't known here; the standard profile claims are returned.
func (s *Storage) SetUserinfoFromToken(ctx context.Context, info *oidc.UserInfo, _, subject, _ string) error {
	return s.userClaims(ctx, info, subject, []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail, oidc.ScopePhone})
}

func (s *Storage) SetIntrospectionFromToken(ctx context.Context, resp *oidc.IntrospectionResponse, _, subject, clientID string) error {
	if strings.HasPrefix(subject, "client:") {
		resp.Subject = subject
		resp.ClientID = clientID
		resp.Active = true
		return nil
	}
	var info oidc.UserInfo
	if err := s.userClaims(ctx, &info, subject, []string{oidc.ScopeOpenID, oidc.ScopeEmail}); err != nil {
		return err
	}
	resp.SetUserInfo(&info)
	resp.ClientID = clientID
	resp.Active = true
	return nil
}

func (s *Storage) GetPrivateClaimsFromScopes(context.Context, string, string, []string) (map[string]any, error) {
	return nil, nil
}

// GetPrivateClaimsFromRequest adds `sid` (the Hayat session) to access tokens,
// so a service can reject tokens of a session that was signed out.
func (s *Storage) GetPrivateClaimsFromRequest(_ context.Context, request op.TokenRequest, _ []string) (map[string]any, error) {
	var sid *uuid.UUID
	switch r := request.(type) {
	case *AuthRequest:
		sid = r.SessionID
	case *RefreshTokenRequest:
		sid = r.row.sessionID
	}
	if sid == nil {
		return nil, nil
	}
	return map[string]any{"sid": sid.String()}, nil
}

func (s *Storage) GetKeyByIDAndClientID(context.Context, string, string) (*jose.JSONWebKey, error) {
	return nil, errors.New("oidc: JWT client authentication is not supported")
}

func (s *Storage) ValidateJWTProfileScopes(context.Context, string, []string) ([]string, error) {
	return nil, errors.New("oidc: the JWT profile grant is not supported")
}

func (s *Storage) Health(ctx context.Context) error { return s.store.Pool.Ping(ctx) }

// RefreshTokenRequest implements op.RefreshTokenRequest.
type RefreshTokenRequest struct {
	row    refreshRow
	scopes []string
}

func (r *RefreshTokenRequest) GetAMR() []string       { return r.row.amr }
func (r *RefreshTokenRequest) GetAudience() []string  { return r.row.audience }
func (r *RefreshTokenRequest) GetAuthTime() time.Time { return r.row.authTime }
func (r *RefreshTokenRequest) GetClientID() string    { return r.row.clientID }
func (r *RefreshTokenRequest) GetScopes() []string    { return r.scopes }
func (r *RefreshTokenRequest) GetSubject() string     { return r.row.userID.String() }
func (r *RefreshTokenRequest) SetCurrentScopes(s []string) {
	r.scopes = s
}
