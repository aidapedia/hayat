package oidc

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/store"
)

const (
	authRequestTTL = 15 * time.Minute
	codeTTL        = 2 * time.Minute
)

// AuthRequest is one /authorize round trip, from the client's redirect to the
// code exchange. It implements op.AuthRequest. Auth requests live in the region
// that served the browser.
type AuthRequest struct {
	ID                  uuid.UUID
	Region              string
	ClientID            string
	RedirectURI         string
	Scopes              []string
	State               string
	Nonce               string
	ResponseType        string
	ResponseMode        string
	CodeChallenge       string
	CodeChallengeMethod string
	Prompt              []string
	MaxAge              *int32
	LoginHint           string
	UserID              *uuid.UUID
	SessionID           *uuid.UUID
	AuthTime            *time.Time
	AMR                 []string
	IsDone              bool
	CreatedAt           time.Time
	ExpiresAt           time.Time
	// ServiceID is the service the client belongs to. It is not stored with
	// the request: Storage looks it up from the client whenever it loads one.
	ServiceID string
}

func (a *AuthRequest) GetID() string    { return a.ID.String() }
func (a *AuthRequest) GetACR() string   { return "" }
func (a *AuthRequest) GetAMR() []string { return a.AMR }

// GetAudience names the client and the service it belongs to, so a service
// can accept a token from any of its sign-in clients by checking its own
// namespace in `aud`. The ID token's `azp` still names the client.
func (a *AuthRequest) GetAudience() []string {
	if a.ServiceID == "" || a.ServiceID == a.ClientID {
		return []string{a.ClientID}
	}
	return []string{a.ClientID, a.ServiceID}
}
func (a *AuthRequest) GetAuthTime() time.Time {
	if a.AuthTime == nil {
		return time.Time{}
	}
	return *a.AuthTime
}
func (a *AuthRequest) GetClientID() string { return a.ClientID }
func (a *AuthRequest) GetCodeChallenge() *oidc.CodeChallenge {
	if a.CodeChallenge == "" {
		return nil
	}
	return &oidc.CodeChallenge{Challenge: a.CodeChallenge, Method: oidc.CodeChallengeMethod(a.CodeChallengeMethod)}
}
func (a *AuthRequest) GetNonce() string                   { return a.Nonce }
func (a *AuthRequest) GetRedirectURI() string             { return a.RedirectURI }
func (a *AuthRequest) GetResponseType() oidc.ResponseType { return oidc.ResponseType(a.ResponseType) }
func (a *AuthRequest) GetResponseMode() oidc.ResponseMode { return oidc.ResponseMode(a.ResponseMode) }
func (a *AuthRequest) GetScopes() []string                { return a.Scopes }
func (a *AuthRequest) GetState() string                   { return a.State }
func (a *AuthRequest) GetSubject() string {
	if a.UserID == nil {
		return ""
	}
	return a.UserID.String()
}
func (a *AuthRequest) Done() bool { return a.IsDone }

// PromptNone, PromptLogin and NeedsFreshLogin tell the login page how to behave.
func (a *AuthRequest) PromptNone() bool  { return slices.Contains(a.Prompt, oidc.PromptNone) }
func (a *AuthRequest) PromptLogin() bool { return slices.Contains(a.Prompt, oidc.PromptLogin) }

// TooOld reports whether a session that signed in at authTime is older than the
// client's max_age allows.
func (a *AuthRequest) TooOld(authTime time.Time) bool {
	return a.MaxAge != nil && time.Since(authTime) > time.Duration(*a.MaxAge)*time.Second
}

const authRequestColumns = `id, region, client_id, redirect_uri, scopes, state, nonce, response_type, response_mode,
	code_challenge, code_challenge_method, prompt, max_age, login_hint, user_id, session_id, auth_time, amr, done, created_at, expires_at`

func scanAuthRequest(row pgx.Row) (*AuthRequest, error) {
	a := &AuthRequest{}
	err := row.Scan(&a.ID, &a.Region, &a.ClientID, &a.RedirectURI, &a.Scopes, &a.State, &a.Nonce, &a.ResponseType,
		&a.ResponseMode, &a.CodeChallenge, &a.CodeChallengeMethod, &a.Prompt, &a.MaxAge, &a.LoginHint, &a.UserID,
		&a.SessionID, &a.AuthTime, &a.AMR, &a.IsDone, &a.CreatedAt, &a.ExpiresAt)
	return a, store.NotFound(err)
}

type requests struct{ store *store.Store }

func (r requests) create(ctx context.Context, req *oidc.AuthRequest) (*AuthRequest, error) {
	var maxAge *int32
	if req.MaxAge != nil {
		v := int32(*req.MaxAge)
		maxAge = &v
	}
	return scanAuthRequest(r.store.Pool.QueryRow(ctx, `INSERT INTO oidc_auth_requests
		(region, id, client_id, redirect_uri, scopes, state, nonce, response_type, response_mode,
		 code_challenge, code_challenge_method, prompt, max_age, login_hint, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING `+authRequestColumns,
		r.store.Region, store.NewID(), req.ClientID, req.RedirectURI, arr(req.Scopes), req.State, req.Nonce,
		string(req.ResponseType), string(req.ResponseMode), req.CodeChallenge, string(req.CodeChallengeMethod),
		arr(req.Prompt), maxAge, req.LoginHint, time.Now().Add(authRequestTTL)))
}

func (r requests) byID(ctx context.Context, id string) (*AuthRequest, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, store.ErrNotFound
	}
	a, err := scanAuthRequest(r.store.Pool.QueryRow(ctx, `SELECT `+authRequestColumns+` FROM oidc_auth_requests
		WHERE region = $1 AND id = $2 AND expires_at > now()`, r.store.Region, uid))
	return a, err
}

// complete records who signed in, which lets the library issue a code.
func (r requests) complete(ctx context.Context, id uuid.UUID, userID, sessionID uuid.UUID, authTime time.Time, amr []string) error {
	tag, err := r.store.Pool.Exec(ctx, `UPDATE oidc_auth_requests SET user_id = $3, session_id = $4, auth_time = $5, amr = $6, done = true
		WHERE region = $1 AND id = $2 AND expires_at > now()`, r.store.Region, id, userID, sessionID, authTime, arr(amr))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r requests) saveCode(ctx context.Context, id, code string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	_, err = r.store.Pool.Exec(ctx, `INSERT INTO oidc_code_refs (code_hash, region, request_id, expires_at) VALUES ($1, $2, $3, $4)`,
		crypto.HashToken(code), r.store.Region, uid, time.Now().Add(codeTTL))
	return err
}

func (r requests) byCode(ctx context.Context, code string) (*AuthRequest, error) {
	var region string
	var id uuid.UUID
	err := r.store.Pool.QueryRow(ctx, `SELECT region, request_id FROM oidc_code_refs WHERE code_hash = $1 AND expires_at > now()`,
		crypto.HashToken(code)).Scan(&region, &id)
	if err != nil {
		return nil, store.NotFound(err)
	}
	return scanAuthRequest(r.store.Pool.QueryRow(ctx, `SELECT `+authRequestColumns+` FROM oidc_auth_requests
		WHERE region = $1 AND id = $2 AND done`, region, id))
}

// delete removes a used request and its code, so a code works exactly once.
func (r requests) delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil
	}
	return r.store.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM oidc_code_refs WHERE request_id = $1`, uid); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM oidc_auth_requests WHERE id = $1`, uid)
		return err
	})
}

// arr turns a nil slice into an empty one: pgx writes nil as NULL, and every
// array column here is NOT NULL.
func arr[S ~[]string](s S) []string {
	if s == nil {
		return []string{}
	}
	return []string(s)
}
