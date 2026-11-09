// Package hayat is the Go client for Hayat's service API.
//
//	hz, err := hayat.New(ctx, hayat.Config{
//		APIURL: "http://hayat-api:7443", Issuer: "https://auth.example.com",
//		ClientID: os.Getenv("HAYAT_CLIENT_ID"), ClientSecret: os.Getenv("HAYAT_CLIENT_SECRET"),
//	})
//	go hz.Watch(ctx) // keep the decision cache honest
//	ok, err := hz.Check(ctx, userID, "billing.ledger.view", hayat.Ref("billing/ledger", ledgerID))
//
// It owns what every service would otherwise write by hand: the client
// credentials token, a short-lived decision cache emptied per tenant by the
// Watch stream, read-your-writes after a resource write, and offline
// verification of user access tokens.
package hayat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/authz/v1/authzv1connect"
)

type Config struct {
	// APIURL is the api role, e.g. http://hayat-api:7443.
	APIURL string
	// Issuer is the auth role's public URL; tokens and keys are discovered from it.
	Issuer       string
	ClientID     string
	ClientSecret string
	// CacheTTL bounds how long a decision is reused. Default 10s; negative
	// turns the cache off.
	CacheTTL time.Duration
	// HTTPClient carries API calls. Default: http.DefaultClient (the Connect
	// protocol over HTTP/1.1 or HTTP/2).
	HTTPClient *http.Client
}

type Client struct {
	Authz     authzv1connect.AuthzServiceClient
	Tree      authzv1connect.TreeServiceClient
	Directory authzv1connect.DirectoryServiceClient
	Schema    authzv1connect.SchemaServiceClient
	watch     authzv1connect.WatchServiceClient

	cfg   Config
	cache *cache
	keys  *keySet

	mu        sync.Mutex
	lastRev   int64
	lastWrite time.Time
}

type discovery struct {
	TokenEndpoint string `json:"token_endpoint"`
	JWKSURI       string `json:"jwks_uri"`
	Issuer        string `json:"issuer"`
}

func discover(ctx context.Context, hc *http.Client, issuer string) (discovery, error) {
	var d discovery
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(issuer, "/")+"/.well-known/openid-configuration", nil)
	resp, err := hc.Do(req)
	if err != nil {
		return d, fmt.Errorf("hayat: discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return d, fmt.Errorf("hayat: discovery answered %d", resp.StatusCode)
	}
	return d, json.NewDecoder(resp.Body).Decode(&d)
}

// New discovers the issuer and prepares the API clients. It makes no API call.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.APIURL == "" || cfg.Issuer == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("hayat: APIURL, Issuer, ClientID and ClientSecret are required")
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 10 * time.Second
	}
	base := cfg.HTTPClient
	if base == nil {
		base = http.DefaultClient
	}
	d, err := discover(ctx, base, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	cc := clientcredentials.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, TokenURL: d.TokenEndpoint,
		AuthStyle: oauth2.AuthStyleInHeader}
	authed := &http.Client{Transport: &oauth2.Transport{
		Source: cc.TokenSource(context.WithValue(context.Background(), oauth2.HTTPClient, base)),
		Base:   base.Transport,
	}}
	api := strings.TrimRight(cfg.APIURL, "/")
	return &Client{
		Authz:     authzv1connect.NewAuthzServiceClient(authed, api),
		Tree:      authzv1connect.NewTreeServiceClient(authed, api),
		Directory: authzv1connect.NewDirectoryServiceClient(authed, api),
		Schema:    authzv1connect.NewSchemaServiceClient(authed, api),
		watch:     authzv1connect.NewWatchServiceClient(authed, api),
		cfg:       cfg,
		cache:     newCache(cfg.CacheTTL),
		keys:      &keySet{url: d.JWKSURI, issuer: d.Issuer, hc: base},
	}, nil
}

// Resource names a node: one of your resources by type and id, or a tenant
// with Ref("hayat/tenant", tenantID).
type Resource struct {
	Type       string
	ExternalID string
	NodeID     string
}

func Ref(typ, id string) Resource { return Resource{Type: typ, ExternalID: id} }

func (r Resource) proto() *authzv1.ResourceRef {
	return &authzv1.ResourceRef{Type: r.Type, ExternalId: r.ExternalID, NodeId: r.NodeID}
}

func (r Resource) key() string { return r.Type + "\x00" + r.ExternalID + "\x00" + r.NodeID }

// Check answers whether a user holds a permission on a resource. Answers are
// cached for CacheTTL and dropped as soon as Watch reports a change in the
// tenant. An unknown resource is a "no", not an error.
func (c *Client) Check(ctx context.Context, userID, permission string, res Resource) (bool, error) {
	key := userID + "\x00" + permission + "\x00" + res.key()
	if v, ok := c.cache.get(key); ok {
		return v, nil
	}
	resp, err := c.Authz.Check(ctx, connect.NewRequest(&authzv1.CheckRequest{UserId: userID, Permission: permission,
		Resource: res.proto(), Consistency: c.consistency(res)}))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return false, nil
		}
		return false, err
	}
	c.cache.put(key, resp.Msg.TenantId, resp.Msg.Allowed)
	return resp.Msg.Allowed, nil
}

// consistency asks, for a moment after a write, for at least the revision that
// write produced, so a resource this client just created is visible to its
// next check. After that window the server's own caching is fresh enough.
func (c *Client) consistency(Resource) *authzv1.Consistency {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.lastWrite) < 2*time.Second {
		return &authzv1.Consistency{AtLeastRev: c.lastRev}
	}
	return nil
}

// UpsertResource mirrors one of your resources into the tree under parent.
func (c *Client) UpsertResource(ctx context.Context, res Resource, name string, parent Resource, ownerUserID string) (*authzv1.Resource, error) {
	resp, err := c.Tree.UpsertResource(ctx, connect.NewRequest(&authzv1.UpsertResourceRequest{Type: res.Type, ExternalId: res.ExternalID,
		Name: name, Parent: parent.proto(), OwnerUserId: ownerUserID}))
	if err != nil {
		return nil, err
	}
	c.wrote(resp.Msg.TenantId, resp.Msg.Rev)
	return resp.Msg.Resource, nil
}

// DeleteResource removes the node of one of your resources.
func (c *Client) DeleteResource(ctx context.Context, res Resource) error {
	resp, err := c.Tree.DeleteResource(ctx, connect.NewRequest(&authzv1.DeleteResourceRequest{Type: res.Type, ExternalId: res.ExternalID}))
	if err != nil {
		return err
	}
	c.wrote(resp.Msg.TenantId, resp.Msg.Rev)
	return nil
}

func (c *Client) wrote(tenant string, rev int64) {
	c.mu.Lock()
	c.lastRev, c.lastWrite = rev, time.Now()
	c.mu.Unlock()
	c.cache.dropTenant(tenant)
}

// Lookup lists the ids of resources of one type where the user holds permission.
func (c *Client) Lookup(ctx context.Context, userID, permission, typ, tenantID string) ([]string, error) {
	var out []string
	token := ""
	for {
		resp, err := c.Authz.LookupResources(ctx, connect.NewRequest(&authzv1.LookupResourcesRequest{UserId: userID, Permission: permission,
			Type: typ, TenantId: tenantID, PageSize: 500, PageToken: token}))
		if err != nil {
			return nil, err
		}
		for _, r := range resp.Msg.Resources {
			out = append(out, r.ExternalId)
		}
		if token = resp.Msg.NextPageToken; token == "" {
			return out, nil
		}
	}
}

// Watch streams policy changes and drops cached decisions for each changed
// tenant, reconnecting until ctx ends. Run it in a goroutine.
func (c *Client) Watch(ctx context.Context) {
	cursor := ""
	backoff := time.Second
	for ctx.Err() == nil {
		stream, err := c.watch.Watch(ctx, connect.NewRequest(&authzv1.WatchRequest{Cursor: cursor}))
		if err == nil {
			for stream.Receive() {
				e := stream.Msg().Event
				cursor = e.Id
				if e.TenantId == globalTenant {
					c.cache.dropAll()
				} else {
					c.cache.dropTenant(e.TenantId)
				}
				backoff = time.Second
			}
			_ = stream.Close()
		}
		// Whatever happened while disconnected is unknown: start clean.
		c.cache.dropAll()
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

const globalTenant = "00000000-0000-0000-0000-000000000000"

// ---------- cache ----------

type entry struct {
	allowed bool
	tenant  string
	expires time.Time
}

type cache struct {
	ttl time.Duration
	mu  sync.Mutex
	m   map[string]entry
}

func newCache(ttl time.Duration) *cache { return &cache{ttl: ttl, m: map[string]entry{}} }

func (c *cache) get(key string) (bool, bool) {
	if c.ttl < 0 {
		return false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || time.Now().After(e.expires) {
		return false, false
	}
	return e.allowed, true
}

func (c *cache) put(key, tenant string, allowed bool) {
	if c.ttl < 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > 100_000 {
		c.m = map[string]entry{}
	}
	c.m[key] = entry{allowed: allowed, tenant: tenant, expires: time.Now().Add(c.ttl)}
}

func (c *cache) dropTenant(tenant string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.m {
		if e.tenant == tenant {
			delete(c.m, k)
		}
	}
}

func (c *cache) dropAll() {
	c.mu.Lock()
	c.m = map[string]entry{}
	c.mu.Unlock()
}
