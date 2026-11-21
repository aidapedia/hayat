package adminserver

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/enrollment"
	"github.com/aidapedia/hayat/internal/store"
)

// Enrollment codes are a service's credentials in waiting, so they are
// guarded like its secret: an owner of the service (hayat.service.manage on
// its node) makes, lists and revokes them; a name not registered yet is an
// instance admin's. Making one steps up, like rotating the secret.

// enrollmentAccess lets the caller manage codes for serviceID.
func (r *registryService) enrollmentAccess(ctx context.Context, serviceID string) error {
	if err := catalog.ValidServiceName(serviceID); err != nil {
		return r.s.fail(err)
	}
	_, err := r.s.cat.GetService(ctx, serviceID)
	if errors.Is(err, store.ErrNotFound) {
		if err := r.s.requireAdmin(ctx); err != nil {
			return connect.NewError(connect.CodePermissionDenied, errors.New("only an instance admin can enroll a new service"))
		}
		return nil
	}
	if err != nil {
		return r.s.fail(err)
	}
	_, n, err := r.serviceNode(ctx, serviceID)
	if err != nil {
		return err
	}
	return r.s.require(ctx, catalog.PermServiceManage, n)
}

func (r *registryService) toEnrollments(ctx context.Context, list []enrollment.Enrollment) ([]*adminv1.Enrollment, error) {
	ids := make([]uuid.UUID, 0, len(list))
	for _, e := range list {
		ids = append(ids, e.CreatedBy)
	}
	users, err := r.s.ids.GetUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	makers := map[uuid.UUID]*adminv1.User{}
	for _, u := range users {
		makers[u.ID] = toUser(u)
	}
	now := time.Now()
	out := make([]*adminv1.Enrollment, 0, len(list))
	for _, e := range list {
		out = append(out, &adminv1.Enrollment{Id: e.ID.String(), ServiceId: e.ServiceID, DisplayName: e.DisplayName,
			CreatedBy: makers[e.CreatedBy], CreatedAt: ts(e.CreatedAt), ExpiresAt: ts(e.ExpiresAt), UsedAt: tsp(e.UsedAt),
			RevokedAt: tsp(e.RevokedAt), Status: e.Status(now), Registered: e.Registered, WebClientId: e.WebClientID})
	}
	return out, nil
}

func (r *registryService) CreateEnrollment(ctx context.Context, req *connect.Request[adminv1.CreateEnrollmentRequest]) (*connect.Response[adminv1.CreateEnrollmentResponse], error) {
	m := req.Msg
	if err := r.enrollmentAccess(ctx, m.ServiceId); err != nil {
		return nil, err
	}
	if err := r.s.stepUp(ctx); err != nil {
		return nil, err
	}
	e, code, err := r.s.enroll.Mint(ctx, enrollment.MintInput{ServiceID: m.ServiceId, DisplayName: m.DisplayName,
		TTL: time.Duration(m.TtlSeconds) * time.Second, CreatedBy: caller(ctx).User.ID}, actor(ctx))
	if err != nil {
		return nil, r.s.fail(err)
	}
	out, err := r.toEnrollments(ctx, []enrollment.Enrollment{e})
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.CreateEnrollmentResponse{Enrollment: out[0], Code: code}), nil
}

func (r *registryService) ListEnrollments(ctx context.Context, req *connect.Request[adminv1.ListEnrollmentsRequest]) (*connect.Response[adminv1.ListEnrollmentsResponse], error) {
	var list []enrollment.Enrollment
	var err error
	if req.Msg.ServiceId == "" {
		if err := r.s.requireAdmin(ctx); err != nil {
			return nil, err
		}
		list, err = r.s.enroll.Unregistered(ctx)
	} else {
		if err := r.enrollmentAccess(ctx, req.Msg.ServiceId); err != nil {
			return nil, err
		}
		list, err = r.s.enroll.List(ctx, req.Msg.ServiceId)
	}
	if err != nil {
		return nil, r.s.fail(err)
	}
	out, err := r.toEnrollments(ctx, list)
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.ListEnrollmentsResponse{Enrollments: out}), nil
}

func (r *registryService) RevokeEnrollment(ctx context.Context, req *connect.Request[adminv1.RevokeEnrollmentRequest]) (*connect.Response[adminv1.RevokeEnrollmentResponse], error) {
	id, err := parseID(req.Msg.Id, "id")
	if err != nil {
		return nil, err
	}
	e, err := r.s.enroll.Get(ctx, id)
	if err != nil {
		return nil, r.s.fail(err)
	}
	if err := r.enrollmentAccess(ctx, e.ServiceID); err != nil {
		return nil, err
	}
	if e, err = r.s.enroll.Revoke(ctx, id, actor(ctx)); err != nil {
		return nil, r.s.fail(err)
	}
	out, err := r.toEnrollments(ctx, []enrollment.Enrollment{e})
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.RevokeEnrollmentResponse{Enrollment: out[0]}), nil
}
