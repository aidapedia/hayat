package adminserver

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/identity"
)

type userService struct{ s *Server }

func (u *userService) ListUsers(ctx context.Context, req *connect.Request[adminv1.ListUsersRequest]) (*connect.Response[adminv1.ListUsersResponse], error) {
	size := pageSize(req.Msg.PageSize)
	list, err := u.s.ids.ListUsers(ctx, req.Msg.Query, parsePageToken(req.Msg.PageToken), size+1)
	if err != nil {
		return nil, u.s.fail(err)
	}
	resp := &adminv1.ListUsersResponse{}
	if len(list) > size {
		list = list[:size]
		resp.NextPageToken = pageToken(list[size-1].CreatedAt)
	}
	for _, x := range list {
		resp.Users = append(resp.Users, toUser(x))
	}
	return connect.NewResponse(resp), nil
}

func (u *userService) GetUser(ctx context.Context, req *connect.Request[adminv1.GetUserRequest]) (*connect.Response[adminv1.GetUserResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	user, err := u.s.ids.GetUser(ctx, id)
	if err != nil {
		return nil, u.s.fail(err)
	}
	tenants, err := u.s.tenants.ForUser(ctx, id)
	if err != nil {
		return nil, u.s.fail(err)
	}
	sessions, err := u.s.ids.ListSessions(ctx, id)
	if err != nil {
		return nil, u.s.fail(err)
	}
	return connect.NewResponse(&adminv1.GetUserResponse{User: toUser(user), Tenants: toTenants(tenants),
		Sessions: toSessions(sessions, caller(ctx).SessionID.String())}), nil
}

func (u *userService) InviteUser(ctx context.Context, req *connect.Request[adminv1.InviteUserRequest]) (*connect.Response[adminv1.InviteUserResponse], error) {
	user, created, err := u.s.ids.Invite(ctx, req.Msg.Email, req.Msg.Name, actor(ctx))
	if err != nil {
		return nil, u.s.fail(err)
	}
	return connect.NewResponse(&adminv1.InviteUserResponse{User: toUser(user), Created: created}), nil
}

func (u *userService) UpdateUser(ctx context.Context, req *connect.Request[adminv1.UpdateUserRequest]) (*connect.Response[adminv1.UpdateUserResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	m := req.Msg
	user, err := u.s.ids.UpdateProfile(ctx, id, identity.Profile{Name: m.Name, Locale: m.Locale, Zoneinfo: m.Zoneinfo, Phone: m.Phone}, actor(ctx))
	if err != nil {
		return nil, u.s.fail(err)
	}
	return connect.NewResponse(&adminv1.UpdateUserResponse{User: toUser(user)}), nil
}

func (u *userService) SetUserDisabled(ctx context.Context, req *connect.Request[adminv1.SetUserDisabledRequest]) (*connect.Response[adminv1.SetUserDisabledResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	if req.Msg.Disabled && id == caller(ctx).User.ID {
		return nil, invalid("you can't disable your own account")
	}
	user, err := u.s.ids.SetStatus(ctx, id, req.Msg.Disabled, actor(ctx))
	if err != nil {
		return nil, u.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SetUserDisabledResponse{User: toUser(user)}), nil
}

func (u *userService) SetUserAdmin(ctx context.Context, req *connect.Request[adminv1.SetUserAdminRequest]) (*connect.Response[adminv1.SetUserAdminResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	user, err := u.s.ids.SetSystemRole(ctx, id, req.Msg.Admin, actor(ctx))
	if err != nil {
		return nil, u.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SetUserAdminResponse{User: toUser(user)}), nil
}

func (u *userService) ResendVerification(ctx context.Context, req *connect.Request[adminv1.ResendVerificationRequest]) (*connect.Response[adminv1.ResendVerificationResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	if err := u.s.ids.ResendVerification(ctx, id); err != nil {
		return nil, u.s.fail(err)
	}
	return connect.NewResponse(&adminv1.ResendVerificationResponse{}), nil
}

func (u *userService) RevokeUserSessions(ctx context.Context, req *connect.Request[adminv1.RevokeUserSessionsRequest]) (*connect.Response[adminv1.RevokeUserSessionsResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	if req.Msg.SessionId == "" {
		err = u.s.ids.RevokeAllSessions(ctx, id, actor(ctx))
	} else {
		var sid uuid.UUID
		if sid, err = parseID(req.Msg.SessionId, "session_id"); err != nil {
			return nil, err
		}
		err = u.s.ids.RevokeSession(ctx, id, sid, actor(ctx))
	}
	if err != nil {
		return nil, u.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RevokeUserSessionsResponse{}), nil
}

// actorNames resolves "user:<id>" actors to display names for the audit view.
func (s *Server) actorNames(ctx context.Context, events []audit.Event) map[string]string {
	seen := map[uuid.UUID]bool{}
	var ids []uuid.UUID
	for _, e := range events {
		if raw, ok := strings.CutPrefix(e.Actor, "user:"); ok {
			if id, err := uuid.Parse(raw); err == nil && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	names := map[string]string{}
	users, err := s.ids.GetUsers(ctx, ids)
	if err != nil {
		return names
	}
	for _, u := range users {
		names[audit.UserActor(u.ID)] = u.DisplayName()
	}
	return names
}
