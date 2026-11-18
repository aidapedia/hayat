package app_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/admin/v1/adminv1connect"
)

func pngOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src) // opaque, like a photo
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{200, 80, 40, 255})
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// served fetches an image from the auth role and answers its content type.
func (e *env) served(url string) (int, string) {
	e.t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Content-Type")
}

// People upload their own picture; a tenant's admins its logo; a service's
// owners theirs. Each is served by Hayat, and a replaced one stops loading.
func TestAvatarsAndLogos(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.a.IDs.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	token, _ := e.signIn("root@example.com", "a long enough password")

	// A person's picture.
	account := adminv1connect.NewAccountServiceClient(http.DefaultClient, e.admin, bearer(token))
	set, err := account.SetMyAvatar(ctx, connect.NewRequest(&adminv1.SetMyAvatarRequest{Data: pngOf(600, 400)}))
	if err != nil {
		t.Fatal(err)
	}
	first := set.Msg.User.AvatarUrl
	if !strings.HasPrefix(first, e.auth+"/media/") {
		t.Fatalf("the picture is served by Hayat, got %q", first)
	}
	if code, kind := e.served(first); code != http.StatusOK || kind != "image/jpeg" {
		t.Fatalf("avatar: %d %s", code, kind)
	}
	again, err := account.SetMyAvatar(ctx, connect.NewRequest(&adminv1.SetMyAvatarRequest{Data: pngOf(100, 100)}))
	if err != nil || again.Msg.User.AvatarUrl == first {
		t.Fatalf("replace: %v %v", again, err)
	}
	if code, _ := e.served(first); code != http.StatusNotFound {
		t.Fatalf("a replaced picture stops loading, got %d", code)
	}
	if _, err := account.SetMyAvatar(ctx, connect.NewRequest(&adminv1.SetMyAvatarRequest{Data: []byte("<svg/>")})); code(err) != connect.CodeFailedPrecondition {
		t.Fatalf("not an image: want FailedPrecondition, got %v", err)
	}
	cleared, err := account.SetMyAvatar(ctx, connect.NewRequest(&adminv1.SetMyAvatarRequest{}))
	if err != nil || cleared.Msg.User.AvatarUrl != "" {
		t.Fatalf("empty data removes the picture: %v %v", cleared, err)
	}

	// A tenant's logo, and the service API hands it to services.
	tenants := adminv1connect.NewTenantServiceClient(http.DefaultClient, e.admin, bearer(token))
	tn, err := tenants.CreateTenant(ctx, connect.NewRequest(&adminv1.CreateTenantRequest{Name: "Acme", OwnerEmail: "root@example.com"}))
	if err != nil {
		t.Fatal(err)
	}
	logo, err := tenants.SetTenantLogo(ctx, connect.NewRequest(&adminv1.SetTenantLogoRequest{Id: tn.Msg.Tenant.Id, Data: pngOf(900, 300)}))
	if err != nil || !strings.HasPrefix(logo.Msg.Tenant.LogoUrl, e.auth+"/media/") {
		t.Fatalf("tenant logo: %v %v", logo, err)
	}
	if code, kind := e.served(logo.Msg.Tenant.LogoUrl); code != http.StatusOK || kind != "image/png" {
		t.Fatalf("tenant logo served: %d %s", code, kind)
	}
	got, err := tenants.GetTenant(ctx, connect.NewRequest(&adminv1.GetTenantRequest{Id: tn.Msg.Tenant.Id}))
	if err != nil || got.Msg.Tenant.LogoUrl != logo.Msg.Tenant.LogoUrl {
		t.Fatalf("the tenant reads back its logo: %v %v", got, err)
	}

	// A service's logo, by its owner only.
	registry := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(token))
	if _, err := registry.RegisterService(ctx, connect.NewRequest(&adminv1.RegisterServiceRequest{Id: "billing", DisplayName: "Billing", OwnerEmail: "olga@example.com"})); err != nil {
		t.Fatal(err)
	}
	olga := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(e.activate("olga@example.com", "olga's long password")))
	svc, err := olga.SetServiceLogo(ctx, connect.NewRequest(&adminv1.SetServiceLogoRequest{ServiceId: "billing", Data: pngOf(256, 256)}))
	if err != nil || !strings.HasPrefix(svc.Msg.Service.LogoUrl, e.auth+"/media/") {
		t.Fatalf("service logo: %v %v", svc, err)
	}
	if _, _, err := e.a.IDs.Invite(ctx, "carl@example.com", "Carl", "test"); err != nil {
		t.Fatal(err)
	}
	carl := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(e.activate("carl@example.com", "carl's long password")))
	if _, err := carl.SetServiceLogo(ctx, connect.NewRequest(&adminv1.SetServiceLogoRequest{ServiceId: "billing", Data: pngOf(10, 10)})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("a stranger sets the logo: want PermissionDenied, got %v", err)
	}
}
