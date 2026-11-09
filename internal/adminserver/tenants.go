package adminserver

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/store"
)

type tenantService struct{ s *Server }

func (t *tenantService) ListTenants(ctx context.Context, req *connect.Request[adminv1.ListTenantsRequest]) (*connect.Response[adminv1.ListTenantsResponse], error) {
	size := pageSize(req.Msg.PageSize)
	list, err := t.s.tenants.List(ctx, req.Msg.Query, parsePageToken(req.Msg.PageToken), size+1)
	if err != nil {
		return nil, t.s.fail(err)
	}
	resp := &adminv1.ListTenantsResponse{}
	if len(list) > size {
		list = list[:size]
		resp.NextPageToken = pageToken(list[size-1].CreatedAt)
	}
	resp.Tenants = toTenants(list)
	return connect.NewResponse(resp), nil
}

func (t *tenantService) GetTenant(ctx context.Context, req *connect.Request[adminv1.GetTenantRequest]) (*connect.Response[adminv1.GetTenantResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	tn, err := t.s.tenants.Get(ctx, id)
	if err != nil {
		return nil, t.s.fail(err)
	}
	members, err := t.members(ctx, id)
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.GetTenantResponse{Tenant: toTenant(tn), Members: members}), nil
}

func (t *tenantService) members(ctx context.Context, tenantID uuid.UUID) ([]*adminv1.Member, error) {
	list, err := t.s.tenants.Members(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(list))
	for i, m := range list {
		ids[i] = m.UserID
	}
	users, err := t.s.ids.GetUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]identity.User{}
	for _, u := range users {
		byID[u.ID] = u
	}
	out := make([]*adminv1.Member, 0, len(list))
	for _, m := range list {
		u, ok := byID[m.UserID]
		if !ok {
			continue
		}
		out = append(out, &adminv1.Member{User: toUser(u), IsOwner: m.IsOwner, JoinedAt: ts(m.JoinedAt)})
	}
	return out, nil
}

// resolveUser finds a user by id, or by email, inviting the address when it has
// no account yet. invited reports the latter.
func (t *tenantService) resolveUser(ctx context.Context, rawID, email, name string) (identity.User, bool, error) {
	if rawID != "" {
		id, err := parseID(rawID, "user_id")
		if err != nil {
			return identity.User{}, false, err
		}
		u, err := t.s.ids.GetUser(ctx, id)
		return u, false, err
	}
	if strings.TrimSpace(email) == "" {
		return identity.User{}, false, invalid("choose a user or enter an email address")
	}
	u, created, err := t.s.ids.Invite(ctx, email, name, actor(ctx))
	return u, created, err
}

func (t *tenantService) CreateTenant(ctx context.Context, req *connect.Request[adminv1.CreateTenantRequest]) (*connect.Response[adminv1.CreateTenantResponse], error) {
	m := req.Msg
	owner, _, err := t.resolveUser(ctx, m.OwnerUserId, m.OwnerEmail, "")
	if err != nil {
		return nil, t.s.fail(err)
	}
	if owner.Status == identity.StatusDisabled {
		return nil, invalid("a disabled account can't own a tenant")
	}
	tn, err := t.s.tenants.Create(ctx, m.Name, strings.TrimSpace(m.Slug), owner.ID, actor(ctx))
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.CreateTenantResponse{Tenant: toTenant(tn)}), nil
}

func (t *tenantService) RenameTenant(ctx context.Context, req *connect.Request[adminv1.RenameTenantRequest]) (*connect.Response[adminv1.RenameTenantResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	tn, err := t.s.tenants.Rename(ctx, id, req.Msg.Name, actor(ctx))
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RenameTenantResponse{Tenant: toTenant(tn)}), nil
}

func (t *tenantService) DeleteTenant(ctx context.Context, req *connect.Request[adminv1.DeleteTenantRequest]) (*connect.Response[adminv1.DeleteTenantResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	if err := t.s.tenants.Delete(ctx, id, actor(ctx)); err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.DeleteTenantResponse{}), nil
}

func (t *tenantService) AddMember(ctx context.Context, req *connect.Request[adminv1.AddMemberRequest]) (*connect.Response[adminv1.AddMemberResponse], error) {
	tenantID, err := parseID(req.Msg.TenantId, "tenant_id")
	if err != nil {
		return nil, err
	}
	if _, err := t.s.tenants.Get(ctx, tenantID); err != nil {
		return nil, t.s.fail(err)
	}
	u, invited, err := t.resolveUser(ctx, req.Msg.UserId, req.Msg.Email, req.Msg.Name)
	if err != nil {
		return nil, t.s.fail(err)
	}
	if err := t.s.tenants.AddMember(ctx, tenantID, u.ID, actor(ctx)); err != nil {
		return nil, t.s.fail(err)
	}
	members, err := t.members(ctx, tenantID)
	if err != nil {
		return nil, t.s.fail(err)
	}
	for _, m := range members {
		if m.User.Id == u.ID.String() {
			return connect.NewResponse(&adminv1.AddMemberResponse{Member: m, Invited: invited}), nil
		}
	}
	return nil, t.s.fail(errors.New("member missing right after it was added"))
}

func (t *tenantService) RemoveMember(ctx context.Context, req *connect.Request[adminv1.RemoveMemberRequest]) (*connect.Response[adminv1.RemoveMemberResponse], error) {
	tenantID, err := parseID(req.Msg.TenantId, "tenant_id")
	if err != nil {
		return nil, err
	}
	userID, err := parseID(req.Msg.UserId, "user_id")
	if err != nil {
		return nil, err
	}
	if err := t.s.tenants.RemoveMember(ctx, tenantID, userID, actor(ctx)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("that user isn't a member"))
		}
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RemoveMemberResponse{}), nil
}

func (t *tenantService) TransferOwnership(ctx context.Context, req *connect.Request[adminv1.TransferOwnershipRequest]) (*connect.Response[adminv1.TransferOwnershipResponse], error) {
	tenantID, err := parseID(req.Msg.TenantId, "tenant_id")
	if err != nil {
		return nil, err
	}
	userID, err := parseID(req.Msg.UserId, "user_id")
	if err != nil {
		return nil, err
	}
	tn, err := t.s.tenants.TransferOwnership(ctx, tenantID, userID, actor(ctx))
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.TransferOwnershipResponse{Tenant: toTenant(tn)}), nil
}
