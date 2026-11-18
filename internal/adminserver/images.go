package adminserver

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/media"
)

// swapImage keeps data as owner's new image, points the record at it with
// set, then forgets the image it replaced. Empty data clears the record. If
// set fails, the new image is forgotten and the old one kept.
func (s *Server) swapImage(ctx context.Context, kind media.Kind, owner string, data []byte, old uuid.UUID, set func(id uuid.UUID) error) error {
	next := uuid.Nil
	if len(data) > 0 {
		md, err := s.media.Upload(ctx, kind, owner, data, actor(ctx))
		if err != nil {
			return err
		}
		next = md.ID
	}
	if err := set(next); err != nil {
		if next != uuid.Nil {
			_ = s.media.Delete(ctx, next)
		}
		return err
	}
	if old != uuid.Nil {
		_ = s.media.Delete(ctx, old)
	}
	return nil
}

func idOrNil(raw string) uuid.UUID {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func (a *accountService) SetMyAvatar(ctx context.Context, req *connect.Request[adminv1.SetMyAvatarRequest]) (*connect.Response[adminv1.SetMyAvatarResponse], error) {
	me := caller(ctx).User
	old, ours := a.s.media.IDFromURL(me.AvatarURL)
	if !ours {
		old = uuid.Nil // a picture from elsewhere isn't ours to delete
	}
	var out = me
	err := a.s.swapImage(ctx, media.Avatar, media.Owner("user", me.ID.String()), req.Msg.Data, old, func(id uuid.UUID) error {
		url := ""
		if id != uuid.Nil {
			url = a.s.media.URL(id)
		}
		u, err := a.s.ids.SetAvatar(ctx, me.ID, url, actor(ctx))
		out = u
		return err
	})
	if err != nil {
		return nil, a.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SetMyAvatarResponse{User: toUser(out)}), nil
}

func (t *tenantService) SetTenantLogo(ctx context.Context, req *connect.Request[adminv1.SetTenantLogoRequest]) (*connect.Response[adminv1.SetTenantLogoResponse], error) {
	id, err := t.require(ctx, req.Msg.Id, catalog.PermTenantManage)
	if err != nil {
		return nil, err
	}
	tn, err := t.s.tenants.Get(ctx, id)
	if err != nil {
		return nil, t.s.fail(err)
	}
	err = t.s.swapImage(ctx, media.Logo, media.Owner("tenant", id.String()), req.Msg.Data, idOrNil(tn.Logo), func(logo uuid.UUID) error {
		v := ""
		if logo != uuid.Nil {
			v = logo.String()
		}
		tn, err = t.s.tenants.SetLogo(ctx, id, v, actor(ctx))
		return err
	})
	if err != nil {
		return nil, t.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SetTenantLogoResponse{Tenant: t.s.toTenant(tn)}), nil
}

func (r *registryService) SetServiceLogo(ctx context.Context, req *connect.Request[adminv1.SetServiceLogoRequest]) (*connect.Response[adminv1.SetServiceLogoResponse], error) {
	if req.Msg.ServiceId == catalog.Platform {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("Hayat's own service keeps Hayat's logo"))
	}
	svc, n, err := r.serviceNode(ctx, req.Msg.ServiceId)
	if err != nil {
		return nil, err
	}
	if err := r.s.require(ctx, catalog.PermServiceManage, n); err != nil {
		return nil, err
	}
	err = r.s.swapImage(ctx, media.Logo, media.Owner("service", svc.ID), req.Msg.Data, idOrNil(svc.Logo), func(logo uuid.UUID) error {
		v := ""
		if logo != uuid.Nil {
			v = logo.String()
		}
		svc, err = r.s.cat.SetServiceLogo(ctx, svc.ID, v, actor(ctx))
		return err
	})
	if err != nil {
		return nil, r.s.fail(err)
	}
	info, err := r.toService(ctx, svc)
	if err != nil {
		return nil, r.s.fail(err)
	}
	return connect.NewResponse(&adminv1.SetServiceLogoResponse{Service: info}), nil
}
