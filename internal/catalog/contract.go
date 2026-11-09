package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
)

// A contract is how a service joins the catalog: the resource types it files
// in the tree, the permissions that mean something on them, and the roles it
// suggests. It is the per-service version of the code-defined platform catalog.
//
// Rules:
//   - Additive only. A new version may add types, parents, permissions, roles
//     and role permissions, never remove or change them.
//   - A version is pending until an owner of every namespace it touches
//     approves it. Its own namespace always; another service's when one of its
//     types is placed under that service's types.
//   - Names are written short in the manifest and stored qualified:
//     "ledger" → "billing/ledger", "ledger.view" → "billing.ledger.view".

type Manifest struct {
	Service       string         `yaml:"service" json:"service"`
	Version       int            `yaml:"version" json:"version"`
	Description   string         `yaml:"description,omitempty" json:"description,omitempty"`
	ResourceTypes []ManifestType `yaml:"resource_types" json:"resource_types"`
	Permissions   []ManifestPerm `yaml:"permissions" json:"permissions"`
	DefaultRoles  []ManifestRole `yaml:"default_roles,omitempty" json:"default_roles,omitempty"`
}

type ManifestType struct {
	Name        string          `yaml:"name" json:"name"`
	Description string          `yaml:"description,omitempty" json:"description,omitempty"`
	Parents     []string        `yaml:"parents" json:"parents"`
	Access      *ManifestAccess `yaml:"access,omitempty" json:"access,omitempty"`
	Degradable  bool            `yaml:"degradable,omitempty" json:"degradable,omitempty"`
}

type ManifestAccess struct {
	Permission         string `yaml:"permission" json:"permission"`
	ViaPlacement       bool   `yaml:"via_placement,omitempty" json:"via_placement,omitempty"`
	RequiresMembership bool   `yaml:"requires_membership,omitempty" json:"requires_membership,omitempty"`
}

type ManifestPerm struct {
	Key         string `yaml:"key" json:"key"`
	Scope       string `yaml:"scope" json:"scope"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type ManifestRole struct {
	Slug        string   `yaml:"slug" json:"slug"`
	Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Permissions []string `yaml:"permissions" json:"permissions"`
	AppliesTo   []string `yaml:"applies_to,omitempty" json:"applies_to,omitempty"`
}

// Contract is one stored version.
type Contract struct {
	Service     string
	Version     int
	Status      string
	Manifest    Manifest
	Namespaces  []string
	Approved    []string
	Summary     Summary
	SubmittedBy string
	SubmittedAt time.Time
	DecidedBy   string
	DecidedAt   *time.Time
	Reason      string
}

// Summary is what a reviewer reads: everything the version adds.
type Summary struct {
	AddedTypes       []string            `json:"added_types,omitempty"`
	AddedParents     map[string][]string `json:"added_parents,omitempty"`
	AddedPermissions []string            `json:"added_permissions,omitempty"`
	AddedRoles       []string            `json:"added_roles,omitempty"`
	AddedRolePerms   map[string][]string `json:"added_role_permissions,omitempty"`
}

var (
	nameRe    = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,47}$`)
	permRe    = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)*$`)
	serviceRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)

	ErrPendingExists = errs.New("a version is already waiting for approval; approve or reject it first")
	ErrNotPending    = errs.New("this version is not waiting for approval")
)

// ParseManifest reads YAML (or JSON, which is YAML too).
func ParseManifest(raw []byte) (Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, errs.Newf("the contract isn't valid YAML: %v", err)
	}
	return m, nil
}

// qualify turns a manifest-local name into its catalog key.
func qualifyType(svc, name string) string {
	if strings.Contains(name, "/") {
		return name
	}
	return svc + "/" + name
}

func qualifyPerm(svc, key string) string {
	if strings.HasPrefix(key, svc+".") || strings.HasPrefix(key, Platform+".") {
		return key
	}
	return svc + "." + key
}

// compiled is a manifest in catalog terms.
type compiled struct {
	types []NodeType
	perms []Permission
	roles []Role
}

// compile validates a manifest against the catalog as it stands (excluding the
// service's own active entries, which the manifest restates) and returns its
// entries and the namespaces it touches.
func compile(m Manifest, snap *Snapshot) (compiled, []string, error) {
	svc := m.Service
	if !serviceRe.MatchString(svc) || svc == Platform {
		return compiled{}, nil, errs.Newf("%q is not a valid service name", svc)
	}
	var out compiled
	namespaces := []string{svc}

	own := map[string]bool{}
	for _, t := range m.ResourceTypes {
		if !nameRe.MatchString(t.Name) {
			return compiled{}, nil, errs.Newf("resource type %q: use lowercase letters, digits, - and _", t.Name)
		}
		key := qualifyType(svc, t.Name)
		if own[key] {
			return compiled{}, nil, errs.Newf("resource type %q is listed twice", t.Name)
		}
		own[key] = true
	}
	known := func(typ string) bool { _, ok := snap.Types[typ]; return ok && ServiceOf(typ) != svc || own[typ] }

	perms := map[string]bool{}
	for _, p := range m.Permissions {
		if !permRe.MatchString(p.Key) {
			return compiled{}, nil, errs.Newf("permission %q: use lowercase words joined by dots", p.Key)
		}
		key := qualifyPerm(svc, p.Key)
		if ServiceOf(key) != svc {
			return compiled{}, nil, errs.Newf("permission %q is outside the %s namespace", p.Key, svc)
		}
		scope := qualifyType(svc, p.Scope)
		if p.Scope == "" || !known(scope) {
			return compiled{}, nil, errs.Newf("permission %q: scope %q is not a known node type", p.Key, p.Scope)
		}
		if perms[key] {
			return compiled{}, nil, errs.Newf("permission %q is listed twice", p.Key)
		}
		perms[key] = true
		out.perms = append(out.perms, Permission{Key: key, Service: svc, Scope: scope, Description: strings.TrimSpace(p.Description)})
	}

	for _, t := range m.ResourceTypes {
		nt := NodeType{Key: qualifyType(svc, t.Name), Service: svc, Description: strings.TrimSpace(t.Description), Degradable: t.Degradable}
		if len(t.Parents) == 0 {
			return compiled{}, nil, errs.Newf("resource type %q needs at least one parent type", t.Name)
		}
		for _, p := range t.Parents {
			parent := qualifyType(svc, p)
			if parent == TypeRoot || parent == TypeService {
				return compiled{}, nil, errs.Newf("resource type %q can't sit directly under %s", t.Name, parent)
			}
			if !known(parent) {
				return compiled{}, nil, errs.Newf("resource type %q: parent %q is not a known node type", t.Name, p)
			}
			if ns := ServiceOf(parent); ns != svc && ns != Platform && !slices.Contains(namespaces, ns) {
				namespaces = append(namespaces, ns)
			}
			nt.Parents = append(nt.Parents, parent)
		}
		if t.Access != nil {
			ap := qualifyPerm(svc, t.Access.Permission)
			if !perms[ap] {
				return compiled{}, nil, errs.Newf("resource type %q: access permission %q is not declared in this contract", t.Name, t.Access.Permission)
			}
			nt.AccessPermission, nt.ViaPlacement, nt.RequiresMembership = ap, t.Access.ViaPlacement, t.Access.RequiresMembership
		}
		out.types = append(out.types, nt)
	}

	slugs := map[string]bool{}
	for _, r := range m.DefaultRoles {
		if !nameRe.MatchString(r.Slug) {
			return compiled{}, nil, errs.Newf("role %q: use lowercase letters, digits, - and _", r.Slug)
		}
		slug := svc + "." + r.Slug
		if slugs[slug] {
			return compiled{}, nil, errs.Newf("role %q is listed twice", r.Slug)
		}
		slugs[slug] = true
		role := Role{Slug: slug, Name: strings.TrimSpace(r.Name), Description: strings.TrimSpace(r.Description), Service: svc}
		if role.Name == "" {
			role.Name = r.Slug
		}
		if len(r.Permissions) == 0 {
			return compiled{}, nil, errs.Newf("role %q grants nothing", r.Slug)
		}
		for _, p := range r.Permissions {
			key := qualifyPerm(svc, p)
			if !perms[key] {
				return compiled{}, nil, errs.Newf("role %q: permission %q is not declared in this contract", r.Slug, p)
			}
			role.Permissions = append(role.Permissions, key)
		}
		for _, t := range r.AppliesTo {
			typ := qualifyType(svc, t)
			if !known(typ) {
				return compiled{}, nil, errs.Newf("role %q: node type %q is not known", r.Slug, t)
			}
			role.AppliesTo = append(role.AppliesTo, typ)
		}
		out.roles = append(out.roles, role)
	}
	return out, namespaces, nil
}

// diff checks that next only adds to prev and summarises what it adds.
func diff(prev, next compiled) (Summary, error) {
	var s Summary
	prevTypes := map[string]NodeType{}
	for _, t := range prev.types {
		prevTypes[t.Key] = t
	}
	nextTypes := map[string]NodeType{}
	for _, t := range next.types {
		nextTypes[t.Key] = t
	}
	for key, old := range prevTypes {
		cur, ok := nextTypes[key]
		if !ok {
			return s, errs.Newf("resource type %s is missing; contracts can only add", key)
		}
		for _, p := range old.Parents {
			if !slices.Contains(cur.Parents, p) {
				return s, errs.Newf("resource type %s lost parent %s; contracts can only add", key, p)
			}
		}
		if old.AccessPermission != "" && (old.AccessPermission != cur.AccessPermission ||
			old.ViaPlacement != cur.ViaPlacement || old.RequiresMembership != cur.RequiresMembership) {
			return s, errs.Newf("resource type %s changed its access rule; once set it can't change", key)
		}
		for _, p := range cur.Parents {
			if !slices.Contains(old.Parents, p) {
				if s.AddedParents == nil {
					s.AddedParents = map[string][]string{}
				}
				s.AddedParents[key] = append(s.AddedParents[key], p)
			}
		}
	}
	for key := range nextTypes {
		if _, ok := prevTypes[key]; !ok {
			s.AddedTypes = append(s.AddedTypes, key)
		}
	}

	prevPerms := map[string]Permission{}
	for _, p := range prev.perms {
		prevPerms[p.Key] = p
	}
	nextPerms := map[string]Permission{}
	for _, p := range next.perms {
		nextPerms[p.Key] = p
	}
	for key, old := range prevPerms {
		cur, ok := nextPerms[key]
		if !ok {
			return s, errs.Newf("permission %s is missing; contracts can only add", key)
		}
		if cur.Scope != old.Scope {
			return s, errs.Newf("permission %s changed its scope; scopes can't change", key)
		}
	}
	for key := range nextPerms {
		if _, ok := prevPerms[key]; !ok {
			s.AddedPermissions = append(s.AddedPermissions, key)
		}
	}

	prevRoles := map[string]Role{}
	for _, r := range prev.roles {
		prevRoles[r.Slug] = r
	}
	nextRoles := map[string]Role{}
	for _, r := range next.roles {
		nextRoles[r.Slug] = r
	}
	for slug, old := range prevRoles {
		cur, ok := nextRoles[slug]
		if !ok {
			return s, errs.Newf("role %s is missing; contracts can only add", slug)
		}
		for _, p := range old.Permissions {
			if !slices.Contains(cur.Permissions, p) {
				return s, errs.Newf("role %s lost permission %s; contracts can only add", slug, p)
			}
		}
		for _, p := range cur.Permissions {
			if !slices.Contains(old.Permissions, p) {
				if s.AddedRolePerms == nil {
					s.AddedRolePerms = map[string][]string{}
				}
				s.AddedRolePerms[slug] = append(s.AddedRolePerms[slug], p)
			}
		}
	}
	for slug := range nextRoles {
		if _, ok := prevRoles[slug]; !ok {
			s.AddedRoles = append(s.AddedRoles, slug)
		}
	}
	slices.Sort(s.AddedTypes)
	slices.Sort(s.AddedPermissions)
	slices.Sort(s.AddedRoles)
	return s, nil
}

// activeCompiled is the service's active contract in catalog terms.
func (c *Service) activeCompiled(ctx context.Context, db store.DB, svc string, snap *Snapshot) (compiled, error) {
	var raw []byte
	err := db.QueryRow(ctx, `SELECT manifest FROM contract_versions WHERE service_id = $1 AND status = 'active'`, svc).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return compiled{}, nil
	}
	if err != nil {
		return compiled{}, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return compiled{}, err
	}
	out, _, err := compile(m, snap)
	return out, err
}

// Publish stores a new pending version of a service's contract.
func (c *Service) Publish(ctx context.Context, svc string, raw []byte, actor string) (Contract, error) {
	m, err := ParseManifest(raw)
	if err != nil {
		return Contract{}, err
	}
	if m.Service != svc {
		return Contract{}, errs.Newf("this contract is for %q, not %q", m.Service, svc)
	}
	snap, err := c.Snapshot(ctx, true)
	if err != nil {
		return Contract{}, err
	}
	next, namespaces, err := compile(m, snap)
	if err != nil {
		return Contract{}, err
	}
	err = c.store.Tx(ctx, func(tx pgx.Tx) error {
		var active, latest int
		var pending bool
		if err := tx.QueryRow(ctx, `SELECT active_version FROM services WHERE id = $1 FOR UPDATE`, svc).Scan(&active); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errs.Newf("service %q isn't registered", svc)
			}
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(version), 0), coalesce(bool_or(status = 'pending'), false)
			FROM contract_versions WHERE service_id = $1`, svc).Scan(&latest, &pending); err != nil {
			return err
		}
		if pending {
			return ErrPendingExists
		}
		if m.Version <= latest {
			return errs.Newf("version must be greater than %d", latest)
		}
		prev, err := c.activeCompiled(ctx, tx, svc, snap)
		if err != nil {
			return err
		}
		summary, err := diff(prev, next)
		if err != nil {
			return err
		}
		manifest, _ := json.Marshal(m)
		sum, _ := json.Marshal(summary)
		if _, err := tx.Exec(ctx, `INSERT INTO contract_versions (service_id, version, manifest, status, namespaces, summary, submitted_by)
			VALUES ($1, $2, $3, 'pending', $4, $5, $6)`, svc, m.Version, manifest, namespaces, sum, actor); err != nil {
			return err
		}
		return audit.Write(ctx, tx, c.store.Region, actor, "contract.published", "service:"+svc, map[string]any{"version": m.Version})
	})
	if err != nil {
		return Contract{}, err
	}
	return c.Contract(ctx, svc, m.Version)
}

// Approve records approver's sign-off for the namespaces the caller checked
// they may approve. When every namespace has signed off, the version goes live.
func (c *Service) Approve(ctx context.Context, svc string, version int, namespaces []string, approver uuid.UUID) (Contract, error) {
	err := c.store.Tx(ctx, func(tx pgx.Tx) error {
		var status string
		var needed []string
		if err := tx.QueryRow(ctx, `SELECT status, namespaces FROM contract_versions WHERE service_id = $1 AND version = $2 FOR UPDATE`,
			svc, version).Scan(&status, &needed); err != nil {
			return store.NotFound(err)
		}
		if status != "pending" {
			return ErrNotPending
		}
		for _, ns := range namespaces {
			if !slices.Contains(needed, ns) {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO contract_approvals (service_id, version, namespace, approved_by) VALUES ($1, $2, $3, $4)
				ON CONFLICT DO NOTHING`, svc, version, ns, approver); err != nil {
				return err
			}
		}
		if err := audit.Write(ctx, tx, c.store.Region, audit.UserActor(approver), "contract.approved", "service:"+svc,
			map[string]any{"version": version, "namespaces": namespaces}); err != nil {
			return err
		}
		var approved int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM contract_approvals WHERE service_id = $1 AND version = $2`, svc, version).Scan(&approved); err != nil {
			return err
		}
		if approved < len(needed) {
			return nil
		}
		return c.activate(ctx, tx, svc, version, approver)
	})
	if err != nil {
		return Contract{}, err
	}
	c.revs.Forget(revision.Global)
	return c.Contract(ctx, svc, version)
}

// activate writes a version's entries into the catalog. The catalog may have
// moved since it was published, so it is validated again.
func (c *Service) activate(ctx context.Context, tx pgx.Tx, svc string, version int, approver uuid.UUID) error {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT manifest FROM contract_versions WHERE service_id = $1 AND version = $2`, svc, version).Scan(&raw); err != nil {
		return err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	snap, err := load(ctx, tx, 0)
	if err != nil {
		return err
	}
	next, _, err := compile(m, snap)
	if err != nil {
		return err
	}
	prev, err := c.activeCompiled(ctx, tx, svc, snap)
	if err != nil {
		return err
	}
	if _, err := diff(prev, next); err != nil {
		return err
	}
	for _, t := range next.types {
		if _, err := tx.Exec(ctx, `INSERT INTO node_types (key, service_id, parents, access_permission, via_placement, requires_membership, degradable, description)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (key) DO UPDATE SET parents = EXCLUDED.parents, access_permission = EXCLUDED.access_permission,
				via_placement = EXCLUDED.via_placement, requires_membership = EXCLUDED.requires_membership,
				degradable = EXCLUDED.degradable, description = EXCLUDED.description`,
			t.Key, t.Service, t.Parents, t.AccessPermission, t.ViaPlacement, t.RequiresMembership, t.Degradable, t.Description); err != nil {
			return err
		}
	}
	for _, p := range next.perms {
		if _, err := tx.Exec(ctx, `INSERT INTO permissions (key, service_id, scope, description) VALUES ($1, $2, $3, $4)
			ON CONFLICT (key) DO UPDATE SET description = EXCLUDED.description`, p.Key, p.Service, p.Scope, p.Description); err != nil {
			return err
		}
	}
	for _, r := range next.roles {
		var id uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO roles (id, slug, name, description, service_id, applies_to) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description, applies_to = EXCLUDED.applies_to, updated_at = now()
			RETURNING id`, store.NewID(), r.Slug, r.Name, r.Description, r.Service, nonNil(r.AppliesTo)).Scan(&id)
		if err != nil {
			return err
		}
		for _, p := range r.Permissions {
			if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, p); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE contract_versions SET status = 'superseded' WHERE service_id = $1 AND status = 'active'`, svc); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE contract_versions SET status = 'active', decided_by = $3, decided_at = now()
		WHERE service_id = $1 AND version = $2`, svc, version, audit.UserActor(approver)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE services SET active_version = $2, updated_at = now() WHERE id = $1`, svc, version); err != nil {
		return err
	}
	if _, err := revision.Bump(ctx, tx, c.store.Region, revision.Global, "catalog.changed"); err != nil {
		return err
	}
	return audit.Write(ctx, tx, c.store.Region, audit.UserActor(approver), "contract.activated", "service:"+svc, map[string]any{"version": version})
}

func (c *Service) Reject(ctx context.Context, svc string, version int, reason, actor string) (Contract, error) {
	err := c.store.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE contract_versions SET status = 'rejected', decided_by = $3, decided_at = now(), reason = $4
			WHERE service_id = $1 AND version = $2 AND status = 'pending'`, svc, version, actor, strings.TrimSpace(reason))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotPending
		}
		return audit.Write(ctx, tx, c.store.Region, actor, "contract.rejected", "service:"+svc, map[string]any{"version": version, "reason": reason})
	})
	if err != nil {
		return Contract{}, err
	}
	return c.Contract(ctx, svc, version)
}

func (c *Service) Contract(ctx context.Context, svc string, version int) (Contract, error) {
	list, err := c.contracts(ctx, `WHERE v.service_id = $1 AND v.version = $2`, svc, version)
	if err != nil {
		return Contract{}, err
	}
	if len(list) == 0 {
		return Contract{}, store.ErrNotFound
	}
	return list[0], nil
}

// Contracts lists a service's versions, newest first.
func (c *Service) Contracts(ctx context.Context, svc string) ([]Contract, error) {
	return c.contracts(ctx, `WHERE v.service_id = $1`, svc)
}

// PendingFor lists pending versions that wait on a namespace.
func (c *Service) PendingFor(ctx context.Context, namespace string) ([]Contract, error) {
	return c.contracts(ctx, `WHERE v.status = 'pending' AND $1 = ANY(v.namespaces)`, namespace)
}

func (c *Service) contracts(ctx context.Context, where string, args ...any) ([]Contract, error) {
	rows, err := c.store.Pool.Query(ctx, `SELECT v.service_id, v.version, v.status, v.manifest, v.namespaces, v.summary,
		v.submitted_by, v.submitted_at, v.decided_by, v.decided_at, v.reason,
		coalesce((SELECT array_agg(a.namespace ORDER BY a.namespace) FROM contract_approvals a
			WHERE a.service_id = v.service_id AND a.version = v.version), '{}')
		FROM contract_versions v `+where+` ORDER BY v.version DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Contract
	for rows.Next() {
		var ct Contract
		var manifest, summary []byte
		if err := rows.Scan(&ct.Service, &ct.Version, &ct.Status, &manifest, &ct.Namespaces, &summary, &ct.SubmittedBy,
			&ct.SubmittedAt, &ct.DecidedBy, &ct.DecidedAt, &ct.Reason, &ct.Approved); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(manifest, &ct.Manifest)
		_ = json.Unmarshal(summary, &ct.Summary)
		out = append(out, ct)
	}
	return out, rows.Err()
}
