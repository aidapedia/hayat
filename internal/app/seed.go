package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tenancy"
	"github.com/aidapedia/hayat/internal/tree"
)

// SeedPassword is every seeded account's password. Seeding is refused outside
// HAYAT_DEV, so it never reaches a real instance.
const SeedPassword = "hayat-dev-password"

const seedActor = "seed"

// SeedAccount is one account Seed makes, for printing the sign-in list.
type SeedAccount struct {
	Email, Name, About string
	admin              bool
}

// SeedAccounts are the people in the dev data, in the order they're created.
var SeedAccounts = []SeedAccount{
	{Email: "admin@hayat.test", Name: "Ada Admin", About: "instance admin", admin: true},
	{Email: "alice@hayat.test", Name: "Alice Owner", About: "owns tenant Acme"},
	{Email: "bob@hayat.test", Name: "Bob Manager", About: "Acme member, group manager of Engineering"},
	{Email: "carol@hayat.test", Name: "Carol Member", About: "Acme member in Engineering/Backend, no grants"},
	{Email: "dave@hayat.test", Name: "Dave Globex", About: "owns tenant Globex"},
}

// Seed fills an empty development database with accounts, two tenants, a few
// groups and a grant, so there's something to sign in to and look at. It runs
// once: if the seed admin already exists it does nothing and returns false.
func (a *App) Seed(ctx context.Context) (bool, error) {
	if !a.Cfg.Dev {
		return false, errors.New("seed only runs with HAYAT_DEV=true")
	}
	if _, err := a.IDs.GetUserByEmail(ctx, SeedAccounts[0].Email); err == nil {
		return false, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return false, err
	}
	if err := a.Bootstrap(ctx); err != nil {
		return false, err
	}

	users := map[string]uuid.UUID{}
	for _, acc := range SeedAccounts {
		u, err := a.IDs.CreateActive(ctx, acc.Email, acc.Name, SeedPassword, acc.admin, seedActor)
		if err != nil {
			return false, fmt.Errorf("account %s: %w", acc.Email, err)
		}
		users[strings.SplitN(acc.Email, "@", 2)[0]] = u.ID
	}

	// Acme: Engineering (Backend, Frontend) and Marketing.
	acme, err := a.seedTenant(ctx, "Acme", users["alice"], users["bob"], users["carol"])
	if err != nil {
		return false, err
	}
	eng, err := a.seedGroup(ctx, acme, "Engineering", users["bob"])
	if err != nil {
		return false, err
	}
	if _, err := a.seedGroup(ctx, eng, "Backend", users["carol"]); err != nil {
		return false, err
	}
	if _, err := a.seedGroup(ctx, eng, "Frontend"); err != nil {
		return false, err
	}
	if _, err := a.seedGroup(ctx, acme, "Marketing"); err != nil {
		return false, err
	}
	if _, _, err := a.Tree.AddGrant(ctx, tree.GrantInput{NodeID: eng, PrincipalType: tree.PrincipalUser,
		PrincipalID: users["bob"], RoleID: catalog.RoleGroupManager, Inherit: true}, seedActor); err != nil {
		return false, fmt.Errorf("grant: %w", err)
	}

	// Globex: one group, to have a second tenant to switch to.
	globex, err := a.seedTenant(ctx, "Globex", users["dave"])
	if err != nil {
		return false, err
	}
	if _, err := a.seedGroup(ctx, globex, "Sales"); err != nil {
		return false, err
	}
	return true, nil
}

// seedTenant creates a tenant owned by owner, adds the members, and returns
// the tenant's node.
func (a *App) seedTenant(ctx context.Context, name string, owner uuid.UUID, members ...uuid.UUID) (uuid.UUID, error) {
	t, err := a.Tenants.Create(ctx, name, "", owner, seedActor)
	if err != nil {
		return uuid.Nil, fmt.Errorf("tenant %s: %w", name, err)
	}
	for _, m := range members {
		if err := a.Tenants.AddMember(ctx, t.ID, m, seedActor); err != nil {
			return uuid.Nil, fmt.Errorf("tenant %s member: %w", name, err)
		}
	}
	n, err := a.Tree.ByRef(ctx, tenancy.TypeTenant, t.ID.String())
	if err != nil {
		return uuid.Nil, err
	}
	return n.ID, nil
}

// seedGroup files a group under parent and puts the members on its roster.
func (a *App) seedGroup(ctx context.Context, parent uuid.UUID, name string, members ...uuid.UUID) (uuid.UUID, error) {
	g, _, err := a.Tree.CreateGroup(ctx, parent, name, seedActor)
	if err != nil {
		return uuid.Nil, fmt.Errorf("group %s: %w", name, err)
	}
	for _, m := range members {
		if _, err := a.Tree.AddGroupMember(ctx, g.ID, m, seedActor); err != nil {
			return uuid.Nil, fmt.Errorf("group %s member: %w", name, err)
		}
	}
	return g.ID, nil
}
