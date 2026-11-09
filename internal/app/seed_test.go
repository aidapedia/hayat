package app_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/aidapedia/hayat/internal/app"
	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/testdb"
)

func newSeedApp(t *testing.T, dev bool) *app.App {
	t.Helper()
	cfg := config.Config{Region: "default", Issuer: "http://localhost:8443", ConsoleURL: consoleURL, MasterKey: make([]byte, 32),
		SessionTTL: time.Hour, AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: time.Hour, Dev: dev}
	a, err := app.New(cfg, testdb.New(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSeed(t *testing.T) {
	ctx := context.Background()
	a := newSeedApp(t, true)

	created, err := a.Seed(ctx)
	if err != nil || !created {
		t.Fatalf("first seed: created=%v err=%v", created, err)
	}
	if created, err := a.Seed(ctx); err != nil || created {
		t.Fatalf("second seed should do nothing: created=%v err=%v", created, err)
	}

	for _, acc := range app.SeedAccounts {
		if _, err := a.IDs.Authenticate(ctx, acc.Email, app.SeedPassword); err != nil {
			t.Fatalf("%s can't sign in: %v", acc.Email, err)
		}
	}
	admin, _ := a.IDs.GetUserByEmail(ctx, "admin@hayat.test")
	if !admin.IsAdmin() {
		t.Fatal("seed admin should be an instance admin")
	}
	bob, _ := a.IDs.GetUserByEmail(ctx, "bob@hayat.test")
	if bob.IsAdmin() {
		t.Fatal("bob should not be an instance admin")
	}
	tenants, err := a.Tenants.ForUser(ctx, bob.ID)
	if err != nil || len(tenants) != 1 || tenants[0].Name != "Acme" {
		t.Fatalf("bob's tenants = %+v, err %v; want Acme", tenants, err)
	}
	if n, _ := a.Tenants.Count(ctx); n != 2 {
		t.Fatalf("tenants = %d, want 2", n)
	}
}

func TestSeedRefusedOutsideDev(t *testing.T) {
	if _, err := newSeedApp(t, false).Seed(context.Background()); err == nil {
		t.Fatal("seed must refuse without HAYAT_DEV")
	}
}
