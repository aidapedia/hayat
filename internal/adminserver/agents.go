package adminserver

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/agents"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/tree"
)

// agentService runs a tenant's agents. Listing is open to the tenant's
// members; every change needs hayat.agents.manage on the tenant's node.
type agentService struct {
	s      *Server
	agents *agents.Service
}

func (g *agentService) tenantNode(ctx context.Context, tenantID uuid.UUID) (tree.Node, error) {
	n, err := g.s.tree.ByRef(ctx, catalog.TypeTenant, tenantID.String())
	if err != nil {
		return tree.Node{}, g.s.fail(err)
	}
	return n, nil
}

// agent loads one agent and checks the caller may see it (manage, when set).
func (g *agentService) agent(ctx context.Context, raw string, manage bool) (agents.Agent, bool, error) {
	id, err := parseID(raw, "agent id")
	if err != nil {
		return agents.Agent{}, false, err
	}
	a, err := g.agents.Get(ctx, id)
	if err != nil {
		return agents.Agent{}, false, g.s.fail(err)
	}
	n, err := g.tenantNode(ctx, a.TenantID)
	if err != nil {
		return agents.Agent{}, false, err
	}
	if manage {
		return a, true, g.s.require(ctx, catalog.PermAgentsManage, n)
	}
	perms, err := g.s.visible(ctx, n)
	if err != nil {
		return agents.Agent{}, false, err
	}
	return a, contains(perms, catalog.PermAgentsManage), nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (g *agentService) toAgents(ctx context.Context, list []agents.Agent) ([]*adminv1.Agent, error) {
	sponsors := make([]uuid.UUID, 0, len(list))
	for _, a := range list {
		sponsors = append(sponsors, a.SponsorID)
	}
	users, err := g.s.ids.GetUsers(ctx, sponsors)
	if err != nil {
		return nil, err
	}
	out := make([]*adminv1.Agent, 0, len(list))
	for _, a := range list {
		pa := &adminv1.Agent{Id: a.ID.String(), TenantId: a.TenantID.String(), Handle: a.Handle, Name: a.Name,
			Status: a.Status, StatusReason: a.StatusReason, Services: a.Services, CreatedBy: a.CreatedBy,
			CreatedAt: ts(a.CreatedAt), UpdatedAt: ts(a.UpdatedAt)}
		for _, u := range users {
			if u.ID == a.SponsorID {
				pa.Sponsor = toUser(u)
			}
		}
		groups, err := g.agents.Groups(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		for _, id := range groups {
			if n, err := g.s.tree.Get(ctx, id); err == nil {
				pa.Groups = append(pa.Groups, &adminv1.AgentGroup{Id: n.ID.String(), Name: n.Name})
			}
		}
		out = append(out, pa)
	}
	return out, nil
}

func (g *agentService) one(ctx context.Context, a agents.Agent) (*adminv1.Agent, error) {
	out, err := g.toAgents(ctx, []agents.Agent{a})
	if err != nil {
		return nil, g.s.fail(err)
	}
	return out[0], nil
}

func (g *agentService) ListAgents(ctx context.Context, req *connect.Request[adminv1.ListAgentsRequest]) (*connect.Response[adminv1.ListAgentsResponse], error) {
	id, err := parseID(req.Msg.TenantId, "tenant_id")
	if err != nil {
		return nil, err
	}
	n, err := g.tenantNode(ctx, id)
	if err != nil {
		return nil, err
	}
	perms, err := g.s.visible(ctx, n)
	if err != nil {
		return nil, err
	}
	list, err := g.agents.List(ctx, id)
	if err != nil {
		return nil, g.s.fail(err)
	}
	out, err := g.toAgents(ctx, list)
	if err != nil {
		return nil, g.s.fail(err)
	}
	return connect.NewResponse(&adminv1.ListAgentsResponse{Agents: out, CanManage: contains(perms, catalog.PermAgentsManage)}), nil
}

func (g *agentService) GetAgent(ctx context.Context, req *connect.Request[adminv1.GetAgentRequest]) (*connect.Response[adminv1.GetAgentResponse], error) {
	a, manage, err := g.agent(ctx, req.Msg.Id, false)
	if err != nil {
		return nil, err
	}
	out, err := g.one(ctx, a)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.GetAgentResponse{Agent: out, CanManage: manage}), nil
}

func (g *agentService) CreateAgent(ctx context.Context, req *connect.Request[adminv1.CreateAgentRequest]) (*connect.Response[adminv1.CreateAgentResponse], error) {
	m := req.Msg
	tenant, err := parseID(m.TenantId, "tenant_id")
	if err != nil {
		return nil, err
	}
	sponsor, err := parseID(m.SponsorUserId, "sponsor_user_id")
	if err != nil {
		return nil, err
	}
	n, err := g.tenantNode(ctx, tenant)
	if err != nil {
		return nil, err
	}
	if err := g.s.require(ctx, catalog.PermAgentsManage, n); err != nil {
		return nil, err
	}
	a, err := g.agents.Create(ctx, agents.Input{TenantID: tenant, Handle: m.Handle, Name: m.Name, SponsorID: sponsor, Services: m.Services}, actor(ctx))
	if err != nil {
		return nil, g.s.fail(err)
	}
	for _, raw := range m.GroupIds {
		gid, err := parseID(raw, "group id")
		if err != nil {
			return nil, err
		}
		if err := g.agents.AddToGroup(ctx, a.ID, gid, actor(ctx)); err != nil {
			return nil, g.s.fail(err)
		}
	}
	out, err := g.one(ctx, a)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.CreateAgentResponse{Agent: out}), nil
}

func (g *agentService) UpdateAgent(ctx context.Context, req *connect.Request[adminv1.UpdateAgentRequest]) (*connect.Response[adminv1.UpdateAgentResponse], error) {
	a, _, err := g.agent(ctx, req.Msg.Id, true)
	if err != nil {
		return nil, err
	}
	if a, err = g.agents.Update(ctx, a.ID, req.Msg.Name, req.Msg.Services, actor(ctx)); err != nil {
		return nil, g.s.fail(err)
	}
	out, err := g.one(ctx, a)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.UpdateAgentResponse{Agent: out}), nil
}

func (g *agentService) SetAgentStatus(ctx context.Context, req *connect.Request[adminv1.SetAgentStatusRequest]) (*connect.Response[adminv1.SetAgentStatusResponse], error) {
	a, _, err := g.agent(ctx, req.Msg.Id, true)
	if err != nil {
		return nil, err
	}
	if a, err = g.agents.SetStatus(ctx, a.ID, req.Msg.Status, actor(ctx)); err != nil {
		return nil, g.s.fail(err)
	}
	out, err := g.one(ctx, a)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.SetAgentStatusResponse{Agent: out}), nil
}

func (g *agentService) ReassignAgent(ctx context.Context, req *connect.Request[adminv1.ReassignAgentRequest]) (*connect.Response[adminv1.ReassignAgentResponse], error) {
	a, _, err := g.agent(ctx, req.Msg.Id, true)
	if err != nil {
		return nil, err
	}
	sponsor, err := parseID(req.Msg.SponsorUserId, "sponsor_user_id")
	if err != nil {
		return nil, err
	}
	if a, err = g.agents.Reassign(ctx, a.ID, sponsor, actor(ctx)); err != nil {
		return nil, g.s.fail(err)
	}
	out, err := g.one(ctx, a)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ReassignAgentResponse{Agent: out}), nil
}

func (g *agentService) DeleteAgent(ctx context.Context, req *connect.Request[adminv1.DeleteAgentRequest]) (*connect.Response[adminv1.DeleteAgentResponse], error) {
	a, _, err := g.agent(ctx, req.Msg.Id, true)
	if err != nil {
		return nil, err
	}
	if err := g.agents.Delete(ctx, a.ID, actor(ctx)); err != nil {
		return nil, g.s.fail(err)
	}
	return connect.NewResponse(&adminv1.DeleteAgentResponse{}), nil
}

func (g *agentService) AddAgentToGroup(ctx context.Context, req *connect.Request[adminv1.AddAgentToGroupRequest]) (*connect.Response[adminv1.AddAgentToGroupResponse], error) {
	a, _, err := g.agent(ctx, req.Msg.Id, true)
	if err != nil {
		return nil, err
	}
	gid, err := parseID(req.Msg.GroupId, "group_id")
	if err != nil {
		return nil, err
	}
	if err := g.agents.AddToGroup(ctx, a.ID, gid, actor(ctx)); err != nil {
		return nil, g.s.fail(err)
	}
	out, err := g.one(ctx, a)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.AddAgentToGroupResponse{Agent: out}), nil
}

func (g *agentService) RemoveAgentFromGroup(ctx context.Context, req *connect.Request[adminv1.RemoveAgentFromGroupRequest]) (*connect.Response[adminv1.RemoveAgentFromGroupResponse], error) {
	a, _, err := g.agent(ctx, req.Msg.Id, true)
	if err != nil {
		return nil, err
	}
	gid, err := parseID(req.Msg.GroupId, "group_id")
	if err != nil {
		return nil, err
	}
	if err := g.agents.RemoveFromGroup(ctx, a.ID, gid, actor(ctx)); err != nil {
		return nil, g.s.fail(err)
	}
	out, err := g.one(ctx, a)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.RemoveAgentFromGroupResponse{Agent: out}), nil
}
