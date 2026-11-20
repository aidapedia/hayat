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

	SessionTTL time.Duration
	// SessionIdleTimeout ends a session nobody has used for this long (no
	// page on Hayat, no token refresh, no console call). 0 turns it off.
	SessionIdleTimeout time.Duration
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	KeyRotation        time.Duration

	// BreachedPasswords is "hibp" (the default) or "off". With hibp, a new
	// password is checked against Have I Been Pwned's range API: only the first
	// five hex characters of its SHA-1 leave the server. A failed lookup lets
	// the password through.
	BreachedPasswords string

	// Turnstile puts Cloudflare's bot check on the sign-up and forgot-password
	// pages when both keys are set. It is the one thing those pages load from
	// another origin, so it is off unless the operator turns it on.
	TurnstileSiteKey   string
	TurnstileSecretKey string

	// Storage is where uploaded images (avatars, logos) are kept.
	Storage Storage

	// Dev relaxes rules that only make sense in production: http redirect URIs
	// and non-Secure cookies. Never set it on a public deployment.
	Dev bool
}

// Storage picks where uploaded images live. The auth role serves them and
// the admin role writes them, so both must see the same files: with "local"
// (the default) they mount the same directory — one volume in compose, a
// ReadWriteMany volume in Kubernetes; "s3" needs nothing shared.
type Storage struct {
	Backend string // local | s3
	Dir     string // local: where files are written

	S3Endpoint  string // https://s3.amazonaws.com, https://<account>.r2.cloudflarestorage.com, http://minio:9000
	S3Region    string
	S3Bucket    string
	S3Prefix    string // key prefix inside the bucket, e.g. "hayat/"
	S3AccessKey string
	S3SecretKey string
	// S3PathStyle addresses the bucket as <endpoint>/<bucket>/… instead of
	// <bucket>.<endpoint>/…; MinIO and most self-hosted stores need it.
	S3PathStyle bool
}

// SecureCookies is true whenever the issuer is served over HTTPS.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.Issuer, "https://") }

// Turnstile reports whether the bot check is on.
func (c Config) Turnstile() bool { return c.TurnstileSiteKey != "" }

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

		BreachedPasswords:  env("HAYAT_BREACHED_PASSWORDS", "hibp"),
		TurnstileSiteKey:   os.Getenv("HAYAT_TURNSTILE_SITE_KEY"),
		TurnstileSecretKey: os.Getenv("HAYAT_TURNSTILE_SECRET_KEY"),
		Storage: Storage{
			Backend:     env("HAYAT_STORAGE", "local"),
			Dir:         env("HAYAT_STORAGE_DIR", "/var/lib/hayat/media"),
			S3Endpoint:  strings.TrimRight(env("HAYAT_S3_ENDPOINT", "https://s3.amazonaws.com"), "/"),
			S3Region:    env("HAYAT_S3_REGION", "us-east-1"),
			S3Bucket:    os.Getenv("HAYAT_S3_BUCKET"),
			S3Prefix:    env("HAYAT_S3_PREFIX", "hayat/"),
			S3AccessKey: os.Getenv("HAYAT_S3_ACCESS_KEY_ID"),
			S3SecretKey: os.Getenv("HAYAT_S3_SECRET_ACCESS_KEY"),
			S3PathStyle: envBool("HAYAT_S3_PATH_STYLE", false),
		},
	}

	var err error
	if c.SessionTTL, err = envDuration("HAYAT_SESSION_TTL", 14*24*time.Hour); err != nil {
		return c, err
	}
	if c.SessionIdleTimeout, err = envDuration("HAYAT_SESSION_IDLE_TIMEOUT", 0); err != nil {
		return c, err
	}
	if c.SessionIdleTimeout != 0 && c.SessionIdleTimeout < 15*time.Minute {
		return c, errors.New("HAYAT_SESSION_IDLE_TIMEOUT must be 0 (off) or at least 15m")
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
	if c.BreachedPasswords != "hibp" && c.BreachedPasswords != "off" {
		return c, fmt.Errorf("HAYAT_BREACHED_PASSWORDS must be hibp or off, got %q", c.BreachedPasswords)
	}
	if (c.TurnstileSiteKey == "") != (c.TurnstileSecretKey == "") {
		return c, errors.New("set both HAYAT_TURNSTILE_SITE_KEY and HAYAT_TURNSTILE_SECRET_KEY, or neither")
	}
	if c.MasterKey, err = decodeKey(os.Getenv("HAYAT_MASTER_KEY")); err != nil {
		return c, err
	}
	if err := c.Storage.check(); err != nil {
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

func (st Storage) check() error {
	switch st.Backend {
	case "local":
		if st.Dir == "" {
			return errors.New("HAYAT_STORAGE=local needs HAYAT_STORAGE_DIR, a directory the auth and admin roles both mount")
		}
		return nil
	case "s3":
		if st.S3Bucket == "" || st.S3AccessKey == "" || st.S3SecretKey == "" {
			return errors.New("HAYAT_STORAGE=s3 needs HAYAT_S3_BUCKET, HAYAT_S3_ACCESS_KEY_ID and HAYAT_S3_SECRET_ACCESS_KEY")
		}
		if u, err := url.ParseRequestURI(st.S3Endpoint); err != nil || u.Host == "" {
			return fmt.Errorf("HAYAT_S3_ENDPOINT is not a URL: %q", st.S3Endpoint)
		}
		return nil
	}
	return fmt.Errorf("HAYAT_STORAGE must be local or s3, got %q", st.Backend)
}
