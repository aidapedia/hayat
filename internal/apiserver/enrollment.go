package apiserver

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/internal/enrollment"
)

// enrollmentService is served without the service-token interceptor: a
// service calling it has no credentials yet, only the code an owner made.
// A code is 256 random bits, so guessing one isn't a way in.
type enrollmentService struct{ s *Server }

func (es *enrollmentService) InspectEnrollment(ctx context.Context, req *connect.Request[authzv1.InspectEnrollmentRequest]) (*connect.Response[authzv1.InspectEnrollmentResponse], error) {
	p, err := es.s.enroll.Inspect(ctx, req.Msg.Code)
	if err != nil {
		return nil, es.s.fail(err)
	}
	return connect.NewResponse(&authzv1.InspectEnrollmentResponse{ServiceId: p.ServiceID, DisplayName: p.DisplayName,
		Registers: p.Registers, MadeByEmail: p.MadeBy.Email, MadeByName: p.MadeBy.Name,
		ExpiresAt: timestamppb.New(p.ExpiresAt), Issuer: es.s.cfg.Issuer}), nil
}

func (es *enrollmentService) Enroll(ctx context.Context, req *connect.Request[authzv1.EnrollRequest]) (*connect.Response[authzv1.EnrollResponse], error) {
	m := req.Msg
	res, err := es.s.enroll.Redeem(ctx, enrollment.RedeemInput{Code: m.Code, Manifest: m.Manifest, DisplayName: m.DisplayName,
		Description: m.Description, AppURL: m.AppUrl, AdminRole: m.AdminRole, AdminEmail: m.AdminEmail})
	if err != nil {
		return nil, es.s.fail(err)
	}
	// The service's next call presents the new secret; forget the cached
	// lookup so nothing about the old registration lingers.
	es.s.mu.Lock()
	delete(es.s.services, res.ClientID)
	es.s.mu.Unlock()
	return connect.NewResponse(&authzv1.EnrollResponse{ServiceId: res.ServiceID, ClientId: res.ClientID, ClientSecret: res.ClientSecret,
		WebClientId: res.WebClientID, Issuer: es.s.cfg.Issuer, Registered: res.Registered, ContractVersion: int32(res.ContractVersion),
		ContractStatus: res.ContractStatus, Warnings: res.Warnings}), nil
}
