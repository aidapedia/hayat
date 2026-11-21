package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/enrollment"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/store"
)

// MintEnrollmentCode is `hayat enrollment-code`: the console's "New enrollment
// code" for a Hayat run without its console. The code is made in someone's
// name (by email, or else the first instance admin), with the same rule the
// admin API applies: an owner of the service, or an instance admin for a name
// not registered yet.
func (a *App) MintEnrollmentCode(ctx context.Context, serviceID, displayName, byEmail string, ttl time.Duration) (enrollment.Enrollment, string, identity.User, error) {
	var by identity.User
	var err error
	if strings.TrimSpace(byEmail) != "" {
		if by, err = a.IDs.GetUserByEmail(ctx, byEmail); err != nil {
			return enrollment.Enrollment{}, "", by, fmt.Errorf("%s: no such account", byEmail)
		}
	} else {
		owner, err := a.importOwner(ctx, nil)
		if err != nil {
			return enrollment.Enrollment{}, "", by, errors.New("Hayat has no instance admin yet; create one first (hayat setup-link)")
		}
		if by, err = a.IDs.GetUser(ctx, owner); err != nil {
			return enrollment.Enrollment{}, "", by, err
		}
	}
	if by.Status != identity.StatusActive {
		return enrollment.Enrollment{}, "", by, fmt.Errorf("%s isn't an active account", by.Email)
	}
	svc, err := a.Catalog.GetService(ctx, serviceID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		if !by.IsAdmin() {
			return enrollment.Enrollment{}, "", by, fmt.Errorf("%q isn't registered, and only an instance admin can enroll a new service", serviceID)
		}
	case err != nil:
		return enrollment.Enrollment{}, "", by, err
	default:
		n, err := a.Tree.Get(ctx, svc.NodeID)
		if err != nil {
			return enrollment.Enrollment{}, "", by, err
		}
		ok, _, _, err := a.Engine.Check(ctx, by.ID, catalog.PermServiceManage, n, authz.Consistency{Fresh: true})
		if err != nil {
			return enrollment.Enrollment{}, "", by, err
		}
		if !ok {
			return enrollment.Enrollment{}, "", by, fmt.Errorf("%s doesn't manage %q", by.Email, serviceID)
		}
	}
	e, code, err := a.Enroll.Mint(ctx, enrollment.MintInput{ServiceID: serviceID, DisplayName: displayName, TTL: ttl, CreatedBy: by.ID}, audit.System)
	return e, code, by, err
}
