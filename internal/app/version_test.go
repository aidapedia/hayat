package app_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aidapedia/hayat/internal/version"
	"github.com/aidapedia/hayat/sdk/go/hayat"
)

// TestServiceChecksHayatVersion: a service reads Hayat's release without
// credentials and refuses one older than the minimum it declares.
func TestServiceChecksHayatVersion(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// A build from source is new enough for anything.
	if have, err := hayat.CheckVersion(ctx, e.api, "9.9.9", nil); err != nil || have != "dev" {
		t.Fatalf("a dev build: %q %v", have, err)
	}

	was := version.Version
	version.Version = "1.2.0"
	t.Cleanup(func() { version.Version = was })

	if _, err := hayat.CheckVersion(ctx, e.api, "1.2.0", nil); err != nil {
		t.Fatalf("1.2.0 meets 1.2.0: %v", err)
	}
	// A minimum below the floor is raised to it, and 1.2.0 meets the floor.
	if _, err := hayat.CheckVersion(ctx, e.api, "1.0.0", nil); err != nil {
		t.Fatalf("1.0.0 is raised to the floor: %v", err)
	}
	have, err := hayat.CheckVersion(ctx, e.api, "1.3.0", nil)
	var ve *hayat.VersionError
	if !errors.Is(err, hayat.ErrHayatTooOld) || !errors.As(err, &ve) || ve.Have != "1.2.0" || ve.Need != "1.3.0" || have != "1.2.0" {
		t.Fatalf("1.2.0 is older than 1.3.0: %q %v", have, err)
	}

	// New checks too, before it asks for a token: no credentials are needed to
	// be told Hayat is too old.
	_, err = hayat.New(ctx, hayat.Config{APIURL: e.api, Issuer: e.auth, ClientID: "x", ClientSecret: "y", MinHayatVersion: "2.0.0"})
	if !errors.Is(err, hayat.ErrHayatTooOld) {
		t.Fatalf("New must refuse an older Hayat: %v", err)
	}
	// And a service must say what it needs.
	_, err = hayat.New(ctx, hayat.Config{APIURL: e.api, Issuer: e.auth, ClientID: "x", ClientSecret: "y"})
	if err == nil || errors.Is(err, hayat.ErrHayatTooOld) {
		t.Fatalf("MinHayatVersion is required: %v", err)
	}
	if _, err := hayat.CheckVersion(ctx, e.api, "latest", nil); err == nil {
		t.Fatal("a minimum must be a release like 1.2.0")
	}
}

// A Hayat from before 1.2.0 has no ServerInfoService: the service is told it's
// too old rather than given a transport error.
func TestServiceRefusesHayatWithoutVersion(t *testing.T) {
	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	_, err := hayat.CheckVersion(context.Background(), old.URL, "1.2.0", nil)
	var ve *hayat.VersionError
	if !errors.As(err, &ve) || ve.Have != "" {
		t.Fatalf("a Hayat without ServerInfoService is older than the floor: %v", err)
	}
}
