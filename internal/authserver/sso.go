package authserver

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/federation"
	"github.com/aidapedia/hayat/internal/store"
)

// ssoCookie binds a trip to a provider to the browser that started it, so a
// callback URL handed to someone else can't sign them in to another account.
const ssoCookie = "hayat_sso"

// providerButton is a "Continue with Google" on the login page.
type providerButton struct {
	Name, Kind, URL string
}

func withRequest(path, authRequestID string) string {
	if authRequestID == "" {
		return path
	}
	return path + "?authRequestID=" + url.QueryEscape(authRequestID)
}

// signInOptions fills the login page's other ways in: the instance's
// enabled providers, and company SSO when any tenant has set it up.
func (s *Server) signInOptions(ctx context.Context, p *page) {
	list, err := s.fed.InstanceProviders(ctx)
	if err != nil {
		s.log.Error("sign-in providers", "err", err)
		return
	}
	for _, pr := range list {
		if pr.Enabled {
			p.Providers = append(p.Providers, providerButton{Name: pr.Name, Kind: pr.Kind, URL: withRequest("/sso/start/"+pr.ID.String(), p.AuthRequestID)})
		}
	}
	if ok, err := s.fed.HasCompanySignIn(ctx); err == nil && ok {
		p.CompanyURL = withRequest("/sso", p.AuthRequestID)
	}
}

// toProvider sends the browser to a provider, the trip bound to it by cookie.
func (s *Server) toProvider(w http.ResponseWriter, r *http.Request, p federation.Provider, authRequestID string, link *uuid.UUID, hint string) {
	start, err := s.fed.Begin(r.Context(), p, authRequestID, link, hint)
	if err != nil {
		s.ssoFailed(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: start.State, Path: "/sso", HttpOnly: true, MaxAge: 600,
		Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, start.URL, http.StatusFound)
}

func (s *Server) ssoFailed(w http.ResponseWriter, r *http.Request, err error) {
	if errs.Is(err) {
		s.message(w, r, "Couldn't sign you in", err.Error(), http.StatusBadRequest)
		return
	}
	s.serverError(w, r, err)
}

func (s *Server) providerParam(w http.ResponseWriter, r *http.Request) (federation.Provider, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err == nil {
		p, err := s.fed.Provider(r.Context(), id)
		if err == nil {
			return p, true
		}
		if !errors.Is(err, store.ErrNotFound) {
			s.serverError(w, r, err)
			return federation.Provider{}, false
		}
	}
	s.message(w, r, "Unknown sign-in", "That way of signing in doesn't exist here. Go back and pick another.", http.StatusNotFound)
	return federation.Provider{}, false
}

// ssoStart: /sso/start/{id}?authRequestID= — "Continue with Google".
func (s *Server) ssoStart(w http.ResponseWriter, r *http.Request) {
	p, ok := s.providerParam(w, r)
	if !ok {
		return
	}
	if !s.limiter.Allow(r.Context(), "sso:ip:"+clientIP(r), 60, 60e9) {
		s.message(w, r, "Slow down", "Too many sign-ins from your network. Wait a minute and try again.", http.StatusTooManyRequests)
		return
	}
	s.toProvider(w, r, p, r.URL.Query().Get("authRequestID"), nil, "")
}

// ssoLink: /sso/link/{id} — link another account from Account, signed in.
func (s *Server) ssoLink(w http.ResponseWriter, r *http.Request) {
	_, u, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	p, ok := s.providerParam(w, r)
	if !ok {
		return
	}
	s.toProvider(w, r, p, "", &u.ID, u.Email)
}

// companyPage: /sso — sign in with your company's identity provider, found from your email.
func (s *Server) companyPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "sso", page{Title: "Company sign-in", Heading: "Sign in with your company",
		Sub: "Enter your work email. We'll send you to your company's sign-in page.", AuthRequestID: r.URL.Query().Get("authRequestID")}, http.StatusOK)
}

func (s *Server) companySubmit(w http.ResponseWriter, r *http.Request) {
	if !s.postGuard(w, r) {
		return
	}
	email := strings.TrimSpace(r.PostFormValue("email"))
	id := r.PostFormValue("authRequestID")
	p := page{Title: "Company sign-in", Heading: "Sign in with your company", AuthRequestID: id, Email: email}
	if !s.limiter.Allow(r.Context(), "sso:ip:"+clientIP(r), 60, 60e9) {
		p.Error = "Too many sign-ins from your network. Wait a minute and try again."
		s.render(w, r, "sso", p, http.StatusTooManyRequests)
		return
	}
	prov, _, ok, err := s.fed.CompanySignIn(r.Context(), email)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !ok {
		p.Error = "No company sign-in is set up for " + federation.DomainOf(email) + ". Go back and sign in another way."
		s.render(w, r, "sso", p, http.StatusBadRequest)
		return
	}
	s.toProvider(w, r, prov, id, nil, email)
}

// ssoCallback: /sso/callback — every provider sends people back here.
func (s *Server) ssoCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	c, cookieErr := r.Cookie(ssoCookie)
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: "", Path: "/sso", HttpOnly: true, MaxAge: -1,
		Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
	if e := q.Get("error"); e != "" {
		s.message(w, r, "Sign-in cancelled", "The provider didn't sign you in. Go back to the application and try again.", http.StatusBadRequest)
		return
	}
	if cookieErr != nil || state == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 {
		s.message(w, r, "Sign-in expired", "This sign-in was started in another browser or took too long. Go back to the application and start again.", http.StatusBadRequest)
		return
	}
	ret, err := s.fed.Complete(r.Context(), state, q.Get("code"))
	if err != nil {
		s.ssoFailed(w, r, err)
		return
	}
	u, err := s.fed.SignIn(r.Context(), ret)
	if err != nil {
		s.ssoFailed(w, r, err)
		return
	}
	if ret.LinkUser != nil {
		http.Redirect(w, r, s.cfg.ConsoleURL+"/account?linked="+url.QueryEscape(ret.Provider.Name), http.StatusFound)
		return
	}
	sess, err := s.startSession(w, r, u, []string{"fed"})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if ret.AuthRequestID == "" {
		http.Redirect(w, r, s.cfg.ConsoleURL, http.StatusFound)
		return
	}
	a, err := s.storage.LoadAuthRequest(r.Context(), ret.AuthRequestID)
	if err != nil {
		s.message(w, r, "Sign-in expired", "This sign-in took too long. Go back to the application and start again.", http.StatusBadRequest)
		return
	}
	s.finish(w, r, a, sess)
}

// enforcedProvider is the provider an address must sign in with, when its
// company forces it; ok is false for everyone else.
func (s *Server) enforcedProvider(ctx context.Context, email string) (federation.Provider, bool) {
	p, enforced, err := s.fed.Enforced(ctx, email)
	if err != nil {
		s.log.Error("company sign-in policy", "err", err)
		return federation.Provider{}, false
	}
	return p, enforced
}
