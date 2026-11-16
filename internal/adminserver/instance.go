package adminserver

import (
	"context"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/audit"
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
