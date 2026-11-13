package adminserver

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/errs"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
)

// Every OAuth client belongs to a service. A service's owners manage its
// sign-in clients here; its API credential is managed by RotateServiceSecret.
// The console's own client, and any client from before clients belonged to
// services, belongs to Hayat itself: the built-in platform service, which
// instance admins manage.

// platformService is Hayat itself, listed beside the registered services. It
// has no contract (the platform catalog is code), owners or API credential.
func (r *registryService) platformService(ctx context.Context) (*adminv1.ServiceInfo, error) {
	counts, err := r.s.clients.CountByService(ctx)
	if err != nil {
		return nil, err
	}
	return &adminv1.ServiceInfo{
		Id: catalog.Platform, DisplayName: "Hayat", Builtin: true, SignInClients: int32(counts[catalog.Platform]),
		Description: "Hayat itself. The console signs in through it; instance admins manage it.",
	}, nil
}

// clientAccess resolves who may see and change a service's clients: its owners
// (or anyone who may approve its contracts) see them, only service.manage
// changes them; Hayat's own are an instance admin's. It returns the service's
// API credential, which is never one of its sign-in clients.
func (r *registryService) clientAccess(ctx context.Context, serviceID string) (apiClient string, manage bool, err error) {
	if serviceID == catalog.Platform {
		if err := r.s.requireAdmin(ctx); err != nil {
			return "", false, err
		}
		return "", true, nil
	}
	svc, n, err := r.serviceNode(ctx, serviceID)
	if err != nil {
		return "", false, err
	}
	if manage, err = r.s.may(ctx, catalog.PermServiceManage, n); err != nil {
		return "", false, r.s.fail(err)
	}
	if !manage {
		approve, err := r.s.may(ctx, catalog.PermContractsApprove, n)
		if err != nil {
			return "", false, r.s.fail(err)
		}
		if !approve {
			return "", false, connect.NewError(connect.CodePermissionDenied, errors.New("only this service's owners can open it"))
		}
	}
	return svc.ClientID, manage, nil
}

func (r *registryService) requireClientManage(ctx context.Context, serviceID string) (string, error) {
	api, manage, err := r.clientAccess(ctx, serviceID)
	if err != nil {
		return "", err
	}
	if !manage {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only this service's owners can change its clients"))
	}
	return api, nil
}

// signInClient loads one of the service's sign-in clients. A client of another
// service, or the service's API credential, is not found here.
func (r *registryService) signInClient(ctx context.Context, serviceID, apiClient, clientID string) (*hoidc.Client, error) {
	c, err := r.s.clients.Get(ctx, clientID)
	if err != nil {
		return nil, r.s.fail(err)
	}
	if c.ServiceID != serviceID || c.ID == apiClient {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
	}
	return c, nil
}

// errMachineGrant: a sign-in client signs people in; a service calls Hayat as
// itself with its API credential.
var errMachineGrant = errs.New("a sign-in client can't use client credentials; the service calls Hayat with its API credential")

// checkGrants refuses client credentials on a sign-in client, unless it already
// had them (a client from before clients belonged to services).
func checkGrants(grants, had []string) error {
	if slices.Contains(grants, "client_credentials") && !slices.Contains(had, "client_credentials") {
		return errMachineGrant
	}
	return nil
}

func (r *registryService) ListServiceClients(ctx context.Context, req *connect.Request[adminv1.ListServiceClientsRequest]) (*connect.Response[adminv1.ListServiceClientsResponse], error) {
	api, manage, err := r.clientAccess(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	list, err := r.s.clients.ForService(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, r.s.fail(err)
	}
	resp := &adminv1.ListServiceClientsResponse{CanManage: manage}
	for _, c := range list {
		if c.ID != api {
			resp.Clients = append(resp.Clients, toClient(c))
		}
	}
	return connect.NewResponse(resp), nil
}

func (r *registryService) GetServiceClient(ctx context.Context, req *connect.Request[adminv1.GetServiceClientRequest]) (*connect.Response[adminv1.GetServiceClientResponse], error) {
	api, manage, err := r.clientAccess(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	c, err := r.signInClient(ctx, req.Msg.ServiceId, api, req.Msg.ClientId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.GetServiceClientResponse{Client: toClient(c), CanManage: manage}), nil
}

func (r *registryService) CreateServiceClient(ctx context.Context, req *connect.Request[adminv1.CreateServiceClientRequest]) (*connect.Response[adminv1.CreateServiceClientResponse], error) {
	m := req.Msg
	if _, err := r.requireClientManage(ctx, m.ServiceId); err != nil {
		return nil, err
	}
	if err := checkGrants(m.GrantTypes, nil); err != nil {
		return nil, r.s.fail(err)
	}
	c, secret, err := r.s.clients.Create(ctx, hoidc.ClientInput{ServiceID: m.ServiceId, Name: m.Name, Kind: m.Kind,
		Redirects: m.RedirectUris, PostLogoutRedirects: m.PostLogoutRedirectUris, Grants: m.GrantTypes}, actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.CreateServiceClientResponse{Client: toClient(c), Secret: secret}), nil
}

func (r *registryService) UpdateServiceClient(ctx context.Context, req *connect.Request[adminv1.UpdateServiceClientRequest]) (*connect.Response[adminv1.UpdateServiceClientResponse], error) {
	m := req.Msg
	api, err := r.requireClientManage(ctx, m.ServiceId)
	if err != nil {
		return nil, err
	}
	cur, err := r.signInClient(ctx, m.ServiceId, api, m.ClientId)
	if err != nil {
		return nil, err
	}
	if err := checkGrants(m.GrantTypes, cur.Grants); err != nil {
		return nil, r.s.fail(err)
	}
	c, err := r.s.clients.Update(ctx, cur.ID, hoidc.ClientInput{Name: m.Name, Redirects: m.RedirectUris,
		PostLogoutRedirects: m.PostLogoutRedirectUris, Grants: m.GrantTypes}, actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.UpdateServiceClientResponse{Client: toClient(c)}), nil
}

func (r *registryService) RotateServiceClientSecret(ctx context.Context, req *connect.Request[adminv1.RotateServiceClientSecretRequest]) (*connect.Response[adminv1.RotateServiceClientSecretResponse], error) {
	api, err := r.requireClientManage(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	cur, err := r.signInClient(ctx, req.Msg.ServiceId, api, req.Msg.ClientId)
	if err != nil {
		return nil, err
	}
	secret, err := r.s.clients.RotateSecret(ctx, cur.ID, actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RotateServiceClientSecretResponse{Secret: secret}), nil
}

func (r *registryService) SetServiceClientDisabled(ctx context.Context, req *connect.Request[adminv1.SetServiceClientDisabledRequest]) (*connect.Response[adminv1.SetServiceClientDisabledResponse], error) {
	api, err := r.requireClientManage(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	cur, err := r.signInClient(ctx, req.Msg.ServiceId, api, req.Msg.ClientId)
	if err != nil {
		return nil, err
	}
	c, err := r.s.clients.SetDisabled(ctx, cur.ID, req.Msg.Disabled, actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SetServiceClientDisabledResponse{Client: toClient(c)}), nil
}
