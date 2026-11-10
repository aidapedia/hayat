package adminserver

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tenancy"
	"github.com/aidapedia/hayat/internal/tree"
)

// tenantService is allowed by permission on the tenant node, like the tree:
// the owner short-circuits to every permission, so they run their tenant
// without being an instance admin. Creating and deleting tenants stay with
// instance admins.
type tenantService struct{ s *Server }

// node is the tenant's node in the tree, which every check is made against.
func (t *tenantService) node(ctx context.Context, tenantID uuid.UUID) (tree.Node, error) {
	n, err := t.s.tree.ByRef(ctx, catalog.TypeTenant, tenantID.String())
	if err != nil {
		return tree.Node{}, t.s.fail(err)
	}
	return n, nil
}

// require parses a tenant id and checks perm on its node.
func (t *tenantService) require(ctx context.Context, raw, perm string) (uuid.UUID, error) {
	id, err := parseID(raw, "tenant id")
	if err != nil {
		return uuid.Nil, err
	}
	n, err := t.node(ctx, id)
	if err != nil {
		return uuid.Nil, err
	}
	if err := t.s.require(ctx, perm, n); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (t *tenantService) ListTenants(ctx context.Context, req *connect.Request[adminv1.ListTenantsRequest]) (*connect.Response[adminv1.ListTenantsResponse], error) {
	if me := caller(ctx).User; !me.IsAdmin() {
		// Someone belongs to a handful of tenants: one page, filtered here.
		list, err := t.s.tenants.ForUser(ctx, me.ID)
		if err != nil {
			return nil, t.s.fail(err)
		}
		q := strings.ToLower(strings.TrimSpace(req.Msg.Query))
		mine := make([]tenancy.Tenant, 0, len(list))
		for _, tn := range list {
			if q == "" || strings.Contains(strings.ToLower(tn.Name), q) || strings.Contains(tn.Slug, q) {
				mine = append(mine, tn)
			}
		}
		return connect.NewResponse(&adminv1.ListTenantsResponse{Tenants: toTenants(mine)}), nil
	}
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
	n, err := t.node(ctx, id)
	if err != nil {
		return nil, err
	}
	perms, err := t.s.visible(ctx, n)
	if err != nil {
		return nil, err
	}
	members, err := t.members(ctx, id)
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.GetTenantResponse{Tenant: toTenant(tn), Members: members, MyPermissions: perms}), nil
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
	if err := t.s.requireAdmin(ctx); err != nil {
		return nil, err
	}
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
	id, err := t.require(ctx, req.Msg.Id, catalog.PermTenantManage)
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
	if err := t.s.requireAdmin(ctx); err != nil {
		return nil, err
	}
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
	tenantID, err := t.require(ctx, req.Msg.TenantId, catalog.PermMembersManage)
	if err != nil {
		return nil, err
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
	tenantID, err := t.require(ctx, req.Msg.TenantId, catalog.PermMembersManage)
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
	// Ownership is given away, never taken: a Tenant admin grant must not be
	// enough to make yourself the owner.
	cur, err := t.s.tenants.Get(ctx, tenantID)
	if err != nil {
		return nil, t.s.fail(err)
	}
	if me := caller(ctx).User; !me.IsAdmin() && cur.OwnerID != me.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the owner can hand the tenant to someone else"))
	}
	tn, err := t.s.tenants.TransferOwnership(ctx, tenantID, userID, actor(ctx))
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.TransferOwnershipResponse{Tenant: toTenant(tn)}), nil
}
