package app_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/admin/v1/adminv1connect"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
)

// A service's owners design its login page; instance admins design Hayat's
// own; nobody else touches either.
func TestLoginPageAPI(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.a.IDs.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	rootToken, _ := e.signIn("root@example.com", "a long enough password")
	registry := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(rootToken))
	if _, err := registry.RegisterService(ctx, connect.NewRequest(&adminv1.RegisterServiceRequest{Id: "billing", DisplayName: "Billing", OwnerEmail: "olga@example.com"})); err != nil {
		t.Fatal(err)
	}
	root := adminv1connect.NewLoginPageServiceClient(http.DefaultClient, e.admin, bearer(rootToken))
	olga := adminv1connect.NewLoginPageServiceClient(http.DefaultClient, e.admin, bearer(e.activate("olga@example.com", "olga's long password")))

	got, err := olga.GetLoginPage(ctx, connect.NewRequest(&adminv1.GetLoginPageRequest{ServiceId: "billing"}))
	if err != nil || got.Msg.Page.ServiceName != "Billing" || got.Msg.Page.Published != nil || !strings.Contains(got.Msg.Page.PreviewUrl, "/preview/login?token=") {
		t.Fatalf("a fresh page: %v %v", got, err)
	}

	var img bytes.Buffer
	_ = png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	logo, err := olga.UploadLoginPageAsset(ctx, connect.NewRequest(&adminv1.UploadLoginPageAssetRequest{ServiceId: "billing", Data: img.Bytes()}))
	if err != nil || !strings.HasSuffix(logo.Msg.Url, "/media/"+logo.Msg.AssetId) {
		t.Fatalf("upload: %v %v", logo, err)
	}
	if _, err := olga.UploadLoginPageAsset(ctx, connect.NewRequest(&adminv1.UploadLoginPageAssetRequest{ServiceId: "billing", Data: []byte("<svg/>")})); code(err) != connect.CodeFailedPrecondition {
		t.Fatalf("an SVG upload: want FailedPrecondition, got %v", err)
	}

	zero := int32(0)
	saved, err := olga.SaveLoginPageDraft(ctx, connect.NewRequest(&adminv1.SaveLoginPageDraftRequest{ServiceId: "billing", Design: &adminv1.LoginPageDesign{
		Layout: "split", Accent: "#3D5FC9", Logo: logo.Msg.AssetId, Radius: &zero, Tagline: "Every invoice in one place.",
	}}))
	if err != nil || saved.Msg.Page.Draft.Accent != "#3d5fc9" || saved.Msg.Page.Draft.Radius == nil || !saved.Msg.Page.HasChanges {
		t.Fatalf("save: %v %v", saved, err)
	}
	if _, err := olga.SaveLoginPageDraft(ctx, connect.NewRequest(&adminv1.SaveLoginPageDraftRequest{ServiceId: "billing",
		Design: &adminv1.LoginPageDesign{TermsUrl: "javascript:alert(1)"}})); code(err) != connect.CodeFailedPrecondition {
		t.Fatalf("an unsafe link: want FailedPrecondition, got %v", err)
	}

	pub, err := olga.PublishLoginPage(ctx, connect.NewRequest(&adminv1.PublishLoginPageRequest{ServiceId: "billing"}))
	if err != nil || pub.Msg.Page.Version != 1 || pub.Msg.Page.HasChanges || pub.Msg.Page.PublishedBy.GetEmail() != "olga@example.com" {
		t.Fatalf("publish: %v %v", pub, err)
	}
	versions, err := olga.ListLoginPageVersions(ctx, connect.NewRequest(&adminv1.ListLoginPageVersionsRequest{ServiceId: "billing"}))
	if err != nil || len(versions.Msg.Versions) != 1 || versions.Msg.Versions[0].Design.Layout != "split" {
		t.Fatalf("versions: %v %v", versions, err)
	}

	// Hayat's own page is the instance admins'.
	if _, err := olga.GetLoginPage(ctx, connect.NewRequest(&adminv1.GetLoginPageRequest{ServiceId: hoidc.PlatformService})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("an owner opens Hayat's page: want PermissionDenied, got %v", err)
	}
	if _, err := root.SaveLoginPageDraft(ctx, connect.NewRequest(&adminv1.SaveLoginPageDraftRequest{ServiceId: hoidc.PlatformService,
		Design: &adminv1.LoginPageDesign{Layout: "centered"}})); err != nil {
		t.Fatalf("an admin designs Hayat's page: %v", err)
	}

	// A stranger gets nowhere.
	if _, _, err := e.a.IDs.Invite(ctx, "carl@example.com", "Carl", "test"); err != nil {
		t.Fatal(err)
	}
	carl := adminv1connect.NewLoginPageServiceClient(http.DefaultClient, e.admin, bearer(e.activate("carl@example.com", "carl's long password")))
	if _, err := carl.PublishLoginPage(ctx, connect.NewRequest(&adminv1.PublishLoginPageRequest{ServiceId: "billing"})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("stranger publishes: want PermissionDenied, got %v", err)
	}
	if _, err := carl.UploadLoginPageAsset(ctx, connect.NewRequest(&adminv1.UploadLoginPageAssetRequest{ServiceId: "billing", Data: img.Bytes()})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("stranger uploads: want PermissionDenied, got %v", err)
	}
}
