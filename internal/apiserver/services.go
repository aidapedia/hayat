package apiserver

import (
	"context"
	"errors"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tenancy"
	"github.com/aidapedia/hayat/internal/tree"
)

// ---------- Shared ----------

func (s *Server) resolve(ctx context.Context, ref *authzv1.ResourceRef) (tree.Node, error) {
	if ref == nil {
		return tree.Node{}, invalid("resource is required")
	}
	if ref.NodeId != "" {
		id, err := uuid.Parse(ref.NodeId)
		if err != nil {
			return tree.Node{}, invalid("node_id is not a valid id")
		}
		return s.tree.Get(ctx, id)
	}
	if ref.Type == "" || ref.ExternalId == "" {
		return tree.Node{}, invalid("name the resource by node_id, or by type and external_id")
	}
	return s.tree.ByRef(ctx, ref.Type, ref.ExternalId)
}

func consistency(c *authzv1.Consistency) authz.Consistency {
	if c == nil {
		return authz.Consistency{}
	}
	return authz.Consistency{AtLeastRev: c.AtLeastRev, Fresh: c.FullyConsistent}
}

func toResource(n tree.Node) *authzv1.Resource {
	r := &authzv1.Resource{NodeId: n.ID.String(), Type: n.Type, ExternalId: n.ExternalID, Name: n.Name}
	if n.TenantID != nil {
		r.TenantId = n.TenantID.String()
	}
	if n.ParentID != nil {
		r.ParentNodeId = n.ParentID.String()
	}
	if n.OwnerID != nil {
		r.OwnerUserId = n.OwnerID.String()
	}
	return r
}

func toReason(r authz.Reason) *authzv1.Reason {
	out := &authzv1.Reason{Kind: r.Kind, NodeType: r.NodeType, NodeName: r.NodeName, Role: r.RoleName, PrincipalType: r.PrincipalType, Text: r.String()}
	if r.NodeID != uuid.Nil {
		out.NodeId = r.NodeID.String()
	}
	if r.PrincipalID != uuid.Nil {
		out.PrincipalId = r.PrincipalID.String()
	}
	return out
}

// ---------- AuthzService ----------

type authzService struct{ s *Server }

func (a *authzService) check(ctx context.Context, req *authzv1.CheckRequest) (*authzv1.CheckResponse, error) {
	user, err := parseUser(req.UserId)
	if err != nil {
		return nil, err
	}
	node, err := a.s.resolve(ctx, req.Resource)
	if err != nil {
		return nil, err
	}
	ok, why, r, err := a.s.engine.Check(ctx, user, req.Permission, node, consistency(req.Consistency))
	if err != nil {
		return nil, err
	}
	resp := &authzv1.CheckResponse{Allowed: ok, Rev: r.Rev, TenantId: r.TenantID.String()}
	if req.Explain && ok {
		resp.Reason = toReason(why)
	}
	return resp, nil
}

func (a *authzService) Check(ctx context.Context, req *connect.Request[authzv1.CheckRequest]) (*connect.Response[authzv1.CheckResponse], error) {
	resp, err := a.check(ctx, req.Msg)
	if err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(resp), nil
}

func (a *authzService) BatchCheck(ctx context.Context, req *connect.Request[authzv1.BatchCheckRequest]) (*connect.Response[authzv1.BatchCheckResponse], error) {
	if len(req.Msg.Checks) > 500 {
		return nil, invalid("at most 500 checks per batch")
	}
	out := &authzv1.BatchCheckResponse{}
	for _, c := range req.Msg.Checks {
		resp, err := a.check(ctx, c)
		if err != nil {
			var ce *connect.Error
			if errors.Is(err, store.ErrNotFound) || errors.As(err, &ce) {
				out.Results = append(out.Results, &authzv1.CheckResponse{})
				continue
			}
			return nil, a.s.fail(err)
		}
		out.Results = append(out.Results, resp)
	}
	return connect.NewResponse(out), nil
}

func (a *authzService) PermissionsAt(ctx context.Context, req *connect.Request[authzv1.PermissionsAtRequest]) (*connect.Response[authzv1.PermissionsAtResponse], error) {
	user, err := parseUser(req.Msg.UserId)
	if err != nil {
		return nil, err
	}
	node, err := a.s.resolve(ctx, req.Msg.Resource)
	if err != nil {
		return nil, a.s.fail(err)
	}
	r, err := a.s.engine.At(ctx, user, node, consistency(req.Msg.Consistency))
	if err != nil {
		return nil, a.s.fail(err)
	}
	resp := &authzv1.PermissionsAtResponse{Rev: r.Rev, TenantId: r.TenantID.String()}
	for p := range r.Perms {
		resp.Permissions = append(resp.Permissions, p)
	}
	return connect.NewResponse(resp), nil
}

func (a *authzService) LookupResources(ctx context.Context, req *connect.Request[authzv1.LookupResourcesRequest]) (*connect.Response[authzv1.LookupResourcesResponse], error) {
	m := req.Msg
	user, err := parseUser(m.UserId)
	if err != nil {
		return nil, err
	}
	var tenant *uuid.UUID
	if m.TenantId != "" {
		t, err := uuid.Parse(m.TenantId)
		if err != nil {
			return nil, invalid("tenant_id is not a valid id")
		}
		tenant = &t
	}
	size := int(m.PageSize)
	if size <= 0 || size > 500 {
		size = 100
	}
	offset, _ := strconv.Atoi(m.PageToken)
	nodes, err := a.s.engine.LookupResources(ctx, user, m.Permission, m.Type, tenant, consistency(m.Consistency), offset+size+1)
	if err != nil {
		return nil, a.s.fail(err)
	}
	resp := &authzv1.LookupResourcesResponse{}
	if offset < len(nodes) {
		page := nodes[offset:]
		if len(page) > size {
			page = page[:size]
			resp.NextPageToken = strconv.Itoa(offset + size)
		}
		for _, n := range page {
			resp.Resources = append(resp.Resources, toResource(n))
		}
	}
	return connect.NewResponse(resp), nil
}

func (a *authzService) LookupSubjects(ctx context.Context, req *connect.Request[authzv1.LookupSubjectsRequest]) (*connect.Response[authzv1.LookupSubjectsResponse], error) {
	node, err := a.s.resolve(ctx, req.Msg.Resource)
	if err != nil {
		return nil, a.s.fail(err)
	}
	subjects, err := a.s.engine.LookupSubjects(ctx, req.Msg.Permission, node, consistency(req.Msg.Consistency))
	if err != nil {
		return nil, a.s.fail(err)
	}
	resp := &authzv1.LookupSubjectsResponse{}
	for _, sub := range subjects {
		resp.Subjects = append(resp.Subjects, &authzv1.Subject{UserId: sub.UserID.String(), Reason: toReason(sub.Reason)})
	}
	return connect.NewResponse(resp), nil
}

// ---------- TreeService ----------

type treeService struct{ s *Server }

func (t *treeService) UpsertResource(ctx context.Context, req *connect.Request[authzv1.UpsertResourceRequest]) (*connect.Response[authzv1.UpsertResourceResponse], error) {
	m := req.Msg
	in := tree.ResourceInput{Type: m.Type, ExternalID: m.ExternalId, Name: m.Name}
	if m.Parent == nil {
		return nil, invalid("parent is required")
	}
	if m.Parent.NodeId != "" {
		id, err := uuid.Parse(m.Parent.NodeId)
		if err != nil {
			return nil, invalid("parent.node_id is not a valid id")
		}
		in.ParentID = id
	} else {
		in.ParentType, in.ParentExternalID = m.Parent.Type, m.Parent.ExternalId
	}
	if m.OwnerUserId != "" {
		owner, err := parseUser(m.OwnerUserId)
		if err != nil {
			return nil, err
		}
		in.OwnerID = &owner
	}
	n, rev, err := t.s.tree.UpsertResource(ctx, callerService(ctx).ID, in)
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&authzv1.UpsertResourceResponse{Rev: rev, TenantId: n.Tenant().String(), Resource: toResource(n)}), nil
}

func (t *treeService) DeleteResource(ctx context.Context, req *connect.Request[authzv1.DeleteResourceRequest]) (*connect.Response[authzv1.DeleteResourceResponse], error) {
	n, err := t.s.tree.ByRef(ctx, req.Msg.Type, req.Msg.ExternalId)
	if err != nil {
		return nil, t.s.fail(err)
	}
	rev, err := t.s.tree.DeleteResource(ctx, callerService(ctx).ID, req.Msg.Type, req.Msg.ExternalId)
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&authzv1.DeleteResourceResponse{Rev: rev, TenantId: n.Tenant().String()}), nil
}

func (t *treeService) GetResource(ctx context.Context, req *connect.Request[authzv1.GetResourceRequest]) (*connect.Response[authzv1.GetResourceResponse], error) {
	n, err := t.s.resolve(ctx, req.Msg.Resource)
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&authzv1.GetResourceResponse{Resource: toResource(n)}), nil
}

// ---------- DirectoryService ----------

type directoryService struct{ s *Server }

func toUser(u identity.User) *authzv1.User {
	return &authzv1.User{Id: u.ID.String(), Email: u.Email, EmailVerified: u.EmailVerified, Name: u.Name, Picture: u.AvatarURL,
		Locale: u.Locale, Zoneinfo: u.Zoneinfo, Status: u.Status}
}

func (d *directoryService) GetUser(ctx context.Context, req *connect.Request[authzv1.GetUserRequest]) (*connect.Response[authzv1.GetUserResponse], error) {
	id, err := parseUser(req.Msg.Id)
	if err != nil {
		return nil, err
	}
	u, err := d.s.ids.GetUser(ctx, id)
	if err != nil {
		return nil, d.s.fail(err)
	}
	return connect.NewResponse(&authzv1.GetUserResponse{User: toUser(u)}), nil
}

func (d *directoryService) BatchGetUsers(ctx context.Context, req *connect.Request[authzv1.BatchGetUsersRequest]) (*connect.Response[authzv1.BatchGetUsersResponse], error) {
	if len(req.Msg.Ids) > 500 {
		return nil, invalid("at most 500 ids per call")
	}
	var ids []uuid.UUID
	for _, raw := range req.Msg.Ids {
		if id, err := uuid.Parse(raw); err == nil {
			ids = append(ids, id)
		}
	}
	users, err := d.s.ids.GetUsers(ctx, ids)
	if err != nil {
		return nil, d.s.fail(err)
	}
	resp := &authzv1.BatchGetUsersResponse{}
	for _, u := range users {
		resp.Users = append(resp.Users, toUser(u))
	}
	return connect.NewResponse(resp), nil
}

// GetUserAppearance answers fresh every time: a service applies it to the
// person's session and keeps no copy, so a change in Hayat's console shows on
// their next page load.
func (d *directoryService) GetUserAppearance(ctx context.Context, req *connect.Request[authzv1.GetUserAppearanceRequest]) (*connect.Response[authzv1.GetUserAppearanceResponse], error) {
	id, err := parseUser(req.Msg.UserId)
	if err != nil {
		return nil, err
	}
	a, err := d.s.ids.Appearance(ctx, id)
	if err != nil {
		return nil, d.s.fail(err)
	}
	resp := &authzv1.GetUserAppearanceResponse{}
	if a != nil {
		resp.Appearance = &authzv1.Appearance{
			Preset: a.Preset, Mode: a.Mode, Accent: a.Accent, Background: a.Background, Surface: a.Surface, Text: a.Text,
			Muted: a.Muted, Border: a.Border, SidebarMatchContent: a.SidebarMatchContent, SidebarBackground: a.SidebarBackground,
			SidebarText: a.SidebarText, SidebarActive: a.SidebarActive, SidebarBorder: a.SidebarBorder, Success: a.Success,
			Warning: a.Warning, Destructive: a.Destructive, Font: a.Font, TextSize: a.TextSize, Density: a.Density,
			Radius: a.Radius, SidebarWidth: a.SidebarWidth, ContentWidth: a.ContentWidth,
		}
	}
	return connect.NewResponse(resp), nil
}

func toTenant(t tenancy.Tenant, nodeID uuid.UUID) *authzv1.Tenant {
	out := &authzv1.Tenant{Id: t.ID.String(), Slug: t.Slug, Name: t.Name, MemberCount: int32(t.MemberCount)}
	if t.OwnerID != uuid.Nil {
		out.OwnerUserId = t.OwnerID.String()
	}
	if nodeID != uuid.Nil {
		out.NodeId = nodeID.String()
	}
	return out
}

func (d *directoryService) ListUserTenants(ctx context.Context, req *connect.Request[authzv1.ListUserTenantsRequest]) (*connect.Response[authzv1.ListUserTenantsResponse], error) {
	user, err := parseUser(req.Msg.UserId)
	if err != nil {
		return nil, err
	}
	tenants, err := d.s.tenants.ForUser(ctx, user)
	if err != nil {
		return nil, d.s.fail(err)
	}
	resp := &authzv1.ListUserTenantsResponse{}
	for _, t := range tenants {
		var nodeID uuid.UUID
		if n, err := d.s.tree.ByRef(ctx, tenancy.TypeTenant, t.ID.String()); err == nil {
			nodeID = n.ID
		}
		resp.Tenants = append(resp.Tenants, toTenant(t, nodeID))
	}
	return connect.NewResponse(resp), nil
}

func (d *directoryService) ListTenantMembers(ctx context.Context, req *connect.Request[authzv1.ListTenantMembersRequest]) (*connect.Response[authzv1.ListTenantMembersResponse], error) {
	tenantID, err := uuid.Parse(req.Msg.TenantId)
	if err != nil {
		return nil, invalid("tenant_id is not a valid id")
	}
	members, err := d.s.tenants.Members(ctx, tenantID)
	if err != nil {
		return nil, d.s.fail(err)
	}
	ids := make([]uuid.UUID, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	users, err := d.s.ids.GetUsers(ctx, ids)
	if err != nil {
		return nil, d.s.fail(err)
	}
	byID := make(map[uuid.UUID]identity.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	resp := &authzv1.ListTenantMembersResponse{}
	for _, m := range members {
		u, ok := byID[m.UserID]
		if !ok {
			continue
		}
		resp.Members = append(resp.Members, &authzv1.TenantMember{User: toUser(u), IsOwner: m.IsOwner, JoinedAt: timestamppb.New(m.JoinedAt)})
	}
	return connect.NewResponse(resp), nil
}

func (d *directoryService) ListTenantGrants(ctx context.Context, req *connect.Request[authzv1.ListTenantGrantsRequest]) (*connect.Response[authzv1.ListTenantGrantsResponse], error) {
	tenantID, err := uuid.Parse(req.Msg.TenantId)
	if err != nil {
		return nil, invalid("tenant_id is not a valid id")
	}
	users := make([]uuid.UUID, 0, len(req.Msg.UserIds))
	for _, raw := range req.Msg.UserIds {
		id, err := parseUser(raw)
		if err != nil {
			return nil, err
		}
		users = append(users, id)
	}
	// The tenant's own nodes, and the caller's types: another service's
	// resources are none of its business.
	svc := callerService(ctx).ID
	keep := func(typ string) bool {
		return typ == catalog.TypeTenant || typ == catalog.TypeGroup || catalog.ServiceOf(typ) == svc
	}
	grants, err := d.s.tree.MemberGrants(ctx, tenantID, users, keep)
	if err != nil {
		return nil, d.s.fail(err)
	}
	resp := &authzv1.ListTenantGrantsResponse{}
	for _, g := range grants {
		out := &authzv1.MemberGrant{UserId: g.UserID.String(), Resource: toResource(g.Node), Via: g.Via,
			Role: g.RoleSlug, RoleName: g.RoleName, Inherit: g.Inherit, GroupName: g.GroupName}
		if g.GroupID != uuid.Nil {
			out.GroupId = g.GroupID.String()
		}
		resp.Grants = append(resp.Grants, out)
	}
	return connect.NewResponse(resp), nil
}

// ---------- WatchService ----------

type watchService struct{ s *Server }

// Watch polls the policy event table. It needs nothing but the database, so it
// works the same on PostgreSQL and YugabyteDB, with or without Redis.
func (w *watchService) Watch(ctx context.Context, req *connect.Request[authzv1.WatchRequest], stream *connect.ServerStream[authzv1.WatchResponse]) error {
	cursor := revision.Cursor()
	if req.Msg.Cursor != "" {
		c, err := uuid.Parse(req.Msg.Cursor)
		if err != nil {
			return invalid("cursor is not a valid event id")
		}
		cursor = c
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		events, err := revision.Since(ctx, w.s.store.Pool, cursor, 200)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return w.s.fail(err)
		}
		for _, e := range events {
			if err := stream.Send(&authzv1.WatchResponse{Event: &authzv1.PolicyEvent{Id: e.ID.String(), TenantId: e.TenantID.String(),
				Rev: e.Rev, Kind: e.Kind, At: timestamppb.New(e.At)}}); err != nil {
				return err
			}
			cursor = e.ID
		}
		if len(events) == 200 {
			continue // more waiting
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// ---------- SchemaService ----------

type schemaService struct{ s *Server }

func (sc *schemaService) PublishContract(ctx context.Context, req *connect.Request[authzv1.PublishContractRequest]) (*connect.Response[authzv1.PublishContractResponse], error) {
	svc := callerService(ctx)
	ct, err := sc.s.cat.Publish(ctx, svc.ID, []byte(req.Msg.Manifest), "service:"+svc.ID)
	if err != nil {
		return nil, sc.s.fail(err)
	}
	return connect.NewResponse(&authzv1.PublishContractResponse{Version: int32(ct.Version), Status: ct.Status, Namespaces: ct.Namespaces}), nil
}
