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
