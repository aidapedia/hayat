package hayat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

// UserClaims is what a Hayat access token says about the person holding it.
// It never says what they may do: ask Check.
type UserClaims struct {
	Subject   string   `json:"sub"`
	SessionID string   `json:"sid"`
	ClientID  string   `json:"client_id"`
	Audience  audience `json:"aud"`
	Issuer    string   `json:"iss"`
	Expiry    int64    `json:"exp"`
	NotBefore int64    `json:"nbf"`
}

type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*a = []string{one}
		return nil
	}
	var many []string
	err := json.Unmarshal(b, &many)
	*a = many
	return err
}

var ErrInvalidToken = errors.New("hayat: invalid access token")

// VerifyAccessToken checks a user's access token offline: signature against
// the published keys, issuer, expiry, and that it was issued for audience.
// A sign-in token's `aud` names its client and the client's service, so pass
// your service's namespace ("billing") to accept any of its sign-in clients,
// or one client's id to accept only that client. Pass "" to accept any.
func (c *Client) VerifyAccessToken(ctx context.Context, raw, audience string) (*UserClaims, error) {
	sig, err := jose.ParseSigned(raw, []jose.SignatureAlgorithm{jose.ES256, jose.RS256, jose.EdDSA})
	if err != nil || len(sig.Signatures) != 1 {
		return nil, ErrInvalidToken
	}
	key, err := c.keys.key(ctx, sig.Signatures[0].Header.KeyID)
	if err != nil {
		return nil, err
	}
	payload, err := sig.Verify(key)
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims UserClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	now := time.Now().Unix()
	switch {
	case claims.Issuer != c.keys.issuer, claims.Expiry < now, claims.NotBefore > now+60:
		return nil, ErrInvalidToken
	case strings.HasPrefix(claims.Subject, "client:"):
		return nil, ErrInvalidToken // a service's token, not a person's
	case audience != "" && claims.ClientID != audience && !slices.Contains(claims.Audience, audience):
		return nil, ErrInvalidToken
	}
	return &claims, nil
}

// Middleware verifies the bearer token on each request and puts the claims in
// the context (read them with ClaimsFrom). Requests without a valid token get
// 401. audience is as for VerifyAccessToken: your service's namespace, or one
// sign-in client's id.
func (c *Client) Middleware(audience string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok {
				http.Error(w, "sign in required", http.StatusUnauthorized)
				return
			}
			claims, err := c.VerifyAccessToken(r.Context(), raw, audience)
			if err != nil {
				http.Error(w, "sign in required", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims)))
		})
	}
}

type claimsKey struct{}

func ClaimsFrom(ctx context.Context) (*UserClaims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*UserClaims)
	return c, ok
}

// keySet caches the JWKS and re-fetches it when a token names a key it hasn't
// seen (Hayat publishes the next key ahead of rotation, so that is rare).
type keySet struct {
	url, issuer string
	hc          *http.Client

	mu      sync.Mutex
	keys    jose.JSONWebKeySet
	fetched time.Time
}

func (k *keySet) key(ctx context.Context, kid string) (any, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if found := k.keys.Key(kid); len(found) > 0 && time.Since(k.fetched) < time.Hour {
		return found[0].Key, nil
	}
	if time.Since(k.fetched) > 10*time.Second {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, k.url, nil)
		resp, err := k.hc.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var set jose.JSONWebKeySet
		if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
			return nil, err
		}
		k.keys, k.fetched = set, time.Now()
	}
	if found := k.keys.Key(kid); len(found) > 0 {
		return found[0].Key, nil
	}
	return nil, ErrInvalidToken
}
