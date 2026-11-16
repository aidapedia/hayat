package app_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/admin/v1/adminv1connect"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
)

// audience reads a token's `aud` without verifying it; the sign-in already did.
func audience(t *testing.T, token string) []string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Aud any `json:"aud"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	switch v := claims.Aud.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, len(v))
		for i, a := range v {
			out[i], _ = a.(string)
		}
		return out
	}
	return nil
}

// Every OAuth client belongs to a service, and the service's owners manage its
// sign-in clients; the console's own client belongs to Hayat itself.
func TestServiceClients(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	if _, err := e.a.IDs.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	token, _ := e.signIn("root@example.com", "a long enough password")

	// A sign-in token names its client and the client's service.
	if aud := audience(t, token); !slices.Contains(aud, hoidc.ConsoleClientID) || !slices.Contains(aud, hoidc.PlatformService) {
		t.Fatalf("console token aud: %v", aud)
	}

	root := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(token))
	reg, err := root.RegisterService(ctx, connect.NewRequest(&adminv1.RegisterServiceRequest{Id: "billing", DisplayName: "Billing", OwnerEmail: "olga@example.com"}))
	if err != nil {
		t.Fatal(err)
	}

	// An instance admin sees Hayat's own service first, with the console's client.
	list, err := root.ListServices(ctx, connect.NewRequest(&adminv1.ListServicesRequest{}))
	if err != nil || len(list.Msg.Services) != 2 || list.Msg.Services[0].Id != hoidc.PlatformService || !list.Msg.Services[0].Builtin {
		t.Fatalf("services: %v %v", list, err)
	}
	platform, err := root.ListServiceClients(ctx, connect.NewRequest(&adminv1.ListServiceClientsRequest{ServiceId: hoidc.PlatformService}))
	if err != nil || len(platform.Msg.Clients) != 1 || platform.Msg.Clients[0].Id != hoidc.ConsoleClientID {
		t.Fatalf("platform clients: %v %v", platform, err)
	}

	// The owner — not an instance admin — adds a sign-in client to their service.
	olga := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(e.activate("olga@example.com", "olga's long password")))
	if _, err := olga.ListServiceClients(ctx, connect.NewRequest(&adminv1.ListServiceClientsRequest{ServiceId: hoidc.PlatformService})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("owner lists Hayat's clients: want PermissionDenied, got %v", err)
	}
	web, err := olga.CreateServiceClient(ctx, connect.NewRequest(&adminv1.CreateServiceClientRequest{ServiceId: "billing", Name: "Billing web",
		Kind: hoidc.KindPublic, RedirectUris: []string{"https://billing.example.com/callback"}, GrantTypes: []string{"authorization_code", "refresh_token"}}))
	if err != nil || web.Msg.Client.ServiceId != "billing" || web.Msg.Secret != "" {
		t.Fatalf("create: %v %v", web, err)
	}
	if _, err := olga.CreateServiceClient(ctx, connect.NewRequest(&adminv1.CreateServiceClientRequest{ServiceId: "billing", Name: "Billing worker",
		Kind: hoidc.KindConfidential, GrantTypes: []string{"client_credentials"}})); code(err) != connect.CodeFailedPrecondition {
		t.Fatalf("a sign-in client with client credentials: want FailedPrecondition, got %v", err)
	}

	// Its sign-in clients don't include its API credential, and a client is
	// reached only through its own service.
	clients, err := olga.ListServiceClients(ctx, connect.NewRequest(&adminv1.ListServiceClientsRequest{ServiceId: "billing"}))
	if err != nil || !clients.Msg.CanManage || len(clients.Msg.Clients) != 1 || clients.Msg.Clients[0].Id != web.Msg.Client.Id {
		t.Fatalf("billing clients: %v %v", clients, err)
	}
	if _, err := olga.GetServiceClient(ctx, connect.NewRequest(&adminv1.GetServiceClientRequest{ServiceId: "billing", ClientId: reg.Msg.ClientId})); code(err) != connect.CodeNotFound {
		t.Fatalf("the API credential as a sign-in client: want NotFound, got %v", err)
	}
	if _, err := root.GetServiceClient(ctx, connect.NewRequest(&adminv1.GetServiceClientRequest{ServiceId: "billing", ClientId: hoidc.ConsoleClientID})); code(err) != connect.CodeNotFound {
		t.Fatalf("another service's client: want NotFound, got %v", err)
	}
	off, err := olga.SetServiceClientDisabled(ctx, connect.NewRequest(&adminv1.SetServiceClientDisabledRequest{ServiceId: "billing", ClientId: web.Msg.Client.Id, Disabled: true}))
	if err != nil || !off.Msg.Client.Disabled {
		t.Fatalf("turn off: %v %v", off, err)
	}

	// Edit service: its owners rename it; the namespace stays.
	upd, err := olga.UpdateService(ctx, connect.NewRequest(&adminv1.UpdateServiceRequest{Id: "billing", DisplayName: "Billing & invoices", Description: "Invoices"}))
	if err != nil || upd.Msg.Service.DisplayName != "Billing & invoices" || upd.Msg.Service.Id != "billing" || upd.Msg.Service.SignInClients != 1 {
		t.Fatalf("update: %v %v", upd, err)
	}

	// Someone who doesn't own it sees none of this.
	if _, _, err := e.a.IDs.Invite(ctx, "carl@example.com", "Carl", "test"); err != nil {
		t.Fatal(err)
	}
	carl := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(e.activate("carl@example.com", "carl's long password")))
	if _, err := carl.ListServiceClients(ctx, connect.NewRequest(&adminv1.ListServiceClientsRequest{ServiceId: "billing"})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("stranger lists clients: want PermissionDenied, got %v", err)
	}
	if _, err := carl.UpdateService(ctx, connect.NewRequest(&adminv1.UpdateServiceRequest{Id: "billing", DisplayName: "Mine"})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("stranger renames: want PermissionDenied, got %v", err)
	}
}
