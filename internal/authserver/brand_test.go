package authserver_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/loginpage"
)

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// The console's client belongs to the built-in hayat service, so publishing
// hayat's login page brands the console's sign-in.
func TestPublishedLoginPageIsDrawn(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	c := browser()
	_, challenge := pkce()

	// Nothing published: Hayat's own page.
	login := location(t, h.authorize(c, challenge, nil))
	page := body(t, h.get(c, login.String()))
	if strings.Contains(page, "branded") || !strings.Contains(page, "Create an account") {
		t.Fatal("with nothing published the page is Hayat's own")
	}

	if _, err := h.pages.SaveDraft(ctx, catalog.Platform, loginpage.Design{
		Layout: "split", Accent: "#3d5fc9", Name: "Acme <Console>", Tagline: "Every ledger in one place.", HideSignup: true, Mode: "dark",
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body(t, h.get(c, login.String())), "Acme") {
		t.Fatal("a draft must not show before it is published")
	}
	if _, err := h.pages.Publish(ctx, catalog.Platform, "test"); err != nil {
		t.Fatal(err)
	}

	resp := h.get(c, login.String())
	csp := resp.Header.Get("Content-Security-Policy")
	page = body(t, resp)
	for _, want := range []string{"layout-split", "Acme &lt;Console&gt;", "Sign in to Acme &lt;Console&gt;", "Every ledger in one place.",
		"Secured by Hayat", `class="mode-dark"`, "--accent:#3d5fc9"} {
		if !strings.Contains(page, want) {
			t.Errorf("branded page lacks %q", want)
		}
	}
	if strings.Contains(page, "Create an account") {
		t.Error("the design hid the sign-up link")
	}
	if !strings.Contains(csp, "'nonce-") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("the brand's style must go through a nonce, and the page must not be framed: %s", csp)
	}

	// The form still signs people in.
	if code := h.signIn(c, challenge); code == "" {
		t.Fatal("a branded page must still sign in")
	}
}

func TestPreviewDrawsTheDraftAndSignsNobodyIn(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.pages.SaveDraft(ctx, "billing", loginpage.Design{Layout: "fullbleed", Name: "Billing"}); err != nil {
		t.Fatal(err)
	}
	tok := h.pages.PreviewToken("billing", time.Now())

	resp := h.get(http.DefaultClient, h.url+"/preview/login?mode=light&token="+url.QueryEscape(tok))
	csp, xfo := resp.Header.Get("Content-Security-Policy"), resp.Header.Get("X-Frame-Options")
	page := body(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(page, "layout-fullbleed") || !strings.Contains(page, `class="mode-light"`) {
		t.Fatalf("preview: %d\n%s", resp.StatusCode, page)
	}
	if !strings.Contains(page, `<fieldset class="lock" disabled>`) || !strings.Contains(page, `name="csrf" value=""`) {
		t.Error("a preview's form must be disabled and carry no CSRF token")
	}
	if !strings.Contains(csp, "frame-ancestors http://console.test") || xfo != "" {
		t.Errorf("the console must be able to frame a preview: csp=%q xfo=%q", csp, xfo)
	}
	if resp := h.get(http.DefaultClient, h.url+"/preview/login?screen=signup&token="+url.QueryEscape(tok)); !strings.Contains(body(t, resp), "Create your account") {
		t.Error("the sign-up screen previews too")
	}
	if resp := h.get(http.DefaultClient, h.url+"/preview/login?token=forged"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a bad token must be refused, got %d", resp.StatusCode)
	}
}

func TestBrandAssetsAreServedInert(t *testing.T) {
	h := newHarness(t)
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	id, err := h.pages.AddAsset(context.Background(), "billing", b.Bytes(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if old := h.get(http.DefaultClient, h.url+"/brand/asset/"+id.String()); old.StatusCode != http.StatusOK {
		t.Fatalf("logos linked before /media existed still load, got %d", old.StatusCode)
	}
	resp := h.get(http.DefaultClient, h.url+"/media/"+id.String())
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" ||
		!strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("asset: %d %v", resp.StatusCode, resp.Header)
	}
	if resp := h.get(http.DefaultClient, h.url+"/media/00000000-0000-0000-0000-000000000009"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("an unknown asset is a 404, got %d", resp.StatusCode)
	}
}
