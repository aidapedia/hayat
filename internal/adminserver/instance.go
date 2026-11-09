package adminserver

import (
	"context"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/audit"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
)

// ---------- Overview ----------

type overviewService struct{ s *Server }

func (o *overviewService) GetOverview(ctx context.Context, _ *connect.Request[adminv1.GetOverviewRequest]) (*connect.Response[adminv1.GetOverviewResponse], error) {
	total, active, err := o.s.ids.CountUsers(ctx)
	if err != nil {
		return nil, o.s.fail(err)
	}
	tenants, err := o.s.tenants.Count(ctx)
	if err != nil {
		return nil, o.s.fail(err)
	}
	clients, err := o.s.clients.Count(ctx)
	if err != nil {
		return nil, o.s.fail(err)
	}
	sessions, err := o.s.ids.CountActiveSessions(ctx)
	if err != nil {
		return nil, o.s.fail(err)
	}
	events, err := audit.List(ctx, o.s.store.Pool, parsePageToken(""), 8)
	if err != nil {
		return nil, o.s.fail(err)
	}
	names := o.s.actorNames(ctx, events)
	resp := &adminv1.GetOverviewResponse{
		Users: int32(total), ActiveUsers: int32(active), Tenants: int32(tenants), Clients: int32(clients),
		ActiveSessions: int32(sessions), Issuer: o.s.cfg.Issuer, Region: o.s.cfg.Region, Version: Version,
	}
	for _, e := range events {
		resp.Recent = append(resp.Recent, toEvent(e, names))
	}
	return connect.NewResponse(resp), nil
}

// ---------- Clients ----------

type clientService struct{ s *Server }

func (c *clientService) ListClients(ctx context.Context, _ *connect.Request[adminv1.ListClientsRequest]) (*connect.Response[adminv1.ListClientsResponse], error) {
	list, err := c.s.clients.List(ctx)
	if err != nil {
		return nil, c.s.fail(err)
	}
	resp := &adminv1.ListClientsResponse{}
	for _, x := range list {
		resp.Clients = append(resp.Clients, toClient(x))
	}
	return connect.NewResponse(resp), nil
}

func (c *clientService) GetClient(ctx context.Context, req *connect.Request[adminv1.GetClientRequest]) (*connect.Response[adminv1.GetClientResponse], error) {
	x, err := c.s.clients.Get(ctx, req.Msg.Id)
	if err != nil {
		return nil, c.s.fail(err)
	}
	return connect.NewResponse(&adminv1.GetClientResponse{Client: toClient(x)}), nil
}

func (c *clientService) CreateClient(ctx context.Context, req *connect.Request[adminv1.CreateClientRequest]) (*connect.Response[adminv1.CreateClientResponse], error) {
	m := req.Msg
	x, secret, err := c.s.clients.Create(ctx, hoidc.ClientInput{Name: m.Name, Kind: m.Kind, Redirects: m.RedirectUris,
		PostLogoutRedirects: m.PostLogoutRedirectUris, Grants: m.GrantTypes}, actor(ctx))
	if err != nil {
		return nil, c.s.fail(err)
	}
	return connect.NewResponse(&adminv1.CreateClientResponse{Client: toClient(x), Secret: secret}), nil
}

func (c *clientService) UpdateClient(ctx context.Context, req *connect.Request[adminv1.UpdateClientRequest]) (*connect.Response[adminv1.UpdateClientResponse], error) {
	m := req.Msg
	x, err := c.s.clients.Update(ctx, m.Id, hoidc.ClientInput{Name: m.Name, Redirects: m.RedirectUris,
		PostLogoutRedirects: m.PostLogoutRedirectUris, Grants: m.GrantTypes}, actor(ctx))
	if err != nil {
		return nil, c.s.fail(err)
	}
	return connect.NewResponse(&adminv1.UpdateClientResponse{Client: toClient(x)}), nil
}

func (c *clientService) RotateClientSecret(ctx context.Context, req *connect.Request[adminv1.RotateClientSecretRequest]) (*connect.Response[adminv1.RotateClientSecretResponse], error) {
	secret, err := c.s.clients.RotateSecret(ctx, req.Msg.Id, actor(ctx))
	if err != nil {
		return nil, c.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RotateClientSecretResponse{Secret: secret}), nil
}

func (c *clientService) SetClientDisabled(ctx context.Context, req *connect.Request[adminv1.SetClientDisabledRequest]) (*connect.Response[adminv1.SetClientDisabledResponse], error) {
	x, err := c.s.clients.SetDisabled(ctx, req.Msg.Id, req.Msg.Disabled, actor(ctx))
	if err != nil {
		return nil, c.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SetClientDisabledResponse{Client: toClient(x)}), nil
}

// ---------- Keys ----------

type keyService struct{ s *Server }

func (k *keyService) ListSigningKeys(ctx context.Context, _ *connect.Request[adminv1.ListSigningKeysRequest]) (*connect.Response[adminv1.ListSigningKeysResponse], error) {
	list, err := k.s.keys.List(ctx)
	if err != nil {
		return nil, k.s.fail(err)
	}
	return connect.NewResponse(&adminv1.ListSigningKeysResponse{Keys: toKeys(list)}), nil
}

func (k *keyService) RotateSigningKeys(ctx context.Context, _ *connect.Request[adminv1.RotateSigningKeysRequest]) (*connect.Response[adminv1.RotateSigningKeysResponse], error) {
	if err := k.s.keys.Rotate(ctx, actor(ctx)); err != nil {
		return nil, k.s.fail(err)
	}
	list, err := k.s.keys.List(ctx)
	if err != nil {
		return nil, k.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RotateSigningKeysResponse{Keys: toKeys(list)}), nil
}

// ---------- Audit ----------

type auditService struct{ s *Server }

func (a *auditService) ListAuditEvents(ctx context.Context, req *connect.Request[adminv1.ListAuditEventsRequest]) (*connect.Response[adminv1.ListAuditEventsResponse], error) {
	size := pageSize(req.Msg.PageSize)
	events, err := audit.List(ctx, a.s.store.Pool, parsePageToken(req.Msg.PageToken), size+1)
	if err != nil {
		return nil, a.s.fail(err)
	}
	resp := &adminv1.ListAuditEventsResponse{}
	if len(events) > size {
		events = events[:size]
		resp.NextPageToken = pageToken(events[size-1].At)
	}
	names := a.s.actorNames(ctx, events)
	for _, e := range events {
		resp.Events = append(resp.Events, toEvent(e, names))
	}
	return connect.NewResponse(resp), nil
}
