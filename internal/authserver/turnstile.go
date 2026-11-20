package authserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Cloudflare Turnstile guards the pages that send mail to any address typed
// in (sign-up, forgot password) against scripts, when the operator sets its
// keys. Password sign-in is guarded by rate limits and lockout instead.

const turnstileOrigin = "https://challenges.cloudflare.com"

// TurnstileVerifyURL is where answers are checked. Tests point it at a fake.
var TurnstileVerifyURL = turnstileOrigin + "/turnstile/v0/siteverify"

var turnstileClient = &http.Client{Timeout: 5 * time.Second}

// withTurnstile puts the widget on a page when the bot check is on.
func (s *Server) withTurnstile(p *page) { p.Turnstile = s.siteKey() }

// siteKey is the widget's key, or "" with the check off.
func (s *Server) siteKey() string {
	if s.cfg.Turnstile() {
		return s.cfg.TurnstileSiteKey
	}
	return ""
}

// human checks the widget's answer. With the check off, everyone passes;
// with it on, an answer Cloudflare can't confirm (or can't be asked about)
// fails: the operator chose to keep scripts out.
func (s *Server) human(ctx context.Context, r *http.Request) bool {
	if !s.cfg.Turnstile() {
		return true
	}
	token := r.PostFormValue("cf-turnstile-response")
	if token == "" || len(token) > 2048 {
		return false
	}
	form := url.Values{"secret": {s.cfg.TurnstileSecretKey}, "response": {token}, "remoteip": {clientIP(r)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TurnstileVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := turnstileClient.Do(req)
	if err != nil {
		s.log.Error("turnstile", "err", err)
		return false
	}
	defer resp.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false
	}
	return out.Success
}
