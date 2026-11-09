// Package oidc is Hayat's OpenID provider: the zitadel/oidc library speaking
// the protocol, over Hayat's own clients, keys, auth requests and tokens.
package oidc

import (
	"crypto/sha256"
	"log/slog"
	"strings"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
	"golang.org/x/text/language"
)

// Scopes Hayat understands. offline_access asks for a refresh token.
var SupportedScopes = []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail, oidc.ScopePhone, oidc.ScopeOfflineAccess}

// NewProvider builds the OpenID provider. masterKey seeds the key the library
// uses to encrypt its own opaque values; signing keys come from the KeyRing.
func NewProvider(issuer string, storage *Storage, masterKey []byte, insecure bool, log *slog.Logger) (*op.Provider, error) {
	cfg := &op.Config{
		CryptoKey:                sha256.Sum256(append([]byte("hayat-op-crypto:"), masterKey...)),
		CryptoKeyId:              "op1",
		DefaultLogoutRedirectURI: "/logged-out",
		CodeMethodS256:           true,
		AuthMethodPost:           true,
		GrantTypeRefreshToken:    true,
		SupportedScopes:          SupportedScopes,
		SupportedUILocales:       []language.Tag{language.English},
	}
	opts := []op.Option{op.WithLogger(log)}
	if insecure {
		opts = append(opts, op.WithAllowInsecure())
	}
	return op.NewOpenIDProvider(strings.TrimRight(issuer, "/"), cfg, storage, opts...)
}

func parseLocale(s string) language.Tag {
	tag, err := language.Parse(s)
	if err != nil {
		return language.Und
	}
	return tag
}
