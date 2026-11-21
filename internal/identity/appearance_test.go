package identity_test

import (
	"context"
	"testing"

	"github.com/aidapedia/hayat/internal/identity"
)

func i32(v int32) *int32 { return &v }

func TestAppearanceStartsUnsetThenSavesAndClears(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	u, err := svc.CreateAdmin(ctx, "theme@example.com", "Theme", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}

	if a, err := svc.Appearance(ctx, u.ID); err != nil || a != nil {
		t.Fatalf("a new account has no theme, got %+v, %v", a, err)
	}

	saved, err := svc.SetAppearance(ctx, u.ID, &identity.Appearance{
		Preset: "custom", Mode: "dark", Accent: "#6FCF6A", SidebarBackground: " #0F172A ", Radius: i32(0), TextSize: i32(15), Density: "compact",
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Accent != "#6fcf6a" || saved.SidebarBackground != "#0f172a" {
		t.Fatalf("colors are stored trimmed and lower-cased, got %+v", saved)
	}

	got, err := svc.Appearance(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Mode != "dark" || got.Radius == nil || *got.Radius != 0 || *got.TextSize != 15 || got.Density != "compact" {
		t.Fatalf("read back what was saved, a radius of 0 included, got %+v", got)
	}

	// A theme that sets nothing goes back to every app's defaults.
	if cleared, err := svc.SetAppearance(ctx, u.ID, &identity.Appearance{}, "test"); err != nil || cleared != nil {
		t.Fatalf("clearing answers nil, got %+v, %v", cleared, err)
	}
	if a, _ := svc.Appearance(ctx, u.ID); a != nil {
		t.Fatalf("cleared theme still stored: %+v", a)
	}
}

func TestAppearanceRefusesWhatNoAppCanDraw(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	u, err := svc.CreateAdmin(ctx, "picky@example.com", "Picky", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	for name, a := range map[string]*identity.Appearance{
		"named color":      {Accent: "green"},
		"short hex":        {Background: "#fff"},
		"unknown mode":     {Mode: "sepia"},
		"unknown font":     {Font: "comic-sans"},
		"text too small":   {TextSize: i32(8)},
		"radius too big":   {Radius: i32(40)},
		"sidebar too wide": {SidebarWidth: i32(600)},
		"bad width":        {ContentWidth: "huge"},
		"preset name":      {Preset: "My Theme!"},
	} {
		if _, err := svc.SetAppearance(ctx, u.ID, a, "test"); err == nil {
			t.Errorf("%s: saved %+v", name, a)
		}
	}
}

func TestAppFollowsTheAccountUntilGivenItsOwnTheme(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	u, err := svc.CreateAdmin(ctx, "apps@example.com", "Apps", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetAppearance(ctx, u.ID, &identity.Appearance{Mode: "dark", Accent: "#3fae86"}, "test"); err != nil {
		t.Fatal(err)
	}

	if a, own, err := svc.AppearanceFor(ctx, u.ID, "tabletsgo"); err != nil || own || a == nil || a.Accent != "#3fae86" {
		t.Fatalf("an app follows the account until given its own, got %+v, own=%v, %v", a, own, err)
	}

	saved, err := svc.SetAppAppearance(ctx, u.ID, "tabletsgo", &identity.Appearance{Preset: "tg-dark", Accent: "#6FCF6A", AccentText: "#FFFFFF", Scale: i32(90)}, true, "test")
	if err != nil || saved.AccentText != "#ffffff" || *saved.Scale != 90 {
		t.Fatalf("an app's own theme is checked like the account's, got %+v, %v", saved, err)
	}
	if a, own, _ := svc.AppearanceFor(ctx, u.ID, "tabletsgo"); !own || a.Accent != "#6fcf6a" {
		t.Fatalf("the app now shows its own, got %+v, own=%v", a, own)
	}
	if a, own, _ := svc.AppearanceFor(ctx, u.ID, "hayat"); own || a.Accent != "#3fae86" {
		t.Fatalf("other apps still follow the account, got %+v, own=%v", a, own)
	}

	// Its own theme setting nothing is the app's defaults, not the account's.
	if _, err := svc.SetAppAppearance(ctx, u.ID, "hayat", &identity.Appearance{}, true, "test"); err != nil {
		t.Fatal(err)
	}
	if a, own, _ := svc.AppearanceFor(ctx, u.ID, "hayat"); !own || a != nil {
		t.Fatalf("an empty own theme is the app's defaults, got %+v, own=%v", a, own)
	}
	_, apps, _ := svc.Appearances(ctx, u.ID)
	if len(apps) != 2 {
		t.Fatalf("two apps have their own, got %+v", apps)
	}

	// Back to following.
	for _, app := range []string{"tabletsgo", "hayat"} {
		if _, err := svc.SetAppAppearance(ctx, u.ID, app, nil, false, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if a, own, _ := svc.AppearanceFor(ctx, u.ID, "tabletsgo"); own || a.Accent != "#3fae86" {
		t.Fatalf("following again shows the account's, got %+v, own=%v", a, own)
	}
	if _, apps, _ := svc.Appearances(ctx, u.ID); len(apps) != 0 {
		t.Fatalf("nothing left of its own, got %+v", apps)
	}

	for name, a := range map[string]*identity.Appearance{"scale": {Scale: i32(150)}, "accent text": {AccentText: "white"}} {
		if _, err := svc.SetAppAppearance(ctx, u.ID, "tabletsgo", a, true, "test"); err == nil {
			t.Errorf("%s: accepted %+v", name, a)
		}
	}
	if _, err := svc.SetAppAppearance(ctx, u.ID, "Not An App", nil, true, "test"); err == nil {
		t.Error("an app name that can't be a service was accepted")
	}
}
