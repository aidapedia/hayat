package adminserver

import (
	"context"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/media"
)

// accountService is the caller's own account. Any active, signed-in user.
type accountService struct{ s *Server }

func (a *accountService) GetMe(ctx context.Context, _ *connect.Request[adminv1.GetMeRequest]) (*connect.Response[adminv1.GetMeResponse], error) {
	me := caller(ctx).User
	tenants, err := a.s.tenants.ForUser(ctx, me.ID)
	if err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.GetMeResponse{User: toUser(me), Tenants: a.s.toTenants(tenants)}), nil
}

func (a *accountService) UpdateMe(ctx context.Context, req *connect.Request[adminv1.UpdateMeRequest]) (*connect.Response[adminv1.UpdateMeResponse], error) {
	m := req.Msg
	u, err := a.s.ids.UpdateProfile(ctx, caller(ctx).User.ID,
		identity.Profile{Name: m.Name, Locale: m.Locale, Zoneinfo: m.Zoneinfo, Phone: m.Phone}, actor(ctx))
	if err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.UpdateMeResponse{User: toUser(u)}), nil
}

func (a *accountService) ChangePassword(ctx context.Context, req *connect.Request[adminv1.ChangePasswordRequest]) (*connect.Response[adminv1.ChangePasswordResponse], error) {
	p := caller(ctx)
	if err := a.s.ids.ChangePassword(ctx, p.User.ID, req.Msg.CurrentPassword, req.Msg.NewPassword, p.SessionID); err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.ChangePasswordResponse{}), nil
}

func (a *accountService) ListMySessions(ctx context.Context, _ *connect.Request[adminv1.ListMySessionsRequest]) (*connect.Response[adminv1.ListMySessionsResponse], error) {
	p := caller(ctx)
	list, err := a.s.ids.ListSessions(ctx, p.User.ID)
	if err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.ListMySessionsResponse{Sessions: toSessions(list, p.SessionID.String())}), nil
}

func (a *accountService) RevokeMySession(ctx context.Context, req *connect.Request[adminv1.RevokeMySessionRequest]) (*connect.Response[adminv1.RevokeMySessionResponse], error) {
	id, err := parseID(req.Msg.SessionId, "session_id")
	if err != nil {
		return nil, err
	}
	p := caller(ctx)
	if err := a.s.ids.RevokeSession(ctx, p.User.ID, id, actor(ctx)); err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RevokeMySessionResponse{}), nil
}

func (a *accountService) GetMyAppearance(ctx context.Context, _ *connect.Request[adminv1.GetMyAppearanceRequest]) (*connect.Response[adminv1.GetMyAppearanceResponse], error) {
	look, own, err := a.s.ids.Appearances(ctx, caller(ctx).User.ID)
	if err != nil {
		return nil, a.s.fail(err)
	}
	apps, err := a.apps(ctx)
	if err != nil {
		return nil, a.s.fail(err)
	}
	for _, app := range apps {
		if theme, ok := own[app.App]; ok {
			app.Own = true
			if theme != (identity.Appearance{}) {
				app.Appearance = toAppearance(&theme)
			}
		}
	}
	return connect.NewResponse(&adminv1.GetMyAppearanceResponse{Appearance: toAppearance(look), Apps: apps}), nil
}

func (a *accountService) SetMyAppAppearance(ctx context.Context, req *connect.Request[adminv1.SetMyAppAppearanceRequest]) (*connect.Response[adminv1.SetMyAppAppearanceResponse], error) {
	apps, err := a.apps(ctx)
	if err != nil {
		return nil, a.s.fail(err)
	}
	var app *adminv1.AppAppearance
	for _, x := range apps {
		if x.App == req.Msg.App {
			app = x
		}
	}
	if app == nil {
		return nil, a.s.fail(errs.New("unknown app"))
	}
	look, err := a.s.ids.SetAppAppearance(ctx, caller(ctx).User.ID, app.App, fromAppearance(req.Msg.Appearance), req.Msg.Own, actor(ctx))
	if err != nil {
		return nil, a.s.fail(err)
	}
	app.Own, app.Appearance = req.Msg.Own, toAppearance(look)
	return connect.NewResponse(&adminv1.SetMyAppAppearanceResponse{App: app}), nil
}

// apps is every app a person can theme: this console, then each service.
func (a *accountService) apps(ctx context.Context) ([]*adminv1.AppAppearance, error) {
	services, err := a.s.cat.Services(ctx)
	if err != nil {
		return nil, err
	}
	out := []*adminv1.AppAppearance{{App: catalog.Platform, Name: "Hayat console"}}
	for _, svc := range services {
		if svc.ID == catalog.Platform {
			continue
		}
		out = append(out, &adminv1.AppAppearance{App: svc.ID, Name: svc.DisplayName, LogoUrl: media.Link(a.s.cfg.Issuer, svc.Logo)})
	}
	return out, nil
}

func (a *accountService) SetMyAppearance(ctx context.Context, req *connect.Request[adminv1.SetMyAppearanceRequest]) (*connect.Response[adminv1.SetMyAppearanceResponse], error) {
	look, err := a.s.ids.SetAppearance(ctx, caller(ctx).User.ID, fromAppearance(req.Msg.Appearance), actor(ctx))
	if err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SetMyAppearanceResponse{Appearance: toAppearance(look)}), nil
}

// The theme between the wire and identity: the same fields under the same names.
func toAppearance(a *identity.Appearance) *adminv1.Appearance {
	if a == nil {
		return nil
	}
	return &adminv1.Appearance{
		Preset: a.Preset, Mode: a.Mode, Accent: a.Accent, AccentText: a.AccentText, Background: a.Background, Surface: a.Surface,
		Text: a.Text, Muted: a.Muted, Border: a.Border, SidebarMatchContent: a.SidebarMatchContent, SidebarBackground: a.SidebarBackground,
		SidebarText: a.SidebarText, SidebarActive: a.SidebarActive, SidebarBorder: a.SidebarBorder, Success: a.Success,
		Warning: a.Warning, Destructive: a.Destructive, Font: a.Font, TextSize: a.TextSize, Scale: a.Scale, Density: a.Density,
		Radius: a.Radius, SidebarWidth: a.SidebarWidth, ContentWidth: a.ContentWidth,
	}
}

func fromAppearance(a *adminv1.Appearance) *identity.Appearance {
	if a == nil {
		return nil
	}
	return &identity.Appearance{
		Preset: a.Preset, Mode: a.Mode, Accent: a.Accent, AccentText: a.AccentText, Background: a.Background, Surface: a.Surface,
		Text: a.Text, Muted: a.Muted, Border: a.Border, SidebarMatchContent: a.SidebarMatchContent, SidebarBackground: a.SidebarBackground,
		SidebarText: a.SidebarText, SidebarActive: a.SidebarActive, SidebarBorder: a.SidebarBorder, Success: a.Success,
		Warning: a.Warning, Destructive: a.Destructive, Font: a.Font, TextSize: a.TextSize, Scale: a.Scale, Density: a.Density,
		Radius: a.Radius, SidebarWidth: a.SidebarWidth, ContentWidth: a.ContentWidth,
	}
}
