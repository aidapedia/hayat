package authserver

import (
	"context"
	"html/template"
	"net/http"
	"strconv"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/loginpage"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
)

// brandView is a service's login page as the templates draw it. Every string
// is escaped by html/template; CSS comes only from loginpage.Design.CSS,
// which writes validated colors, numbers and fixed font stacks.
type brandView struct {
	Name, Initial   string
	Logo, LogoDark  string
	Layout, Tagline string
	Heading, Sub    string
	Terms, Privacy  string
	Support         string
	HideSignup      bool
	CSS             template.CSS
}

// service answers the service's name and logo (a media id), for a design
// that leaves them empty.
func (s *Server) service(ctx context.Context, id string) (name, logo string) {
	if id == "" || id == catalog.Platform {
		return "Hayat", ""
	}
	svc, err := s.cat.GetService(ctx, id)
	if err != nil {
		return id, ""
	}
	return svc.DisplayName, svc.Logo
}

func (s *Server) assetURL(id string) string {
	if id == "" {
		return ""
	}
	return "/media/" + id
}

// view turns a design into what the templates need.
func (s *Server) view(ctx context.Context, serviceID string, d loginpage.Design) *brandView {
	name, logo := s.service(ctx, serviceID)
	if d.Name != "" {
		name = d.Name
	}
	if d.Logo != "" {
		logo = d.Logo
	}
	first, _ := utf8.DecodeRuneInString(name)
	heading := d.Heading
	if heading == "" {
		heading = "Sign in to " + name
	}
	return &brandView{
		Name: name, Initial: string(unicode.ToUpper(first)), Logo: s.assetURL(logo), LogoDark: s.assetURL(d.LogoDark),
		Layout: d.LayoutOrDefault(), Tagline: d.Tagline, Heading: heading, Sub: d.Subheading,
		Terms: d.TermsURL, Privacy: d.PrivacyURL, Support: d.SupportURL, HideSignup: d.HideSignup,
		CSS: template.CSS(d.CSS()),
	}
}

func modeClass(mode string) string {
	if mode == "light" || mode == "dark" {
		return "mode-" + mode
	}
	return ""
}

// apply draws p as the service's page: its brand, and the heading it chose.
func (s *Server) apply(ctx context.Context, p *page, serviceID string, d loginpage.Design) {
	b := s.view(ctx, serviceID, d)
	p.Brand, p.ModeClass = b, modeClass(d.Mode)
	p.Heading = b.Heading
	if b.Sub != "" {
		p.Sub = b.Sub
	}
}

// brandFor draws the page as the service behind the auth request published
// it. A failure to read the design never blocks a sign-in: Hayat's own page
// shows instead.
func (s *Server) brandFor(ctx context.Context, p *page, a *hoidc.AuthRequest) {
	if a == nil || a.ServiceID == "" {
		return
	}
	d, err := s.pages.Live(ctx, a.ServiceID)
	if err != nil {
		s.log.Error("login page", "service", a.ServiceID, "err", err)
		return
	}
	if d != nil {
		s.apply(ctx, p, a.ServiceID, *d)
	}
}

// mediaFile serves an image internal/media keeps: avatars, logos. An image
// never changes (a new upload is a new id), so it is cached for good; it is
// sandboxed so that even a file a browser misread could run nothing.
func (s *Server) mediaFile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	kind, data, err := s.media.Open(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", kind)
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = w.Write(data)
}

// previewPage draws a service's draft for whoever holds a preview token from
// the console: inside the console's Login page tab, or opened on its own.
// Its forms are disabled, so it can't sign anyone in.
//
//	?token=…&screen=login|signup|error&mode=light|dark
func (s *Server) previewPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	serviceID, err := s.pages.CheckPreview(q.Get("token"), time.Now())
	if err != nil {
		s.message(w, r, "Preview expired", err.Error(), http.StatusForbidden)
		return
	}
	pg, err := s.pages.Get(r.Context(), serviceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	p := page{Title: "Sign in", Heading: "Sign in", Preview: true, Email: q.Get("email")}
	s.apply(r.Context(), &p, serviceID, pg.Draft)
	if m := modeClass(q.Get("mode")); m != "" {
		p.ModeClass = m
	}
	name := "login"
	switch q.Get("screen") {
	case "signup":
		name, p.Title, p.Heading, p.Sub = "signup", "Create account", "Create your account", ""
	case "error":
		p.Error = identity.ErrBadCredentials.Error()
		if p.Email == "" {
			p.Email = "dana@example.com"
		}
	}
	p.AuthRequestID = "preview"
	s.render(w, r, name, p, http.StatusOK)
}
