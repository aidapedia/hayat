package authserver

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"rsc.io/qr"

	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/identity"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
)

// After the first factor (a password, or another account) comes, in order:
//   1. a second factor, when the person has one: a code from an
//      authenticator app, a passkey, or a recovery code (/mfa);
//   2. setting one up, when a tenant they belong to requires it (/mfa/setup);
//   3. the session.
// A passkey sign-in is two factors on its own and goes straight to 3.
//
// The half-done sign-in waits in a challenge row (internal/identity), found by
// the hayat_challenge cookie, so nothing about it can be forged in the form.

const (
	challengeCookie = "hayat_challenge"
	passkeyCookie   = "hayat_passkey"
	reauthCookie    = "hayat_reauth"

	challengeTTL = 10 * time.Minute
	ceremonyTTL  = 5 * time.Minute

	// reauthTrip stands in a provider trip's auth request id when the trip
	// confirms someone already signed in, instead of signing them in.
	// Real auth request ids are UUIDs, so the two never meet.
	reauthTrip = "reauth"
)

// firstFactor is a sign-in whose first step went right.
type firstFactor struct {
	user          identity.User
	amr           []string
	authRequestID string
	// company: the person came through a company's own identity provider,
	// which decides how its people prove who they are. Hayat asks nothing more.
	company bool
}

// next decides what follows a right first factor and returns where the
// browser goes: the second-factor page, the setup page, or onwards with a
// fresh session.
func (s *Server) next(w http.ResponseWriter, r *http.Request, in firstFactor) (string, error) {
	ctx := r.Context()
	if !slices.Contains(in.amr, identity.AMRMFA) && !in.company {
		has, err := s.ids.HasSecondFactor(ctx, in.user.ID)
		if err != nil {
			return "", err
		}
		if has {
			return s.challenge(w, r, identity.ChallengeMFA, in, "/mfa")
		}
		required, _, err := s.tenants.RequiresMFA(ctx, in.user.ID)
		if err != nil {
			return "", err
		}
		if required {
			return s.challenge(w, r, identity.ChallengeEnroll, in, "/mfa/setup")
		}
	}
	return s.complete(w, r, in.user, in.amr, in.authRequestID)
}

func (s *Server) challenge(w http.ResponseWriter, r *http.Request, kind string, in firstFactor, to string) (string, error) {
	cookie, _, err := s.ids.NewChallenge(r.Context(), kind, &in.user.ID, in.amr, in.authRequestID, nil, challengeTTL)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{Name: challengeCookie, Value: cookie, Path: "/", HttpOnly: true, MaxAge: int(challengeTTL.Seconds()),
		Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
	return to, nil
}

func (s *Server) clearChallenge(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: challengeCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
		Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
}

// complete starts the session and returns where the sign-in goes on: the
// application that asked, or the console.
func (s *Server) complete(w http.ResponseWriter, r *http.Request, u identity.User, amr []string, authRequestID string) (string, error) {
	sess, err := s.startSession(w, r, u, amr)
	if err != nil {
		return "", err
	}
	if authRequestID == "" {
		return s.cfg.ConsoleURL, nil
	}
	a, err := s.storage.LoadAuthRequest(r.Context(), authRequestID)
	if err != nil {
		return "", errExpired
	}
	return s.completeURL(r.Context(), a, sess)
}

var errExpired = errs.New("this sign-in took too long: go back to the application and start again")

// completeURL completes an auth request with a session; the provider's
// callback then issues the code.
func (s *Server) completeURL(ctx context.Context, a *hoidc.AuthRequest, sess identity.Session) (string, error) {
	if err := s.storage.CompleteAuthRequest(ctx, a, sess); err != nil {
		return "", errExpired
	}
	return s.cfg.Issuer + "/authorize/callback?id=" + url.QueryEscape(a.GetID()), nil
}

// goNext sends the browser where next decided, or explains what went wrong.
func (s *Server) goNext(w http.ResponseWriter, r *http.Request, in firstFactor) {
	to, err := s.next(w, r, in)
	if err != nil {
		s.signInFailed(w, r, err)
		return
	}
	http.Redirect(w, r, to, http.StatusFound)
}

func (s *Server) signInFailed(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errExpired) {
		s.message(w, r, "Sign-in expired", "This sign-in took too long. Go back to the application and start again.", http.StatusBadRequest)
		return
	}
	if errs.Is(err) {
		s.message(w, r, "Couldn't sign you in", err.Error(), http.StatusBadRequest)
		return
	}
	s.serverError(w, r, err)
}

// pendingChallenge is the half-done sign-in this browser holds, of one kind.
func (s *Server) pendingChallenge(r *http.Request, kind string) (identity.Challenge, identity.User, bool) {
	c, err := r.Cookie(challengeCookie)
	if err != nil {
		return identity.Challenge{}, identity.User{}, false
	}
	ch, err := s.ids.ChallengeByCookie(r.Context(), c.Value, kind)
	if err != nil || ch.UserID == nil {
		return identity.Challenge{}, identity.User{}, false
	}
	u, err := s.ids.GetUser(r.Context(), *ch.UserID)
	if err != nil || u.Status != identity.StatusActive {
		return identity.Challenge{}, identity.User{}, false
	}
	return ch, u, true
}

func (s *Server) challengeExpired(w http.ResponseWriter, r *http.Request) {
	s.clearChallenge(w)
	s.message(w, r, "Sign-in expired", "This sign-in took too long or had too many wrong codes. Start again from the sign-in page.", http.StatusBadRequest)
}

// finishChallenge uses a challenge up and finishes its sign-in with the
// methods it adds.
func (s *Server) finishChallenge(w http.ResponseWriter, r *http.Request, ch identity.Challenge, u identity.User, add ...string) (string, error) {
	ok, err := s.ids.EndChallenge(r.Context(), ch)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errExpired
	}
	s.clearChallenge(w)
	amr := slices.Clone(ch.AMR)
	for _, m := range add {
		if !slices.Contains(amr, m) {
			amr = append(amr, m)
		}
	}
	return s.complete(w, r, u, amr, ch.AuthRequestID)
}

// ---------- The second factor: /mfa ----------

func (s *Server) methodsFor(ctx context.Context, p *page, u identity.User) {
	p.HasTOTP, _ = s.ids.HasTOTP(ctx, u.ID)
	p.HasPasskey, _ = s.ids.HasPasskey(ctx, u.ID)
	p.HasPassword = u.HasPassword
}

func (s *Server) mfaPage(w http.ResponseWriter, r *http.Request) {
	ch, u, ok := s.pendingChallenge(r, identity.ChallengeMFA)
	if !ok {
		s.challengeExpired(w, r)
		return
	}
	p := page{Title: "Two-step verification", Heading: "Confirm it's you", Sub: "Signing in as " + u.Email, AuthRequestID: ch.AuthRequestID}
	s.methodsFor(r.Context(), &p, u)
	s.brandForID(r.Context(), &p, ch.AuthRequestID)
	s.render(w, r, "mfa", p, http.StatusOK)
}

func (s *Server) mfaSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.postGuard(w, r) {
		return
	}
	ch, u, ok := s.pendingChallenge(r, identity.ChallengeMFA)
	if !ok {
		s.challengeExpired(w, r)
		return
	}
	ctx := r.Context()
	code := strings.TrimSpace(r.PostFormValue("code"))
	p := page{Title: "Two-step verification", Heading: "Confirm it's you", Sub: "Signing in as " + u.Email, AuthRequestID: ch.AuthRequestID}
	s.methodsFor(ctx, &p, u)
	s.brandForID(ctx, &p, ch.AuthRequestID)
	if !s.limiter.Allow(ctx, "mfa:user:"+u.ID.String(), 10, 15*time.Minute) {
		p.Error = "Too many attempts. Wait a few minutes and try again."
		s.render(w, r, "mfa", p, http.StatusTooManyRequests)
		return
	}
	method, err := s.secondFactor(ctx, u, code)
	if err != nil {
		if !errs.Is(err) {
			s.serverError(w, r, err)
			return
		}
		left, ferr := s.ids.FailChallenge(ctx, ch)
		if ferr != nil {
			s.serverError(w, r, ferr)
			return
		}
		if left <= 0 {
			s.challengeExpired(w, r)
			return
		}
		p.Error = err.Error()
		s.render(w, r, "mfa", p, http.StatusUnauthorized)
		return
	}
	to, err := s.finishChallenge(w, r, ch, u, method, identity.AMRMFA)
	if err != nil {
		s.signInFailed(w, r, err)
		return
	}
	http.Redirect(w, r, to, http.StatusFound)
}

// secondFactor checks a typed code: six digits from an authenticator app, or
// a recovery code. It answers the amr method the code proves.
func (s *Server) secondFactor(ctx context.Context, u identity.User, code string) (string, error) {
	digits := strings.ReplaceAll(code, " ", "")
	if len(digits) == 6 && strings.Trim(digits, "0123456789") == "" {
		return identity.AMROTP, s.ids.VerifyTOTP(ctx, u.ID, digits)
	}
	if code == "" {
		return "", errs.New("enter the code from your authenticator app, or a recovery code")
	}
	// A recovery code proves something the person kept, not a device; amr
	// carries "mfa" alone for it.
	return identity.AMRMFA, s.ids.UseRecoveryCode(ctx, u.ID, code)
}

// ---------- Setting up a second factor: /mfa/setup ----------

// setupSubject is whose factor is being set up: the person a required setup
// holds back from signing in (an enroll challenge), or someone signed in
// setting one up from Account.
type setupSubject struct {
	user      identity.User
	challenge *identity.Challenge
	returnTo  string
}

func (s *Server) setupSubjectFor(w http.ResponseWriter, r *http.Request) (setupSubject, bool) {
	if ch, u, ok := s.pendingChallenge(r, identity.ChallengeEnroll); ok {
		return setupSubject{user: u, challenge: &ch}, true
	}
	sess, u, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return setupSubject{}, false
	}
	returnTo := s.safeReturn(r.FormValue("return_to"))
	// Adding a way in to an account is sensitive: confirm it's them first.
	if !sess.RecentlyVerified() {
		http.Redirect(w, r, "/reauth?return_to="+url.QueryEscape(s.cfg.Issuer+r.URL.RequestURI()), http.StatusFound)
		return setupSubject{}, false
	}
	return setupSubject{user: u, returnTo: returnTo}, true
}

func (s *Server) setupPageData(ctx context.Context, sub setupSubject) page {
	p := page{Title: "Two-step verification", Heading: "Add a second step", ReturnTo: sub.returnTo, Passkeys: s.ids.PasskeysAvailable()}
	if sub.challenge != nil {
		_, name, _ := s.tenants.RequiresMFA(ctx, sub.user.ID)
		p.Sub = name + " requires two-step verification. Set it up to finish signing in as " + sub.user.Email + "."
		p.AuthRequestID = sub.challenge.AuthRequestID
	} else {
		p.Sub = "Choose how you'll confirm it's you when you sign in."
	}
	return p
}

func (s *Server) mfaSetupPage(w http.ResponseWriter, r *http.Request) {
	sub, ok := s.setupSubjectFor(w, r)
	if !ok {
		return
	}
	s.render(w, r, "mfa_setup", s.setupPageData(r.Context(), sub), http.StatusOK)
}

func (s *Server) appSetupPage(w http.ResponseWriter, r *http.Request) {
	sub, ok := s.setupSubjectFor(w, r)
	if !ok {
		return
	}
	setup, err := s.ids.BeginTOTP(r.Context(), sub.user)
	if err != nil {
		s.signInFailed(w, r, err)
		return
	}
	p := s.setupPageData(r.Context(), sub)
	p.Title, p.Heading = "Authenticator app", "Set up an authenticator app"
	p.Sub = "Scan this with Google Authenticator, 1Password, Authy or any app that makes 6-digit codes, then enter the code it shows."
	p.Secret, p.FactorID = groupSecret(setup.Secret), setup.FactorID.String()
	p.QR = qrDataURL(setup.URI)
	s.render(w, r, "mfa_app", p, http.StatusOK)
}

func (s *Server) appSetupSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.postGuard(w, r) {
		return
	}
	sub, ok := s.setupSubjectFor(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	factorID, err := uuid.Parse(r.PostFormValue("factor"))
	if err != nil {
		s.message(w, r, "Setup expired", "Start setting up the authenticator app again.", http.StatusBadRequest)
		return
	}
	if !s.limiter.Allow(ctx, "mfa:user:"+sub.user.ID.String(), 10, 15*time.Minute) {
		s.message(w, r, "Slow down", "Too many attempts. Wait a few minutes and try again.", http.StatusTooManyRequests)
		return
	}
	first, err := s.ids.ConfirmTOTP(ctx, sub.user.ID, factorID, r.PostFormValue("code"), "user:"+sub.user.ID.String())
	if err != nil {
		if errors.Is(err, identity.ErrBadCode) {
			p := s.setupPageData(ctx, sub)
			p.Title, p.Heading, p.Error = "Authenticator app", "Set up an authenticator app", err.Error()
			p.Sub = "The code didn't match. Scan the code again (or enter the key) and type the newest code."
			if setup, err := s.ids.BeginTOTP(ctx, sub.user); err == nil {
				p.Secret, p.FactorID, p.QR = groupSecret(setup.Secret), setup.FactorID.String(), qrDataURL(setup.URI)
			}
			s.render(w, r, "mfa_app", p, http.StatusBadRequest)
			return
		}
		s.signInFailed(w, r, err)
		return
	}
	s.afterFactorAdded(w, r, sub, first, identity.AMROTP)
}

// afterFactorAdded shows the recovery codes when this was the first factor,
// then carries on: into the sign-in a requirement held back, or back to
// where the person came from.
func (s *Server) afterFactorAdded(w http.ResponseWriter, r *http.Request, sub setupSubject, first bool, method string) {
	ctx := r.Context()
	if first {
		codes, err := s.ids.NewRecoveryCodes(ctx, sub.user.ID, "user:"+sub.user.ID.String())
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		p := page{Title: "Recovery codes", Heading: "Save your recovery codes",
			Sub:   "If you lose your device, each of these signs you in once. Keep them somewhere safe; they won't be shown again.",
			Codes: codes, ReturnTo: sub.returnTo, Method: method}
		if sub.challenge != nil {
			p.Confirm = "/mfa/setup/done"
		}
		s.render(w, r, "recovery", p, http.StatusOK)
		return
	}
	if sub.challenge != nil {
		to, err := s.finishChallenge(w, r, *sub.challenge, sub.user, method, identity.AMRMFA)
		if err != nil {
			s.signInFailed(w, r, err)
			return
		}
		http.Redirect(w, r, to, http.StatusFound)
		return
	}
	http.Redirect(w, r, s.returnOrConsole(sub.returnTo), http.StatusFound)
}

// setupDone: the recovery codes are saved; finish the sign-in a requirement held back.
func (s *Server) setupDone(w http.ResponseWriter, r *http.Request) {
	if !s.postGuard(w, r) {
		return
	}
	ch, u, ok := s.pendingChallenge(r, identity.ChallengeEnroll)
	if !ok {
		s.challengeExpired(w, r)
		return
	}
	method := r.PostFormValue("method")
	if method != identity.AMROTP && method != identity.AMRPasskey {
		method = identity.AMROTP
	}
	to, err := s.finishChallenge(w, r, ch, u, method, identity.AMRMFA)
	if err != nil {
		s.signInFailed(w, r, err)
		return
	}
	http.Redirect(w, r, to, http.StatusFound)
}

func groupSecret(secret string) string {
	var b strings.Builder
	for i, r := range secret {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// qrDataURL draws the otpauth address as a PNG inline in the page: nothing is
// loaded from anywhere, and the secret never leaves the response.
func qrDataURL(text string) template.URL {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return ""
	}
	// PNG draws the quiet zone and scales each module (Image() doesn't scale).
	code.Scale = 4
	return template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG())) //nolint:gosec // our own PNG
}

// ---------- Confirming it's you: /reauth ----------

// safeReturn keeps a return address only when it is the console or Hayat
// itself, so /reauth can't be used to bounce people elsewhere.
func (s *Server) safeReturn(raw string) string {
	if raw == "" {
		return ""
	}
	for _, base := range []string{s.cfg.ConsoleURL, s.cfg.Issuer} {
		if raw == base || strings.HasPrefix(raw, base+"/") || strings.HasPrefix(raw, base+"?") {
			return raw
		}
	}
	return ""
}

func (s *Server) returnOrConsole(to string) string {
	if to == "" {
		return s.cfg.ConsoleURL
	}
	return to
}

func (s *Server) reauthPageData(ctx context.Context, u identity.User, returnTo string) page {
	p := page{Title: "Confirm it's you", Heading: "Confirm it's you", ReturnTo: returnTo, Email: u.Email,
		Sub: "You're about to change something sensitive on " + u.Email + ". Confirm it's you to go on."}
	s.methodsFor(ctx, &p, u)
	if !p.HasPassword && !p.HasTOTP && !p.HasPasskey {
		linked, err := s.fed.Linked(ctx, u)
		if err == nil {
			for _, l := range linked {
				if l.Provider.Kind == "" {
					continue // a removed provider
				}
				p.Providers = append(p.Providers, providerButton{Name: l.Provider.Name, Kind: l.Provider.Kind,
					URL: "/reauth/sso/" + l.Provider.ID.String() + "?return_to=" + url.QueryEscape(returnTo)})
			}
		}
	}
	return p
}

func (s *Server) reauthPage(w http.ResponseWriter, r *http.Request) {
	_, u, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	s.render(w, r, "reauth", s.reauthPageData(r.Context(), u, s.safeReturn(r.URL.Query().Get("return_to"))), http.StatusOK)
}

func (s *Server) reauthSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.postGuard(w, r) {
		return
	}
	sess, u, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	ctx := r.Context()
	returnTo := s.safeReturn(r.PostFormValue("return_to"))
	p := s.reauthPageData(ctx, u, returnTo)
	if !s.limiter.Allow(ctx, "reauth:user:"+u.ID.String(), 10, 15*time.Minute) {
		p.Error = "Too many attempts. Wait a few minutes and try again."
		s.render(w, r, "reauth", p, http.StatusTooManyRequests)
		return
	}
	var method string
	var err error
	if pw := r.PostFormValue("password"); pw != "" {
		method = identity.AMRPassword
		_, err = s.ids.Authenticate(ctx, u.Email, pw)
	} else {
		method, err = s.secondFactor(ctx, u, strings.TrimSpace(r.PostFormValue("code")))
	}
	if err != nil {
		if errs.Is(err) {
			p.Error = err.Error()
			s.render(w, r, "reauth", p, http.StatusUnauthorized)
			return
		}
		s.serverError(w, r, err)
		return
	}
	if err := s.ids.Reverify(ctx, sess, method); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, s.returnOrConsole(returnTo), http.StatusFound)
}

// reauthSSO: /reauth/sso/{id} — confirm it's you with the account you signed
// in with, for someone who has no password, app or passkey.
func (s *Server) reauthSSO(w http.ResponseWriter, r *http.Request) {
	_, u, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	p, ok := s.providerParam(w, r)
	if !ok {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: reauthCookie, Value: s.safeReturn(r.URL.Query().Get("return_to")), Path: "/sso", HttpOnly: true,
		MaxAge: 600, Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
	s.toProvider(w, r, p, reauthTrip, nil, u.Email)
}

// ---------- Passkeys: the JSON half of the ceremonies hayat.js runs ----------

// jsonGuard is postGuard for fetch(): the CSRF token rides in a header.
func (s *Server) jsonGuard(w http.ResponseWriter, r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil || c.Value == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "This page expired. Reload it and try again."})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) jsonFail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errExpired) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errExpired.Error()})
		return
	}
	if errs.Is(err) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.log.Error("passkey", "path", r.URL.Path, "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Something went wrong. Try again in a moment."})
}

// ceremony is what a passkey ceremony keeps between begin and finish.
type ceremony struct {
	Purpose string          `json:"purpose"` // login | mfa | reauth | register
	State   json.RawMessage `json:"state"`
}

type passkeyBody struct {
	Purpose       string          `json:"purpose"`
	AuthRequestID string          `json:"authRequestID"`
	ReturnTo      string          `json:"returnTo"`
	Name          string          `json:"name"`
	Credential    json.RawMessage `json:"credential"`
}

func readPasskeyBody(w http.ResponseWriter, r *http.Request) (passkeyBody, bool) {
	var b passkeyBody
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil || json.Unmarshal(raw, &b) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That request didn't make sense. Reload the page and try again."})
		return b, false
	}
	return b, true
}

// passkeyBegin: POST /passkey/begin — options for navigator.credentials.
//
//	login:    sign in with a passkey, nobody named yet
//	mfa:      the second factor of a pending sign-in
//	reauth:   confirm it's you, signed in
//	register: add a passkey (a required setup, or from Account)
func (s *Server) passkeyBegin(w http.ResponseWriter, r *http.Request) {
	if !s.jsonGuard(w, r) {
		return
	}
	b, ok := readPasskeyBody(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if !s.limiter.Allow(ctx, "passkey:ip:"+clientIP(r), 60, time.Minute) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many attempts. Wait a minute and try again."})
		return
	}
	var options, state []byte
	var err error
	var who *uuid.UUID
	switch b.Purpose {
	case "login":
		options, state, err = s.ids.BeginPasskeyLogin(ctx, nil)
	case "mfa":
		_, u, ok := s.pendingChallenge(r, identity.ChallengeMFA)
		if !ok {
			s.jsonFail(w, r, errExpired)
			return
		}
		who = &u.ID
		options, state, err = s.ids.BeginPasskeyLogin(ctx, &u)
	case "reauth":
		_, u, ok := s.currentSession(r)
		if !ok {
			s.jsonFail(w, r, errs.New("you're signed out: sign in again"))
			return
		}
		who = &u.ID
		options, state, err = s.ids.BeginPasskeyLogin(ctx, &u)
	case "register":
		u, ok := s.registrant(r)
		if !ok {
			s.jsonFail(w, r, errs.New("confirm it's you again, then add the passkey"))
			return
		}
		who = &u.ID
		options, state, err = s.ids.BeginPasskeyRegistration(ctx, u)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unknown passkey request."})
		return
	}
	if err != nil {
		s.jsonFail(w, r, err)
		return
	}
	cookie, _, err := s.ids.NewChallenge(ctx, identity.ChallengePasskey, who, nil, b.AuthRequestID, ceremony{Purpose: b.Purpose, State: state}, ceremonyTTL)
	if err != nil {
		s.jsonFail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: passkeyCookie, Value: cookie, Path: "/passkey", HttpOnly: true, MaxAge: int(ceremonyTTL.Seconds()),
		Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteStrictMode})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(options)
}

// registrant is who may add a passkey now: the person a required setup holds
// back, or someone signed in who confirmed it's them in the last minutes.
func (s *Server) registrant(r *http.Request) (identity.User, bool) {
	if _, u, ok := s.pendingChallenge(r, identity.ChallengeEnroll); ok {
		return u, true
	}
	sess, u, ok := s.currentSession(r)
	if !ok || !sess.RecentlyVerified() {
		return identity.User{}, false
	}
	return u, true
}

// passkeyFinish: POST /passkey/finish — the device's answer. Replies with
// where the page goes next, or an error to show.
func (s *Server) passkeyFinish(w http.ResponseWriter, r *http.Request) {
	if !s.jsonGuard(w, r) {
		return
	}
	b, ok := readPasskeyBody(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	pc, err := r.Cookie(passkeyCookie)
	if err != nil {
		s.jsonFail(w, r, errExpired)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: passkeyCookie, Value: "", Path: "/passkey", HttpOnly: true, MaxAge: -1,
		Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteStrictMode})
	ch, err := s.ids.ChallengeByCookie(ctx, pc.Value, identity.ChallengePasskey)
	if err != nil {
		s.jsonFail(w, r, errExpired)
		return
	}
	// A ceremony answers once.
	if used, err := s.ids.EndChallenge(ctx, ch); err != nil || !used {
		s.jsonFail(w, r, errExpired)
		return
	}
	var cer ceremony
	if err := json.Unmarshal(ch.Data, &cer); err != nil || cer.Purpose != b.Purpose {
		s.jsonFail(w, r, errExpired)
		return
	}

	var to string
	switch cer.Purpose {
	case "login":
		u, err := s.ids.FinishPasskeyLogin(ctx, cer.State, b.Credential, nil)
		if err != nil {
			s.jsonFail(w, r, err)
			return
		}
		if u.Status != identity.StatusActive {
			s.jsonFail(w, r, identity.ErrDisabled)
			return
		}
		to, err = s.next(w, r, firstFactor{user: u, amr: []string{identity.AMRPasskey, identity.AMRMFA}, authRequestID: ch.AuthRequestID})
		if err != nil {
			s.jsonFail(w, r, err)
			return
		}
	case "mfa":
		pending, u, ok := s.pendingChallenge(r, identity.ChallengeMFA)
		if !ok || ch.UserID == nil || *ch.UserID != u.ID {
			s.jsonFail(w, r, errExpired)
			return
		}
		if _, err := s.ids.FinishPasskeyLogin(ctx, cer.State, b.Credential, &u.ID); err != nil {
			s.jsonFail(w, r, err)
			return
		}
		to, err = s.finishChallenge(w, r, pending, u, identity.AMRPasskey, identity.AMRMFA)
		if err != nil {
			s.jsonFail(w, r, err)
			return
		}
	case "reauth":
		sess, u, ok := s.currentSession(r)
		if !ok || ch.UserID == nil || *ch.UserID != u.ID {
			s.jsonFail(w, r, errs.New("you're signed out: sign in again"))
			return
		}
		if _, err := s.ids.FinishPasskeyLogin(ctx, cer.State, b.Credential, &u.ID); err != nil {
			s.jsonFail(w, r, err)
			return
		}
		if err := s.ids.Reverify(ctx, sess, identity.AMRPasskey); err != nil {
			s.jsonFail(w, r, err)
			return
		}
		to = s.returnOrConsole(s.safeReturn(b.ReturnTo))
	case "register":
		u, ok := s.registrant(r)
		if !ok || ch.UserID == nil || *ch.UserID != u.ID {
			s.jsonFail(w, r, errs.New("confirm it's you again, then add the passkey"))
			return
		}
		first, err := s.ids.FinishPasskeyRegistration(ctx, u, cer.State, b.Credential, b.Name, "user:"+u.ID.String())
		if err != nil {
			s.jsonFail(w, r, err)
			return
		}
		// The page posts on to /mfa/setup/added, which shows the recovery
		// codes or carries on, like the authenticator app's setup.
		v := url.Values{"method": {identity.AMRPasskey}}
		if first {
			v.Set("first", "1")
		}
		if rt := s.safeReturn(b.ReturnTo); rt != "" {
			v.Set("return_to", rt)
		}
		to = "/mfa/setup/added?" + v.Encode()
	}
	writeJSON(w, http.StatusOK, map[string]string{"redirect": to})
}

// passkeyAdded: GET /mfa/setup/added — after a passkey was registered.
func (s *Server) passkeyAdded(w http.ResponseWriter, r *http.Request) {
	sub, ok := s.setupSubjectFor(w, r)
	if !ok {
		return
	}
	first := r.URL.Query().Get("first") == "1"
	if first {
		// Recovery codes are made once, for the first factor; a reload of
		// this page must not mint another set behind the person's back.
		if n, err := s.ids.RecoveryCodesLeft(r.Context(), sub.user.ID); err == nil && n > 0 {
			first = false
		}
	}
	s.afterFactorAdded(w, r, sub, first, identity.AMRPasskey)
}
