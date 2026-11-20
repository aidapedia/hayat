package identity_test

import (
	"context"
	"crypto/sha1" //nolint:gosec // the test server answers like Have I Been Pwned
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/testkey"
)

func secureService(t *testing.T, sec identity.Security) (*identity.Service, *store.Store) {
	t.Helper()
	svc, s := newService(t)
	if sec.Box == nil {
		box, err := crypto.NewBox(make([]byte, 32))
		if err != nil {
			t.Fatal(err)
		}
		sec.Box = box
	}
	svc.SetSecurity(sec)
	return svc, s
}

func activeUser(t *testing.T, svc *identity.Service, email string) identity.User {
	t.Helper()
	u, err := svc.CreateActive(context.Background(), email, "", "correct horse battery", false, "test")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestLockoutAfterTenWrongPasswords(t *testing.T) {
	svc, s := secureService(t, identity.Security{})
	ctx := context.Background()
	activeUser(t, svc, "lee@example.com")

	for i := 0; i < 10; i++ {
		if _, err := svc.Authenticate(ctx, "lee@example.com", "wrong password!!"); !errors.Is(err, identity.ErrBadCredentials) {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	// Locked: even the right password is refused, with the rate-limit wording.
	if _, err := svc.Authenticate(ctx, "lee@example.com", "correct horse battery"); !errors.Is(err, identity.ErrLocked) {
		t.Fatalf("a locked account signed in: %v", err)
	}
	var subject string
	if err := s.Pool.QueryRow(ctx, `SELECT subject FROM email_outbox WHERE to_addr = 'lee@example.com' ORDER BY created_at DESC LIMIT 1`).Scan(&subject); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(subject, "locked") {
		t.Fatalf("the owner should hear about the lock, got %q", subject)
	}

	// A reset lifts it: the mailbox answered.
	if err := svc.RequestPasswordReset(ctx, "lee@example.com"); err != nil {
		t.Fatal(err)
	}
	_, region, token := lastLink(t, s, "lee@example.com")
	if _, err := svc.ResetPassword(ctx, region, token, "a brand new password"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, "lee@example.com", "a brand new password"); err != nil {
		t.Fatalf("a reset should lift the lock: %v", err)
	}
}

func TestLockoutCountsOnlyConsecutiveFailures(t *testing.T) {
	svc, _ := secureService(t, identity.Security{})
	ctx := context.Background()
	activeUser(t, svc, "kim@example.com")
	for round := 0; round < 3; round++ {
		for i := 0; i < 9; i++ {
			_, _ = svc.Authenticate(ctx, "kim@example.com", "wrong password!!")
		}
		if _, err := svc.Authenticate(ctx, "kim@example.com", "correct horse battery"); err != nil {
			t.Fatalf("round %d: a success should reset the count: %v", round, err)
		}
	}
}

// pwnedServer answers like api.pwnedpasswords.com, knowing one password.
func pwnedServer(t *testing.T, known string) *httptest.Server {
	sum := sha1.Sum([]byte(known)) //nolint:gosec
	full := strings.ToUpper(hex.EncodeToString(sum[:]))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Add-Padding") != "true" {
			t.Error("ask for padding, so the answer's size says nothing")
		}
		prefix := strings.TrimPrefix(r.URL.Path, "/range/")
		if len(prefix) != 5 {
			t.Errorf("only five characters may leave the server, sent %q", prefix)
		}
		fmt.Fprintln(w, "0018A45C4D1DEF81644B54AB7F969B88D65:0") // padding
		if prefix == full[:5] {
			fmt.Fprintf(w, "%s:42\r\n", full[5:])
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBreachedPasswordsAreRefused(t *testing.T) {
	srv := pwnedServer(t, "password1234")
	hibp := identity.NewHIBP()
	hibp.BaseURL = srv.URL
	svc, s := secureService(t, identity.Security{Breached: hibp})
	ctx := context.Background()

	if err := svc.Register(ctx, "pat@example.com", ""); err != nil {
		t.Fatal(err)
	}
	_, region, token := lastLink(t, s, "pat@example.com")
	if _, err := svc.CompleteVerification(ctx, region, token, "password1234"); !errors.Is(err, identity.ErrBreachedPassword) {
		t.Fatalf("a breached password was accepted: %v", err)
	}
	u, err := svc.CompleteVerification(ctx, region, token, "plenty unusual words")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(ctx, u.ID, "plenty unusual words", "password1234", u.ID); !errors.Is(err, identity.ErrBreachedPassword) {
		t.Fatalf("change password took a breached one: %v", err)
	}

	// An outage lets the password through rather than blocking people.
	srv.Close()
	if err := svc.ChangePassword(ctx, u.ID, "plenty unusual words", "another fine choice", u.ID); err != nil {
		t.Fatalf("an unreachable breach list must not block: %v", err)
	}
}

func TestTOTPSetupAndSecondFactor(t *testing.T) {
	svc, _ := secureService(t, identity.Security{})
	ctx := context.Background()
	u := activeUser(t, svc, "toni@example.com")

	if ok, _ := svc.HasSecondFactor(ctx, u.ID); ok {
		t.Fatal("no factor yet")
	}
	setup, err := svc.BeginTOTP(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(setup.URI, "otpauth://totp/auth.test:toni@example.com?") || !strings.Contains(setup.URI, "secret="+setup.Secret) {
		t.Fatalf("otpauth uri: %s", setup.URI)
	}
	// Not counted until a code from it is entered.
	if ok, _ := svc.HasSecondFactor(ctx, u.ID); ok {
		t.Fatal("an unconfirmed app must not count")
	}
	wrong := "000000"
	if identity.TOTPCode(setup.Secret, time.Now()) == wrong {
		wrong = "111111"
	}
	if _, err := svc.ConfirmTOTP(ctx, u.ID, setup.FactorID, wrong, "test"); !errors.Is(err, identity.ErrBadCode) {
		t.Fatalf("wrong code accepted: %v", err)
	}
	// The code of the previous step: confirming uses it up.
	prev := identity.TOTPCode(setup.Secret, time.Now().Add(-30*time.Second))
	first, err := svc.ConfirmTOTP(ctx, u.ID, setup.FactorID, prev, "test")
	if err != nil || !first {
		t.Fatalf("confirm: first=%v err=%v", first, err)
	}
	if ok, _ := svc.HasSecondFactor(ctx, u.ID); !ok {
		t.Fatal("a confirmed app counts")
	}
	if err := svc.VerifyTOTP(ctx, u.ID, prev); !errors.Is(err, identity.ErrBadCode) {
		t.Fatalf("the code used to confirm must not work again: %v", err)
	}
	now := identity.TOTPCode(setup.Secret, time.Now())
	if err := svc.VerifyTOTP(ctx, u.ID, now); err != nil {
		t.Fatalf("current code: %v", err)
	}
	if err := svc.VerifyTOTP(ctx, u.ID, now); !errors.Is(err, identity.ErrBadCode) {
		t.Fatalf("a code works once: %v", err)
	}
	if err := svc.VerifyTOTP(ctx, u.ID, identity.TOTPCode(setup.Secret, time.Now().Add(-5*time.Minute))); !errors.Is(err, identity.ErrBadCode) {
		t.Fatalf("an old code must fail: %v", err)
	}
}

func TestRecoveryCodes(t *testing.T) {
	svc, _ := secureService(t, identity.Security{})
	ctx := context.Background()
	u := activeUser(t, svc, "rae@example.com")

	codes, err := svc.NewRecoveryCodes(ctx, u.ID, "test")
	if err != nil || len(codes) != 10 {
		t.Fatalf("codes: %v %v", codes, err)
	}
	if err := svc.UseRecoveryCode(ctx, u.ID, strings.ToUpper(codes[3])); err != nil {
		t.Fatalf("a code works whatever its case: %v", err)
	}
	if err := svc.UseRecoveryCode(ctx, u.ID, codes[3]); err == nil {
		t.Fatal("a recovery code works once")
	}
	if n, _ := svc.RecoveryCodesLeft(ctx, u.ID); n != 9 {
		t.Fatalf("left: %d", n)
	}
	fresh, err := svc.NewRecoveryCodes(ctx, u.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UseRecoveryCode(ctx, u.ID, codes[0]); err == nil {
		t.Fatal("a fresh set voids the old one")
	}
	if err := svc.UseRecoveryCode(ctx, u.ID, fresh[0]); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveFactorRules(t *testing.T) {
	svc, _ := secureService(t, identity.Security{})
	ctx := context.Background()
	u := activeUser(t, svc, "rob@example.com")
	setup, _ := svc.BeginTOTP(ctx, u)
	if _, err := svc.ConfirmTOTP(ctx, u.ID, setup.FactorID, identity.TOTPCode(setup.Secret, time.Now()), "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.NewRecoveryCodes(ctx, u.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveFactor(ctx, u.ID, setup.FactorID, true, false, "test"); !errors.Is(err, identity.ErrLastFactor) {
		t.Fatalf("a required last factor was removed: %v", err)
	}
	if err := svc.RemoveFactor(ctx, u.ID, setup.FactorID, false, false, "test"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.HasSecondFactor(ctx, u.ID); ok {
		t.Fatal("factor still there")
	}
	if n, _ := svc.RecoveryCodesLeft(ctx, u.ID); n != 0 {
		t.Fatalf("with no factor left the recovery codes go too, %d left", n)
	}
}

func TestPasskeyRegistrationAndSignIn(t *testing.T) {
	svc, _ := secureService(t, identity.Security{})
	ctx := context.Background()
	u := activeUser(t, svc, "pia@example.com")
	key := testkey.New("http://auth.test")

	opts, state, err := svc.BeginPasskeyRegistration(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := key.Register(opts)
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.FinishPasskeyRegistration(ctx, u, state, resp, "Laptop", "test")
	if err != nil || !first {
		t.Fatalf("register: first=%v err=%v", first, err)
	}
	if _, err := svc.FinishPasskeyRegistration(ctx, u, state, resp, "Again", "test"); err == nil {
		t.Fatal("a registration answer must not be accepted twice")
	}

	// Sign in with nothing but the passkey.
	opts, state, err = svc.BeginPasskeyLogin(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = key.Sign(opts)
	if err != nil {
		t.Fatal(err)
	}
	who, err := svc.FinishPasskeyLogin(ctx, state, resp, nil)
	if err != nil || who.ID != u.ID {
		t.Fatalf("passkey sign-in: %v %v", who.Email, err)
	}

	// As a second factor for a named account, another account's passkey fails.
	other := activeUser(t, svc, "oz@example.com")
	opts, state, err = svc.BeginPasskeyLogin(ctx, &u)
	if err != nil {
		t.Fatal(err)
	}
	resp, _ = key.Sign(opts)
	if _, err := svc.FinishPasskeyLogin(ctx, state, resp, &other.ID); err == nil {
		t.Fatal("a passkey confirmed someone else")
	}
	if _, err := svc.FinishPasskeyLogin(ctx, state, resp, &u.ID); err != nil {
		t.Fatalf("second factor: %v", err)
	}

	// The last passkey of an account with no other way in stays.
	f, _ := svc.Factors(ctx, u.ID)
	if len(f) != 1 || f[0].Kind != identity.FactorPasskey || f[0].Name != "Laptop" || f[0].LastUsedAt == nil {
		t.Fatalf("factors: %+v", f)
	}
	if err := svc.RemoveFactor(ctx, u.ID, f[0].ID, false, true, "test"); !errors.Is(err, identity.ErrLastWayIn) {
		t.Fatalf("last way in removed: %v", err)
	}
}

func TestIdleTimeoutAndReverify(t *testing.T) {
	svc, s := secureService(t, identity.Security{IdleTimeout: time.Hour})
	ctx := context.Background()
	u := activeUser(t, svc, "ida@example.com")
	cookie, sess, err := svc.CreateSession(ctx, u, []string{identity.AMRPassword}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !sess.RecentlyVerified() {
		t.Fatal("a new session was just verified")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE sessions SET verified_at = now() - interval '1 hour' WHERE id = $1`, sess.ID); err != nil {
		t.Fatal(err)
	}
	sess, _ = svc.SessionByID(ctx, sess.Region, sess.ID)
	if sess.RecentlyVerified() {
		t.Fatal("an hour ago is not recent")
	}
	if err := svc.Reverify(ctx, sess, "pwd"); err != nil {
		t.Fatal(err)
	}
	sess, _ = svc.SessionByID(ctx, sess.Region, sess.ID)
	if !sess.RecentlyVerified() {
		t.Fatal("reverify should count")
	}

	if _, err := s.Pool.Exec(ctx, `UPDATE sessions SET last_seen_at = now() - interval '2 hours' WHERE id = $1`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.SessionByCookie(ctx, cookie); !errors.Is(err, identity.ErrNoSession) {
		t.Fatalf("an idle session still works: %v", err)
	}
	sess, _ = svc.SessionByID(ctx, sess.Region, sess.ID)
	if sess.Active() {
		t.Fatal("an idle session is not active, so its tokens stop refreshing")
	}
	if list, _ := svc.ListSessions(ctx, u.ID); len(list) != 0 {
		t.Fatalf("idle sessions are not listed: %d", len(list))
	}
}

func TestChallenges(t *testing.T) {
	svc, _ := secureService(t, identity.Security{})
	ctx := context.Background()
	u := activeUser(t, svc, "cy@example.com")
	cookie, c, err := svc.NewChallenge(ctx, identity.ChallengeMFA, &u.ID, []string{"pwd"}, "req-1", nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChallengeByCookie(ctx, cookie, identity.ChallengePasskey); err == nil {
		t.Fatal("a challenge is only good for its kind")
	}
	got, err := svc.ChallengeByCookie(ctx, cookie, identity.ChallengeMFA)
	if err != nil || got.AuthRequestID != "req-1" || *got.UserID != u.ID {
		t.Fatalf("challenge: %+v %v", got, err)
	}
	for i := 0; i < identity.MaxChallengeAttempts; i++ {
		if _, err := svc.FailChallenge(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ChallengeByCookie(ctx, cookie, identity.ChallengeMFA); err == nil {
		t.Fatal("too many wrong answers end a challenge")
	}
}
