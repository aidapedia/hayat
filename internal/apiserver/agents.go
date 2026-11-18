package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"connectrpc.com/connect"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/internal/agents"
	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/store"
)

// AgentTokenTTL is how long an agent token lives. Short: the service asks for
// a new one for each run, so a paused agent stops within the hour at worst,
// and at once for every service that checks each action (it should).
const AgentTokenTTL = time.Hour

// ---------- AgentService ----------

type agentService struct{ s *Server }

func toAgent(a agents.Agent, groups []uuid.UUID) *authzv1.Agent {
	out := &authzv1.Agent{Id: a.ID.String(), TenantId: a.TenantID.String(), Handle: a.Handle, Name: a.Name,
		SponsorUserId: a.SponsorID.String(), Status: a.Status, StatusReason: a.StatusReason, Services: a.Services}
	for _, g := range groups {
		out.GroupIds = append(out.GroupIds, g.String())
	}
	return out
}

// visible finds an agent the calling service may see: one that works in it.
// Any other is "not found", so a service can't learn about another's agents.
func (g *agentService) visible(ctx context.Context, raw string) (agents.Agent, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return agents.Agent{}, invalid("agent_id is not a valid id")
	}
	a, err := g.s.agents.Get(ctx, id)
	if err != nil {
		return agents.Agent{}, err
	}
	if !a.WorksIn(callerService(ctx).ID) {
		return agents.Agent{}, store.ErrNotFound
	}
	return a, nil
}

func (g *agentService) GetAgent(ctx context.Context, req *connect.Request[authzv1.GetAgentRequest]) (*connect.Response[authzv1.GetAgentResponse], error) {
	a, err := g.visible(ctx, req.Msg.AgentId)
	if err != nil {
		return nil, g.s.fail(err)
	}
	groups, err := g.s.agents.Groups(ctx, a.ID)
	if err != nil {
		return nil, g.s.fail(err)
	}
	return connect.NewResponse(&authzv1.GetAgentResponse{Agent: toAgent(a, groups)}), nil
}

func (g *agentService) ListTenantAgents(ctx context.Context, req *connect.Request[authzv1.ListTenantAgentsRequest]) (*connect.Response[authzv1.ListTenantAgentsResponse], error) {
	tenant, err := uuid.Parse(req.Msg.TenantId)
	if err != nil {
		return nil, invalid("tenant_id is not a valid id")
	}
	list, err := g.s.agents.List(ctx, tenant)
	if err != nil {
		return nil, g.s.fail(err)
	}
	svc := callerService(ctx).ID
	resp := &authzv1.ListTenantAgentsResponse{}
	for _, a := range list {
		if !a.WorksIn(svc) {
			continue
		}
		groups, err := g.s.agents.Groups(ctx, a.ID)
		if err != nil {
			return nil, g.s.fail(err)
		}
		resp.Agents = append(resp.Agents, toAgent(a, groups))
	}
	return connect.NewResponse(resp), nil
}

func (g *agentService) IssueAgentToken(ctx context.Context, req *connect.Request[authzv1.IssueAgentTokenRequest]) (*connect.Response[authzv1.IssueAgentTokenResponse], error) {
	a, err := g.visible(ctx, req.Msg.AgentId)
	if err != nil {
		return nil, g.s.fail(err)
	}
	if err := g.s.agents.Usable(ctx, a); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("this agent isn't active, or its sponsor isn't"))
	}
	svc := callerService(ctx)
	raw, exp, err := g.s.signAgentToken(ctx, a, svc)
	if err != nil {
		return nil, g.s.fail(err)
	}
	if err := audit.Write(ctx, g.s.store.Pool, a.Region, "service:"+svc.ID, "agent.token_issued", "agent:"+a.ID.String(),
		map[string]any{"sponsor": a.SponsorID}); err != nil {
		return nil, g.s.fail(err)
	}
	return connect.NewResponse(&authzv1.IssueAgentTokenResponse{AccessToken: raw, ExpiresAt: timestamppb.New(exp)}), nil
}

// AgentClaims is what an agent token says. Like a person's token it never
// carries permissions: the service checks every action as the agent.
type AgentClaims struct {
	Issuer        string   `json:"iss"`
	Subject       string   `json:"sub"` // "agent:<id>"
	Audience      []string `json:"aud"` // the service's namespace and its client id
	ClientID      string   `json:"client_id"`
	IssuedAt      int64    `json:"iat"`
	NotBefore     int64    `json:"nbf"`
	Expiry        int64    `json:"exp"`
	ID            string   `json:"jti"`
	PrincipalType string   `json:"principal_type"` // "agent"
	Tenant        string   `json:"tenant"`
	Sponsor       string   `json:"sponsor"` // "user:<id>"
}

func (s *Server) signAgentToken(ctx context.Context, a agents.Agent, svc catalog.ServiceInfo) (string, time.Time, error) {
	key, err := s.keys.Active(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: key.SignatureAlgorithm(),
		Key: jose.JSONWebKey{Key: key.Key(), KeyID: key.ID(), Algorithm: string(key.SignatureAlgorithm())}},
		(&jose.SignerOptions{}).WithType("at+jwt"))
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	exp := now.Add(AgentTokenTTL)
	payload, err := json.Marshal(AgentClaims{
		Issuer: s.cfg.Issuer, Subject: "agent:" + a.ID.String(), Audience: []string{svc.ID, svc.ClientID}, ClientID: svc.ClientID,
		IssuedAt: now.Unix(), NotBefore: now.Unix(), Expiry: exp.Unix(), ID: store.NewID().String(),
		PrincipalType: "agent", Tenant: a.TenantID.String(), Sponsor: "user:" + a.SponsorID.String(),
	})
	if err != nil {
		return "", time.Time{}, err
	}
	sig, err := signer.Sign(payload)
	if err != nil {
		return "", time.Time{}, err
	}
	raw, err := sig.CompactSerialize()
	return raw, exp, err
}
