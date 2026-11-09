// Package config is the only place Hayat reads the environment.
//
// Everything an operator can tune is resolved here once, at start-up, into a
// Config value that is passed down explicitly. No other package calls
// os.Getenv, so the full list of knobs is this file.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Engine names the database the store talks to. The SQL is shared; the
// migration history and a few index statements differ per engine.
type Engine string

const (
	EnginePostgres Engine = "postgres"
	EngineYugabyte Engine = "yugabyte"
)

type Config struct {
	// Region is this deployment's home region. Every regional row is written
	// with it. v1 runs one region, "default".
	Region string

	DatabaseURL string
	Engine      Engine

	// RedisURL is optional. Without it, rate limits are kept in process memory,
	// which is fine for a single auth pod and wrong for several.
	RedisURL string

	// Issuer is the public base URL of the auth role, e.g. https://auth.example.com.
	// It is the OIDC issuer and the origin of the session cookie.
	Issuer string

	// ConsoleURL is where the optional console is served. Hayat registers the
	// built-in console client with <ConsoleURL>/callback as its redirect URI.
	ConsoleURL string

	// MasterKey encrypts signing keys at rest (AES-256-GCM). 32 bytes, base64.
	MasterKey []byte

	AuthAddr   string
	APIAddr    string
	AdminAddr  string
	HealthAddr string

	SMTPURL  string // smtp://user:pass@host:port; empty logs mail instead of sending it
	MailFrom string

	SessionTTL      time.Duration
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	KeyRotation     time.Duration

	// Dev relaxes rules that only make sense in production: http redirect URIs
	// and non-Secure cookies. Never set it on a public deployment.
	Dev bool
}

// SecureCookies is true whenever the issuer is served over HTTPS.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.Issuer, "https://") }

// ConsoleRedirectURI is the redirect URI of the built-in console client.
func (c Config) ConsoleRedirectURI() string {
	return strings.TrimRight(c.ConsoleURL, "/") + "/callback"
}

// Load reads the environment. It fails on anything required that is missing or
// malformed, so a bad deployment stops at boot instead of misbehaving later.
func Load() (Config, error) {
	c := Config{
		Region:      env("HAYAT_REGION", "default"),
		DatabaseURL: os.Getenv("HAYAT_DATABASE_URL"),
		Engine:      Engine(env("HAYAT_DATABASE_ENGINE", string(EnginePostgres))),
		RedisURL:    os.Getenv("HAYAT_REDIS_URL"),
		Issuer:      strings.TrimRight(env("HAYAT_ISSUER", "http://localhost:8443"), "/"),
		ConsoleURL:  strings.TrimRight(env("HAYAT_CONSOLE_URL", "http://localhost:3000"), "/"),
		AuthAddr:    env("HAYAT_AUTH_ADDR", ":8443"),
		APIAddr:     env("HAYAT_API_ADDR", ":7443"),
		AdminAddr:   env("HAYAT_ADMIN_ADDR", ":8080"),
		HealthAddr:  env("HAYAT_HEALTH_ADDR", ":9090"),
		SMTPURL:     os.Getenv("HAYAT_SMTP_URL"),
		MailFrom:    env("HAYAT_MAIL_FROM", "Hayat <no-reply@localhost>"),
		Dev:         envBool("HAYAT_DEV", false),
	}

	var err error
	if c.SessionTTL, err = envDuration("HAYAT_SESSION_TTL", 14*24*time.Hour); err != nil {
		return c, err
	}
	if c.AccessTokenTTL, err = envDuration("HAYAT_ACCESS_TOKEN_TTL", 10*time.Minute); err != nil {
		return c, err
	}
	if c.RefreshTokenTTL, err = envDuration("HAYAT_REFRESH_TOKEN_TTL", 30*24*time.Hour); err != nil {
		return c, err
	}
	if c.KeyRotation, err = envDuration("HAYAT_KEY_ROTATION", 90*24*time.Hour); err != nil {
		return c, err
	}

	if c.DatabaseURL == "" {
		return c, errors.New("HAYAT_DATABASE_URL is required")
	}
	if c.Engine != EnginePostgres && c.Engine != EngineYugabyte {
		return c, fmt.Errorf("HAYAT_DATABASE_ENGINE must be postgres or yugabyte, got %q", c.Engine)
	}
	if _, err := url.ParseRequestURI(c.Issuer); err != nil {
		return c, fmt.Errorf("HAYAT_ISSUER is not a URL: %w", err)
	}
	if !c.Dev && !strings.HasPrefix(c.Issuer, "https://") {
		return c, errors.New("HAYAT_ISSUER must be https:// unless HAYAT_DEV=true")
	}
	if c.MasterKey, err = decodeKey(os.Getenv("HAYAT_MASTER_KEY")); err != nil {
		return c, err
	}
	return c, nil
}

func decodeKey(v string) ([]byte, error) {
	if v == "" {
		return nil, errors.New("HAYAT_MASTER_KEY is required (32 random bytes, base64; generate with: openssl rand -base64 32)")
	}
	key, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("HAYAT_MASTER_KEY is not base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("HAYAT_MASTER_KEY must decode to 32 bytes, got %d", len(key))
	}
	return key, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return v
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}
