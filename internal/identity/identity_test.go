package identity_test

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/testdb"
)

func newService(t *testing.T) (*identity.Service, *store.Store) {
	s := testdb.New(t)
	return identity.New(s, "http://auth.test", time.Hour), s
}

var linkRe = regexp.MustCompile(`http://auth\.test(/\w+)\?r=(\w+)&t=([\w-]+)`)

// lastLink returns the path, region and token of the newest emailed link to addr.
func lastLink(t *testing.T, s *store.Store, addr string) (string, string, string) {
	t.Helper()
	var body string
	err := s.Pool.QueryRow(context.Background(),
		`SELECT body_text FROM email_outbox WHERE to_addr = $1 ORDER BY created_at DESC, id DESC LIMIT 1`, addr).Scan(&body)
	if err != nil {
		t.Fatalf("no email to %s: %v", addr, err)
	}
	m := linkRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no link in email: %s", body)
	}
	token, _ := url.QueryUnescape(m[3])
	return m[1], m[2], token
}

func TestRegisterThenVerifyThenSignIn(t *testing.T) {
	svc, s := newService(t)
	ctx := context.Background()

	if err := svc.Register(ctx, "ana@example.com", "Ana"); err != nil {
		t.Fatal(err)
	}
	u, err := svc.GetUserByEmail(ctx, "ANA@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.Status != identity.StatusPending || u.HasPassword {
		t.Fatalf("a new sign-up must be pending with no password, got %+v", u)
	}
	if _, err := svc.Authenticate(ctx, "ana@example.com", "anything-at-all"); !errors.Is(err, identity.ErrPending) {
		t.Fatalf("pending account signed in: %v", err)
	}

	path, region, token := lastLink(t, s, "ana@example.com")
	if path != "/verify" {
		t.Fatalf("want a /verify link, got %s", path)
	}
	if _, err := svc.CompleteVerification(ctx, region, token, "short"); !errors.Is(err, identity.ErrWeakPassword) {
		t.Fatalf("weak password accepted: %v", err)
	}
	u, err = svc.CompleteVerification(ctx, region, token, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if u.Status != identity.StatusActive || !u.EmailVerified {
		t.Fatalf("verified account should be active and verified: %+v", u)
	}
	if _, err := svc.CompleteVerification(ctx, region, token, "another long password"); !errors.Is(err, identity.ErrLinkInvalid) {
		t.Fatalf("a link must work once: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "ana@example.com", "wrong password!!"); !errors.Is(err, identity.ErrBadCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "ana@example.com", "correct horse battery"); err != nil {
		t.Fatalf("sign in: %v", err)
	}
}

func TestRegisterDoesNotRevealExistingAccounts(t *testing.T) {
	svc, s := newService(t)
	ctx := context.Background()
	if _, err := svc.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Register(ctx, "root@example.com", "Mallory"); err != nil {
		t.Fatalf("registering an existing address must look like success, got %v", err)
	}
	var subject string
	_ = s.Pool.QueryRow(context.Background(), `SELECT subject FROM email_outbox WHERE to_addr = 'root@example.com'`).Scan(&subject)
	if subject != "You already have a Hayat account" {
		t.Fatalf("the owner should be told someone tried, got %q", subject)
	}
	u, _ := svc.GetUserByEmail(ctx, "root@example.com")
	if u.Name != "Root" {
		t.Fatal("registering must never change an existing account")
	}
}

func TestResetPasswordEndsSessions(t *testing.T) {
	svc, s := newService(t)
	ctx := context.Background()
	u, err := svc.CreateAdmin(ctx, "ops@example.com", "Ops", "first long password")
	if err != nil {
		t.Fatal(err)
	}
	cookie, _, err := svc.CreateSession(ctx, u, []string{"pwd"}, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.SessionByCookie(ctx, cookie); err != nil {
		t.Fatalf("fresh session: %v", err)
	}

	if err := svc.RequestPasswordReset(ctx, "nobody@example.com"); err != nil {
		t.Fatalf("unknown address must look like success: %v", err)
	}
	if err := svc.RequestPasswordReset(ctx, "ops@example.com"); err != nil {
		t.Fatal(err)
	}
	path, region, token := lastLink(t, s, "ops@example.com")
	if path != "/reset" {
		t.Fatalf("want /reset, got %s", path)
	}
	if _, err := svc.ResetPassword(ctx, region, token, "second long password"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.SessionByCookie(ctx, cookie); !errors.Is(err, identity.ErrNoSession) {
		t.Fatal("a password reset must end existing sessions")
	}
	if _, err := svc.Authenticate(ctx, "ops@example.com", "second long password"); err != nil {
		t.Fatalf("new password: %v", err)
	}
}

func TestLastAdminIsProtected(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	u, err := svc.CreateAdmin(ctx, "only@example.com", "Only", "a long enough password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSystemRole(ctx, u.ID, false, "test"); !errors.Is(err, identity.ErrLastAdmin) {
		t.Fatalf("demoting the last admin: %v", err)
	}
	if _, err := svc.SetStatus(ctx, u.ID, true, "test"); !errors.Is(err, identity.ErrLastAdmin) {
		t.Fatalf("disabling the last admin: %v", err)
	}
}

func TestDisableEndsSessions(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	_, _ = svc.CreateAdmin(ctx, "admin@example.com", "Admin", "a long enough password")
	invited, created, err := svc.Invite(ctx, "bea@example.com", "Bea", "test")
	if err != nil || !created {
		t.Fatalf("invite: %v %v", created, err)
	}
	if invited.Status != identity.StatusPending {
		t.Fatal("an invited account is pending until they choose a password")
	}
	again, created, err := svc.Invite(ctx, "BEA@example.com", "Someone else", "test")
	if err != nil || created || again.ID != invited.ID {
		t.Fatal("inviting an existing address returns that account unchanged")
	}
}

func TestUpdateProfileNormalizesPhoneAndLocale(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	u, err := svc.CreateAdmin(ctx, "cia@example.com", "Cia", "a long enough password")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.UpdateProfile(ctx, u.ID, identity.Profile{Name: "Cia", Locale: "id-id", Zoneinfo: "Asia/Jakarta", Phone: "+62 812-3456 (7890)"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if got.Phone != "+6281234567890" || got.Locale != "id-ID" {
		t.Fatalf("want +6281234567890 and id-ID, got %q and %q", got.Phone, got.Locale)
	}
	for _, bad := range []identity.Profile{
		{Phone: "0812 3456 7890"}, // no country code
		{Phone: "+0812345678"},    // a code never starts with 0
		{Phone: "+62 12"},         // too short
		{Locale: "not a language"},
	} {
		if _, err := svc.UpdateProfile(ctx, u.ID, bad, "test"); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	if got, err := svc.UpdateProfile(ctx, u.ID, identity.Profile{Name: "Cia"}, "test"); err != nil || got.Phone != "" || got.Locale != "" {
		t.Fatalf("clearing the fields: %+v %v", got, err)
	}
}
