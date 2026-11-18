package loginpage_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/loginpage"
	"github.com/aidapedia/hayat/internal/media"
	"github.com/aidapedia/hayat/internal/testdb"
)

// TestMain stops the package's embedded PostgreSQL when its tests finish.
func TestMain(m *testing.M) { os.Exit(testdb.Main(m)) }

func i32(v int32) *int32 { return &v }

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{61, 95, 201, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func newService(t *testing.T) *loginpage.Service {
	db := testdb.New(t)
	blobs, _ := media.NewStore(config.Storage{Backend: "local", Dir: t.TempDir()})
	return loginpage.New(db, make([]byte, 32), media.New(db, blobs, "http://auth.test", nil))
}

func TestDraftPublishDiscardRestore(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	p, err := svc.Get(ctx, "billing")
	if err != nil || p.Published != nil || p.Version != 0 {
		t.Fatalf("a service that never edited its page starts empty, got %+v, %v", p, err)
	}
	if live, err := svc.Live(ctx, "billing"); err != nil || live != nil {
		t.Fatalf("nothing published means Hayat's own page, got %+v, %v", live, err)
	}

	p, err = svc.SaveDraft(ctx, "billing", loginpage.Design{Layout: "split", Accent: " #3D5FC9 ", Name: "  Billing \n", Radius: i32(0)})
	if err != nil {
		t.Fatal(err)
	}
	if p.Draft.Accent != "#3d5fc9" || p.Draft.Name != "Billing" || !p.HasChanges() {
		t.Fatalf("the draft is stored normalized and counts as a change, got %+v", p)
	}
	if live, _ := svc.Live(ctx, "billing"); live != nil {
		t.Fatal("a draft must not show until it is published")
	}

	p, err = svc.Publish(ctx, "billing", "user:ana")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || p.HasChanges() || p.PublishedAt == nil || p.PublishedBy != "user:ana" {
		t.Fatalf("publishing makes version 1, got %+v", p)
	}
	live, err := svc.Live(ctx, "billing")
	if err != nil || live == nil || live.Layout != "split" || live.Radius == nil || *live.Radius != 0 {
		t.Fatalf("the live design is the published one, a radius of 0 included, got %+v, %v", live, err)
	}

	if _, err := svc.SaveDraft(ctx, "billing", loginpage.Design{Layout: "fullbleed"}); err != nil {
		t.Fatal(err)
	}
	p, err = svc.Discard(ctx, "billing", "user:ana")
	if err != nil || p.Draft.Layout != "split" || p.HasChanges() {
		t.Fatalf("discarding goes back to what's published, got %+v, %v", p, err)
	}

	if _, err := svc.SaveDraft(ctx, "billing", loginpage.Design{Layout: "centered"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(ctx, "billing", "user:ana"); err != nil {
		t.Fatal(err)
	}
	versions, err := svc.Versions(ctx, "billing")
	if err != nil || len(versions) != 2 || versions[0].Version != 2 || versions[1].Design.Layout != "split" {
		t.Fatalf("both versions are kept, newest first, got %+v, %v", versions, err)
	}
	p, err = svc.Restore(ctx, "billing", 1, "user:ana")
	if err != nil || p.Draft.Layout != "split" || p.Version != 2 || !p.HasChanges() {
		t.Fatalf("restoring copies v1 into the draft without publishing, got %+v, %v", p, err)
	}
	if _, err := svc.Restore(ctx, "billing", 9, "user:ana"); err == nil {
		t.Fatal("restoring a version that doesn't exist must fail")
	}
}

func TestDesignIsChecked(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	for name, d := range map[string]loginpage.Design{
		"color":    {Accent: "blue"},
		"layout":   {Layout: "sidebar"},
		"font":     {Font: "comic"},
		"radius":   {Radius: i32(40)},
		"link":     {TermsURL: "javascript:alert(1)"},
		"http":     {PrivacyURL: "http://example.com/privacy"},
		"heading":  {Heading: strings.Repeat("x", 81)},
		"logo":     {Logo: "not-an-id"},
		"mode":     {Mode: "sepia"},
		"panel":    {Panel: "#12"},
		"logoDark": {LogoDark: "../../etc"},
	} {
		if _, err := svc.SaveDraft(ctx, "billing", d); err == nil {
			t.Errorf("%s: an invalid design was saved: %+v", name, d)
		}
	}
	if _, err := svc.SaveDraft(ctx, "billing", loginpage.Design{SupportURL: "http://localhost:3000/help"}); err != nil {
		t.Fatalf("http on this machine is allowed for development: %v", err)
	}
}

func TestAssetsBelongToTheirService(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.AddAsset(ctx, "billing", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), "user:ana"); !errors.Is(err, loginpage.ErrBadAsset) {
		t.Fatalf("an SVG must be refused, got %v", err)
	}
	if _, err := svc.AddAsset(ctx, "billing", make([]byte, loginpage.MaxAssetBytes+1), "user:ana"); !errors.Is(err, loginpage.ErrBadAsset) {
		t.Fatalf("an oversized upload must be refused, got %v", err)
	}
	id, err := svc.AddAsset(ctx, "billing", pngBytes(t), "user:ana")
	if err != nil {
		t.Fatal(err)
	}
	kind, data, err := svc.Asset(ctx, id)
	if err != nil || kind != "image/png" || len(data) == 0 {
		t.Fatalf("read the logo back, got %q %d bytes, %v", kind, len(data), err)
	}

	if _, err := svc.SaveDraft(ctx, "billing", loginpage.Design{Logo: id.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveDraft(ctx, "payments", loginpage.Design{Logo: id.String()}); !errors.Is(err, loginpage.ErrUnknownAsset) {
		t.Fatalf("another service can't use billing's logo, got %v", err)
	}
}

func TestPreviewTokens(t *testing.T) {
	svc := newService(t)
	now := time.Now()
	tok := svc.PreviewToken("billing", now)

	if id, err := svc.CheckPreview(tok, now.Add(time.Minute)); err != nil || id != "billing" {
		t.Fatalf("a fresh token names its service, got %q, %v", id, err)
	}
	if _, err := svc.CheckPreview(tok, now.Add(loginpage.PreviewTTL+time.Minute)); err == nil {
		t.Fatal("an expired token must be refused")
	}
	other := loginpage.New(nil, bytes.Repeat([]byte{1}, 32), nil)
	if _, err := other.CheckPreview(tok, now); err == nil {
		t.Fatal("a token signed with another key must be refused")
	}
	head, sig, _ := strings.Cut(tok, ".")
	forged := strings.Replace(head, head[:4], "cGF5", 1) + "." + sig
	if _, err := svc.CheckPreview(forged, now); err == nil {
		t.Fatal("a token whose service was changed must be refused")
	}
}

func TestColorsStayReadable(t *testing.T) {
	for _, accent := range []string{"#3d5fc9", "#f2b134", "#6fcf6a", "#111111", "#ffffff"} {
		d := loginpage.Design{Accent: accent}
		for _, mode := range []string{"light", "dark"} {
			c := d.ColorsFor(mode)
			if r := loginpage.Contrast(c.Accent, c.OnAccent); r < 4.5 {
				t.Errorf("%s %s: button text at %.2f:1", accent, mode, r)
			}
			if r := loginpage.Contrast(c.Link, c.Card); r < 4.5 {
				t.Errorf("%s %s: links at %.2f:1 on the card", accent, mode, r)
			}
		}
	}
	css := loginpage.Design{Mode: "dark", Font: "serif", Radius: i32(4)}.CSS()
	if !strings.Contains(css, "--radius:4px") || !strings.Contains(css, "Charter") || !strings.Contains(css, ":root.mode-light") {
		t.Fatalf("unexpected CSS: %s", css)
	}
}
