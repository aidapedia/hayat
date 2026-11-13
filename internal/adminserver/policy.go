package adminserver

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/identity"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/tree"
)

// ---------- Permission checks ----------

// may reports whether the caller holds perm at node. Admin calls are rare, so
// they always read fresh.
func (s *Server) may(ctx context.Context, perm string, node tree.Node) (bool, error) {
	ok, _, _, err := s.engine.Check(ctx, caller(ctx).User.ID, perm, node, authz.Consistency{Fresh: true})
	return ok, err
}

func (s *Server) require(ctx context.Context, perm string, node tree.Node) error {
	ok, err := s.may(ctx, perm, node)
	if err != nil {
		return s.fail(err)
	}
	if !ok {
		return connect.NewError(connect.CodePermissionDenied, errors.New("you don't have permission to do this here"))
	}
	return nil
}

// visible lets someone open a node of a tenant when they belong to the tenant
// or hold any permission there, and returns those permissions, sorted.
func (s *Server) visible(ctx context.Context, n tree.Node) ([]string, error) {
	r, err := s.engine.At(ctx, caller(ctx).User.ID, n, authz.Consistency{Fresh: true})
	if err != nil {
		return nil, s.fail(err)
	}
	if !r.Member && len(r.Perms) == 0 {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("you don't belong to this tenant"))
	}
	perms := make([]string, 0, len(r.Perms))
	for p := range r.Perms {
		perms = append(perms, p)
	}
	slices.Sort(perms)
	return perms, nil
}

func (s *Server) requireAdmin(ctx context.Context) error {
	if !caller(ctx).User.IsAdmin() {
		return connect.NewError(connect.CodePermissionDenied, errors.New("only an instance admin can do this"))
	}
	return nil
}

func (s *Server) node(ctx context.Context, raw string) (tree.Node, error) {
	id, err := parseID(raw, "node id")
	if err != nil {
		return tree.Node{}, err
	}
	n, err := s.tree.Get(ctx, id)
	if err != nil {
		return tree.Node{}, s.fail(err)
	}
	return n, nil
}

// ---------- Converters ----------

func toNode(n tree.Node) *adminv1.Node {
	out := &adminv1.Node{Id: n.ID.String(), Type: n.Type, ExternalId: n.ExternalID, Name: n.Name, Depth: int32(n.Depth),
		CreatedAt: ts(n.CreatedAt), UpdatedAt: ts(n.UpdatedAt)}
	if n.TenantID != nil {
		out.TenantId = n.TenantID.String()
	}
	if n.ParentID != nil {
		out.ParentId = n.ParentID.String()
	}
	if n.OwnerID != nil {
		out.OwnerId = n.OwnerID.String()
	}
	return out
}

func toNodes(list []tree.Node) []*adminv1.Node {
	out := make([]*adminv1.Node, 0, len(list))
	for _, n := range list {
		out = append(out, toNode(n))
	}
	return out
}

func toExplanation(r authz.Reason) *adminv1.Explanation {
	out := &adminv1.Explanation{Kind: r.Kind, NodeType: r.NodeType, NodeName: r.NodeName, RoleName: r.RoleName, PrincipalType: r.PrincipalType, Text: r.String()}
	if r.NodeID != uuid.Nil {
		out.NodeId = r.NodeID.String()
	}
	if r.PrincipalID != uuid.Nil {
		out.PrincipalId = r.PrincipalID.String()
	}
	return out
}

func toRole(r catalog.Role, usage map[uuid.UUID]int) *adminv1.Role {
	return &adminv1.Role{Id: r.ID.String(), Slug: r.Slug, Name: r.Name, Description: r.Description, Service: r.Service,
		AppliesTo: r.AppliesTo, Permissions: r.Permissions, GrantCount: int32(usage[r.ID]), Builtin: catalog.IsBuiltinRole(r.ID)}
}

func (s *Server) toGrants(ctx context.Context, grants []tree.Grant) ([]*adminv1.Grant, error) {
	snap, err := s.cat.Snapshot(ctx, false)
	if err != nil {
		return nil, err
	}
	var userIDs []uuid.UUID
	for _, g := range grants {
		if g.PrincipalType == tree.PrincipalUser {
			userIDs = append(userIDs, g.PrincipalID)
		}
	}
	users, err := s.ids.GetUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]identity.User{}
	for _, u := range users {
		byID[u.ID] = u
	}
	out := make([]*adminv1.Grant, 0, len(grants))
	for _, g := range grants {
		pg := &adminv1.Grant{Id: g.ID.String(), NodeId: g.NodeID.String(), PrincipalType: g.PrincipalType, PrincipalId: g.PrincipalID.String(),
			RoleId: g.RoleID.String(), RoleName: snap.Roles[g.RoleID].Name, Inherit: g.Inherit, ExpiresAt: tsp(g.ExpiresAt),
			CreatedBy: g.CreatedBy, CreatedAt: ts(g.CreatedAt)}
		if g.PrincipalType == tree.PrincipalUser {
			u := byID[g.PrincipalID]
			pg.PrincipalName, pg.PrincipalEmail = u.Name, u.Email
		} else if n, err := s.tree.Get(ctx, g.PrincipalID); err == nil {
			pg.PrincipalName = n.Name
			if n.Type == catalog.TypeTenant {
				pg.PrincipalName = "Everyone in " + n.Name
			}
		}
		out = append(out, pg)
	}
	return out, nil
}

// ---------- CatalogService ----------

type catalogService struct{ s *Server }

func (c *catalogService) GetCatalog(ctx context.Context, _ *connect.Request[adminv1.GetCatalogRequest]) (*connect.Response[adminv1.GetCatalogResponse], error) {
	snap, err := c.s.cat.Snapshot(ctx, true)
	if err != nil {
		return nil, c.s.fail(err)
	}
	resp := &adminv1.GetCatalogResponse{}
	for _, t := range snap.Types {
		resp.Types = append(resp.Types, &adminv1.NodeType{Key: t.Key, Service: t.Service, Parents: t.Parents, AccessPermission: t.AccessPermission,
			ViaPlacement: t.ViaPlacement, RequiresMembership: t.RequiresMembership, Degradable: t.Degradable, Description: t.Description})
	}
	for _, p := range snap.Permissions {
		resp.Permissions = append(resp.Permissions, &adminv1.PermissionInfo{Key: p.Key, Service: p.Service, Scope: p.Scope, Label: p.Label, Description: p.Description})
	}
	slices.SortFunc(resp.Types, func(a, b *adminv1.NodeType) int { return strings.Compare(a.Key, b.Key) })
	slices.SortFunc(resp.Permissions, func(a, b *adminv1.PermissionInfo) int { return strings.Compare(a.Key, b.Key) })
	return connect.NewResponse(resp), nil
}

func (c *catalogService) ListRoles(ctx context.Context, _ *connect.Request[adminv1.ListRolesRequest]) (*connect.Response[adminv1.ListRolesResponse], error) {
	roles, err := c.s.cat.Roles(ctx)
	if err != nil {
		return nil, c.s.fail(err)
	}
	usage, err := c.s.cat.RoleUsage(ctx)
	if err != nil {
		return nil, c.s.fail(err)
	}
	resp := &adminv1.ListRolesResponse{}
	for _, r := range roles {
		resp.Roles = append(resp.Roles, toRole(r, usage))
	}
	return connect.NewResponse(resp), nil
}

func (c *catalogService) CreateRole(ctx context.Context, req *connect.Request[adminv1.CreateRoleRequest]) (*connect.Response[adminv1.CreateRoleResponse], error) {
	if err := c.s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	m := req.Msg
	r, err := c.s.cat.CreateRole(ctx, catalog.RoleInput{Name: m.Name, Description: m.Description, AppliesTo: m.AppliesTo, Permissions: m.Permissions}, actor(ctx))
	if err != nil {
		return nil, c.s.fail(err)
	}
	return connect.NewResponse(&adminv1.CreateRoleResponse{Role: toRole(r, nil)}), nil
}

func (c *catalogService) UpdateRole(ctx context.Context, req *connect.Request[adminv1.UpdateRoleRequest]) (*connect.Response[adminv1.UpdateRoleResponse], error) {
	if err := c.s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	m := req.Msg
	id, err := parseID(m.Id, "id")
	if err != nil {
		return nil, err
	}
	r, err := c.s.cat.UpdateRole(ctx, id, catalog.RoleInput{Name: m.Name, Description: m.Description, AppliesTo: m.AppliesTo, Permissions: m.Permissions}, actor(ctx))
	if err != nil {
		return nil, c.s.fail(err)
	}
	return connect.NewResponse(&adminv1.UpdateRoleResponse{Role: toRole(r, nil)}), nil
}

func (c *catalogService) DeleteRole(ctx context.Context, req *connect.Request[adminv1.DeleteRoleRequest]) (*connect.Response[adminv1.DeleteRoleResponse], error) {
	if err := c.s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	if err := c.s.cat.DeleteRole(ctx, id, actor(ctx)); err != nil {
		return nil, c.s.fail(err)
	}
	return connect.NewResponse(&adminv1.DeleteRoleResponse{}), nil
}

// roleGrantLimit caps one ListRoleGrants answer; the console says when there are more.
const roleGrantLimit = 200

func (c *catalogService) ListRoleGrants(ctx context.Context, req *connect.Request[adminv1.ListRoleGrantsRequest]) (*connect.Response[adminv1.ListRoleGrantsResponse], error) {
	// It names who holds access in every tenant, so only an instance admin may ask.
	if err := c.s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	id, err := parseID(req.Msg.RoleId, "role id")
	if err != nil {
		return nil, err
	}
	grants, err := c.s.tree.RoleGrants(ctx, id, roleGrantLimit+1)
	if err != nil {
		return nil, c.s.fail(err)
	}
	resp := &adminv1.ListRoleGrantsResponse{}
	if len(grants) > roleGrantLimit {
		grants, resp.Truncated = grants[:roleGrantLimit], true
	}
	converted, err := c.s.toGrants(ctx, grants)
	if err != nil {
		return nil, c.s.fail(err)
	}
	// Several grants often sit on one node; read each node's chain once.
	type place struct {
		node tree.Node
		path []string
	}
	places := map[uuid.UUID]place{}
	for i, g := range grants {
		p, ok := places[g.NodeID]
		if !ok {
			n, err := c.s.tree.Get(ctx, g.NodeID)
			if err != nil {
				return nil, c.s.fail(err)
			}
			chain, err := c.s.tree.Ancestors(ctx, n)
			if err != nil {
				return nil, c.s.fail(err)
			}
			p = place{node: n}
			for _, a := range chain {
				// The root is implied; the node itself is named on its own.
				if a.ID != catalog.RootID && a.ID != n.ID {
					p.path = append(p.path, a.Name)
				}
			}
			places[g.NodeID] = p
		}
		rg := &adminv1.RoleGrant{Grant: converted[i], NodeType: p.node.Type, NodeName: p.node.Name, Path: p.path}
		if p.node.TenantID != nil {
			rg.TenantId = p.node.TenantID.String()
		}
		resp.Grants = append(resp.Grants, rg)
	}
	return connect.NewResponse(resp), nil
}

// ---------- RegistryService ----------

type registryService struct{ s *Server }

func (r *registryService) serviceNode(ctx context.Context, id string) (catalog.ServiceInfo, tree.Node, error) {
	svc, err := r.s.cat.GetService(ctx, id)
	if err != nil {
		return catalog.ServiceInfo{}, tree.Node{}, r.s.fail(err)
	}
	n, err := r.s.tree.Get(ctx, svc.NodeID)
	if err != nil {
		return catalog.ServiceInfo{}, tree.Node{}, r.s.fail(err)
	}
	return svc, n, nil
}

func (r *registryService) toService(ctx context.Context, svc catalog.ServiceInfo) (*adminv1.ServiceInfo, error) {
	owners, err := r.s.cat.ServiceOwners(ctx, svc.ID)
	if err != nil {
		return nil, err
	}
	users, err := r.s.ids.GetUsers(ctx, owners)
	if err != nil {
		return nil, err
	}
	out := &adminv1.ServiceInfo{Id: svc.ID, DisplayName: svc.DisplayName, Description: svc.Description, ClientId: svc.ClientID,
		ActiveVersion: int32(svc.ActiveVersion), CreatedAt: ts(svc.CreatedAt)}
	for _, u := range users {
		out.Owners = append(out.Owners, toUser(u))
	}
	pending, err := r.s.cat.PendingFor(ctx, svc.ID)
	if err != nil {
		return nil, err
	}
	for _, ct := range pending {
		if len(r.canApprove(ctx, ct)) > 0 {
			out.AwaitingYou = true
		}
	}
	return out, nil
}

// canApprove lists the namespaces of a pending version the caller may still
// approve.
func (r *registryService) canApprove(ctx context.Context, ct catalog.Contract) []string {
	if ct.Status != "pending" {
		return nil
	}
	var out []string
	for _, ns := range ct.Namespaces {
		if slices.Contains(ct.Approved, ns) {
			continue
		}
		_, n, err := r.serviceNode(ctx, ns)
		if err != nil {
			continue
		}
		if ok, err := r.s.may(ctx, catalog.PermContractsApprove, n); err == nil && ok {
			out = append(out, ns)
		}
	}
	return out
}

func (r *registryService) toContract(ctx context.Context, ct catalog.Contract) *adminv1.Contract {
	manifest, _ := json.MarshalIndent(ct.Manifest, "", "  ")
	summary, _ := json.Marshal(ct.Summary)
	return &adminv1.Contract{ServiceId: ct.Service, Version: int32(ct.Version), Status: ct.Status, ManifestJson: string(manifest),
		Namespaces: ct.Namespaces, Approved: ct.Approved, SummaryJson: string(summary), SubmittedBy: ct.SubmittedBy,
		SubmittedAt: ts(ct.SubmittedAt), DecidedBy: ct.DecidedBy, DecidedAt: tsp(ct.DecidedAt), Reason: ct.Reason,
		CanApprove: r.canApprove(ctx, ct)}
}

func (r *registryService) ListServices(ctx context.Context, _ *connect.Request[adminv1.ListServicesRequest]) (*connect.Response[adminv1.ListServicesResponse], error) {
	list, err := r.s.cat.Services(ctx)
	if err != nil {
		return nil, r.s.fail(err)
	}
	resp := &adminv1.ListServicesResponse{}
	for _, svc := range list {
		info, err := r.toService(ctx, svc)
		if err != nil {
			return nil, r.s.fail(err)
		}
		// Owners and admins see a service; anyone else sees it only when it
		// waits for their approval.
		mine := caller(ctx).User.IsAdmin() || info.AwaitingYou || slices.ContainsFunc(info.Owners, func(u *adminv1.User) bool { return u.Id == caller(ctx).User.ID.String() })
		if mine {
			resp.Services = append(resp.Services, info)
		}
	}
	return connect.NewResponse(resp), nil
}

func (r *registryService) GetService(ctx context.Context, req *connect.Request[adminv1.GetServiceRequest]) (*connect.Response[adminv1.GetServiceResponse], error) {
	svc, n, err := r.serviceNode(ctx, req.Msg.Id)
	if err != nil {
		return nil, err
	}
	manage, err := r.s.may(ctx, catalog.PermServiceManage, n)
	if err != nil {
		return nil, r.s.fail(err)
	}
	approve, err := r.s.may(ctx, catalog.PermContractsApprove, n)
	if err != nil {
		return nil, r.s.fail(err)
	}
	if !manage && !approve {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only this service's owners can open it"))
	}
	info, err := r.toService(ctx, svc)
	if err != nil {
		return nil, r.s.fail(err)
	}
	resp := &adminv1.GetServiceResponse{Service: info, CanManage: manage}
	contracts, err := r.s.cat.Contracts(ctx, svc.ID)
	if err != nil {
		return nil, r.s.fail(err)
	}
	for _, ct := range contracts {
		resp.Contracts = append(resp.Contracts, r.toContract(ctx, ct))
	}
	incoming, err := r.s.cat.PendingFor(ctx, svc.ID)
	if err != nil {
		return nil, r.s.fail(err)
	}
	for _, ct := range incoming {
		if ct.Service != svc.ID {
			resp.Incoming = append(resp.Incoming, r.toContract(ctx, ct))
		}
	}
	return connect.NewResponse(resp), nil
}

func (r *registryService) RegisterService(ctx context.Context, req *connect.Request[adminv1.RegisterServiceRequest]) (*connect.Response[adminv1.RegisterServiceResponse], error) {
	if err := r.s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	m := req.Msg
	if err := catalog.ValidServiceName(m.Id); err != nil {
		return nil, r.s.fail(err)
	}
	owner, _, err := r.s.ids.Invite(ctx, m.OwnerEmail, "", actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	name := strings.TrimSpace(m.DisplayName)
	if name == "" {
		name = m.Id
	}
	client, secret, err := r.s.clients.Create(ctx, hoidc.ClientInput{Name: name + " (service API)", Kind: hoidc.KindConfidential,
		Grants: []string{"client_credentials"}}, actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	svc, err := r.s.cat.RegisterService(ctx, m.Id, name, m.Description, client.ID, owner.ID, actor(ctx))
	if err != nil {
		_ = r.s.clients.Delete(ctx, client.ID)
		return nil, r.s.fail(err)
	}
	info, err := r.toService(ctx, svc)
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RegisterServiceResponse{Service: info, ClientId: client.ID, ClientSecret: secret}), nil
}

func (r *registryService) AddServiceOwner(ctx context.Context, req *connect.Request[adminv1.AddServiceOwnerRequest]) (*connect.Response[adminv1.AddServiceOwnerResponse], error) {
	svc, n, err := r.serviceNode(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	if err := r.s.require(ctx, catalog.PermServiceManage, n); err != nil {
		return nil, err
	}
	u, _, err := r.s.ids.Invite(ctx, req.Msg.Email, "", actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	if _, _, err := r.s.tree.AddGrant(ctx, tree.GrantInput{NodeID: n.ID, PrincipalType: tree.PrincipalUser, PrincipalID: u.ID,
		RoleID: catalog.RoleServiceOwner, Inherit: true}, actor(ctx)); err != nil && !errors.Is(err, tree.ErrGrantExists) {
		return nil, r.s.fail(err)
	}
	info, err := r.toService(ctx, svc)
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.AddServiceOwnerResponse{Service: info}), nil
}

func (r *registryService) RemoveServiceOwner(ctx context.Context, req *connect.Request[adminv1.RemoveServiceOwnerRequest]) (*connect.Response[adminv1.RemoveServiceOwnerResponse], error) {
	svc, n, err := r.serviceNode(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	if err := r.s.require(ctx, catalog.PermServiceManage, n); err != nil {
		return nil, err
	}
	userID, err := parseID(req.Msg.UserId, "user_id")
	if err != nil {
		return nil, err
	}
	owners, err := r.s.cat.ServiceOwners(ctx, svc.ID)
	if err != nil {
		return nil, r.s.fail(err)
	}
	if len(owners) <= 1 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("a service always keeps at least one owner; add another first"))
	}
	grants, err := r.s.tree.Grants(ctx, n.ID)
	if err != nil {
		return nil, r.s.fail(err)
	}
	for _, g := range grants {
		if g.PrincipalType == tree.PrincipalUser && g.PrincipalID == userID && g.RoleID == catalog.RoleServiceOwner {
			if _, err := r.s.tree.RemoveGrant(ctx, g.ID, actor(ctx)); err != nil {
				return nil, r.s.fail(err)
			}
		}
	}
	info, err := r.toService(ctx, svc)
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RemoveServiceOwnerResponse{Service: info}), nil
}

func (r *registryService) RotateServiceSecret(ctx context.Context, req *connect.Request[adminv1.RotateServiceSecretRequest]) (*connect.Response[adminv1.RotateServiceSecretResponse], error) {
	svc, n, err := r.serviceNode(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	if err := r.s.require(ctx, catalog.PermServiceManage, n); err != nil {
		return nil, err
	}
	secret, err := r.s.clients.RotateSecret(ctx, svc.ClientID, actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RotateServiceSecretResponse{ClientSecret: secret}), nil
}

func (r *registryService) PublishContract(ctx context.Context, req *connect.Request[adminv1.RegistryServicePublishContractRequest]) (*connect.Response[adminv1.RegistryServicePublishContractResponse], error) {
	_, n, err := r.serviceNode(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	if err := r.s.require(ctx, catalog.PermServiceManage, n); err != nil {
		return nil, err
	}
	ct, err := r.s.cat.Publish(ctx, req.Msg.ServiceId, []byte(req.Msg.Manifest), actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RegistryServicePublishContractResponse{Contract: r.toContract(ctx, ct)}), nil
}

func (r *registryService) ApproveContract(ctx context.Context, req *connect.Request[adminv1.ApproveContractRequest]) (*connect.Response[adminv1.ApproveContractResponse], error) {
	ct, err := r.s.cat.Contract(ctx, req.Msg.ServiceId, int(req.Msg.Version))
	if err != nil {
		return nil, r.s.fail(err)
	}
	namespaces := r.canApprove(ctx, ct)
	if len(namespaces) == 0 {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("nothing in this version waits for your approval"))
	}
	ct, err = r.s.cat.Approve(ctx, ct.Service, ct.Version, namespaces, caller(ctx).User.ID)
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.ApproveContractResponse{Contract: r.toContract(ctx, ct)}), nil
}

func (r *registryService) RejectContract(ctx context.Context, req *connect.Request[adminv1.RejectContractRequest]) (*connect.Response[adminv1.RejectContractResponse], error) {
	ct, err := r.s.cat.Contract(ctx, req.Msg.ServiceId, int(req.Msg.Version))
	if err != nil {
		return nil, r.s.fail(err)
	}
	if len(r.canApprove(ctx, ct)) == 0 {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only an owner of a namespace this version touches can reject it"))
	}
	ct, err = r.s.cat.Reject(ctx, ct.Service, ct.Version, req.Msg.Reason, actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RejectContractResponse{Contract: r.toContract(ctx, ct)}), nil
}

// ---------- TreeAdminService ----------

type treeAdminService struct{ s *Server }

func (t *treeAdminService) canSee(ctx context.Context, n tree.Node) error {
	_, err := t.s.visible(ctx, n)
	return err
}

func (t *treeAdminService) GetTenantTree(ctx context.Context, req *connect.Request[adminv1.GetTenantTreeRequest]) (*connect.Response[adminv1.GetTenantTreeResponse], error) {
	tenantID, err := parseID(req.Msg.TenantId, "tenant_id")
	if err != nil {
		return nil, err
	}
	tn, err := t.s.tenants.Get(ctx, tenantID)
	if err != nil {
		return nil, t.s.fail(err)
	}
	root, err := t.s.tree.ByRef(ctx, catalog.TypeTenant, tenantID.String())
	if err != nil {
		return nil, t.s.fail(err)
	}
	if err := t.canSee(ctx, root); err != nil {
		return nil, err
	}
	nodes, err := t.s.tree.TenantNodes(ctx, tenantID)
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.GetTenantTreeResponse{Tenant: toTenant(tn), Nodes: toNodes(nodes)}), nil
}

func (t *treeAdminService) GetNode(ctx context.Context, req *connect.Request[adminv1.GetNodeRequest]) (*connect.Response[adminv1.GetNodeResponse], error) {
	n, err := t.s.node(ctx, req.Msg.Id)
	if err != nil {
		return nil, err
	}
	if err := t.canSee(ctx, n); err != nil {
		return nil, err
	}
	ancestors, err := t.s.tree.Ancestors(ctx, n)
	if err != nil {
		return nil, t.s.fail(err)
	}
	grants, err := t.s.tree.Grants(ctx, n.ID)
	if err != nil {
		return nil, t.s.fail(err)
	}
	pg, err := t.s.toGrants(ctx, grants)
	if err != nil {
		return nil, t.s.fail(err)
	}
	resp := &adminv1.GetNodeResponse{Node: toNode(n), Ancestors: toNodes(ancestors), Grants: pg}
	if n.Type == catalog.TypeGroup {
		members, err := t.s.tree.GroupMembers(ctx, n.ID)
		if err != nil {
			return nil, t.s.fail(err)
		}
		ids := make([]uuid.UUID, len(members))
		for i, m := range members {
			ids[i] = m.UserID
		}
		users, err := t.s.ids.GetUsers(ctx, ids)
		if err != nil {
			return nil, t.s.fail(err)
		}
		joined := map[uuid.UUID]time.Time{}
		for _, m := range members {
			joined[m.UserID] = m.JoinedAt
		}
		for _, u := range users {
			resp.Members = append(resp.Members, &adminv1.Member{User: toUser(u), JoinedAt: ts(joined[u.ID])})
		}
	}
	r, err := t.s.engine.At(ctx, caller(ctx).User.ID, n, authz.Consistency{Fresh: true})
	if err != nil {
		return nil, t.s.fail(err)
	}
	for p := range r.Perms {
		resp.MyPermissions = append(resp.MyPermissions, p)
	}
	slices.Sort(resp.MyPermissions)
	snap, err := t.s.cat.Snapshot(ctx, false)
	if err != nil {
		return nil, t.s.fail(err)
	}
	for key, p := range snap.Permissions {
		if snap.CanContain(n.Type, p.Scope) {
			resp.Permissions = append(resp.Permissions, key)
		}
	}
	slices.Sort(resp.Permissions)
	return connect.NewResponse(resp), nil
}

func (t *treeAdminService) CreateGroup(ctx context.Context, req *connect.Request[adminv1.CreateGroupRequest]) (*connect.Response[adminv1.CreateGroupResponse], error) {
	parent, err := t.s.node(ctx, req.Msg.ParentId)
	if err != nil {
		return nil, err
	}
	if err := t.s.require(ctx, catalog.PermResourcesOrganise, parent); err != nil {
		return nil, err
	}
	n, _, err := t.s.tree.CreateGroup(ctx, parent.ID, req.Msg.Name, actor(ctx))
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.CreateGroupResponse{Node: toNode(n)}), nil
}

func (t *treeAdminService) RenameGroup(ctx context.Context, req *connect.Request[adminv1.RenameGroupRequest]) (*connect.Response[adminv1.RenameGroupResponse], error) {
	n, err := t.s.node(ctx, req.Msg.Id)
	if err != nil {
		return nil, err
	}
	if err := t.s.require(ctx, catalog.PermResourcesOrganise, n); err != nil {
		return nil, err
	}
	n, _, err = t.s.tree.Rename(ctx, n.ID, req.Msg.Name, actor(ctx))
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RenameGroupResponse{Node: toNode(n)}), nil
}

// MoveNode needs organise where the node leaves and where it lands. Moving a
// resource whose type grants access by placement re-decides who can open it,
// so that also needs the right to grant on the resource (or owning it).
func (t *treeAdminService) MoveNode(ctx context.Context, req *connect.Request[adminv1.MoveNodeRequest]) (*connect.Response[adminv1.MoveNodeResponse], error) {
	n, err := t.s.node(ctx, req.Msg.Id)
	if err != nil {
		return nil, err
	}
	dest, err := t.s.node(ctx, req.Msg.ParentId)
	if err != nil {
		return nil, err
	}
	if n.ParentID == nil {
		return nil, invalid("this node can't be moved")
	}
	from, err := t.s.tree.Get(ctx, *n.ParentID)
	if err != nil {
		return nil, t.s.fail(err)
	}
	for _, at := range []tree.Node{from, dest} {
		if err := t.s.require(ctx, catalog.PermResourcesOrganise, at); err != nil {
			return nil, err
		}
	}
	snap, err := t.s.cat.Snapshot(ctx, false)
	if err != nil {
		return nil, t.s.fail(err)
	}
	if snap.Types[n.Type].ViaPlacement {
		if err := t.s.require(ctx, catalog.PermResourcesGrant, n); err != nil {
			return nil, err
		}
	}
	moved, _, err := t.s.tree.Move(ctx, n.ID, dest.ID, actor(ctx))
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.MoveNodeResponse{Node: toNode(moved)}), nil
}

func (t *treeAdminService) DeleteGroup(ctx context.Context, req *connect.Request[adminv1.DeleteGroupRequest]) (*connect.Response[adminv1.DeleteGroupResponse], error) {
	n, err := t.s.node(ctx, req.Msg.Id)
	if err != nil {
		return nil, err
	}
	if err := t.s.require(ctx, catalog.PermResourcesOrganise, n); err != nil {
		return nil, err
	}
	if _, err := t.s.tree.DeleteGroup(ctx, n.ID, actor(ctx)); err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.DeleteGroupResponse{}), nil
}

// grantPermission is what granting on a node takes: on a service's node it is
// managing the service, everywhere else granting on resources.
func grantPermission(n tree.Node) string {
	if n.Type == catalog.TypeService {
		return catalog.PermServiceManage
	}
	return catalog.PermResourcesGrant
}

func (t *treeAdminService) AddGrant(ctx context.Context, req *connect.Request[adminv1.AddGrantRequest]) (*connect.Response[adminv1.AddGrantResponse], error) {
	m := req.Msg
	n, err := t.s.node(ctx, m.NodeId)
	if err != nil {
		return nil, err
	}
	if err := t.s.require(ctx, grantPermission(n), n); err != nil {
		return nil, err
	}
	principal, err := parseID(m.PrincipalId, "principal_id")
	if err != nil {
		return nil, err
	}
	role, err := parseID(m.RoleId, "role_id")
	if err != nil {
		return nil, err
	}
	in := tree.GrantInput{NodeID: n.ID, PrincipalType: m.PrincipalType, PrincipalID: principal, RoleID: role, Inherit: m.Inherit}
	if m.ExpiresAt != nil {
		exp := m.ExpiresAt.AsTime()
		in.ExpiresAt = &exp
	}
	g, _, err := t.s.tree.AddGrant(ctx, in, actor(ctx))
	if err != nil {
		return nil, t.s.fail(err)
	}
	out, err := t.s.toGrants(ctx, []tree.Grant{g})
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.AddGrantResponse{Grant: out[0]}), nil
}

func (t *treeAdminService) RemoveGrant(ctx context.Context, req *connect.Request[adminv1.RemoveGrantRequest]) (*connect.Response[adminv1.RemoveGrantResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	g, err := t.s.tree.GetGrant(ctx, id)
	if err != nil {
		return nil, t.s.fail(err)
	}
	n, err := t.s.tree.Get(ctx, g.NodeID)
	if err != nil {
		return nil, t.s.fail(err)
	}
	if err := t.s.require(ctx, grantPermission(n), n); err != nil {
		return nil, err
	}
	if n.Type == catalog.TypeService && g.RoleID == catalog.RoleServiceOwner {
		owners, err := t.s.cat.ServiceOwners(ctx, n.ExternalID)
		if err != nil {
			return nil, t.s.fail(err)
		}
		if len(owners) <= 1 {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("a service always keeps at least one owner"))
		}
	}
	if _, err := t.s.tree.RemoveGrant(ctx, id, actor(ctx)); err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RemoveGrantResponse{}), nil
}

func (t *treeAdminService) AddGroupMember(ctx context.Context, req *connect.Request[adminv1.AddGroupMemberRequest]) (*connect.Response[adminv1.AddGroupMemberResponse], error) {
	g, err := t.s.node(ctx, req.Msg.GroupId)
	if err != nil {
		return nil, err
	}
	if err := t.s.require(ctx, catalog.PermGroupsManage, g); err != nil {
		return nil, err
	}
	user, err := parseID(req.Msg.UserId, "user_id")
	if err != nil {
		return nil, err
	}
	if _, err := t.s.tree.AddGroupMember(ctx, g.ID, user, actor(ctx)); err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.AddGroupMemberResponse{}), nil
}

func (t *treeAdminService) RemoveGroupMember(ctx context.Context, req *connect.Request[adminv1.RemoveGroupMemberRequest]) (*connect.Response[adminv1.RemoveGroupMemberResponse], error) {
	g, err := t.s.node(ctx, req.Msg.GroupId)
	if err != nil {
		return nil, err
	}
	if err := t.s.require(ctx, catalog.PermGroupsManage, g); err != nil {
		return nil, err
	}
	user, err := parseID(req.Msg.UserId, "user_id")
	if err != nil {
		return nil, err
	}
	if _, err := t.s.tree.RemoveGroupMember(ctx, g.ID, user, actor(ctx)); err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RemoveGroupMemberResponse{}), nil
}

// ListNodeAccess is open to whoever may see the node: the grants, owners and
// rosters it adds up are each readable there already, node by node.
// Accounts that aren't active reach nothing, so they are left out.
func (t *treeAdminService) ListNodeAccess(ctx context.Context, req *connect.Request[adminv1.ListNodeAccessRequest]) (*connect.Response[adminv1.ListNodeAccessResponse], error) {
	n, err := t.s.node(ctx, req.Msg.NodeId)
	if err != nil {
		return nil, err
	}
	if err := t.canSee(ctx, n); err != nil {
		return nil, err
	}
	entries, err := t.s.tree.NodeAccess(ctx, n)
	if err != nil {
		return nil, t.s.fail(err)
	}
	ids := make([]uuid.UUID, 0, len(entries))
	for _, a := range entries {
		ids = append(ids, a.UserID)
	}
	users, err := t.s.ids.GetUsers(ctx, ids)
	if err != nil {
		return nil, t.s.fail(err)
	}
	byID := map[uuid.UUID]identity.User{}
	for _, u := range users {
		if u.Status == identity.StatusActive {
			byID[u.ID] = u
		}
	}
	resp := &adminv1.ListNodeAccessResponse{}
	for _, a := range entries {
		u, ok := byID[a.UserID]
		if !ok {
			continue
		}
		out := &adminv1.NodeAccess{User: toUser(u), Via: a.Via, Source: toNode(a.Source), RoleName: a.RoleName,
			Inherit: a.Inherit, GroupName: a.GroupName, Permission: a.Permission}
		if a.RoleID != uuid.Nil {
			out.RoleId = a.RoleID.String()
		}
		if a.GroupID != uuid.Nil {
			out.GroupId = a.GroupID.String()
		}
		if a.ExpiresAt != nil {
			out.ExpiresAt = ts(*a.ExpiresAt)
		}
		resp.Entries = append(resp.Entries, out)
	}
	return connect.NewResponse(resp), nil
}

func (t *treeAdminService) WhoHasAccess(ctx context.Context, req *connect.Request[adminv1.WhoHasAccessRequest]) (*connect.Response[adminv1.WhoHasAccessResponse], error) {
	n, err := t.s.node(ctx, req.Msg.NodeId)
	if err != nil {
		return nil, err
	}
	if err := t.s.require(ctx, grantPermission(n), n); err != nil {
		return nil, err
	}
	subjects, err := t.s.engine.LookupSubjects(ctx, req.Msg.Permission, n, authz.Consistency{Fresh: true})
	if err != nil {
		return nil, t.s.fail(err)
	}
	ids := make([]uuid.UUID, len(subjects))
	for i, sub := range subjects {
		ids[i] = sub.UserID
	}
	users, err := t.s.ids.GetUsers(ctx, ids)
	if err != nil {
		return nil, t.s.fail(err)
	}
	byID := map[uuid.UUID]identity.User{}
	for _, u := range users {
		byID[u.ID] = u
	}
	resp := &adminv1.WhoHasAccessResponse{}
	for _, sub := range subjects {
		if u, ok := byID[sub.UserID]; ok {
			resp.Holders = append(resp.Holders, &adminv1.AccessHolder{User: toUser(u), Reason: toExplanation(sub.Reason)})
		}
	}
	return connect.NewResponse(resp), nil
}

func (t *treeAdminService) ExplainAccess(ctx context.Context, req *connect.Request[adminv1.ExplainAccessRequest]) (*connect.Response[adminv1.ExplainAccessResponse], error) {
	n, err := t.s.node(ctx, req.Msg.NodeId)
	if err != nil {
		return nil, err
	}
	if err := t.s.require(ctx, grantPermission(n), n); err != nil {
		return nil, err
	}
	user, err := parseID(req.Msg.UserId, "user_id")
	if err != nil {
		return nil, err
	}
	r, err := t.s.engine.At(ctx, user, n, authz.Consistency{Fresh: true})
	if err != nil {
		return nil, t.s.fail(err)
	}
	resp := &adminv1.ExplainAccessResponse{}
	if why, ok := r.Perms[req.Msg.Permission]; ok {
		resp.Allowed, resp.Reason = true, toExplanation(why)
	}
	for p := range r.Perms {
		resp.Permissions = append(resp.Permissions, p)
	}
	slices.Sort(resp.Permissions)
	return connect.NewResponse(resp), nil
}
