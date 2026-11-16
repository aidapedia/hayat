package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/identity"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tenancy"
	"github.com/aidapedia/hayat/internal/tree"
)

// A Bundle moves another system's people and their access into Hayat, so that
// system can stop keeping its own: tabletsgo writes one before it drops its
// account and permission tables. `hayat import --file bundle.json` loads it.
//
// The bundle is already in Hayat's terms — the exporting system maps its own
// roles onto role slugs and its folders onto groups — so importing is only
// writing. Ids of people and tenants are kept, because the other system's
// rows point at them; everything else (groups, grants) gets new ids.
//
// Re-running is safe: an address Hayat already knows is matched rather than
// created (the result's IDMap says which ids changed), and a tenant that
// already exists is skipped.
type Bundle struct {
	Format  string        `json:"format"`
	Version int           `json:"version"`
	Source  string        `json:"source"`
	Service BundleService `json:"service"`
	Users   []BundleUser  `json:"users"`
	// Roles the exporting system defined itself, created as admin-owned roles.
	Roles []BundleRole `json:"roles"`
	// Instance-wide grants, made at the root.
	RootGrants []BundleRootGrant `json:"rootGrants"`
	Tenants    []BundleTenant    `json:"tenants"`
}

const BundleFormat = "hayat-import"

type BundleService struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	// The service's contract manifest (YAML or JSON). Applied and approved by
	// the import, so the resources and grants below have types to land on.
	Contract string `json:"contract"`
}

type BundleUser struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"passwordHash"`
	Status       string `json:"status"`
	Zoneinfo     string `json:"zoneinfo"`
	// Administered the other system. Becomes a Hayat instance admin only when
	// Hayat has none yet (a Hayat deployed for this system); either way the
	// bundle's root grants carry what they may do in the system itself.
	Admin bool `json:"admin"`
	// The theme they chose in the other system (a solo tabletsgo's owner), in
	// the shape of identity.Appearance. Only an account the import creates
	// takes it; one Hayat already knew keeps its own.
	Appearance *identity.Appearance `json:"appearance,omitempty"`
}

type BundleRole struct {
	// What grants in this bundle call it, e.g. "custom:analyst".
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	AppliesTo   []string `json:"appliesTo"`
}

type BundleRootGrant struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

type BundleTenant struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	OwnerID string   `json:"ownerId"`
	Members []string `json:"members"`
	// Parents before children.
	Groups    []BundleGroup    `json:"groups"`
	Resources []BundleResource `json:"resources"`
	Grants    []BundleGrant    `json:"grants"`
}

type BundleGroup struct {
	Key string `json:"key"`
	// "" files it under the tenant; otherwise another group's key.
	Parent  string   `json:"parent"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type BundleResource struct {
	Type       string `json:"type"`
	ExternalID string `json:"externalId"`
	Name       string `json:"name"`
	Parent     string `json:"parent"`
	OwnerID    string `json:"ownerId"`
}

type BundleGrant struct {
	// "tenant" | "group:<key>" | "resource:<type>:<externalId>"
	On string `json:"on"`
	// "user:<id>" | "group:<key>" | "tenant" (everyone in it)
	Principal string `json:"principal"`
	// A role slug (hayat.tenant-admin, tabletsgo.owner) or a bundle role key.
	Role    string `json:"role"`
	Inherit bool   `json:"inherit"`
}

type ImportOptions struct {
	// The application's public URL. When set, a public sign-in client is
	// registered for it with <AppURL>/callback as its redirect.
	AppURL string
}

type ImportResult struct {
	UsersCreated   int `json:"usersCreated"`
	UsersMatched   int `json:"usersMatched"`
	TenantsCreated int `json:"tenantsCreated"`
	TenantsSkipped int `json:"tenantsSkipped"`
	Groups         int `json:"groups"`
	Resources      int `json:"resources"`
	Grants         int `json:"grants"`
	// Bundle user id → the Hayat id it was matched to, for every address Hayat
	// already had under another id. The exporting system rewrites its
	// references with it.
	IDMap    map[string]string `json:"idMap"`
	Warnings []string          `json:"warnings"`

	ServiceRegistered bool   `json:"serviceRegistered"`
	ClientID          string `json:"clientId,omitempty"`
	ClientSecret      string `json:"clientSecret,omitempty"`
	ContractVersion   int    `json:"contractVersion"`
	WebClientID       string `json:"webClientId,omitempty"`
}

const importActor = "import"

// ParseBundle reads and checks a bundle's envelope.
func ParseBundle(raw []byte) (Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return Bundle{}, fmt.Errorf("the bundle isn't valid JSON: %w", err)
	}
	if b.Format != BundleFormat || b.Version != 1 {
		return Bundle{}, fmt.Errorf("not a version 1 %s bundle (format %q, version %d)", BundleFormat, b.Format, b.Version)
	}
	if err := catalog.ValidServiceName(b.Service.ID); err != nil {
		return Bundle{}, err
	}
	if strings.TrimSpace(b.Service.Contract) == "" {
		return Bundle{}, errors.New("the bundle carries no contract for its service")
	}
	return b, nil
}

// Import loads a bundle. It stops at the first thing it can't make sense of in
// people, the service or its contract; a grant or resource that can't be made
// is skipped with a warning, so one odd row never strands the rest.
func (a *App) Import(ctx context.Context, b Bundle, opts ImportOptions) (ImportResult, error) {
	res := ImportResult{IDMap: map[string]string{}}
	if err := a.Bootstrap(ctx); err != nil {
		return res, err
	}
	hadAdmin, err := a.IDs.HasAdmin(ctx)
	if err != nil {
		return res, err
	}

	// 1. People. Every id in the rest of the bundle goes through `user`.
	ids := map[string]uuid.UUID{}
	var admins []uuid.UUID
	for _, bu := range b.Users {
		id, err := uuid.Parse(bu.ID)
		if err != nil {
			return res, fmt.Errorf("user %s: id %q isn't a uuid", bu.Email, bu.ID)
		}
		u, err := a.IDs.GetUserByEmail(ctx, bu.Email)
		switch {
		case err == nil:
			res.UsersMatched++
			if u.ID != id {
				res.IDMap[bu.ID] = u.ID.String()
			}
		case errors.Is(err, store.ErrNotFound):
			if _, err := a.IDs.GetUser(ctx, id); err == nil {
				return res, fmt.Errorf("user %s: id %s already belongs to another account", bu.Email, id)
			}
			if u, err = a.IDs.Import(ctx, identity.ImportedUser{ID: id, Email: bu.Email, Name: bu.Name, PasswordHash: bu.PasswordHash,
				Status: bu.Status, Zoneinfo: bu.Zoneinfo, Admin: bu.Admin && !hadAdmin, Appearance: bu.Appearance}, importActor); err != nil {
				return res, fmt.Errorf("user %s: %w", bu.Email, err)
			}
			res.UsersCreated++
		default:
			return res, err
		}
		ids[bu.ID] = u.ID
		if bu.Admin {
			admins = append(admins, u.ID)
		}
	}
	user := func(raw string) (uuid.UUID, bool) {
		id, ok := ids[raw]
		return id, ok
	}

	// 2. The service and its contract.
	svc, err := a.ensureService(ctx, b.Service, admins, &res)
	if err != nil {
		return res, err
	}
	if opts.AppURL != "" {
		base := strings.TrimRight(opts.AppURL, "/")
		c, _, err := a.Clients.Create(ctx, hoidc.ClientInput{ServiceID: svc.ID, Name: svc.DisplayName + " (web)", Kind: hoidc.KindPublic,
			Redirects: []string{base + "/callback"}, PostLogoutRedirects: []string{base, base + "/"},
			Grants: []string{"authorization_code", "refresh_token"}}, importActor)
		if err != nil {
			return res, fmt.Errorf("web client: %w", err)
		}
		res.WebClientID = c.ID
	}

	// 3. Roles: the catalog's by slug, then the bundle's own.
	snap, err := a.Catalog.Snapshot(ctx, true)
	if err != nil {
		return res, err
	}
	roles := map[string]uuid.UUID{}
	for id, r := range snap.Roles {
		roles[r.Slug] = id
	}
	for _, br := range b.Roles {
		r, err := a.Catalog.CreateRole(ctx, catalog.RoleInput{Name: br.Name, Description: br.Description,
			Permissions: br.Permissions, AppliesTo: br.AppliesTo}, importActor)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("role %q not created: %v", br.Name, err))
			continue
		}
		roles[br.Key] = r.ID
	}

	// 4. Instance-wide grants.
	for _, g := range b.RootGrants {
		uid, ok := user(g.UserID)
		role, rok := roles[g.Role]
		if !ok || !rok {
			res.Warnings = append(res.Warnings, fmt.Sprintf("root grant %s to %s skipped: unknown role or person", g.Role, g.UserID))
			continue
		}
		if _, _, err := a.Tree.AddGrant(ctx, tree.GrantInput{NodeID: catalog.RootID, PrincipalType: tree.PrincipalUser, PrincipalID: uid,
			RoleID: role, Inherit: true}, importActor); err != nil && !errors.Is(err, tree.ErrGrantExists) {
			res.Warnings = append(res.Warnings, fmt.Sprintf("root grant %s skipped: %v", g.Role, err))
			continue
		}
		res.Grants++
	}

	// 5. Tenants, each with its groups, resources and grants.
	for _, bt := range b.Tenants {
		if err := a.importTenant(ctx, svc.ID, bt, user, roles, &res); err != nil {
			return res, fmt.Errorf("tenant %q: %w", bt.Name, err)
		}
	}
	return res, nil
}

// ensureService registers the service if Hayat doesn't know it yet, then makes
// the bundle's contract version active, approving it as the service's owner.
func (a *App) ensureService(ctx context.Context, bs BundleService, admins []uuid.UUID, res *ImportResult) (catalog.ServiceInfo, error) {
	m, err := catalog.ParseManifest([]byte(bs.Contract))
	if err != nil {
		return catalog.ServiceInfo{}, err
	}
	if m.Service != bs.ID {
		return catalog.ServiceInfo{}, fmt.Errorf("the contract is for %q, not %q", m.Service, bs.ID)
	}
	svc, err := a.Catalog.GetService(ctx, bs.ID)
	if errors.Is(err, store.ErrNotFound) {
		owner, err := a.importOwner(ctx, admins)
		if err != nil {
			return svc, err
		}
		name := bs.DisplayName
		if name == "" {
			name = bs.ID
		}
		client, secret, err := a.Clients.Create(ctx, hoidc.ClientInput{ServiceID: bs.ID, Name: name + " (service API)", Kind: hoidc.KindConfidential,
			Grants: []string{"client_credentials"}}, importActor)
		if err != nil {
			return svc, err
		}
		if svc, err = a.Catalog.RegisterService(ctx, bs.ID, name, bs.Description, client.ID, owner, importActor); err != nil {
			_ = a.Clients.Delete(ctx, client.ID)
			return svc, err
		}
		res.ServiceRegistered, res.ClientID, res.ClientSecret = true, client.ID, secret
	} else if err != nil {
		return svc, err
	}

	if svc.ActiveVersion < m.Version {
		approver, err := a.importOwner(ctx, admins)
		if err != nil {
			return svc, err
		}
		ct, err := a.Catalog.Contract(ctx, bs.ID, m.Version)
		if errors.Is(err, store.ErrNotFound) {
			ct, err = a.Catalog.Publish(ctx, bs.ID, []byte(bs.Contract), importActor)
		}
		if err != nil {
			return svc, fmt.Errorf("contract: %w", err)
		}
		if ct.Status == "pending" {
			if ct, err = a.Catalog.Approve(ctx, bs.ID, m.Version, ct.Namespaces, approver); err != nil {
				return svc, fmt.Errorf("contract approval: %w", err)
			}
		}
		if ct.Status != "active" {
			return svc, fmt.Errorf("contract version %d is %s, not active", m.Version, ct.Status)
		}
		if svc, err = a.Catalog.GetService(ctx, bs.ID); err != nil {
			return svc, err
		}
	}
	res.ContractVersion = svc.ActiveVersion
	return svc, nil
}

// importOwner is who the import acts as where Hayat wants a person: the first
// admin in the bundle, or else any Hayat instance admin.
func (a *App) importOwner(ctx context.Context, admins []uuid.UUID) (uuid.UUID, error) {
	if len(admins) > 0 {
		return admins[0], nil
	}
	var id uuid.UUID
	err := a.Store.Pool.QueryRow(ctx, `SELECT id FROM users WHERE system_role = 'admin' ORDER BY created_at LIMIT 1`).Scan(&id)
	if err != nil {
		return uuid.Nil, errors.New("nobody to own the service: the bundle has no admin and Hayat has no instance admin yet")
	}
	return id, nil
}

func (a *App) importTenant(ctx context.Context, svcID string, bt BundleTenant, user func(string) (uuid.UUID, bool),
	roles map[string]uuid.UUID, res *ImportResult) error {
	tid, err := uuid.Parse(bt.ID)
	if err != nil {
		return fmt.Errorf("id %q isn't a uuid", bt.ID)
	}
	if _, err := a.Tenants.Get(ctx, tid); err == nil {
		res.TenantsSkipped++
		res.Warnings = append(res.Warnings, fmt.Sprintf("tenant %q already exists; left as it is", bt.Name))
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	owner, ok := user(bt.OwnerID)
	if !ok {
		return fmt.Errorf("owner %s isn't among the bundle's users", bt.OwnerID)
	}
	if _, err := a.Tenants.Import(ctx, tid, bt.Name, owner, importActor); err != nil {
		return err
	}
	res.TenantsCreated++
	for _, m := range bt.Members {
		if uid, ok := user(m); ok {
			if err := a.Tenants.AddMember(ctx, tid, uid, importActor); err != nil {
				return err
			}
		}
	}
	tenantNode, err := a.Tree.ByRef(ctx, tenancy.TypeTenant, tid.String())
	if err != nil {
		return err
	}

	groups := map[string]uuid.UUID{"": tenantNode.ID}
	for _, bg := range bt.Groups {
		parent, ok := groups[bg.Parent]
		if !ok {
			res.Warnings = append(res.Warnings, fmt.Sprintf("group %q skipped: its parent %q comes later or doesn't exist", bg.Name, bg.Parent))
			continue
		}
		g, _, err := a.Tree.CreateGroup(ctx, parent, bg.Name, importActor)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("group %q skipped: %v", bg.Name, err))
			continue
		}
		groups[bg.Key] = g.ID
		res.Groups++
		for _, m := range bg.Members {
			uid, ok := user(m)
			if !ok {
				continue
			}
			if _, err := a.Tree.AddGroupMember(ctx, g.ID, uid, importActor); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("group %q: member %s skipped: %v", bg.Name, m, err))
			}
		}
	}

	for _, r := range bt.Resources {
		parent, ok := groups[r.Parent]
		if !ok {
			parent = tenantNode.ID
		}
		in := tree.ResourceInput{Type: r.Type, ExternalID: r.ExternalID, Name: r.Name, ParentID: parent}
		if uid, ok := user(r.OwnerID); ok {
			in.OwnerID = &uid
		}
		if _, _, err := a.Tree.UpsertResource(ctx, svcID, in); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s %q skipped: %v", r.Type, r.Name, err))
			continue
		}
		res.Resources++
	}

	for _, g := range bt.Grants {
		if err := a.importGrant(ctx, g, tenantNode.ID, groups, user, roles); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("tenant %q: grant %s on %s to %s skipped: %v", bt.Name, g.Role, g.On, g.Principal, err))
			continue
		}
		res.Grants++
	}
	return nil
}

func (a *App) importGrant(ctx context.Context, g BundleGrant, tenantNode uuid.UUID, groups map[string]uuid.UUID,
	user func(string) (uuid.UUID, bool), roles map[string]uuid.UUID) error {
	role, ok := roles[g.Role]
	if !ok {
		return errors.New("unknown role")
	}
	in := tree.GrantInput{RoleID: role, Inherit: g.Inherit}
	switch {
	case g.On == "tenant":
		in.NodeID = tenantNode
	case strings.HasPrefix(g.On, "group:"):
		if in.NodeID, ok = groups[strings.TrimPrefix(g.On, "group:")]; !ok {
			return errors.New("unknown group")
		}
	case strings.HasPrefix(g.On, "resource:"):
		rest := strings.TrimPrefix(g.On, "resource:")
		i := strings.LastIndex(rest, ":")
		if i <= 0 {
			return errors.New("write a resource as resource:<type>:<id>")
		}
		n, err := a.Tree.ByRef(ctx, rest[:i], rest[i+1:])
		if err != nil {
			return errors.New("unknown resource")
		}
		in.NodeID = n.ID
	default:
		return errors.New("unknown node")
	}
	switch {
	case g.Principal == "tenant":
		in.PrincipalType, in.PrincipalID = tree.PrincipalGroup, tenantNode
	case strings.HasPrefix(g.Principal, "group:"):
		in.PrincipalType = tree.PrincipalGroup
		if in.PrincipalID, ok = groups[strings.TrimPrefix(g.Principal, "group:")]; !ok {
			return errors.New("unknown group")
		}
	case strings.HasPrefix(g.Principal, "user:"):
		in.PrincipalType = tree.PrincipalUser
		if in.PrincipalID, ok = user(strings.TrimPrefix(g.Principal, "user:")); !ok {
			return errors.New("unknown person")
		}
	default:
		return errors.New("unknown principal")
	}
	_, _, err := a.Tree.AddGrant(ctx, in, importActor)
	if errors.Is(err, tree.ErrGrantExists) {
		return nil
	}
	return err
}
