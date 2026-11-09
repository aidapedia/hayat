package adminserver

import (
	"encoding/json"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/identity"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/tenancy"
)

func ts(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func tsp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func toUser(u identity.User) *adminv1.User {
	return &adminv1.User{
		Id: u.ID.String(), Email: u.Email, EmailVerified: u.EmailVerified, Name: u.Name, AvatarUrl: u.AvatarURL,
		Locale: u.Locale, Zoneinfo: u.Zoneinfo, Phone: u.Phone, Status: u.Status, IsAdmin: u.IsAdmin(),
		HasPassword: u.HasPassword, Region: u.Region, CreatedAt: ts(u.CreatedAt), UpdatedAt: ts(u.UpdatedAt),
	}
}

func toSessions(list []identity.Session, current string) []*adminv1.Session {
	out := make([]*adminv1.Session, 0, len(list))
	for _, s := range list {
		out = append(out, &adminv1.Session{
			Id: s.ID.String(), Amr: s.AMR, Ip: s.IP, UserAgent: s.UserAgent, AuthTime: ts(s.AuthTime),
			LastSeenAt: ts(s.LastSeenAt), ExpiresAt: ts(s.ExpiresAt), Current: s.ID.String() == current,
		})
	}
	return out
}

func toTenant(t tenancy.Tenant) *adminv1.Tenant {
	return &adminv1.Tenant{
		Id: t.ID.String(), Slug: t.Slug, Name: t.Name, OwnerId: t.OwnerID.String(), MemberCount: int32(t.MemberCount),
		Region: t.Region, CreatedAt: ts(t.CreatedAt), UpdatedAt: ts(t.UpdatedAt),
	}
}

func toTenants(list []tenancy.Tenant) []*adminv1.Tenant {
	out := make([]*adminv1.Tenant, 0, len(list))
	for _, t := range list {
		out = append(out, toTenant(t))
	}
	return out
}

func toClient(c *hoidc.Client) *adminv1.Client {
	return &adminv1.Client{
		Id: c.ID, Name: c.Name, Kind: c.Kind, RedirectUris: c.Redirects, PostLogoutRedirectUris: c.PostLogoutRedirects,
		GrantTypes: c.Grants, Builtin: c.Builtin, Disabled: c.DisabledAt != nil, CreatedAt: ts(c.CreatedAt), UpdatedAt: ts(c.UpdatedAt),
	}
}

func toKeys(list []hoidc.Key) []*adminv1.SigningKey {
	out := make([]*adminv1.SigningKey, 0, len(list))
	for _, k := range list {
		out = append(out, &adminv1.SigningKey{Id: k.ID, Status: k.Status, CreatedAt: ts(k.CreatedAt),
			ActivatedAt: tsp(k.ActivatedAt), RetiredAt: tsp(k.RetiredAt)})
	}
	return out
}

func toEvent(e audit.Event, names map[string]string) *adminv1.AuditEvent {
	detail, _ := json.Marshal(e.Detail)
	return &adminv1.AuditEvent{Id: e.ID.String(), At: ts(e.At), Actor: e.Actor, ActorName: names[e.Actor],
		Action: e.Action, Target: e.Target, DetailJson: string(detail)}
}

// Page tokens are the created_at of the last row returned, as Unix nanoseconds.
func pageToken(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }

func parsePageToken(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(0, n)
}

func pageSize(n int32) int {
	switch {
	case n <= 0:
		return 50
	case n > 200:
		return 200
	}
	return int(n)
}
