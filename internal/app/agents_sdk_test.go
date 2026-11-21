package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/admin/v1/adminv1connect"
	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/internal/agents"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/tree"
	"github.com/aidapedia/hayat/sdk/go/hayat"
)

// TestAgentJourney: a service acts as an agent through the real servers. It
// gets a token only for an agent that works in it, verifies it offline, and
// checks every action as the agent.
func TestAgentJourney(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	admin, err := e.a.IDs.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password")
	must(t, err)
	token, _ := e.signIn("root@example.com", "a long enough password")
	registry := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(token))
	reg, err := registry.RegisterService(ctx, connect.NewRequest(&adminv1.RegisterServiceRequest{Id: "billing", DisplayName: "Billing", OwnerEmail: "root@example.com"}))
	must(t, err)
	hz, err := hayat.New(ctx, hayat.Config{APIURL: e.api, Issuer: e.auth, ClientID: reg.Msg.ClientId, ClientSecret: reg.Msg.ClientSecret, CacheTTL: -1})
	must(t, err)
	_, err = hz.Schema.PublishContract(ctx, connect.NewRequest(&authzv1.PublishContractRequest{Manifest: billingContract}))
	must(t, err)

	tn, err := e.a.Tenants.Create(ctx, "Acme", "acme", admin.ID, "test")
	must(t, err)
	svc := agents.New(e.a.Store)
	juno, err := svc.Create(ctx, agents.Input{TenantID: tn.ID, Handle: "juno", Name: "Juno", SponsorID: admin.ID, Services: []string{"billing"}}, "test")
	must(t, err)
	mira, err := svc.Create(ctx, agents.Input{TenantID: tn.ID, Handle: "mira", Name: "Mira", SponsorID: admin.ID, Services: []string{"tether"}}, "test")
	must(t, err)

	tenantNode, _ := e.a.Tree.ByRef(ctx, catalog.TypeTenant, tn.ID.String())
	_, _, err = e.a.Tree.UpsertResource(ctx, "billing", tree.ResourceInput{Type: "billing/ledger", ExternalID: "l1", Name: "Ledger 1", ParentID: tenantNode.ID})
	must(t, err)
	snap, _ := e.a.Catalog.Snapshot(ctx, true)
	_, _, err = e.a.Tree.AddGrant(ctx, tree.GrantInput{NodeID: tenantNode.ID, PrincipalType: tree.PrincipalAgent, PrincipalID: juno.ID,
		RoleID: snap.RolesWith("billing.ledger.post")[0], Inherit: true}, "test")
	must(t, err)

	ok, err := hz.CheckAgent(ctx, juno.ID.String(), "billing.ledger.post", hayat.Ref("billing/ledger", "l1"))
	must(t, err)
	if !ok {
		t.Fatal("Juno was granted accountant on the tenant")
	}
	ids, err := hz.LookupAgent(ctx, juno.ID.String(), "billing.ledger.view", "billing/ledger", tn.ID.String())
	must(t, err)
	if len(ids) != 1 || ids[0] != "l1" {
		t.Errorf("LookupAgent = %v", ids)
	}

	raw, _, err := hz.IssueAgentToken(ctx, juno.ID.String())
	must(t, err)
	claims, err := hz.VerifyAgentToken(ctx, raw, "billing")
	must(t, err)
	if claims.AgentID() != juno.ID.String() || claims.SponsorID() != admin.ID.String() || claims.Tenant != tn.ID.String() {
		t.Errorf("claims %+v", claims)
	}
	if _, err := hz.VerifyAccessToken(ctx, raw, ""); err == nil {
		t.Error("an agent's token is never taken for a person's")
	}
	if _, err := hz.VerifyAgentToken(ctx, raw, "payroll"); err == nil {
		t.Error("the token was issued for billing")
	}

	// A service sees only the agents that work in it.
	if _, err := hz.GetAgent(ctx, mira.ID.String()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("Mira works in tether, not billing: %v", err)
	}
	if _, _, err := hz.IssueAgentToken(ctx, mira.ID.String()); err == nil {
		t.Error("no token for an agent that doesn't work here")
	}
	list, err := hz.TenantAgents(ctx, tn.ID.String())
	must(t, err)
	if len(list) != 1 || list[0].Handle != "juno" {
		t.Errorf("TenantAgents = %v", list)
	}

	// Paused: no token, and every check says no.
	_, err = svc.SetStatus(ctx, juno.ID, agents.StatusPaused, "test")
	must(t, err)
	if _, _, err := hz.IssueAgentToken(ctx, juno.ID.String()); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("a paused agent gets no token: %v", err)
	}
	// Hayat reuses a tenant's revision for up to a second; past it, the pause
	// shows in every decision.
	time.Sleep(1100 * time.Millisecond)
	if ok, _ := hz.CheckAgent(ctx, juno.ID.String(), "billing.ledger.post", hayat.Ref("billing/ledger", "l1")); ok {
		t.Error("a paused agent may do nothing")
	}
}
