package adminserver

import (
	"context"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/identity"
)

// Two-step verification on the caller's own account. Factors are added on
// Hayat's sign-in pages (a passkey's relying party is the issuer, and an
// authenticator app's secret never passes through the console); here they
// are listed and removed.

func toFactors(list []identity.Factor) []*adminv1.Factor {
	out := make([]*adminv1.Factor, 0, len(list))
	for _, f := range list {
		out = append(out, &adminv1.Factor{Id: f.ID.String(), Kind: f.Kind, Name: f.Name, CreatedAt: ts(f.CreatedAt), LastUsedAt: tsp(f.LastUsedAt)})
	}
	return out
}

func (a *accountService) GetMySecurity(ctx context.Context, _ *connect.Request[adminv1.GetMySecurityRequest]) (*connect.Response[adminv1.GetMySecurityResponse], error) {
	me := caller(ctx).User
	factors, err := a.s.ids.Factors(ctx, me.ID)
	if err != nil {
		return nil, a.s.fail(err)
	}
	left, err := a.s.ids.RecoveryCodesLeft(ctx, me.ID)
	if err != nil {
		return nil, a.s.fail(err)
	}
	required, by, err := a.s.tenants.RequiresMFA(ctx, me.ID)
	if err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.GetMySecurityResponse{
		Factors: toFactors(factors), RecoveryCodesLeft: int32(left), MfaRequired: required, MfaRequiredBy: by,
		SetupUrl: a.s.cfg.Issuer + "/mfa/setup", ReauthUrl: a.s.cfg.Issuer + "/reauth",
	}), nil
}

func (a *accountService) RemoveMyFactor(ctx context.Context, req *connect.Request[adminv1.RemoveMyFactorRequest]) (*connect.Response[adminv1.RemoveMyFactorResponse], error) {
	id, err := parseID(req.Msg.FactorId, "factor_id")
	if err != nil {
		return nil, err
	}
	if err := a.s.stepUp(ctx); err != nil {
		return nil, err
	}
	me := caller(ctx).User
	required, _, err := a.s.tenants.RequiresMFA(ctx, me.ID)
	if err != nil {
		return nil, a.s.fail(err)
	}
	// A passkey can be someone's only way in: no password, no linked account.
	linked, err := a.s.fed.Linked(ctx, me)
	if err != nil {
		return nil, a.s.fail(err)
	}
	keepWayIn := !me.HasPassword && len(linked) == 0
	if err := a.s.ids.RemoveFactor(ctx, me.ID, id, required, keepWayIn, actor(ctx)); err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RemoveMyFactorResponse{}), nil
}

func (a *accountService) RegenerateRecoveryCodes(ctx context.Context, _ *connect.Request[adminv1.RegenerateRecoveryCodesRequest]) (*connect.Response[adminv1.RegenerateRecoveryCodesResponse], error) {
	if err := a.s.stepUp(ctx); err != nil {
		return nil, err
	}
	me := caller(ctx).User
	if has, err := a.s.ids.HasSecondFactor(ctx, me.ID); err != nil {
		return nil, a.s.fail(err)
	} else if !has {
		return nil, invalid("set up an authenticator app or a passkey first")
	}
	codes, err := a.s.ids.NewRecoveryCodes(ctx, me.ID, actor(ctx))
	if err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RegenerateRecoveryCodesResponse{Codes: codes}), nil
}
