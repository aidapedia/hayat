package authserver

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/store"
)

// First run: until an instance admin exists, /setup creates one. It needs a
// token printed in the server log, so whoever runs the server, not whoever
// finds the URL first, becomes the admin.

const setupKey = "setup_token"
const setupTTL = 24 * time.Hour

type setupValue struct {
	Hash      string    `json:"hash"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SetupLink returns a setup link when no admin exists yet. With fresh=false it
// reuses a live token and returns "" for the link (the token was printed when
// it was made); fresh=true always mints a new one.
func SetupLink(ctx context.Context, s *store.Store, ids *identity.Service, issuer string, fresh bool) (string, error) {
	has, err := ids.HasAdmin(ctx)
	if err != nil || has {
		return "", err
	}
	var link string
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		link = ""
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT value FROM instance_settings WHERE key = $1 FOR UPDATE`, setupKey).Scan(&raw)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var cur setupValue
		_ = json.Unmarshal(raw, &cur)
		if !fresh && cur.Hash != "" && time.Now().Before(cur.ExpiresAt) {
			return nil
		}
		token := crypto.NewToken()
		val, _ := json.Marshal(setupValue{Hash: hex.EncodeToString(crypto.HashToken(token)), ExpiresAt: time.Now().Add(setupTTL)})
		if _, err := tx.Exec(ctx, `INSERT INTO instance_settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, setupKey, val); err != nil {
			return err
		}
		link = strings.TrimRight(issuer, "/") + "/setup?token=" + token
		return nil
	})
	return link, err
}

func (s *Server) setupTokenValid(ctx context.Context, token string) bool {
	if token == "" {
		return false
	}
	var raw []byte
	if err := s.store.Pool.QueryRow(ctx, `SELECT value FROM instance_settings WHERE key = $1`, setupKey).Scan(&raw); err != nil {
		return false
	}
	var cur setupValue
	if json.Unmarshal(raw, &cur) != nil || time.Now().After(cur.ExpiresAt) {
		return false
	}
	want, _ := hex.DecodeString(cur.Hash)
	return subtle.ConstantTimeCompare(want, crypto.HashToken(token)) == 1
}

func (s *Server) setupPage(w http.ResponseWriter, r *http.Request) {
	if has, _ := s.ids.HasAdmin(r.Context()); has {
		s.message(w, r, "Already set up", "This Hayat instance already has an admin.", http.StatusOK)
		return
	}
	token := r.URL.Query().Get("token")
	if !s.setupTokenValid(r.Context(), token) {
		s.render(w, r, "message", page{Title: "Set up Hayat", Heading: "Set up Hayat",
			Body: "Open the setup link printed in the Hayat server log. If it has expired, run `hayat setup-link` to print a new one."}, http.StatusForbidden)
		return
	}
	s.render(w, r, "setup", page{Title: "Set up Hayat", Heading: "Create the first admin",
		Sub: "Nobody has signed in to this instance yet.", Token: token}, http.StatusOK)
}

func (s *Server) setupSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.postGuard(w, r) {
		return
	}
	token := r.PostFormValue("token")
	if has, _ := s.ids.HasAdmin(r.Context()); has || !s.setupTokenValid(r.Context(), token) {
		s.message(w, r, "Setup closed", "This setup link is no longer valid.", http.StatusForbidden)
		return
	}
	p := page{Title: "Set up Hayat", Heading: "Create the first admin", Token: token,
		Name: r.PostFormValue("name"), Email: r.PostFormValue("email")}
	u, err := s.ids.CreateAdmin(r.Context(), p.Email, p.Name, r.PostFormValue("password"))
	if err != nil {
		if errors.Is(err, identity.ErrInvalidEmail) || errors.Is(err, identity.ErrWeakPassword) || errors.Is(err, identity.ErrEmailTaken) {
			p.Error = err.Error()
			s.render(w, r, "setup", p, http.StatusBadRequest)
			return
		}
		s.serverError(w, r, err)
		return
	}
	_, _ = s.store.Pool.Exec(r.Context(), `DELETE FROM instance_settings WHERE key = $1`, setupKey)
	if _, err := s.startSession(w, r, u, []string{"pwd"}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, "message", page{Title: "Hayat is ready", Heading: "Hayat is ready",
		Body: "You're the instance admin, signed in as " + u.Email + ".", Continue: s.cfg.ConsoleURL, Button: "Open the console"}, http.StatusOK)
}
