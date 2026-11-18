package loginpage

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Palette is the neutral half of a page in one mode. A brand sets the accent
// and the panel; the neutrals stay Hayat's, so text is always readable.
type Palette struct {
	Bg, Card, Elevated, Edge, Ink, InkDim, InkFaint string
}

var (
	lightNeutrals = Palette{Bg: "#f6f7f9", Card: "#ffffff", Elevated: "#f1f3f6", Edge: "#dfe3e8", Ink: "#14171c", InkDim: "#5b6472", InkFaint: "#6b7380"}
	darkNeutrals  = Palette{Bg: "#0f1216", Card: "#161a20", Elevated: "#1c2128", Edge: "#2a3038", Ink: "#eef1f4", InkDim: "#9aa3ae", InkFaint: "#8a93a0"}

	// Hayat's own accent, for a design that doesn't set one.
	defaultAccentLight = "#1d6a54"
	defaultAccentDark  = "#3fae86"
)

// Colors is everything a page needs in one mode, worked out from the design.
type Colors struct {
	Palette
	Accent, AccentHover, OnAccent, Link, Ring string
	Panel, OnPanel, OnPanelDim                string
}

// ColorsFor works out the page's colors in "light" or "dark". Text on the
// accent and on the panel is white or near-black, whichever reads better, and
// links are moved toward the text color until they reach 4.5:1 on the card.
func (d Design) ColorsFor(mode string) Colors {
	n, accent := lightNeutrals, defaultAccentLight
	if mode == "dark" {
		n, accent = darkNeutrals, defaultAccentDark
	}
	if d.Accent != "" {
		accent = d.Accent
	}
	panel := d.Panel
	if panel == "" {
		panel = accent
	}
	on := OnColor(accent)
	hoverToward := "#000000"
	if mode == "dark" {
		hoverToward = "#ffffff"
	}
	onPanel := OnColor(panel)
	return Colors{
		Palette:     n,
		Accent:      accent,
		AccentHover: mix(accent, hoverToward, 0.14),
		OnAccent:    on,
		Link:        readableOn(accent, n.Card, n.Ink),
		Ring:        withAlpha(accent, 0.22),
		Panel:       panel,
		OnPanel:     onPanel,
		OnPanelDim:  withAlpha(onPanel, 0.78),
	}
}

// CSS is the stylesheet a branded page adds after Hayat's own. Its values are
// validated colors, numbers and fixed font stacks, never text a person typed.
func (d Design) CSS() string {
	var b strings.Builder
	vars := func(c Colors, scheme string) string {
		return fmt.Sprintf("--bg:%s;--panel:%s;--card:%s;--elevated:%s;--edge:%s;--edge-strong:%s;--ink:%s;--ink-dim:%s;--ink-faint:%s;"+
			"--accent:%s;--accent-bright:%s;--on-accent:%s;--link:%s;--notice-soft:%s;--brand-panel:%s;--on-panel:%s;--on-panel-dim:%s;color-scheme:%s",
			c.Bg, c.Card, c.Card, c.Elevated, c.Edge, c.Edge, c.Ink, c.InkDim, c.InkFaint,
			c.Accent, c.AccentHover, c.OnAccent, c.Link, c.Ring, c.Panel, c.OnPanel, c.OnPanelDim, scheme)
	}
	light, dark := vars(d.ColorsFor("light"), "light"), vars(d.ColorsFor("dark"), "dark")
	switch d.Mode {
	case "dark":
		fmt.Fprintf(&b, ":root{%s}\n:root.mode-light{%s}\n", dark, light)
	case "light":
		fmt.Fprintf(&b, ":root{%s}\n:root.mode-dark{%s}\n", light, dark)
	default:
		fmt.Fprintf(&b, ":root{%s}\n@media (prefers-color-scheme: dark){:root:not(.mode-light){%s}}\n:root.mode-dark{%s}\n", light, dark, dark)
	}
	radius := int32(10)
	if d.Radius != nil {
		radius = *d.Radius
	}
	font := Fonts["system"]
	if f, ok := Fonts[d.Font]; ok {
		font = f
	}
	fmt.Fprintf(&b, ":root{--radius:%dpx;--card-radius:%dpx;--font:%s}\n", radius, min(radius+4, 24), font)
	return b.String()
}

// ---------- Color arithmetic ----------

type rgb struct{ r, g, b float64 }

func parse(hex string) rgb {
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil || len(hex) != 7 {
		return rgb{}
	}
	return rgb{float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff)}
}

func (c rgb) hex() string {
	clamp := func(x float64) int { return int(math.Max(0, math.Min(255, math.Round(x)))) }
	return fmt.Sprintf("#%02x%02x%02x", clamp(c.r), clamp(c.g), clamp(c.b))
}

func luminance(hex string) float64 {
	c := parse(hex)
	ch := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.r) + 0.7152*ch(c.g) + 0.0722*ch(c.b)
}

// Contrast is the WCAG contrast ratio of two colors, 1 to 21.
func Contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// OnColor is the text color for a fill: white or near-black, whichever reads
// better. One of them always reaches 4.5:1.
func OnColor(fill string) string {
	if Contrast(fill, "#ffffff") >= Contrast(fill, "#0b0d10") {
		return "#ffffff"
	}
	return "#0b0d10"
}

func mix(a, b string, t float64) string {
	x, y := parse(a), parse(b)
	return rgb{x.r + (y.r-x.r)*t, x.g + (y.g-x.g)*t, x.b + (y.b-x.b)*t}.hex()
}

// readableOn moves color toward ink until it reads at 4.5:1 on bg.
func readableOn(color, bg, ink string) string {
	for t := 0.0; t <= 1.0; t += 0.1 {
		c := mix(color, ink, t)
		if Contrast(c, bg) >= 4.5 {
			return c
		}
	}
	return ink
}

func withAlpha(hex string, a float64) string {
	c := parse(hex)
	return fmt.Sprintf("rgba(%d,%d,%d,%.2f)", int(c.r), int(c.g), int(c.b), a)
}
