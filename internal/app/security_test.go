package app_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/admin/v1/adminv1connect"
	"github.com/aidapedia/hayat/internal/identity"
)

func stepUpRequired(err error) bool {
	var ce *connect.Error
	return errors.As(err, &ce) && ce.Code() == connect.CodePermissionDenied && ce.Meta().Get("Hayat-Step-Up") == "required"
}

func TestSecurityAPIAndStepUp(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root, err := e.a.IDs.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password")
	if err != nil {
		t.Fatal(err)
	}
	other, err := e.a.IDs.CreateActive(ctx, "oli@example.com", "Oli", "another long password", false, "test")
	if err != nil {
		t.Fatal(err)
	}
	token, browser := e.signIn("root@example.com", "a long enough password")
	account := adminv1connect.NewAccountServiceClient(http.DefaultClient, e.admin, bearer(token))

	sec, err := account.GetMySecurity(ctx, connect.NewRequest(&adminv1.GetMySecurityRequest{}))
	if err != nil || len(sec.Msg.Factors) != 0 || sec.Msg.SetupUrl != e.auth+"/mfa/setup" || sec.Msg.ReauthUrl != e.auth+"/reauth" {
		t.Fatalf("security: %+v %v", sec, err)
	}
	if _, err := account.RegenerateRecoveryCodes(ctx, connect.NewRequest(&adminv1.RegenerateRecoveryCodesRequest{})); code(err) != connect.CodeInvalidArgument {
		t.Fatalf("no codes without a factor: %v", err)
	}

	setup, _ := e.a.IDs.BeginTOTP(ctx, root)
	if _, err := e.a.IDs.ConfirmTOTP(ctx, root.ID, setup.FactorID, identity.TOTPCode(setup.Secret, time.Now()), "test"); err != nil {
		t.Fatal(err)
	}
	// Just signed in: recent enough for sensitive changes.
	codes, err := account.RegenerateRecoveryCodes(ctx, connect.NewRequest(&adminv1.RegenerateRecoveryCodesRequest{}))
	if err != nil || len(codes.Msg.Codes) != 10 {
		t.Fatalf("codes: %v", err)
	}

	// Twenty minutes later, the same change asks to confirm it's them.
	if _, err := e.a.Store.Pool.Exec(ctx, `UPDATE sessions SET verified_at = now() - interval '20 minutes' WHERE user_id = $1`, root.ID); err != nil {
		t.Fatal(err)
	}
	sec, _ = account.GetMySecurity(ctx, connect.NewRequest(&adminv1.GetMySecurityRequest{}))
	factorID := sec.Msg.Factors[0].Id
	if _, err := account.RemoveMyFactor(ctx, connect.NewRequest(&adminv1.RemoveMyFactorRequest{FactorId: factorID})); !stepUpRequired(err) {
		t.Fatalf("an old confirmation must ask again: %v", err)
	}
	users := adminv1connect.NewUserServiceClient(http.DefaultClient, e.admin, bearer(token))
	if _, err := users.SetUserAdmin(ctx, connect.NewRequest(&adminv1.SetUserAdminRequest{Id: other.ID.String(), Admin: true})); !stepUpRequired(err) {
		t.Fatalf("promoting someone is sensitive: %v", err)
	}

	// Confirm on Hayat's page (the browser holds the session cookie).
	page := e.must(browser.Get(e.auth + "/reauth?return_to=" + url.QueryEscape(consoleURL+"/account")))
	body, _ := io.ReadAll(page.Body)
	csrf := csrfRe.FindSubmatch(body)
	resp := e.must(browser.PostForm(e.auth+"/reauth", url.Values{"csrf": {string(csrf[1])}, "password": {"a long enough password"},
		"return_to": {consoleURL + "/account"}}))
	if loc, _ := resp.Location(); loc == nil || loc.String() != consoleURL+"/account" {
		t.Fatalf("reauth should return to the console: %d %v", resp.StatusCode, loc)
	}

	// A tenant that requires a second factor keeps the last one.
	tenants := adminv1connect.NewTenantServiceClient(http.DefaultClient, e.admin, bearer(token))
	tn, err := tenants.CreateTenant(ctx, connect.NewRequest(&adminv1.CreateTenantRequest{Name: "Strict", OwnerUserId: root.ID.String()}))
	if err != nil {
		t.Fatal(err)
	}
	strict, err := tenants.SetTenantRequireMFA(ctx, connect.NewRequest(&adminv1.SetTenantRequireMFARequest{Id: tn.Msg.Tenant.Id, RequireMfa: true}))
	if err != nil || !strict.Msg.Tenant.RequireMfa {
		t.Fatalf("require mfa: %v", err)
	}
	if _, err := account.RemoveMyFactor(ctx, connect.NewRequest(&adminv1.RemoveMyFactorRequest{FactorId: factorID})); code(err) != connect.CodeFailedPrecondition {
		t.Fatalf("the required last factor stays: %v", err)
	}
	if _, err := tenants.SetTenantRequireMFA(ctx, connect.NewRequest(&adminv1.SetTenantRequireMFARequest{Id: tn.Msg.Tenant.Id})); err != nil {
		t.Fatal(err)
	}
	if _, err := account.RemoveMyFactor(ctx, connect.NewRequest(&adminv1.RemoveMyFactorRequest{FactorId: factorID})); err != nil {
		t.Fatalf("remove after confirming: %v", err)
	}

	// An admin resets someone who lost every factor.
	osetup, _ := e.a.IDs.BeginTOTP(ctx, other)
	if _, err := e.a.IDs.ConfirmTOTP(ctx, other.ID, osetup.FactorID, identity.TOTPCode(osetup.Secret, time.Now()), "test"); err != nil {
		t.Fatal(err)
	}
	got, err := users.GetUser(ctx, connect.NewRequest(&adminv1.GetUserRequest{Id: other.ID.String()}))
	if err != nil || len(got.Msg.Factors) != 1 {
		t.Fatalf("an admin sees the factors: %v", err)
	}
	if _, err := users.ResetUserFactors(ctx, connect.NewRequest(&adminv1.ResetUserFactorsRequest{Id: other.ID.String()})); err != nil {
		t.Fatal(err)
	}
	if has, _ := e.a.IDs.HasSecondFactor(ctx, other.ID); has {
		t.Fatal("reset left a factor")
	}
}
