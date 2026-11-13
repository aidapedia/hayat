package catalog

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Platform is Hayat's own namespace. Its node types and permissions are code:
// a permission exists because Hayat enforces it.
const Platform = "hayat"

const (
	TypeRoot    = "hayat/root"
	TypeTenant  = "hayat/tenant"
	TypeGroup   = "hayat/group"
	TypeService = "hayat/service"
)

const (
	PermTenantManage      = "hayat.tenant.manage"
	PermMembersManage     = "hayat.members.manage"
	PermGroupsManage      = "hayat.groups.manage"
	PermResourcesOrganise = "hayat.resources.organise"
	PermResourcesGrant    = "hayat.resources.grant"
	PermContractsApprove  = "hayat.contracts.approve"
	PermServiceManage     = "hayat.service.manage"
)

var platformTypes = []NodeType{
	{Key: TypeRoot, Service: Platform, Description: "The instance root. Everything hangs off it."},
	{Key: TypeTenant, Service: Platform, Parents: []string{TypeRoot}, Description: "An organisation every service shares."},
	{Key: TypeGroup, Service: Platform, Parents: []string{TypeTenant, TypeGroup},
		Description: "A folder resources are filed under and a set of people a grant can name, at once."},
	{Key: TypeService, Service: Platform, Parents: []string{TypeRoot},
		Description: "A service's namespace. Owner grants here decide who approves its contracts."},
}

var platformPermissions = []Permission{
	{Key: PermTenantManage, Service: Platform, Scope: TypeTenant, Label: "Manage tenant settings", Description: "Rename the tenant and change its settings."},
	{Key: PermMembersManage, Service: Platform, Scope: TypeTenant, Label: "Manage members", Description: "Add people to the tenant and remove them."},
	{Key: PermGroupsManage, Service: Platform, Scope: TypeGroup, Label: "Manage group membership", Description: "Change who belongs to a group, which decides who every grant to that group reaches."},
	{Key: PermResourcesOrganise, Service: Platform, Scope: TypeGroup, Label: "Organise resources", Description: "Create, rename, move and delete groups, and move resources between them."},
	{Key: PermResourcesGrant, Service: Platform, Scope: TypeGroup, Label: "Grant access", Description: "Give a person or group a role on a node, and take it away."},
	{Key: PermContractsApprove, Service: Platform, Scope: TypeService, Label: "Approve contracts", Description: "Approve contract versions that touch this service's namespace."},
	{Key: PermServiceManage, Service: Platform, Scope: TypeService, Label: "Manage the service", Description: "Name the service's owners and rotate its API secret."},
}

// Built-in roles, with fixed ids so grants made from them survive anything.
// They are ordinary rows: an admin may change their permissions, but they
// can't be deleted.
var (
	RoleTenantAdmin  = uuid.MustParse("00000000-0000-7000-8000-000000000101")
	RoleGroupManager = uuid.MustParse("00000000-0000-7000-8000-000000000102")
	RoleServiceOwner = uuid.MustParse("00000000-0000-7000-8000-000000000103")
)

var platformRoles = []Role{
	{ID: RoleTenantAdmin, Slug: "hayat.tenant-admin", Name: "Tenant admin", Service: Platform,
		Description: "Runs a tenant: its members, groups, and who may do what in it.",
		AppliesTo:   []string{TypeRoot, TypeTenant},
		Permissions: []string{PermTenantManage, PermMembersManage, PermGroupsManage, PermResourcesOrganise, PermResourcesGrant}},
	{ID: RoleGroupManager, Slug: "hayat.group-manager", Name: "Group manager", Service: Platform,
		Description: "Runs one group: its members, what is filed in it, and grants inside it.",
		AppliesTo:   []string{TypeGroup},
		Permissions: []string{PermGroupsManage, PermResourcesOrganise, PermResourcesGrant}},
	{ID: RoleServiceOwner, Slug: "hayat.service-owner", Name: "Service owner", Service: Platform,
		Description: "Owns a service's namespace: approves its contracts and names its other owners.",
		AppliesTo:   []string{TypeService},
		Permissions: []string{PermContractsApprove, PermServiceManage}},
}

// EnsurePlatform writes Hayat's own types, permissions and built-in roles. It
// runs at boot; roles are inserted only when missing, so an admin's edits stay.
func EnsurePlatform(ctx context.Context, tx pgx.Tx) error {
	for _, t := range platformTypes {
		if _, err := tx.Exec(ctx, `INSERT INTO node_types (key, service_id, parents, description) VALUES ($1, $2, $3, $4)
			ON CONFLICT (key) DO UPDATE SET parents = EXCLUDED.parents, description = EXCLUDED.description`,
			t.Key, t.Service, nonNil(t.Parents), t.Description); err != nil {
			return err
		}
	}
	for _, p := range platformPermissions {
		if _, err := tx.Exec(ctx, `INSERT INTO permissions (key, service_id, scope, label, description) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (key) DO UPDATE SET scope = EXCLUDED.scope, label = EXCLUDED.label, description = EXCLUDED.description`,
			p.Key, p.Service, p.Scope, p.Label, p.Description); err != nil {
			return err
		}
	}
	for _, r := range platformRoles {
		tag, err := tx.Exec(ctx, `INSERT INTO roles (id, slug, name, description, service_id, applies_to) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO NOTHING`, r.ID, r.Slug, r.Name, r.Description, r.Service, r.AppliesTo)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		for _, p := range r.Permissions {
			if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`, r.ID, p); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsBuiltinRole reports whether a role is one of Hayat's own.
func IsBuiltinRole(id uuid.UUID) bool {
	return id == RoleTenantAdmin || id == RoleGroupManager || id == RoleServiceOwner
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
