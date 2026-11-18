package adminserver

import (
	"context"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/loginpage"
)

type loginPageService struct{ s *Server }

// authorize lets the service's owners in (hayat.service.manage on its node),
// and instance admins for Hayat's own page. → the service's display name.
func (l *loginPageService) authorize(ctx context.Context, serviceID string) (string, error) {
	if serviceID == catalog.Platform {
		return "Hayat", l.s.requireAdmin(ctx)
	}
	svc, n, err := (&registryService{l.s}).serviceNode(ctx, serviceID)
	if err != nil {
		return "", err
	}
	return svc.DisplayName, l.s.require(ctx, catalog.PermServiceManage, n)
}

func toDesign(d loginpage.Design) *adminv1.LoginPageDesign {
	return &adminv1.LoginPageDesign{
		Layout: d.Layout, Mode: d.Mode, Accent: d.Accent, Panel: d.Panel, Font: d.Font, Radius: d.Radius,
		Logo: d.Logo, LogoDark: d.LogoDark, Name: d.Name, Heading: d.Heading, Subheading: d.Subheading, Tagline: d.Tagline,
		TermsUrl: d.TermsURL, PrivacyUrl: d.PrivacyURL, SupportUrl: d.SupportURL, HideSignup: d.HideSignup,
	}
}

func fromDesign(d *adminv1.LoginPageDesign) loginpage.Design {
	if d == nil {
		return loginpage.Design{}
	}
	return loginpage.Design{
		Layout: d.Layout, Mode: d.Mode, Accent: d.Accent, Panel: d.Panel, Font: d.Font, Radius: d.Radius,
		Logo: d.Logo, LogoDark: d.LogoDark, Name: d.Name, Heading: d.Heading, Subheading: d.Subheading, Tagline: d.Tagline,
		TermsURL: d.TermsUrl, PrivacyURL: d.PrivacyUrl, SupportURL: d.SupportUrl, HideSignup: d.HideSignup,
	}
}

// publisher resolves an audit actor ("user:<id>") to the person, when it is one.
func (l *loginPageService) publisher(ctx context.Context, actor string) *adminv1.User {
	id, err := uuid.Parse(strings.TrimPrefix(actor, "user:"))
	if err != nil {
		return nil
	}
	users, err := l.s.ids.GetUsers(ctx, []uuid.UUID{id})
	if err != nil || len(users) == 0 {
		return nil
	}
	return toUser(users[0])
}

func (l *loginPageService) toPage(ctx context.Context, p loginpage.Page, name string) *adminv1.LoginPage {
	out := &adminv1.LoginPage{
		ServiceId: p.ServiceID, ServiceName: name, Draft: toDesign(p.Draft), Version: int32(p.Version),
		PublishedAt: tsp(p.PublishedAt), UpdatedAt: ts(p.UpdatedAt), HasChanges: p.HasChanges(),
		PreviewUrl:   l.s.cfg.Issuer + "/preview/login?token=" + url.QueryEscape(l.s.pages.PreviewToken(p.ServiceID, time.Now())),
		AssetBaseUrl: l.s.cfg.Issuer + "/media/",
	}
	if p.Published != nil {
		out.Published = toDesign(*p.Published)
		out.PublishedBy = l.publisher(ctx, p.PublishedBy)
	}
	return out
}

func (l *loginPageService) GetLoginPage(ctx context.Context, req *connect.Request[adminv1.GetLoginPageRequest]) (*connect.Response[adminv1.GetLoginPageResponse], error) {
	name, err := l.authorize(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	p, err := l.s.pages.Get(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, l.s.fail(err)
	}
	return connect.NewResponse(&adminv1.GetLoginPageResponse{Page: l.toPage(ctx, p, name), CanManage: true}), nil
}

func (l *loginPageService) SaveLoginPageDraft(ctx context.Context, req *connect.Request[adminv1.SaveLoginPageDraftRequest]) (*connect.Response[adminv1.SaveLoginPageDraftResponse], error) {
	name, err := l.authorize(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	p, err := l.s.pages.SaveDraft(ctx, req.Msg.ServiceId, fromDesign(req.Msg.Design))
	if err != nil {
		return nil, l.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SaveLoginPageDraftResponse{Page: l.toPage(ctx, p, name)}), nil
}

func (l *loginPageService) PublishLoginPage(ctx context.Context, req *connect.Request[adminv1.PublishLoginPageRequest]) (*connect.Response[adminv1.PublishLoginPageResponse], error) {
	name, err := l.authorize(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	p, err := l.s.pages.Publish(ctx, req.Msg.ServiceId, actor(ctx))
	if err != nil {
		return nil, l.s.fail(err)
	}
	return connect.NewResponse(&adminv1.PublishLoginPageResponse{Page: l.toPage(ctx, p, name)}), nil
}

func (l *loginPageService) DiscardLoginPageDraft(ctx context.Context, req *connect.Request[adminv1.DiscardLoginPageDraftRequest]) (*connect.Response[adminv1.DiscardLoginPageDraftResponse], error) {
	name, err := l.authorize(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	p, err := l.s.pages.Discard(ctx, req.Msg.ServiceId, actor(ctx))
	if err != nil {
		return nil, l.s.fail(err)
	}
	return connect.NewResponse(&adminv1.DiscardLoginPageDraftResponse{Page: l.toPage(ctx, p, name)}), nil
}

func (l *loginPageService) ListLoginPageVersions(ctx context.Context, req *connect.Request[adminv1.ListLoginPageVersionsRequest]) (*connect.Response[adminv1.ListLoginPageVersionsResponse], error) {
	if _, err := l.authorize(ctx, req.Msg.ServiceId); err != nil {
		return nil, err
	}
	list, err := l.s.pages.Versions(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, l.s.fail(err)
	}
	resp := &adminv1.ListLoginPageVersionsResponse{}
	for _, v := range list {
		resp.Versions = append(resp.Versions, &adminv1.LoginPageVersion{
			Version: int32(v.Version), Design: toDesign(v.Design), PublishedBy: l.publisher(ctx, v.PublishedBy), PublishedAt: ts(v.PublishedAt),
		})
	}
	return connect.NewResponse(resp), nil
}

func (l *loginPageService) RestoreLoginPageVersion(ctx context.Context, req *connect.Request[adminv1.RestoreLoginPageVersionRequest]) (*connect.Response[adminv1.RestoreLoginPageVersionResponse], error) {
	name, err := l.authorize(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	p, err := l.s.pages.Restore(ctx, req.Msg.ServiceId, int(req.Msg.Version), actor(ctx))
	if err != nil {
		return nil, l.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RestoreLoginPageVersionResponse{Page: l.toPage(ctx, p, name)}), nil
}

func (l *loginPageService) UploadLoginPageAsset(ctx context.Context, req *connect.Request[adminv1.UploadLoginPageAssetRequest]) (*connect.Response[adminv1.UploadLoginPageAssetResponse], error) {
	if _, err := l.authorize(ctx, req.Msg.ServiceId); err != nil {
		return nil, err
	}
	id, err := l.s.pages.AddAsset(ctx, req.Msg.ServiceId, req.Msg.Data, actor(ctx))
	if err != nil {
		return nil, l.s.fail(err)
	}
	return connect.NewResponse(&adminv1.UploadLoginPageAssetResponse{AssetId: id.String(), Url: l.s.cfg.Issuer + "/media/" + id.String()}), nil
}
