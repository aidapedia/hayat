package identity

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/store"
)

// Appearance is how every app looks for one person: one theme per account,
// edited in Hayat's console and read by every service that signs people in
// with Hayat. Every field is optional; an empty one means "the app's own
// default", so an account that never chose anything looks like each app
// ships, and the stored theme only overrides what the person changed.
//
// The JSON names are the proto3 JSON of hayat.*.v1.Appearance, which is also
// what services hand their browsers, so the shape is the same end to end.
type Appearance struct {
	// The preset it started from ("tg-dark", "hayat-light", …) or "custom".
	Preset string `json:"preset,omitempty"`
	// system | light | dark: which neutral palette fills the colors left empty.
	Mode string `json:"mode,omitempty"`

	Accent     string `json:"accent,omitempty"`
	Background string `json:"background,omitempty"`
	Surface    string `json:"surface,omitempty"`
	Text       string `json:"text,omitempty"`
	Muted      string `json:"muted,omitempty"`
	Border     string `json:"border,omitempty"`

	// The sidebar follows the content colors; its own four are ignored.
	SidebarMatchContent bool   `json:"sidebarMatchContent,omitempty"`
	SidebarBackground   string `json:"sidebarBackground,omitempty"`
	SidebarText         string `json:"sidebarText,omitempty"`
	SidebarActive       string `json:"sidebarActive,omitempty"`
	SidebarBorder       string `json:"sidebarBorder,omitempty"`

	Success     string `json:"success,omitempty"`
	Warning     string `json:"warning,omitempty"`
	Destructive string `json:"destructive,omitempty"`

	// system | inter | plex
	Font string `json:"font,omitempty"`
	// Pixels of the root font size, 12–18.
	TextSize *int32 `json:"textSize,omitempty"`
	// comfortable | compact | condensed
	Density string `json:"density,omitempty"`
	// Corner radius in pixels, 0–18. 0 is square, so "unset" is nil.
	Radius *int32 `json:"radius,omitempty"`
	// Sidebar width in pixels, 200–320.
	SidebarWidth *int32 `json:"sidebarWidth,omitempty"`
	// narrow | default | full
	ContentWidth string `json:"contentWidth,omitempty"`
}

var (
	hexColor   = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	presetName = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
)

func oneOf(field, v string, allowed ...string) error {
	if v == "" {
		return nil
	}
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return errs.Newf("%s must be one of %s", field, strings.Join(allowed, ", "))
}

func within(field string, v *int32, lo, hi int32) error {
	if v != nil && (*v < lo || *v > hi) {
		return errs.Newf("%s must be between %d and %d", field, lo, hi)
	}
	return nil
}

// Normalize checks every field and lower-cases the colors. It answers nil for
// a theme that sets nothing, which clears what was stored.
func (a *Appearance) Normalize() (*Appearance, error) {
	if a == nil {
		return nil, nil
	}
	out := *a
	colors := map[string]*string{
		"accent": &out.Accent, "background": &out.Background, "surface": &out.Surface, "text": &out.Text,
		"muted": &out.Muted, "border": &out.Border, "sidebar background": &out.SidebarBackground,
		"sidebar text": &out.SidebarText, "sidebar active": &out.SidebarActive, "sidebar border": &out.SidebarBorder,
		"success": &out.Success, "warning": &out.Warning, "destructive": &out.Destructive,
	}
	for name, c := range colors {
		*c = strings.ToLower(strings.TrimSpace(*c))
		if *c != "" && !hexColor.MatchString(*c) {
			return nil, errs.Newf("the %s color must look like #1d6a54", name)
		}
	}
	if out.Preset = strings.TrimSpace(out.Preset); out.Preset != "" && !presetName.MatchString(out.Preset) {
		return nil, errs.New("the preset name may use lower-case letters, digits and dashes")
	}
	for _, err := range []error{
		oneOf("mode", out.Mode, "system", "light", "dark"),
		oneOf("font", out.Font, "system", "inter", "plex"),
		oneOf("density", out.Density, "comfortable", "compact", "condensed"),
		oneOf("content width", out.ContentWidth, "narrow", "default", "full"),
		within("text size", out.TextSize, 12, 18),
		within("corner radius", out.Radius, 0, 18),
		within("sidebar width", out.SidebarWidth, 200, 320),
	} {
		if err != nil {
			return nil, err
		}
	}
	if out == (Appearance{}) {
		return nil, nil
	}
	return &out, nil
}

// Appearance is the person's theme, or nil when they never chose one.
func (s *Service) Appearance(ctx context.Context, id uuid.UUID) (*Appearance, error) {
	region, err := s.regionOf(ctx, s.store.Pool, id)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if err := s.store.Pool.QueryRow(ctx, `SELECT appearance FROM users WHERE region = $1 AND id = $2`, region, id).Scan(&raw); err != nil {
		return nil, store.NotFound(err)
	}
	return decodeAppearance(raw)
}

func decodeAppearance(raw []byte) (*Appearance, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var a Appearance
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// SetAppearance stores the person's theme; nil (or a theme that sets nothing)
// goes back to every app's defaults. → what was stored.
func (s *Service) SetAppearance(ctx context.Context, id uuid.UUID, a *Appearance, actor string) (*Appearance, error) {
	a, err := a.Normalize()
	if err != nil {
		return nil, err
	}
	var value any // SQL NULL unless there is something to keep
	if a != nil {
		b, err := json.Marshal(a)
		if err != nil {
			return nil, err
		}
		value = string(b)
	}
	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, err := s.regionOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET appearance = $3::jsonb, updated_at = now() WHERE region = $1 AND id = $2`,
			region, id, value); err != nil {
			return err
		}
		return audit.Write(ctx, tx, region, actor, "user.appearance_updated", "user:"+id.String(), nil)
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}
