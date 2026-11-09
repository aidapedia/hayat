package adminserver

import (
	"context"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/identity"
)

// accountService is the caller's own account. Any active, signed-in user.
type accountService struct{ s *Server }

func (a *accountService) GetMe(ctx context.Context, _ *connect.Request[adminv1.GetMeRequest]) (*connect.Response[adminv1.GetMeResponse], error) {
	me := caller(ctx).User
	tenants, err := a.s.tenants.ForUser(ctx, me.ID)
	if err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.GetMeResponse{User: toUser(me), Tenants: toTenants(tenants)}), nil
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
