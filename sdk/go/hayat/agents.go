package hayat

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	jose "github.com/go-jose/go-jose/v4"

	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
)

// Agents are members that aren't people: they hold roles and work in your
// service, but never sign in. Act as one with an agent token, and check every
// action as the agent with CheckAgent. Hayat answers "no" for an agent outside
// its tenant, without an active sponsor, or for a permission only people hold.

// CheckAgent is Check for an agent.
func (c *Client) CheckAgent(ctx context.Context, agentID, permission string, res Resource) (bool, error) {
	key := "agent:" + agentID + "\x00" + permission + "\x00" + res.key()
	if v, ok := c.cache.get(key); ok {
		return v, nil
	}
	resp, err := c.Authz.Check(ctx, connect.NewRequest(&authzv1.CheckRequest{AgentId: agentID, Permission: permission,
		Resource: res.proto(), Consistency: c.consistency(res)}))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return false, nil
		}
		return false, err
	}
	c.cache.put(key, resp.Msg.TenantId, resp.Msg.Allowed)
	return resp.Msg.Allowed, nil
}

// LookupAgent is Lookup for an agent.
func (c *Client) LookupAgent(ctx context.Context, agentID, permission, typ, tenantID string) ([]string, error) {
	var out []string
	token := ""
	for {
		resp, err := c.Authz.LookupResources(ctx, connect.NewRequest(&authzv1.LookupResourcesRequest{AgentId: agentID, Permission: permission,
			Type: typ, TenantId: tenantID, PageSize: 500, PageToken: token}))
		if err != nil {
			return nil, err
		}
		for _, r := range resp.Msg.Resources {
			out = append(out, r.ExternalId)
		}
		if token = resp.Msg.NextPageToken; token == "" {
			return out, nil
		}
	}
}

// GetAgent reads an agent that works in your service.
func (c *Client) GetAgent(ctx context.Context, agentID string) (*authzv1.Agent, error) {
	resp, err := c.Agents.GetAgent(ctx, connect.NewRequest(&authzv1.GetAgentRequest{AgentId: agentID}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Agent, nil
}

// TenantAgents lists a tenant's agents that work in your service.
func (c *Client) TenantAgents(ctx context.Context, tenantID string) ([]*authzv1.Agent, error) {
	resp, err := c.Agents.ListTenantAgents(ctx, connect.NewRequest(&authzv1.ListTenantAgentsRequest{TenantId: tenantID}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Agents, nil
}

// IssueAgentToken gets a short-lived token to act as an agent. Refused unless
// the agent and its sponsor are active and it works in your service.
func (c *Client) IssueAgentToken(ctx context.Context, agentID string) (string, time.Time, error) {
	resp, err := c.Agents.IssueAgentToken(ctx, connect.NewRequest(&authzv1.IssueAgentTokenRequest{AgentId: agentID}))
	if err != nil {
		return "", time.Time{}, err
	}
	return resp.Msg.AccessToken, resp.Msg.ExpiresAt.AsTime(), nil
}

// AgentClaims is what an agent token says about who it acts as. Like a
// person's token it never says what the agent may do: ask CheckAgent.
type AgentClaims struct {
	Subject       string   `json:"sub"` // "agent:<id>"
	Audience      audience `json:"aud"`
	ClientID      string   `json:"client_id"`
	Issuer        string   `json:"iss"`
	Expiry        int64    `json:"exp"`
	NotBefore     int64    `json:"nbf"`
	PrincipalType string   `json:"principal_type"`
	Tenant        string   `json:"tenant"`
	Sponsor       string   `json:"sponsor"` // "user:<id>"
}

// AgentID is the agent's id, without the "agent:" prefix.
func (a *AgentClaims) AgentID() string { return strings.TrimPrefix(a.Subject, "agent:") }

// SponsorID is the sponsor's user id.
func (a *AgentClaims) SponsorID() string { return strings.TrimPrefix(a.Sponsor, "user:") }

// VerifyAgentToken checks an agent token offline: signature, issuer, expiry,
// that it is an agent's, and that it was issued for audience (your service's
// namespace, or "" for any).
func (c *Client) VerifyAgentToken(ctx context.Context, raw, audience string) (*AgentClaims, error) {
	sig, err := jose.ParseSigned(raw, []jose.SignatureAlgorithm{jose.ES256, jose.RS256, jose.EdDSA})
	if err != nil || len(sig.Signatures) != 1 {
		return nil, ErrInvalidToken
	}
	key, err := c.keys.key(ctx, sig.Signatures[0].Header.KeyID)
	if err != nil {
		return nil, err
	}
	payload, err := sig.Verify(key)
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims AgentClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	now := time.Now().Unix()
	switch {
	case claims.Issuer != c.keys.issuer, claims.Expiry < now, claims.NotBefore > now+60:
		return nil, ErrInvalidToken
	case !strings.HasPrefix(claims.Subject, "agent:"), claims.PrincipalType != "agent":
		return nil, ErrInvalidToken
	case audience != "" && claims.ClientID != audience && !slices.Contains(claims.Audience, audience):
		return nil, ErrInvalidToken
	}
	return &claims, nil
}
