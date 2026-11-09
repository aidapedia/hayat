package catalog

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
)

// RootID is the application root.
var RootID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

var ErrServiceTaken = errs.New("a service with this name is already registered")

// ServiceInfo is a registered service: a catalog namespace, its tree node, and
// the client it calls Hayat's API with.
type ServiceInfo struct {
	ID            string
	DisplayName   string
	Description   string
	NodeID        uuid.UUID
	ClientID      string
	ActiveVersion int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const serviceColumns = `id, display_name, description, node_id, client_id, active_version, created_at, updated_at`

func scanService(row pgx.Row) (ServiceInfo, error) {
	var s ServiceInfo
	err := row.Scan(&s.ID, &s.DisplayName, &s.Description, &s.NodeID, &s.ClientID, &s.ActiveVersion, &s.CreatedAt, &s.UpdatedAt)
	return s, store.NotFound(err)
}

// ValidServiceName checks a namespace before anything is created for it.
func ValidServiceName(id string) error {
	if !serviceRe.MatchString(id) || id == Platform {
		return errs.New("a service name is 2–31 lowercase letters, digits and dashes, starting with a letter")
	}
	return nil
}

// RegisterService creates the namespace, its hayat/service node, and an
// owner grant for ownerID. The node mirrors the service row, so they are
// created together, here.
func (c *Service) RegisterService(ctx context.Context, id, displayName, description, clientID string, ownerID uuid.UUID, actor string) (ServiceInfo, error) {
	if err := ValidServiceName(id); err != nil {
		return ServiceInfo{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = id
	}
	nodeID := store.NewID()
	err := c.store.Tx(ctx, func(tx pgx.Tx) error {
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM services WHERE id = $1)`, id).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return ErrServiceTaken
		}
		region := c.store.Region
		if _, err := tx.Exec(ctx, `INSERT INTO services (id, display_name, description, node_id, client_id) VALUES ($1, $2, $3, $4, $5)`,
			id, displayName, strings.TrimSpace(description), nodeID, clientID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO nodes (region, id, parent_id, type, external_id, name, path, depth)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 1)`, region, nodeID, RootID, TypeService, id, displayName, "/root/"+nodeID.String()+"/"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO node_refs (type, external_id, id, region) VALUES ($1, $2, $3, $4)`, TypeService, id, nodeID, region); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO grants (region, id, node_id, principal_type, principal_id, role_id, created_by)
			VALUES ($1, $2, $3, 'user', $4, $5, $6)`, region, store.NewID(), nodeID, ownerID, RoleServiceOwner, actor); err != nil {
			return err
		}
		if _, err := revision.Bump(ctx, tx, region, revision.Global, "service.registered"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, region, actor, "service.registered", "service:"+id, map[string]any{"owner": ownerID, "client": clientID})
	})
	if err != nil {
		return ServiceInfo{}, err
	}
	c.revs.Forget(revision.Global)
	return c.GetService(ctx, id)
}

func (c *Service) GetService(ctx context.Context, id string) (ServiceInfo, error) {
	return scanService(c.store.Pool.QueryRow(ctx, `SELECT `+serviceColumns+` FROM services WHERE id = $1`, id))
}

// ServiceByClient finds the service an API caller is, from its client id.
func (c *Service) ServiceByClient(ctx context.Context, clientID string) (ServiceInfo, error) {
	return scanService(c.store.Pool.QueryRow(ctx, `SELECT `+serviceColumns+` FROM services WHERE client_id = $1`, clientID))
}

func (c *Service) Services(ctx context.Context) ([]ServiceInfo, error) {
	rows, err := c.store.Pool.Query(ctx, `SELECT `+serviceColumns+` FROM services ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServiceInfo
	for rows.Next() {
		s, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ServiceOwners lists users granted a role on the service's node.
func (c *Service) ServiceOwners(ctx context.Context, id string) ([]uuid.UUID, error) {
	rows, err := c.store.Pool.Query(ctx, `SELECT g.principal_id FROM grants g JOIN services s ON s.node_id = g.node_id
		WHERE s.id = $1 AND g.principal_type = 'user' AND g.role_id = $2 ORDER BY g.created_at`, id, RoleServiceOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var u uuid.UUID
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
