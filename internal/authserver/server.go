// Package authserver is the `auth` role: the public HTTPS face of Hayat. It
// serves the OpenID provider's endpoints and the hosted pages people sign in,
// sign up and reset passwords on.
//
// The pages are server-rendered and embedded in the binary, so sign-in works
// whether or not the console is deployed.
package authserver

import (
	"context"
	"crypto/subtle"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/federation"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/loginpage"
	"github.com/aidapedia/hayat/internal/media"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/ratelimit"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tenancy"
)

const (
	sessionCookie = "hayat_session"
	csrfCookie    = "hayat_csrf"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

type Server struct {
	cfg       config.Config
	store     *store.Store
	ids       *identity.Service
	storage   *hoidc.Storage
	clients   *hoidc.Clients
	pages     *loginpage.Service
	cat       *catalog.Service
	media     *media.Service
	fed       *federation.Service
	tenants   *tenancy.Service
	provider  *op.Provider
	limiter   ratelimit.Limiter
	log       *slog.Logger
	templates map[string]*template.Template
}

func New(cfg config.Config, s *store.Store, ids *identity.Service, storage *hoidc.Storage, clients *hoidc.Clients,
	pages *loginpage.Service, cat *catalog.Service, med *media.Service, fed *federation.Service, tenants *tenancy.Service,
	provider *op.Provider, limiter ratelimit.Limiter, log *slog.Logger) (*Server, error) {
	srv := &Server{cfg: cfg, store: s, ids: ids, storage: storage, clients: clients, pages: pages, cat: cat, media: med, fed: fed, tenants: tenants,
		provider: provider, limiter: limiter, log: log, templates: map[string]*template.Template{}}
	for _, name := range []string{"login", "signup", "password", "forgot", "setup", "message", "sso",
		"mfa", "mfa_setup", "mfa_app", "recovery", "reauth"} {
		t, err := template.ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		srv.templates[name] = t
	}
	return srv, nil
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(securityHeaders)
	static, _ := fs.Sub(staticFS, "static")
	r.Handle("/assets/*", http.StripPrefix("/assets/", cacheFor(http.FileServer(http.FS(static)), time.Hour)))

	r.Get("/login", s.loginPage)
	r.Post("/login", s.loginSubmit)
	r.Get("/signup", s.signupPage)
	r.Post("/signup", s.signupSubmit)
	r.Get("/verify", s.verifyPage)
	r.Post("/verify", s.verifySubmit)
	r.Get("/forgot", s.forgotPage)
	r.Post("/forgot", s.forgotSubmit)
	r.Get("/reset", s.resetPage)
	r.Post("/reset", s.resetSubmit)
	r.Get("/setup", s.setupPage)
	r.Post("/setup", s.setupSubmit)
	r.Get("/logout", s.logoutPage)
	r.Post("/logout", s.logoutSubmit)
	r.Get("/logged-out", s.loggedOutPage)
	r.Get("/sso", s.companyPage)
	r.Post("/sso", s.companySubmit)
	r.Get("/sso/start/{id}", s.ssoStart)
	r.Get("/sso/link/{id}", s.ssoLink)
	r.Get("/sso/callback", s.ssoCallback)
	r.Get("/mfa", s.mfaPage)
	r.Post("/mfa", s.mfaSubmit)
	r.Get("/mfa/setup", s.mfaSetupPage)
	r.Get("/mfa/setup/app", s.appSetupPage)
	r.Post("/mfa/setup/app", s.appSetupSubmit)
	r.Get("/mfa/setup/added", s.passkeyAdded)
	r.Post("/mfa/setup/done", s.setupDone)
	r.Get("/reauth", s.reauthPage)
	r.Post("/reauth", s.reauthSubmit)
	r.Get("/reauth/sso/{id}", s.reauthSSO)
	r.Post("/passkey/begin", s.passkeyBegin)
	r.Post("/passkey/finish", s.passkeyFinish)
	r.Get("/media/{id}", s.mediaFile)
	r.Get("/brand/asset/{id}", s.mediaFile) // login page logos published before media existed
	r.Get("/preview/login", s.previewPage)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, s.cfg.ConsoleURL, http.StatusFound) })

	// Everything else is the OpenID provider. It gets the session cookie in its
	// context so RP-initiated logout can end the browser's session.
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if c, err := req.Cookie(sessionCookie); err == nil {
			req = req.WithContext(hoidc.WithSessionCookie(req.Context(), c.Value))
		}
		s.provider.ServeHTTP(w, req)
	}))
	return r
}

// ---------- Rendering ----------

type page struct {
	Title, Heading, Sub, Error, Notice string
	CSRF                               string
	ConsoleURL                         string
	AuthRequestID                      string
	Email, Name                        string
	Action, Region, Token, Button      string
	MinLength                          int
	Body, Continue, Confirm            string

	// A service's login page, when the request came from one that published
	// a design. nil draws Hayat's own page.
	Brand *brandView
	// mode-light | mode-dark: forces a palette (the design's, or a preview's).
	ModeClass string
	// Nonce lets the brand's <style> through the Content-Security-Policy.
	Nonce string
	// A preview of a draft: forms are disabled and the console may frame it.
	Preview bool

	// The login page's other ways in: Google, GitHub, company SSO.
	Providers  []providerButton
	CompanyURL string

	// Second factors and passkeys.
	Passkeys    bool // this server can use passkeys at all
	HasPassword bool
	HasTOTP     bool
	HasPasskey  bool
	QR          template.URL // the authenticator app's setup, drawn as a PNG
	Secret      string
	FactorID    string
	Codes       []string // recovery codes, shown once
	Method      string
	ReturnTo    string

	// Turnstile is the site key when the bot check is on for this page.
	Turnstile string
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, p page, status int) {
	if !p.Preview {
		p.CSRF = s.csrfToken(w, r)
	}
	p.ConsoleURL = s.cfg.ConsoleURL
	if name == "login" {
		s.signInOptions(r.Context(), &p)
		p.Passkeys = s.ids.PasskeysAvailable()
	}
	styles, frames := "'self'", "'none'"
	scripts, childFrames := "'self'", ""
	if p.Turnstile != "" {
		// Cloudflare's widget: the one thing a sign-in page loads from
		// elsewhere, and only when the operator turned it on.
		scripts += " " + turnstileOrigin
		childFrames = "; frame-src " + turnstileOrigin
	}
	if p.Brand != nil {
		p.Nonce = crypto.NewToken()
		styles += " 'nonce-" + p.Nonce + "'"
	}
	if p.Preview {
		frames = s.cfg.ConsoleURL
		w.Header().Del("X-Frame-Options")
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src "+styles+
		"; script-src "+scripts+childFrames+"; frame-ancestors "+frames+"; base-uri 'none'")
	if p.MinLength == 0 {
		p.MinLength = identity.MinPasswordLength
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := s.templates[name].ExecuteTemplate(w, "layout", p); err != nil {
		s.log.Error("render page", "page", name, "err", err)
	}
}

func (s *Server) message(w http.ResponseWriter, r *http.Request, title, body string, status int) {
	s.render(w, r, "message", page{Title: title, Heading: title, Body: body, Continue: s.cfg.ConsoleURL, Button: "Go to the console"}, status)
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("auth server", "path", r.URL.Path, "err", err)
	s.render(w, r, "message", page{Title: "Something went wrong", Heading: "Something went wrong",
		Body: "Hayat couldn't finish that. Try again in a moment."}, http.StatusInternalServerError)
}

// ---------- CSRF: a random cookie echoed in every form ----------

func (s *Server) csrfToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && len(c.Value) >= 32 {
		return c.Value
	}
	token := crypto.NewToken()
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteStrictMode})
	return token
}

func (s *Server) checkCSRF(r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil || c.Value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.PostFormValue("csrf"))) == 1
}

func (s *Server) postGuard(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil || !s.checkCSRF(r) {
		s.message(w, r, "Page expired", "This form was open too long or came from somewhere else. Go back and try again.", http.StatusForbidden)
		return false
	}
	return true
}

// ---------- Sessions ----------

func (s *Server) currentSession(r *http.Request) (identity.Session, identity.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return identity.Session{}, identity.User{}, false
	}
	sess, u, err := s.ids.SessionByCookie(r.Context(), c.Value)
	return sess, u, err == nil
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u identity.User, amr []string) (identity.Session, error) {
	cookie, sess, err := s.ids.CreateSession(r.Context(), u, amr, clientIP(r), r.UserAgent())
	if err != nil {
		return identity.Session{}, err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: cookie, Path: "/", HttpOnly: true,
		Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: int(s.cfg.SessionTTL.Seconds())})
	return sess, nil
}

func (s *Server) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ---------- Sign in ----------

// finish completes an auth request with a session and sends the browser back
// through the provider's callback, which issues the code.
func (s *Server) finish(w http.ResponseWriter, r *http.Request, a *hoidc.AuthRequest, sess identity.Session) {
	to, err := s.completeURL(r.Context(), a, sess)
	if err != nil {
		s.signInFailed(w, r, err)
		return
	}
	http.Redirect(w, r, to, http.StatusFound)
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("authRequestID")
	if id == "" {
		if _, _, ok := s.currentSession(r); ok {
			http.Redirect(w, r, s.cfg.ConsoleURL, http.StatusFound)
			return
		}
		s.render(w, r, "login", page{Title: "Sign in", Heading: "Sign in to Hayat"}, http.StatusOK)
		return
	}
	a, err := s.storage.LoadAuthRequest(r.Context(), id)
	if err != nil {
		s.message(w, r, "Sign-in expired", "This sign-in link has expired. Go back to the application and start again.", http.StatusBadRequest)
		return
	}
	sess, u, ok := s.currentSession(r)
	// Single sign-on: an existing session completes the request without a
	// prompt, unless the client asked for a fresh sign-in, or the session is
	// weaker than the account now needs.
	if ok && !a.PromptLogin() && !a.TooOld(sess.AuthTime) && s.strongEnough(r.Context(), sess, u) {
		s.finish(w, r, a, sess)
		return
	}
	if a.PromptNone() {
		s.redirectError(w, r, a, "login_required")
		return
	}
	p := page{Title: "Sign in", Heading: "Sign in", Sub: s.continueTo(r.Context(), a), AuthRequestID: id, Email: a.LoginHint}
	s.brandFor(r.Context(), &p, a)
	s.render(w, r, "login", p, http.StatusOK)
}

func (s *Server) continueTo(ctx context.Context, a *hoidc.AuthRequest) string {
	c, err := s.clients.Get(ctx, a.ClientID)
	if err != nil {
		return ""
	}
	return "to continue to " + c.Name
}

// redirectError answers prompt=none without a session, as OIDC requires: back
// to the client with an error, never a page.
func (s *Server) redirectError(w http.ResponseWriter, r *http.Request, a *hoidc.AuthRequest, code string) {
	u, err := url.Parse(a.RedirectURI)
	if err != nil {
		s.message(w, r, "Sign-in failed", "The application sent an invalid address.", http.StatusBadRequest)
		return
	}
	q := u.Query()
	q.Set("error", code)
	if a.State != "" {
		q.Set("state", a.State)
	}
	u.RawQuery = q.Encode()
	_ = s.storage.DeleteAuthRequest(r.Context(), a.GetID())
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.postGuard(w, r) {
		return
	}
	id := r.PostFormValue("authRequestID")
	email := strings.TrimSpace(r.PostFormValue("email"))
	p := page{Title: "Sign in", Heading: "Sign in to Hayat", AuthRequestID: id, Email: email}

	var a *hoidc.AuthRequest
	if id != "" {
		var err error
		if a, err = s.storage.LoadAuthRequest(r.Context(), id); err != nil {
			s.message(w, r, "Sign-in expired", "This sign-in took too long. Go back to the application and start again.", http.StatusBadRequest)
			return
		}
		p.Heading, p.Sub = "Sign in", s.continueTo(r.Context(), a)
		s.brandFor(r.Context(), &p, a)
	}

	if !s.limiter.Allow(r.Context(), "login:ip:"+clientIP(r), 60, time.Minute) ||
		!s.limiter.Allow(r.Context(), "login:email:"+strings.ToLower(email), 10, 15*time.Minute) {
		p.Error = "Too many sign-in attempts. Wait a few minutes and try again."
		s.render(w, r, "login", p, http.StatusTooManyRequests)
		return
	}

	// A company that forces its own sign-in: no password for its people, here or anywhere.
	if prov, enforced := s.enforcedProvider(r.Context(), email); enforced {
		s.toProvider(w, r, prov, id, nil, email)
		return
	}
	u, err := s.ids.Authenticate(r.Context(), email, r.PostFormValue("password"))
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrLocked):
			p.Error = "Too many sign-in attempts. Wait a few minutes and try again."
			s.render(w, r, "login", p, http.StatusTooManyRequests)
		case errors.Is(err, identity.ErrBadCredentials), errors.Is(err, identity.ErrPending), errors.Is(err, identity.ErrDisabled):
			p.Error = err.Error()
			s.render(w, r, "login", p, http.StatusUnauthorized)
		default:
			s.serverError(w, r, err)
		}
		return
	}
	s.goNext(w, r, firstFactor{user: u, amr: []string{identity.AMRPassword}, authRequestID: id})
}

// strongEnough reports whether an existing session may sign the person in to
// another application without asking again: a session made before they set
// up a second factor, or before a tenant of theirs required one, may not.
// Sessions from another account keep working: their provider decided.
func (s *Server) strongEnough(ctx context.Context, sess identity.Session, u identity.User) bool {
	if sess.MFA() || slices.Contains(sess.AMR, identity.AMRFed) {
		return true
	}
	if has, err := s.ids.HasSecondFactor(ctx, u.ID); err != nil || has {
		return false
	}
	required, _, err := s.tenants.RequiresMFA(ctx, u.ID)
	return err == nil && !required
}

// ---------- Sign up and verification ----------

func (s *Server) signupPage(w http.ResponseWriter, r *http.Request) {
	p := page{Title: "Create account", Heading: "Create your account", AuthRequestID: r.URL.Query().Get("authRequestID")}
	if p.AuthRequestID != "" {
		if a, err := s.storage.LoadAuthRequest(r.Context(), p.AuthRequestID); err == nil {
			s.brandFor(r.Context(), &p, a)
			p.Heading = "Create your account"
		}
	}
	s.withTurnstile(&p)
	s.render(w, r, "signup", p, http.StatusOK)
}

func (s *Server) signupSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.postGuard(w, r) {
		return
	}
	email := strings.TrimSpace(r.PostFormValue("email"))
	name := strings.TrimSpace(r.PostFormValue("name"))
	p := page{Title: "Create account", Heading: "Create your account", Email: email, Name: name}
	s.withTurnstile(&p)
	if !s.human(r.Context(), r) {
		p.Error = "We couldn't confirm you're a person. Complete the check and try again."
		s.render(w, r, "signup", p, http.StatusBadRequest)
		return
	}
	if !s.limiter.Allow(r.Context(), "signup:ip:"+clientIP(r), 10, time.Hour) {
		p.Error = "Too many sign-ups from your network. Try again later."
		s.render(w, r, "signup", p, http.StatusTooManyRequests)
		return
	}
	if prov, enforced := s.enforcedProvider(r.Context(), email); enforced {
		p.Error = federation.DomainOf(email) + " signs in with " + prov.Name + ". Use Continue with company SSO on the sign-in page."
		s.render(w, r, "signup", p, http.StatusBadRequest)
		return
	}
	if err := s.ids.Register(r.Context(), email, name); err != nil {
		if errors.Is(err, identity.ErrInvalidEmail) {
			p.Error = err.Error()
			s.render(w, r, "signup", p, http.StatusBadRequest)
			return
		}
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, "message", page{Title: "Check your email", Heading: "Check your email",
		Body: "If " + email + " can receive mail, a link to choose your password is on its way. It expires in 3 days."}, http.StatusOK)
}

func (s *Server) verifyPage(w http.ResponseWriter, r *http.Request) {
	region, token := r.URL.Query().Get("r"), r.URL.Query().Get("t")
	u, err := s.ids.CheckLink(r.Context(), region, token, "verify")
	if err != nil {
		s.message(w, r, "Link expired", identity.ErrLinkInvalid.Error(), http.StatusBadRequest)
		return
	}
	s.render(w, r, "password", page{Title: "Choose a password", Heading: "Choose a password",
		Sub: "for " + u.Email, Action: "/verify", Region: region, Token: token, Email: u.Email, Button: "Save and continue"}, http.StatusOK)
}

func (s *Server) verifySubmit(w http.ResponseWriter, r *http.Request) {
	s.passwordSubmit(w, r, "/verify", "Choose a password", "Save and continue", func(ctx context.Context, region, token, pw string) (identity.User, error) {
		return s.ids.CompleteVerification(ctx, region, token, pw)
	})
}

// ---------- Password reset ----------

func (s *Server) forgotPage(w http.ResponseWriter, r *http.Request) {
	p := page{Title: "Reset password", Heading: "Reset your password", Sub: "We'll email you a link to choose a new one."}
	s.withTurnstile(&p)
	s.render(w, r, "forgot", p, http.StatusOK)
}

func (s *Server) forgotSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.postGuard(w, r) {
		return
	}
	email := strings.TrimSpace(r.PostFormValue("email"))
	if !s.human(r.Context(), r) {
		p := page{Title: "Reset password", Heading: "Reset your password", Email: email,
			Error: "We couldn't confirm you're a person. Complete the check and try again."}
		s.withTurnstile(&p)
		s.render(w, r, "forgot", p, http.StatusBadRequest)
		return
	}
	if !s.limiter.Allow(r.Context(), "forgot:ip:"+clientIP(r), 10, time.Hour) ||
		!s.limiter.Allow(r.Context(), "forgot:email:"+strings.ToLower(email), 3, time.Hour) {
		s.render(w, r, "forgot", page{Title: "Reset password", Heading: "Reset your password", Email: email,
			Error: "Too many reset requests. Try again in an hour.", Turnstile: s.siteKey()}, http.StatusTooManyRequests)
		return
	}
	if _, enforced := s.enforcedProvider(r.Context(), email); enforced {
		email = strings.TrimSpace(email) // no password to reset: the company signs them in; answer the same
	} else if err := s.ids.RequestPasswordReset(r.Context(), email); err != nil {
		if errors.Is(err, identity.ErrInvalidEmail) {
			s.render(w, r, "forgot", page{Title: "Reset password", Heading: "Reset your password", Email: email, Error: err.Error(), Turnstile: s.siteKey()}, http.StatusBadRequest)
			return
		}
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, "message", page{Title: "Check your email", Heading: "Check your email",
		Body: "If " + email + " has an account, a reset link is on its way. It expires in 1 hour."}, http.StatusOK)
}

func (s *Server) resetPage(w http.ResponseWriter, r *http.Request) {
	region, token := r.URL.Query().Get("r"), r.URL.Query().Get("t")
	u, err := s.ids.CheckLink(r.Context(), region, token, "reset")
	if err != nil {
		s.message(w, r, "Link expired", identity.ErrLinkInvalid.Error(), http.StatusBadRequest)
		return
	}
	s.render(w, r, "password", page{Title: "New password", Heading: "Choose a new password", Sub: "for " + u.Email,
		Action: "/reset", Region: region, Token: token, Email: u.Email, Button: "Save new password"}, http.StatusOK)
}

func (s *Server) resetSubmit(w http.ResponseWriter, r *http.Request) {
	s.passwordSubmit(w, r, "/reset", "Choose a new password", "Save new password", func(ctx context.Context, region, token, pw string) (identity.User, error) {
		return s.ids.ResetPassword(ctx, region, token, pw)
	})
}

// passwordSubmit is the shared second half of verification and reset: check
// the two fields, use the link, sign the person in.
func (s *Server) passwordSubmit(w http.ResponseWriter, r *http.Request, action, heading, button string,
	apply func(ctx context.Context, region, token, pw string) (identity.User, error)) {
	if !s.postGuard(w, r) {
		return
	}
	region, token := r.PostFormValue("r"), r.PostFormValue("t")
	pw, confirm := r.PostFormValue("password"), r.PostFormValue("confirm")
	p := page{Title: heading, Heading: heading, Action: action, Region: region, Token: token, Button: button}
	if pw != confirm {
		p.Error = "The two passwords don't match."
		s.render(w, r, "password", p, http.StatusBadRequest)
		return
	}
	u, err := apply(r.Context(), region, token, pw)
	switch {
	case errors.Is(err, identity.ErrWeakPassword), errors.Is(err, identity.ErrBreachedPassword):
		p.Error = err.Error()
		s.render(w, r, "password", p, http.StatusBadRequest)
		return
	case errors.Is(err, identity.ErrLinkInvalid), errors.Is(err, identity.ErrDisabled):
		s.message(w, r, "Link expired", err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	// A new password is one factor: someone with a second factor still
	// confirms it before they are in.
	to, err := s.next(w, r, firstFactor{user: u, amr: []string{identity.AMRPassword}})
	if err != nil {
		s.signInFailed(w, r, err)
		return
	}
	if to != s.cfg.ConsoleURL {
		http.Redirect(w, r, to, http.StatusFound)
		return
	}
	s.render(w, r, "message", page{Title: "You're all set", Heading: "You're all set",
		Body: "Your password is saved and you're signed in as " + u.Email + ".", Continue: s.cfg.ConsoleURL, Button: "Continue"}, http.StatusOK)
}

// ---------- Sign out ----------

func (s *Server) logoutPage(w http.ResponseWriter, r *http.Request) {
	_, u, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/logged-out", http.StatusFound)
		return
	}
	s.render(w, r, "message", page{Title: "Sign out", Heading: "Sign out of Hayat?",
		Body:    "You're signed in as " + u.Email + ". Signing out here signs you out of every application that uses Hayat in this browser.",
		Confirm: "/logout", Button: "Sign out"}, http.StatusOK)
}

func (s *Server) logoutSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.postGuard(w, r) {
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.ids.EndSessionByCookie(r.Context(), c.Value)
	}
	s.clearSession(w)
	http.Redirect(w, r, "/logged-out", http.StatusFound)
}

func (s *Server) loggedOutPage(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w)
	s.render(w, r, "message", page{Title: "Signed out", Heading: "You're signed out",
		Continue: s.cfg.Issuer + "/login", Button: "Sign in again"}, http.StatusOK)
}

// ---------- Headers ----------

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		// Links carry one-time tokens; never send them on as a Referer.
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}

func cacheFor(h http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age="+itoa(int(d.Seconds())))
		h.ServeHTTP(w, r)
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
