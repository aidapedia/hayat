package adminserver

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/federation"
)

type signInService struct{ s *Server }

func toProvider(p federation.Provider) *adminv1.SignInProvider {
	out := &adminv1.SignInProvider{Id: p.ID.String(), Kind: p.Kind, Name: p.Name, Issuer: p.Issuer, ClientId: p.ClientID,
		HasSecret: p.HasSecret, Enabled: p.Enabled, Jit: p.JIT, UpdatedAt: ts(p.UpdatedAt)}
	if p.TenantID != nil {
		out.TenantId = p.TenantID.String()
	}
	return out
}

func toDomain(d federation.Domain) *adminv1.TenantDomain {
	out := &adminv1.TenantDomain{Domain: d.Domain, Verified: d.Verified(), VerifiedAt: tsp(d.VerifiedAt),
		TxtName: d.TXTName(), TxtValue: d.TXTValue(), Policy: d.Policy}
	if d.ProviderID != nil {
		out.ProviderId = d.ProviderID.String()
	}
	return out
}

// tenant checks hayat.tenant.manage on the tenant and answers its id.
func (si *signInService) tenant(ctx context.Context, raw string) (uuid.UUID, error) {
	return (&tenantService{si.s}).require(ctx, raw, catalog.PermTenantManage)
}

func optionalID(raw, what string) (*uuid.UUID, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	id, err := parseID(raw, what)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (si *signInService) ListInstanceProviders(ctx context.Context, _ *connect.Request[adminv1.ListInstanceProvidersRequest]) (*connect.Response[adminv1.ListInstanceProvidersResponse], error) {
	if err := si.s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	list, err := si.s.fed.InstanceProviders(ctx)
	if err != nil {
		return nil, si.s.fail(err)
	}
	resp := &adminv1.ListInstanceProvidersResponse{CallbackUrl: si.s.fed.CallbackURL()}
	for _, p := range list {
		resp.Providers = append(resp.Providers, toProvider(p))
	}
	return connect.NewResponse(resp), nil
}

func (si *signInService) SaveInstanceProvider(ctx context.Context, req *connect.Request[adminv1.SaveInstanceProviderRequest]) (*connect.Response[adminv1.SaveInstanceProviderResponse], error) {
	if err := si.s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	m := req.Msg
	p, err := si.s.fed.SaveInstanceProvider(ctx, m.Kind, federation.ProviderInput{Name: m.Name, Issuer: m.Issuer, ClientID: m.ClientId,
		ClientSecret: m.ClientSecret, Enabled: m.Enabled}, actor(ctx))
	if err != nil {
		return nil, si.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SaveInstanceProviderResponse{Provider: toProvider(p)}), nil
}

func (si *signInService) GetTenantSignIn(ctx context.Context, req *connect.Request[adminv1.GetTenantSignInRequest]) (*connect.Response[adminv1.GetTenantSignInResponse], error) {
	id, err := si.tenant(ctx, req.Msg.TenantId)
	if err != nil {
		return nil, err
	}
	providers, err := si.s.fed.TenantProviders(ctx, id)
	if err != nil {
		return nil, si.s.fail(err)
	}
	domains, err := si.s.fed.TenantDomains(ctx, id)
	if err != nil {
		return nil, si.s.fail(err)
	}
	resp := &adminv1.GetTenantSignInResponse{CallbackUrl: si.s.fed.CallbackURL()}
	for _, p := range providers {
		resp.Providers = append(resp.Providers, toProvider(p))
	}
	for _, d := range domains {
		resp.Domains = append(resp.Domains, toDomain(d))
	}
	return connect.NewResponse(resp), nil
}

func (si *signInService) SaveTenantProvider(ctx context.Context, req *connect.Request[adminv1.SaveTenantProviderRequest]) (*connect.Response[adminv1.SaveTenantProviderResponse], error) {
	m := req.Msg
	id, err := si.tenant(ctx, m.TenantId)
	if err != nil {
		return nil, err
	}
	pid, err := optionalID(m.ProviderId, "provider_id")
	if err != nil {
		return nil, err
	}
	p, err := si.s.fed.SaveTenantProvider(ctx, id, pid, federation.ProviderInput{Name: m.Name, Issuer: m.Issuer, ClientID: m.ClientId,
		ClientSecret: m.ClientSecret, Enabled: m.Enabled, JIT: m.Jit}, actor(ctx))
	if err != nil {
		return nil, si.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SaveTenantProviderResponse{Provider: toProvider(p)}), nil
}

func (si *signInService) DeleteTenantProvider(ctx context.Context, req *connect.Request[adminv1.DeleteTenantProviderRequest]) (*connect.Response[adminv1.DeleteTenantProviderResponse], error) {
	id, err := si.tenant(ctx, req.Msg.TenantId)
	if err != nil {
		return nil, err
	}
	pid, err := parseID(req.Msg.ProviderId, "provider_id")
	if err != nil {
		return nil, err
	}
	if err := si.s.fed.DeleteTenantProvider(ctx, id, pid, actor(ctx)); err != nil {
		return nil, si.s.fail(err)
	}
	return connect.NewResponse(&adminv1.DeleteTenantProviderResponse{}), nil
}

func (si *signInService) AddTenantDomain(ctx context.Context, req *connect.Request[adminv1.AddTenantDomainRequest]) (*connect.Response[adminv1.AddTenantDomainResponse], error) {
	id, err := si.tenant(ctx, req.Msg.TenantId)
	if err != nil {
		return nil, err
	}
	d, err := si.s.fed.AddDomain(ctx, id, req.Msg.Domain, actor(ctx))
	if err != nil {
		return nil, si.s.fail(err)
	}
	return connect.NewResponse(&adminv1.AddTenantDomainResponse{Domain: toDomain(d)}), nil
}

func (si *signInService) VerifyTenantDomain(ctx context.Context, req *connect.Request[adminv1.VerifyTenantDomainRequest]) (*connect.Response[adminv1.VerifyTenantDomainResponse], error) {
	id, err := si.tenant(ctx, req.Msg.TenantId)
	if err != nil {
		return nil, err
	}
	d, err := si.s.fed.VerifyDomain(ctx, id, req.Msg.Domain, actor(ctx))
	if err != nil {
		return nil, si.s.fail(err)
	}
	return connect.NewResponse(&adminv1.VerifyTenantDomainResponse{Domain: toDomain(d)}), nil
}

func (si *signInService) SetTenantDomainSignIn(ctx context.Context, req *connect.Request[adminv1.SetTenantDomainSignInRequest]) (*connect.Response[adminv1.SetTenantDomainSignInResponse], error) {
	id, err := si.tenant(ctx, req.Msg.TenantId)
	if err != nil {
		return nil, err
	}
	pid, err := optionalID(req.Msg.ProviderId, "provider_id")
	if err != nil {
		return nil, err
	}
	d, err := si.s.fed.SetDomainSignIn(ctx, id, req.Msg.Domain, pid, req.Msg.Policy, actor(ctx))
	if err != nil {
		return nil, si.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SetTenantDomainSignInResponse{Domain: toDomain(d)}), nil
}

func (si *signInService) RemoveTenantDomain(ctx context.Context, req *connect.Request[adminv1.RemoveTenantDomainRequest]) (*connect.Response[adminv1.RemoveTenantDomainResponse], error) {
	id, err := si.tenant(ctx, req.Msg.TenantId)
	if err != nil {
		return nil, err
	}
	if err := si.s.fed.RemoveDomain(ctx, id, req.Msg.Domain, actor(ctx)); err != nil {
		return nil, si.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RemoveTenantDomainResponse{}), nil
}

func (si *signInService) ListMySignInMethods(ctx context.Context, _ *connect.Request[adminv1.ListMySignInMethodsRequest]) (*connect.Response[adminv1.ListMySignInMethodsResponse], error) {
	me := caller(ctx).User
	linked, err := si.s.fed.Linked(ctx, me)
	if err != nil {
		return nil, si.s.fail(err)
	}
	resp := &adminv1.ListMySignInMethodsResponse{HasPassword: me.HasPassword}
	have := map[uuid.UUID]bool{}
	for _, l := range linked {
		have[l.Provider.ID] = true
		resp.Linked = append(resp.Linked, &adminv1.LinkedAccount{Id: l.ID.String(), ProviderName: l.Provider.Name, ProviderKind: l.Provider.Kind,
			Email: l.Email, CreatedAt: ts(l.CreatedAt), LastUsedAt: tsp(l.LastUsedAt)})
	}
	providers, err := si.s.fed.InstanceProviders(ctx)
	if err != nil {
		return nil, si.s.fail(err)
	}
	for _, p := range providers {
		if p.Enabled && !have[p.ID] {
			resp.Available = append(resp.Available, &adminv1.LinkableProvider{Id: p.ID.String(), Name: p.Name, Kind: p.Kind,
				LinkUrl: si.s.cfg.Issuer + "/sso/link/" + p.ID.String()})
		}
	}
	return connect.NewResponse(resp), nil
}

func (si *signInService) UnlinkMyAccount(ctx context.Context, req *connect.Request[adminv1.UnlinkMyAccountRequest]) (*connect.Response[adminv1.UnlinkMyAccountResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	if err := si.s.fed.Unlink(ctx, caller(ctx).User, id); err != nil {
		return nil, si.s.fail(err)
	}
	return connect.NewResponse(&adminv1.UnlinkMyAccountResponse{}), nil
}
