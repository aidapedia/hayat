package loginpage

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/errs"
)

// Design is one service's login page: a layout, a brand and the words on it.
// It is data, never markup or CSS, so no service can change what the form
// does or hide it behind a fake one. Every field is optional; an empty one
// takes Hayat's default.
//
// The JSON names are the proto3 JSON of hayat.admin.v1.LoginPageDesign.
type Design struct {
	// centered | split | fullbleed
	Layout string `json:"layout,omitempty"`
	// system | light | dark: which palette the page uses.
	Mode string `json:"mode,omitempty"`
	// The buttons, links and focus rings. "#rrggbb".
	Accent string `json:"accent,omitempty"`
	// The brand panel of the split and full-bleed layouts; empty follows the accent.
	Panel string `json:"panel,omitempty"`
	// One of Fonts. Font stacks only: the page downloads no fonts.
	Font string `json:"font,omitempty"`
	// Corner radius of fields and buttons in pixels, 0–20. nil is the default.
	Radius *int32 `json:"radius,omitempty"`
	// Asset ids of the logo, for light and dark backgrounds.
	Logo     string `json:"logo,omitempty"`
	LogoDark string `json:"logoDark,omitempty"`

	// Shown beside the logo. Empty is the service's name.
	Name string `json:"name,omitempty"`
	// "Sign in to Billing" when empty.
	Heading    string `json:"heading,omitempty"`
	Subheading string `json:"subheading,omitempty"`
	// The line on the brand panel (split and full-bleed).
	Tagline string `json:"tagline,omitempty"`

	TermsURL   string `json:"termsUrl,omitempty"`
	PrivacyURL string `json:"privacyUrl,omitempty"`
	SupportURL string `json:"supportUrl,omitempty"`

	// Leaves out the "Create an account" link. Whether people may sign up at
	// all is the instance's decision, not the page's.
	HideSignup bool `json:"hideSignup,omitempty"`
}

// Fonts maps each font choice to its stack. System fonts only, so the page
// stays fast and tells no font host who is signing in.
var Fonts = map[string]string{
	"system":    `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif`,
	"humanist":  `Seravek, "Gill Sans Nova", Ubuntu, Calibri, "DejaVu Sans", source-sans-pro, sans-serif`,
	"geometric": `Avenir, "Avenir Next", Montserrat, Corbel, "URW Gothic", source-sans-pro, sans-serif`,
	"rounded":   `ui-rounded, "Hiragino Maru Gothic ProN", Quicksand, Comfortaa, Manjari, "Arial Rounded MT", Calibri, sans-serif`,
	"serif":     `Charter, "Bitstream Charter", "Sitka Text", Cambria, Georgia, serif`,
	"mono":      `ui-monospace, "Cascadia Code", "Source Code Pro", Menlo, Consolas, "DejaVu Sans Mono", monospace`,
}

var hexColor = regexp.MustCompile(`^#[0-9a-f]{6}$`)

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

// line trims a piece of copy to one line of printable text.
func line(field, v string, max int) (string, error) {
	v = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, v)
	v = strings.Join(strings.Fields(v), " ")
	if len([]rune(v)) > max {
		return "", errs.Newf("keep the %s to %d characters", field, max)
	}
	return v, nil
}

// link accepts an https address, or http on this machine for development.
func link(field, v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" || len(v) > 500 {
		return "", errs.Newf("the %s link must be a full address, like https://example.com/%s", field, field)
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", errs.Newf("the %s link must start with https://", field)
	}
	return u.String(), nil
}

// Normalize checks every field, trims the copy and lower-cases the colors.
func (d Design) Normalize() (Design, error) {
	out := d
	for name, c := range map[string]*string{"accent": &out.Accent, "panel": &out.Panel} {
		*c = strings.ToLower(strings.TrimSpace(*c))
		if *c != "" && !hexColor.MatchString(*c) {
			return Design{}, errs.Newf("the %s color must look like #3d5fc9", name)
		}
	}
	if err := oneOf("layout", out.Layout, "centered", "split", "fullbleed"); err != nil {
		return Design{}, err
	}
	if err := oneOf("mode", out.Mode, "system", "light", "dark"); err != nil {
		return Design{}, err
	}
	if _, ok := Fonts[out.Font]; out.Font != "" && !ok {
		return Design{}, errs.New("pick one of the listed fonts")
	}
	if out.Radius != nil && (*out.Radius < 0 || *out.Radius > 20) {
		return Design{}, errs.New("corners must be between 0 and 20 pixels")
	}
	for name, id := range map[string]*string{"logo": &out.Logo, "dark logo": &out.LogoDark} {
		*id = strings.TrimSpace(*id)
		if *id != "" {
			if _, err := uuid.Parse(*id); err != nil {
				return Design{}, errs.Newf("the %s isn't an uploaded image", name)
			}
		}
	}
	var err error
	for _, f := range []struct {
		name string
		v    *string
		max  int
	}{{"name", &out.Name, 60}, {"heading", &out.Heading, 80}, {"subheading", &out.Subheading, 160}, {"tagline", &out.Tagline, 140}} {
		if *f.v, err = line(f.name, *f.v, f.max); err != nil {
			return Design{}, err
		}
	}
	for _, f := range []struct {
		name string
		v    *string
	}{{"terms", &out.TermsURL}, {"privacy", &out.PrivacyURL}, {"support", &out.SupportURL}} {
		if *f.v, err = link(f.name, *f.v); err != nil {
			return Design{}, err
		}
	}
	return out, nil
}

// Assets are the uploaded images the design points at.
func (d Design) Assets() []string {
	var out []string
	for _, id := range []string{d.Logo, d.LogoDark} {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// LayoutOrDefault is the layout to draw.
func (d Design) LayoutOrDefault() string {
	if d.Layout == "" {
		return "centered"
	}
	return d.Layout
}
