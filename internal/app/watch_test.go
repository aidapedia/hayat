package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/admin/v1/adminv1connect"
	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/authz/v1/authzv1connect"
)

// TestWatchHeartbeat: a watcher that asks for heartbeats hears one as soon as
// the stream opens, before any policy change, so a proxy that ends a quiet
// request (Cloudflare, after 100 seconds) never sees one go quiet.
func TestWatchHeartbeat(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := e.a.IDs.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password")
	must(t, err)
	token, _ := e.signIn("root@example.com", "a long enough password")
	registry := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(token))
	reg, err := registry.RegisterService(ctx, connect.NewRequest(&adminv1.RegisterServiceRequest{Id: "billing", DisplayName: "Billing", OwnerEmail: "root@example.com"}))
	must(t, err)

	cc := clientcredentials.Config{ClientID: reg.Msg.ClientId, ClientSecret: reg.Msg.ClientSecret, TokenURL: e.auth + "/oauth/token", AuthStyle: oauth2.AuthStyleInHeader}
	watch := authzv1connect.NewWatchServiceClient(cc.Client(ctx), e.api)

	stream, err := watch.Watch(ctx, connect.NewRequest(&authzv1.WatchRequest{Heartbeat: true}))
	must(t, err)
	defer stream.Close()
	if !stream.Receive() {
		t.Fatalf("want a heartbeat, the stream ended: %v", stream.Err())
	}
	if stream.Msg().Event != nil {
		t.Fatalf("want a heartbeat (no event) first, got %v", stream.Msg().Event)
	}
}
