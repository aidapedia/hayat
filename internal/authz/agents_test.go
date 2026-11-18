package authz_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/agents"
	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/tenancy"

	"github.com/aidapedia/hayat/internal/tree"
)

// billing v2 adds a permission only people may hold.
const billingV2 = `
service: billing
version: 2
resource_types:
  - name: ledger
    parents: [hayat/tenant, hayat/group]
    access: { permission: ledger.view, via_placement: true, requires_membership: true }
permissions:
  - { key: ledger.view, scope: ledger }
  - { key: ledger.post, scope: ledger }
  - { key: ledger.approve, scope: ledger, people_only: true }
default_roles:
  - { slug: accountant, name: Accountant, permissions: [ledger.view, ledger.post] }
  - { slug: approver, name: Approver, permissions: [ledger.view, ledger.approve] }
`

type agentWorld struct {
	*world
	agents *agents.Service
	juno   agents.Agent
}

func buildAgents(t *testing.T) *agentWorld {
	w := build(t)
	_, err := w.cat.Publish(w.ctx, "billing", []byte(billingV2), "test")
	must(t, err)
	svc := agents.New(w.store)
	juno, err := svc.Create(w.ctx, agents.Input{TenantID: w.tenant.ID, Handle: "@Juno", Name: "Juno", SponsorID: w.users["ana"], Services: []string{"billing"}}, "test")
	must(t, err)
	if juno.Handle != "juno" || juno.Status != agents.StatusActive {
		t.Fatalf("created %+v", juno)
	}
	return &agentWorld{world: w, agents: svc, juno: juno}
}

func (w *agentWorld) agentCan(perm string, n tree.Node) bool {
	w.t.Helper()
	n, err := w.tree.Get(w.ctx, n.ID)
	must(w.t, err)
	ok, _, _, err := w.engine.Check(w.ctx, w.juno.ID, perm, n, authz.Consistency{Fresh: true})
	must(w.t, err)
	return ok
}

func (w *agentWorld) role(slug string) uuid.UUID {
	snap, err := w.cat.Snapshot(w.ctx, true)
	must(w.t, err)
	for id, r := range snap.Roles {
		if r.Slug == slug {
			return id
		}
	}
	w.t.Fatalf("no role %s", slug)
	return uuid.Nil
}

func TestAgentGetsWhatIsGrantedAndNothingPeopleOnly(t *testing.T) {
	w := buildAgents(t)
	if w.agentCan("billing.ledger.view", w.l2) {
		t.Fatal("nothing granted means nothing")
	}
	_, _, err := w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: w.finance.ID, PrincipalType: tree.PrincipalAgent,
		PrincipalID: w.juno.ID, RoleID: w.accRole, Inherit: true}, "test")
	must(t, err)
	if !w.agentCan("billing.ledger.post", w.l1) {
		t.Error("an inheriting grant on Finance reaches Ledger 1 for the agent")
	}
	// A role holding a people_only permission is never granted to an agent.
	_, _, err = w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: w.finance.ID, PrincipalType: tree.PrincipalAgent,
		PrincipalID: w.juno.ID, RoleID: w.role("billing.approver"), Inherit: true}, "test")
	if err == nil {
		t.Error("the approver role holds a people_only permission")
	}
	// Nor Hayat's own roles.
	_, _, err = w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: w.tenantN.ID, PrincipalType: tree.PrincipalAgent,
		PrincipalID: w.juno.ID, RoleID: catalog.RoleTenantAdmin, Inherit: true}, "test")
	if err == nil {
		t.Error("an agent never administers a tenant")
	}
}

func TestEveryoneMeansPeople(t *testing.T) {
	w := buildAgents(t)
	_, _, err := w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: w.tenantN.ID, PrincipalType: tree.PrincipalGroup,
		PrincipalID: w.tenantN.ID, RoleID: w.accRole, Inherit: true}, "test")
	must(t, err)
	if !w.can("ben", "billing.ledger.post", w.l2) {
		t.Fatal("a grant to the tenant reaches everyone in it")
	}
	if w.agentCan("billing.ledger.post", w.l2) {
		t.Error("…people, not agents")
	}
}

func TestAgentGroupsAndPlacement(t *testing.T) {
	w := buildAgents(t)
	must(t, w.agents.AddToGroup(w.ctx, w.juno.ID, w.sub.ID, "test"))
	if !w.agentCan("billing.ledger.view", w.l1) {
		t.Error("Ledger 1 is filed under Payables, Juno's group")
	}
	if w.agentCan("billing.ledger.post", w.l1) {
		t.Error("placement gives only the access permission")
	}
	// Agents stay out of every people roster.
	members, err := w.tenants.Members(w.ctx, w.tenant.ID)
	must(t, err)
	for _, m := range members {
		if m.UserID == w.juno.ID {
			t.Error("an agent is never on the people roster")
		}
	}
	subs, err := w.engine.LookupSubjects(w.ctx, "billing.ledger.view", w.l1, authz.Consistency{Fresh: true})
	must(t, err)
	found := false
	for _, s := range subs {
		if s.UserID == w.juno.ID {
			found = s.Agent
		}
	}
	if !found {
		t.Error("LookupSubjects lists the agent, marked as one")
	}
	res, err := w.engine.LookupResources(w.ctx, w.juno.ID, "billing.ledger.view", "billing/ledger", nil, authz.Consistency{Fresh: true}, 0)
	must(t, err)
	if len(res) != 1 || res[0].ID != w.l1.ID {
		t.Errorf("LookupResources for the agent = %v", res)
	}
}

func TestAgentStopsWithItsSponsor(t *testing.T) {
	w := buildAgents(t)
	_, _, err := w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: w.tenantN.ID, PrincipalType: tree.PrincipalAgent,
		PrincipalID: w.juno.ID, RoleID: w.accRole, Inherit: true}, "test")
	must(t, err)
	if !w.agentCan("billing.ledger.post", w.l2) {
		t.Fatal("granted on the tenant")
	}
	must(t, w.tenants.RemoveMember(w.ctx, w.tenant.ID, w.users["ana"], "test"))
	a, err := w.agents.Get(w.ctx, w.juno.ID)
	must(t, err)
	if a.Status != agents.StatusPaused || a.StatusReason != agents.ReasonSponsorLeft {
		t.Fatalf("after the sponsor left: %s (%s)", a.Status, a.StatusReason)
	}
	if w.agentCan("billing.ledger.post", w.l2) {
		t.Error("a paused agent may do nothing")
	}
	if _, err := w.agents.SetStatus(w.ctx, w.juno.ID, agents.StatusActive, "test"); !errors.Is(err, agents.ErrSponsorGone) {
		t.Errorf("resuming without a sponsor: %v", err)
	}
	if _, err := w.agents.Reassign(w.ctx, w.juno.ID, w.users["ben"], "test"); err != nil {
		t.Fatal(err)
	}
	if !w.agentCan("billing.ledger.post", w.l2) {
		t.Error("a new sponsor resumes it")
	}
	if _, err := w.agents.Reassign(w.ctx, w.juno.ID, w.users["cy"], "test"); !errors.Is(err, agents.ErrSponsor) {
		t.Errorf("Cy isn't on the roster: %v", err)
	}
}

func TestAgentNeverOwnsAndTenantKeepsIt(t *testing.T) {
	w := buildAgents(t)
	_, _, err := w.tree.UpsertResource(w.ctx, "billing", tree.ResourceInput{Type: "billing/ledger", ExternalID: "l3", Name: "Ledger 3",
		ParentID: w.finance.ID, OwnerID: &w.juno.ID})
	if !errors.Is(err, tree.ErrAgentOwner) {
		t.Errorf("owning a node is never an agent's: %v", err)
	}
	if _, err := w.agents.Create(w.ctx, agents.Input{TenantID: w.tenant.ID, Handle: "juno", SponsorID: w.users["ana"]}, "test"); !errors.Is(err, agents.ErrHandleTaken) {
		t.Errorf("handles are unique in a tenant: %v", err)
	}
	// Outside its tenant an agent may do nothing, whatever it holds.
	other, err := w.tenants.Create(w.ctx, "Globex", "globex", w.users["ben"], "test")
	must(t, err)
	otherN, err := w.tree.ByRef(w.ctx, catalog.TypeTenant, other.ID.String())
	must(t, err)
	if _, _, err := w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: otherN.ID, PrincipalType: tree.PrincipalAgent,
		PrincipalID: w.juno.ID, RoleID: w.accRole, Inherit: true}, "test"); err == nil {
		t.Error("an agent can't be granted in another tenant")
	}
	if err := w.tenants.Delete(w.ctx, w.tenant.ID, "test"); !errors.Is(err, tenancy.ErrHasAgents) && !errors.Is(err, tenancy.ErrNotEmpty) {
		t.Errorf("a tenant with agents isn't deleted: %v", err)
	}
	if _, err := w.agents.ForService(w.ctx, w.juno.ID, "tether"); !errors.Is(err, agents.ErrNotAllowed) {
		t.Errorf("Juno works in billing only: %v", err)
	}
	if _, err := w.agents.ForService(w.ctx, w.juno.ID, "billing"); err != nil {
		t.Error(err)
	}
}
