package tenancy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/tenancy"
	"github.com/aidapedia/hayat/internal/testdb"
)

func TestTenantLifecycle(t *testing.T) {
	s := testdb.New(t)
	ctx := context.Background()
	ids := identity.New(s, "http://auth.test", time.Hour)
	svc := tenancy.New(s)

	owner, err := ids.CreateAdmin(ctx, "owner@example.com", "Owner", "a long enough password")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := ids.Invite(ctx, "other@example.com", "Other", "test")
	if err != nil {
		t.Fatal(err)
	}

	tn, err := svc.Create(ctx, "Acme Corp", "", owner.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if tn.Slug != "acme-corp" || tn.OwnerID != owner.ID || tn.MemberCount != 1 {
		t.Fatalf("unexpected tenant %+v", tn)
	}
	if _, err := svc.Create(ctx, "Acme again", "acme-corp", owner.ID, "test"); !errors.Is(err, tenancy.ErrSlugTaken) {
		t.Fatalf("duplicate slug: %v", err)
	}
	if _, err := svc.Create(ctx, "Bad", "Bad Slug", owner.ID, "test"); !errors.Is(err, tenancy.ErrBadSlug) {
		t.Fatalf("bad slug: %v", err)
	}

	if err := svc.AddMember(ctx, tn.ID, other.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddMember(ctx, tn.ID, other.ID, "test"); err != nil {
		t.Fatalf("adding twice is not an error: %v", err)
	}
	members, err := svc.Members(ctx, tn.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members: %v %v", members, err)
	}
	page, err := svc.MembersPage(ctx, tn.ID, tenancy.MemberCursor{}, 1)
	if err != nil || len(page) != 1 || page[0].UserID != owner.ID || !page[0].IsOwner {
		t.Fatalf("first page: %v %v", page, err)
	}
	page, err = svc.MembersPage(ctx, tn.ID, tenancy.MemberCursor{JoinedAt: page[0].JoinedAt, UserID: page[0].UserID}, 1)
	if err != nil || len(page) != 1 || page[0].UserID != other.ID {
		t.Fatalf("second page: %v %v", page, err)
	}
	if err := svc.RemoveMember(ctx, tn.ID, owner.ID, "test"); !errors.Is(err, tenancy.ErrOwner) {
		t.Fatalf("removing the owner: %v", err)
	}
	if _, err := svc.TransferOwnership(ctx, tn.ID, other.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveMember(ctx, tn.ID, owner.ID, "test"); err != nil {
		t.Fatalf("the former owner can be removed: %v", err)
	}

	mine, err := svc.ForUser(ctx, other.ID)
	if err != nil || len(mine) != 1 || mine[0].ID != tn.ID {
		t.Fatalf("ForUser: %v %v", mine, err)
	}

	renamed, err := svc.Rename(ctx, tn.ID, "Acme Inc", "test")
	if err != nil || renamed.Name != "Acme Inc" {
		t.Fatalf("rename: %v %v", renamed, err)
	}
	var rev int64
	_ = s.Pool.QueryRow(ctx, `SELECT rev FROM tenant_revisions WHERE tenant_id = $1`, tn.ID).Scan(&rev)
	if rev < 4 {
		t.Fatalf("membership changes must bump the revision, got %d", rev)
	}

	if err := svc.Delete(ctx, tn.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, tn.ID); err == nil {
		t.Fatal("deleted tenant still readable")
	}
}
