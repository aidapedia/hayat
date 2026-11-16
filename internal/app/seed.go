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
	{Email: "bob@hayat.test", Name: "Bob Manager", About: "head of Engineering: on its roster, group manager there"},
	{Email: "paula@hayat.test", Name: "Paula Lead", About: "team lead of Engineering › Payment"},
	{Email: "pete@hayat.test", Name: "Pete Payment", About: "member of Engineering › Payment"},
	{Email: "priya@hayat.test", Name: "Priya Payment", About: "member of Engineering › Payment"},
	{Email: "leo@hayat.test", Name: "Leo Lead", About: "team lead of Engineering › Promo"},
	{Email: "mia@hayat.test", Name: "Mia Promo", About: "member of Engineering › Promo"},
	{Email: "carol@hayat.test", Name: "Carol Member", About: "member of Engineering › Platform, no grants"},
	{Email: "mona@hayat.test", Name: "Mona Marketing", About: "team lead of Marketing"},
	{Email: "dave@hayat.test", Name: "Dave Globex", About: "owns tenant Globex"},
}

// SeedTeamLead is the custom role the seed makes for leading one team: who is
// on it and who may use what it files, but not reshaping the tree (that is
// the built-in Group manager's).
const SeedTeamLead = "Team lead"

// Seed fills an empty development database with accounts, two tenants, a
// small org chart of nested teams with their leads, so there's something to sign in to and look at. It runs
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

	// Acme, shaped like a company. Teams nest under their department, so a
	// team's people are in the department too; each team has a lead.
	//
	//	Acme                 alice (owner)
	//	├─ Engineering       bob: group manager, the head of engineering
	//	│  ├─ Payment        paula: team lead; pete, priya
	//	│  ├─ Promo          leo: team lead; mia
	//	│  └─ Platform       carol
	//	└─ Marketing         mona: team lead
	acme, err := a.seedTenant(ctx, "Acme", users["alice"],
		users["bob"], users["paula"], users["pete"], users["priya"], users["leo"], users["mia"], users["carol"], users["mona"])
	if err != nil {
		return false, err
	}
	lead, err := a.Catalog.CreateRole(ctx, catalog.RoleInput{
		Name:        SeedTeamLead,
		Description: "Leads one team: decides who is on it and who may use what it files. Reshaping the tree stays with group managers.",
		AppliesTo:   []string{catalog.TypeGroup},
		Permissions: []string{catalog.PermGroupsManage, catalog.PermResourcesGrant},
	}, seedActor)
	if err != nil {
		return false, fmt.Errorf("role %s: %w", SeedTeamLead, err)
	}
	eng, err := a.seedGroup(ctx, acme, "Engineering", users["bob"])
	if err != nil {
		return false, err
	}
	teams := []struct {
		parent  uuid.UUID
		name    string
		lead    string
		members []string
	}{
		{eng, "Payment", "paula", []string{"pete", "priya"}},
		{eng, "Promo", "leo", []string{"mia"}},
		{eng, "Platform", "", []string{"carol"}},
		{acme, "Marketing", "mona", nil},
	}
	for _, t := range teams {
		roster := []uuid.UUID{}
		if t.lead != "" {
			roster = append(roster, users[t.lead])
		}
		for _, m := range t.members {
			roster = append(roster, users[m])
		}
		g, err := a.seedGroup(ctx, t.parent, t.name, roster...)
		if err != nil {
			return false, err
		}
		if t.lead != "" {
			if err := a.seedGrant(ctx, g, users[t.lead], lead.ID); err != nil {
				return false, err
			}
		}
	}
	if err := a.seedGrant(ctx, eng, users["bob"], catalog.RoleGroupManager); err != nil {
		return false, err
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

// seedGrant gives a person a role on a node and everything below it.
func (a *App) seedGrant(ctx context.Context, node, user, role uuid.UUID) error {
	if _, _, err := a.Tree.AddGrant(ctx, tree.GrantInput{NodeID: node, PrincipalType: tree.PrincipalUser,
		PrincipalID: user, RoleID: role, Inherit: true}, seedActor); err != nil {
		return fmt.Errorf("grant: %w", err)
	}
	return nil
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
