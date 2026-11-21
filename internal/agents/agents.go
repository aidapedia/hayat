// Package agents keeps the members that aren't people: AI agents that hold
// roles and work in services. An agent is its own kind, never a row in users.
// It has no email, password, session or admin role; it lives in exactly one
// tenant; it always answers to a person there, its sponsor; and it never signs
// in. A service it works in asks for a short-lived token on its behalf.
//
// Agents are kept out of every people table: an agent's tenant roster is
// implied by its row, and its groups are agent_groups, so no member list or
// count has to remember to skip them.
package agents

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
)

const (
	StatusActive   = "active"
	StatusPaused   = "paused"
	StatusDisabled = "disabled"

	// ReasonSponsorLeft: the sponsor left the tenant; an owner names a new one.
	ReasonSponsorLeft = "sponsor_left"
)

var (
	ErrBadHandle     = errs.New("a handle is 2–32 lowercase letters, digits and dashes, starting with a letter")
	ErrHandleTaken   = errs.New("another agent in this tenant already has that handle")
	ErrNoName        = errs.New("give the agent a name")
	ErrSponsor       = errs.New("the sponsor must be a person on this tenant's roster")
	ErrSponsorGone   = errs.New("name a new sponsor before resuming this agent")
	ErrBadStatus     = errs.New("an agent is active, paused or disabled")
	ErrNotGroup      = errs.New("an agent can only join a group of its own tenant")
	ErrNotService    = errs.New("a service is named by its namespace, like \"tether\"")
	ErrNotAllowed    = errs.New("this agent doesn't work in that service")
	ErrInactive      = errs.New("this agent isn't active")
	ErrTenantMissing = errs.New("that tenant doesn't exist")
)

var handleRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)
var serviceRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,39}$`)

type Agent struct {
	ID           uuid.UUID
	Region       string
	TenantID     uuid.UUID
	Handle       string
	Name         string
	SponsorID    uuid.UUID
	Status       string
	StatusReason string
	Services     []string
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Actor is how an agent appears in the audit log, next to its sponsor.
func (a Agent) Actor() string { return "agent:" + a.ID.String() }

// WorksIn reports whether a service may ask for this agent's tokens.
func (a Agent) WorksIn(service string) bool { return slices.Contains(a.Services, service) }

const columns = `id, region, tenant_id, handle, name, sponsor_user_id, status, status_reason, services, created_by, created_at, updated_at`

func scan(row pgx.Row) (Agent, error) {
	var a Agent
	err := row.Scan(&a.ID, &a.Region, &a.TenantID, &a.Handle, &a.Name, &a.SponsorID, &a.Status, &a.StatusReason, &a.Services,
		&a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	return a, store.NotFound(err)
}

type Service struct{ store *store.Store }

func New(s *store.Store) *Service { return &Service{store: s} }

// Get finds an agent by id alone, through the global ref table.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Agent, error) {
	return get(ctx, s.store.Pool, id)
}

func get(ctx context.Context, db store.DB, id uuid.UUID) (Agent, error) {
	var region string
	if err := db.QueryRow(ctx, `SELECT region FROM agent_refs WHERE id = $1`, id).Scan(&region); err != nil {
		return Agent{}, store.NotFound(err)
	}
	return scan(db.QueryRow(ctx, `SELECT `+columns+` FROM agents WHERE region = $1 AND id = $2`, region, id))
}

// GetMany returns the agents found among ids, in no particular order.
func (s *Service) GetMany(ctx context.Context, ids []uuid.UUID) ([]Agent, error) {
	rows, err := s.store.Pool.Query(ctx, `SELECT `+columns+` FROM agents WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	return collect(rows)
}

// List is a tenant's agents, by handle.
func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]Agent, error) {
	rows, err := s.store.Pool.Query(ctx, `SELECT `+columns+` FROM agents WHERE tenant_id = $1 ORDER BY handle`, tenantID)
	if err != nil {
		return nil, err
	}
	return collect(rows)
}

func collect(rows pgx.Rows) ([]Agent, error) {
	defer rows.Close()
	out := []Agent{}
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type Input struct {
	TenantID  uuid.UUID
	Handle    string
	Name      string
	SponsorID uuid.UUID
	Services  []string
}

func cleanServices(in []string) ([]string, error) {
	out := []string{}
	for _, svc := range in {
		svc = strings.TrimSpace(svc)
		if svc == "" {
			continue
		}
		if !serviceRe.MatchString(svc) {
			return nil, ErrNotService
		}
		if !slices.Contains(out, svc) {
			out = append(out, svc)
		}
	}
	slices.Sort(out)
	return out, nil
}

// Create adds an agent to a tenant. It starts active, answering to its sponsor.
func (s *Service) Create(ctx context.Context, in Input, actor string) (Agent, error) {
	in.Handle = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(in.Handle), "@")))
	in.Name = strings.TrimSpace(in.Name)
	if !handleRe.MatchString(in.Handle) {
		return Agent{}, ErrBadHandle
	}
	if in.Name == "" {
		in.Name = in.Handle
	}
	services, err := cleanServices(in.Services)
	if err != nil {
		return Agent{}, err
	}
	var a Agent
	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, err := tenantRegion(ctx, tx, in.TenantID)
		if err != nil {
			return err
		}
		if err := requireSponsor(ctx, tx, region, in.TenantID, in.SponsorID); err != nil {
			return err
		}
		id := store.NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO agent_refs (id, tenant_id, handle_lower, region) VALUES ($1, $2, $3, $4)`,
			id, in.TenantID, in.Handle, region); err != nil {
			if store.IsUniqueViolation(err) {
				return ErrHandleTaken
			}
			return err
		}
		if a, err = scan(tx.QueryRow(ctx, `INSERT INTO agents (region, id, tenant_id, handle, name, sponsor_user_id, status, services, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $8) RETURNING `+columns,
			region, id, in.TenantID, in.Handle, in.Name, in.SponsorID, services, actor)); err != nil {
			return err
		}
		return changed(ctx, tx, a, actor, "agent.created", map[string]any{"handle": a.Handle, "sponsor": a.SponsorID})
	})
	return a, err
}

// Update renames an agent and sets the services it works in.
func (s *Service) Update(ctx context.Context, id uuid.UUID, name string, services []string, actor string) (Agent, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Agent{}, ErrNoName
	}
	clean, err := cleanServices(services)
	if err != nil {
		return Agent{}, err
	}
	return s.write(ctx, id, actor, "agent.updated", func(ctx context.Context, tx pgx.Tx, a Agent) (Agent, error) {
		return scan(tx.QueryRow(ctx, `UPDATE agents SET name = $3, services = $4, updated_at = now() WHERE region = $1 AND id = $2 RETURNING `+columns,
			a.Region, a.ID, name, clean))
	})
}

// SetStatus pauses, resumes or disables an agent. Resuming needs a sponsor
// who is still on the tenant's roster.
func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status, actor string) (Agent, error) {
	if status != StatusActive && status != StatusPaused && status != StatusDisabled {
		return Agent{}, ErrBadStatus
	}
	return s.write(ctx, id, actor, "agent."+status, func(ctx context.Context, tx pgx.Tx, a Agent) (Agent, error) {
		if status == StatusActive {
			if err := requireSponsor(ctx, tx, a.Region, a.TenantID, a.SponsorID); err != nil {
				return Agent{}, ErrSponsorGone
			}
		}
		return scan(tx.QueryRow(ctx, `UPDATE agents SET status = $3, status_reason = '', updated_at = now() WHERE region = $1 AND id = $2 RETURNING `+columns,
			a.Region, a.ID, status))
	})
}

// Reassign names a new sponsor. An agent paused because its sponsor left
// resumes with the new one.
func (s *Service) Reassign(ctx context.Context, id, sponsorID uuid.UUID, actor string) (Agent, error) {
	return s.write(ctx, id, actor, "agent.reassigned", func(ctx context.Context, tx pgx.Tx, a Agent) (Agent, error) {
		if err := requireSponsor(ctx, tx, a.Region, a.TenantID, sponsorID); err != nil {
			return Agent{}, err
		}
		status, reason := a.Status, a.StatusReason
		if a.Status == StatusPaused && a.StatusReason == ReasonSponsorLeft {
			status, reason = StatusActive, ""
		}
		return scan(tx.QueryRow(ctx, `UPDATE agents SET sponsor_user_id = $3, status = $4, status_reason = $5, updated_at = now()
			WHERE region = $1 AND id = $2 RETURNING `+columns, a.Region, a.ID, sponsorID, status, reason))
	})
}

// Delete removes an agent, its groups and every grant made to it.
func (s *Service) Delete(ctx context.Context, id uuid.UUID, actor string) error {
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		a, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		for _, q := range []string{
			`DELETE FROM agent_groups WHERE region = $1 AND agent_id = $2`,
			`DELETE FROM grants WHERE region = $1 AND principal_type = 'agent' AND principal_id = $2`,
			`DELETE FROM agents WHERE region = $1 AND id = $2`,
		} {
			if _, err := tx.Exec(ctx, q, a.Region, a.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM agent_refs WHERE id = $1`, a.ID); err != nil {
			return err
		}
		return changed(ctx, tx, a, actor, "agent.deleted", map[string]any{"handle": a.Handle})
	})
}

// ---------- Groups ----------

// Groups is the ids of the groups an agent is on.
func (s *Service) Groups(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.store.Pool.Query(ctx, `SELECT group_id FROM agent_groups WHERE agent_id = $1 ORDER BY created_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var g uuid.UUID
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GroupAgents is the agents on one group's roster.
func (s *Service) GroupAgents(ctx context.Context, groupID uuid.UUID) ([]Agent, error) {
	rows, err := s.store.Pool.Query(ctx, `SELECT `+prefixed+` FROM agent_groups m JOIN agents a ON a.region = m.region AND a.id = m.agent_id
		WHERE m.group_id = $1 ORDER BY a.handle`, groupID)
	if err != nil {
		return nil, err
	}
	return collect(rows)
}

const prefixed = `a.id, a.region, a.tenant_id, a.handle, a.name, a.sponsor_user_id, a.status, a.status_reason, a.services, a.created_by, a.created_at, a.updated_at`

// AddToGroup puts an agent on a group of its own tenant.
func (s *Service) AddToGroup(ctx context.Context, id, groupID uuid.UUID, actor string) error {
	_, err := s.write(ctx, id, actor, "agent.group_added", func(ctx context.Context, tx pgx.Tx, a Agent) (Agent, error) {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM nodes WHERE region = $1 AND id = $2 AND tenant_id = $3 AND type = 'hayat/group')`,
			a.Region, groupID, a.TenantID).Scan(&ok); err != nil {
			return Agent{}, err
		}
		if !ok {
			return Agent{}, ErrNotGroup
		}
		_, err := tx.Exec(ctx, `INSERT INTO agent_groups (region, group_id, tenant_id, agent_id) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			a.Region, groupID, a.TenantID, a.ID)
		return a, err
	})
	return err
}

func (s *Service) RemoveFromGroup(ctx context.Context, id, groupID uuid.UUID, actor string) error {
	_, err := s.write(ctx, id, actor, "agent.group_removed", func(ctx context.Context, tx pgx.Tx, a Agent) (Agent, error) {
		tag, err := tx.Exec(ctx, `DELETE FROM agent_groups WHERE region = $1 AND group_id = $2 AND agent_id = $3`, a.Region, groupID, a.ID)
		if err == nil && tag.RowsAffected() == 0 {
			err = store.ErrNotFound
		}
		return a, err
	})
	return err
}

// ---------- Acting ----------

// ForService returns an agent a service may act as right now: active, its
// sponsor active and still on the tenant's roster, and working in that service.
func (s *Service) ForService(ctx context.Context, id uuid.UUID, service string) (Agent, error) {
	a, err := s.Get(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	if !a.WorksIn(service) {
		return Agent{}, ErrNotAllowed
	}
	if err := s.Usable(ctx, a); err != nil {
		return Agent{}, err
	}
	return a, nil
}

// Usable reports whether an agent may act at all.
func (s *Service) Usable(ctx context.Context, a Agent) error {
	if a.Status != StatusActive {
		return ErrInactive
	}
	if err := requireSponsor(ctx, s.store.Pool, a.Region, a.TenantID, a.SponsorID); err != nil {
		return ErrInactive
	}
	return nil
}

// ---------- Helpers ----------

func (s *Service) write(ctx context.Context, id uuid.UUID, actor, action string, fn func(context.Context, pgx.Tx, Agent) (Agent, error)) (Agent, error) {
	var out Agent
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		a, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		if out, err = fn(ctx, tx, a); err != nil {
			return err
		}
		return changed(ctx, tx, out, actor, action, nil)
	})
	return out, err
}

// changed bumps the tenant's revision, so every cached decision about the
// agent is recomputed, and writes the audit line.
func changed(ctx context.Context, tx pgx.Tx, a Agent, actor, action string, detail map[string]any) error {
	if _, err := revision.Bump(ctx, tx, a.Region, a.TenantID, action); err != nil {
		return err
	}
	return audit.Write(ctx, tx, a.Region, actor, action, "agent:"+a.ID.String(), detail)
}

func tenantRegion(ctx context.Context, db store.DB, tenantID uuid.UUID) (string, error) {
	var region string
	err := db.QueryRow(ctx, `SELECT region FROM tenant_refs WHERE id = $1`, tenantID).Scan(&region)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrTenantMissing
	}
	return region, err
}

// requireSponsor: a sponsor is an active person on the tenant's roster.
func requireSponsor(ctx context.Context, db store.DB, region string, tenantID, userID uuid.UUID) error {
	var ok bool
	err := db.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM group_members m
			JOIN nodes n ON n.region = m.region AND n.id = m.group_id AND n.type = 'hayat/tenant'
			JOIN users u ON u.id = m.user_id AND u.status = 'active'
			WHERE m.region = $1 AND m.tenant_id = $2 AND m.user_id = $3)`, region, tenantID, userID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrSponsor
	}
	return nil
}
