// Package enrollment hands a service its credentials without anyone copying a
// secret. An owner makes a one-time code in the console (or `hayat
// enrollment-code`); the service's deployment is given the code and redeems it
// once on the api role, receiving its API client id and secret and, for a web
// app, the id of a sign-in client registered for its address.
//
// A code is for one service namespace. Redeeming one for a service Hayat
// doesn't know yet registers it, with the code's maker as its first owner (only
// an instance admin can make such a code). Redeeming one for a service that
// exists rotates its API secret: the old one stops working at once. Either way
// the service's own contract is published from what it sends, and its sign-in
// client is made or kept for the address it gives.
//
// The rules: one open code per service (making a new one revokes the last),
// single use, an expiry (5 minutes to 7 days, a day by default), revocable
// until used, and only its SHA-256 is stored. When it's redeemed the maker's
// access is checked again, so a code outlives nobody's permission. A
// redemption that fails gives the code back; one that crashes holds it for
// two minutes.
package enrollment

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/identity"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tree"
)

// Prefix starts every code, so one is recognisable in a config file or a leak scan.
const Prefix = "hye_"

const (
	DefaultTTL = 24 * time.Hour
	MinTTL     = 5 * time.Minute
	MaxTTL     = 7 * 24 * time.Hour
	// claimFor is how long a redemption in progress holds its code.
	claimFor = 2 * time.Minute
)

const (
	StatusOpen    = "open"
	StatusUsed    = "used"
	StatusExpired = "expired"
	StatusRevoked = "revoked"
)

var (
	ErrUnknown = errs.New("this enrollment code isn't valid; check it was copied whole")
	ErrUsed    = errs.New("this enrollment code was already used; make a new one in Hayat's console")
	ErrExpired = errs.New("this enrollment code has expired; make a new one in Hayat's console")
	ErrRevoked = errs.New("this enrollment code was revoked; make a new one in Hayat's console")
	ErrBusy    = errs.New("this enrollment code is being redeemed right now; try again in a minute")
)

// Enrollment is one code, never the code itself.
type Enrollment struct {
	ID           uuid.UUID
	ServiceID    string
	DisplayName  string
	CreatedBy    uuid.UUID
	CreatedAt    time.Time
	ExpiresAt    time.Time
	ClaimedUntil *time.Time
	UsedAt       *time.Time
	RevokedAt    *time.Time
	Registered   bool
	WebClientID  string
}

// Status is where the code stands now: open, used, expired or revoked.
func (e Enrollment) Status(now time.Time) string {
	switch {
	case e.UsedAt != nil:
		return StatusUsed
	case e.RevokedAt != nil:
		return StatusRevoked
	case !now.Before(e.ExpiresAt):
		return StatusExpired
	}
	return StatusOpen
}

const columns = `id, service_id, display_name, created_by, created_at, expires_at, claimed_until, used_at, revoked_at, registered, web_client_id`

func scan(row pgx.Row) (Enrollment, error) {
	var e Enrollment
	err := row.Scan(&e.ID, &e.ServiceID, &e.DisplayName, &e.CreatedBy, &e.CreatedAt, &e.ExpiresAt, &e.ClaimedUntil, &e.UsedAt,
		&e.RevokedAt, &e.Registered, &e.WebClientID)
	return e, store.NotFound(err)
}

func collect(rows pgx.Rows, err error) ([]Enrollment, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Enrollment
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Service owns service_enrollments.
type Service struct {
	store   *store.Store
	cat     *catalog.Service
	clients *hoidc.Clients
	tree    *tree.Service
	engine  *authz.Engine
	ids     *identity.Service
	now     func() time.Time
}

func New(s *store.Store, cat *catalog.Service, clients *hoidc.Clients, t *tree.Service, engine *authz.Engine, ids *identity.Service) *Service {
	return &Service{store: s, cat: cat, clients: clients, tree: t, engine: engine, ids: ids, now: time.Now}
}

// ---------- Making, listing and revoking codes ----------

type MintInput struct {
	ServiceID string
	// The name a new service registers under. Ignored for one that exists.
	DisplayName string
	// 0 means DefaultTTL.
	TTL       time.Duration
	CreatedBy uuid.UUID
}

// Mint makes a code and returns it; it is never shown again. Any other open
// code for the service is revoked, so there is one way in at a time. Who may
// make it is the caller's question: an owner of the service, or an instance
// admin for a service not registered yet.
func (s *Service) Mint(ctx context.Context, in MintInput, actor string) (Enrollment, string, error) {
	if err := catalog.ValidServiceName(in.ServiceID); err != nil {
		return Enrollment{}, "", err
	}
	if in.TTL == 0 {
		in.TTL = DefaultTTL
	}
	if in.TTL < MinTTL || in.TTL > MaxTTL {
		return Enrollment{}, "", errs.New("an enrollment code lasts from 5 minutes to 7 days")
	}
	name := strings.TrimSpace(in.DisplayName)
	if len(name) > 100 {
		return Enrollment{}, "", errs.New("give the service a name of up to 100 characters")
	}
	code := Prefix + crypto.NewToken()
	var e Enrollment
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE service_enrollments SET revoked_at = now()
			WHERE service_id = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now()`, in.ServiceID)
		if err != nil {
			return err
		}
		e, err = scan(tx.QueryRow(ctx, `INSERT INTO service_enrollments (id, service_id, display_name, code_hash, created_by, expires_at)
			VALUES ($1, $2, $3, $4, $5, now() + $6::interval) RETURNING `+columns,
			store.NewID(), in.ServiceID, name, crypto.HashToken(code), in.CreatedBy, fmt.Sprintf("%d seconds", int(in.TTL.Seconds()))))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, s.store.Region, actor, "enrollment.created", "service:"+in.ServiceID,
			map[string]any{"enrollment": e.ID, "expires_at": e.ExpiresAt, "replaced": tag.RowsAffected()})
	})
	if err != nil {
		return Enrollment{}, "", err
	}
	return e, code, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Enrollment, error) {
	return scan(s.store.Pool.QueryRow(ctx, `SELECT `+columns+` FROM service_enrollments WHERE id = $1`, id))
}

// List is a service's codes, newest first.
func (s *Service) List(ctx context.Context, serviceID string) ([]Enrollment, error) {
	return collect(s.store.Pool.Query(ctx, `SELECT `+columns+` FROM service_enrollments WHERE service_id = $1
		ORDER BY created_at DESC LIMIT 50`, serviceID))
}

// Unregistered lists codes for services that aren't registered yet: the ones
// an instance admin made to register a new service, newest first.
func (s *Service) Unregistered(ctx context.Context) ([]Enrollment, error) {
	return collect(s.store.Pool.Query(ctx, `SELECT `+columns+` FROM service_enrollments e
		WHERE NOT EXISTS (SELECT 1 FROM services s WHERE s.id = e.service_id) ORDER BY created_at DESC LIMIT 50`))
}

// Revoke turns an unused code off. Revoking one that is already off is a no-op;
// a used one has nothing left to revoke.
func (s *Service) Revoke(ctx context.Context, id uuid.UUID, actor string) (Enrollment, error) {
	var e Enrollment
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		if e, err = scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM service_enrollments WHERE id = $1 FOR UPDATE`, id)); err != nil {
			return err
		}
		if e.UsedAt != nil {
			return errs.New("this code was already used; to cut the service off, rotate its secret instead")
		}
		if e.RevokedAt != nil {
			return nil
		}
		if e, err = scan(tx.QueryRow(ctx, `UPDATE service_enrollments SET revoked_at = now() WHERE id = $1 RETURNING `+columns, id)); err != nil {
			return err
		}
		return audit.Write(ctx, tx, s.store.Region, actor, "enrollment.revoked", "service:"+e.ServiceID, map[string]any{"enrollment": id})
	})
	return e, err
}

// ---------- Redeeming ----------

type RedeemInput struct {
	Code string
	// The service's contract (YAML or JSON), published when newer than the
	// latest version. Optional.
	Manifest string
	// Used when this registers the service. DisplayName falls back to the
	// code's, then to the namespace.
	DisplayName string
	Description string
	// The service's public address. When set, a public sign-in client is kept
	// with <AppURL>/callback as a redirect.
	AppURL string
	// A role of the service's own namespace (e.g. "tabletsgo.admin") granted at
	// the root to the code's maker when this registers the service: who set it
	// up can run it.
	AdminRole string
	// Who gets AdminRole instead of the code's maker; someone Hayat doesn't
	// know yet is invited.
	AdminEmail string
}

type Result struct {
	ServiceID    string
	ClientID     string
	ClientSecret string
	WebClientID  string
	// Registered says this redemption registered the service; otherwise its
	// secret was rotated.
	Registered      bool
	ContractVersion int
	// "active" or "pending" (it touches another service's types and waits for
	// their owners); empty when nothing was published.
	ContractStatus string
	Warnings       []string
}

// Redeem spends a code and returns the service's credentials. Everything it
// is given is checked before anything is written; a step that fails gives the
// code back, so the service can fix what it sent and try again.
func (s *Service) Redeem(ctx context.Context, in RedeemInput) (res Result, err error) {
	e, err := s.claim(ctx, strings.TrimSpace(in.Code))
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if err != nil {
			s.release(e.ID)
		}
	}()
	actor := "enrollment:" + e.ID.String()
	res.ServiceID = e.ServiceID

	// 1. Check what was sent, and that the code's maker may still do this.
	var manifest *catalog.Manifest
	if strings.TrimSpace(in.Manifest) != "" {
		m, err := catalog.ParseManifest([]byte(in.Manifest))
		if err != nil {
			return Result{}, err
		}
		if m.Service != e.ServiceID {
			return Result{}, errs.Newf("this code is for %q, but the contract is for %q", e.ServiceID, m.Service)
		}
		manifest = &m
	}
	if in.AdminRole != "" && !strings.HasPrefix(in.AdminRole, e.ServiceID+".") {
		return Result{}, errs.Newf("the admin role must be one of %q's own roles", e.ServiceID)
	}
	if in.AdminEmail != "" {
		if in.AdminRole == "" {
			return Result{}, errs.New("name the admin role to grant the admin")
		}
		if _, err := identity.NormalizeEmail(in.AdminEmail); err != nil {
			return Result{}, err
		}
	}
	var base string
	if in.AppURL != "" {
		base = strings.TrimRight(strings.TrimSpace(in.AppURL), "/")
		if u, err := url.Parse(base); err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			return Result{}, errs.Newf("%q isn't a valid address for the service", in.AppURL)
		}
		if err := s.clients.CheckRedirect(base + "/callback"); err != nil {
			return Result{}, err
		}
	}
	maker, err := s.ids.GetUser(ctx, e.CreatedBy)
	if err != nil || maker.Status != identity.StatusActive {
		return Result{}, errs.New("whoever made this code can no longer sign in to Hayat; make a new one")
	}
	svc, err := s.cat.GetService(ctx, e.ServiceID)
	exists := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return Result{}, err
	}
	if exists {
		n, err := s.tree.Get(ctx, svc.NodeID)
		if err != nil {
			return Result{}, err
		}
		ok, _, _, err := s.engine.Check(ctx, maker.ID, catalog.PermServiceManage, n, authz.Consistency{Fresh: true})
		if err != nil {
			return Result{}, err
		}
		if !ok {
			return Result{}, errs.Newf("whoever made this code no longer manages %q; ask one of its owners for a new one", e.ServiceID)
		}
	} else if !maker.IsAdmin() {
		return Result{}, errs.New("only an instance admin's code registers a new service; whoever made this one no longer is")
	}

	// 2. A service Hayat doesn't know yet is registered, owned by the maker.
	registered := e.Registered
	if !exists {
		name := firstNonEmpty(in.DisplayName, e.DisplayName, e.ServiceID)
		client, secret, err := s.clients.Create(ctx, hoidc.ClientInput{ServiceID: e.ServiceID, Name: name + " (service API)",
			Kind: hoidc.KindConfidential, Grants: []string{"client_credentials"}}, actor)
		if err != nil {
			return Result{}, err
		}
		if svc, err = s.cat.RegisterService(ctx, e.ServiceID, name, in.Description, client.ID, maker.ID, actor); err != nil {
			_ = s.clients.Delete(ctx, client.ID)
			return Result{}, err
		}
		// Remembered at once: should a later step fail, the retry finds the
		// service registered and still grants the admin role.
		if _, err := s.store.Pool.Exec(ctx, `UPDATE service_enrollments SET registered = true WHERE id = $1`, e.ID); err != nil {
			return Result{}, err
		}
		registered = true
		res.Registered, res.ClientID, res.ClientSecret = true, client.ID, secret
	}

	// 3. The contract, when it is newer than what Hayat has.
	if manifest != nil {
		cts, err := s.cat.Contracts(ctx, e.ServiceID)
		if err != nil {
			return Result{}, err
		}
		latest := 0
		for _, ct := range cts {
			latest = max(latest, ct.Version)
		}
		if manifest.Version > latest {
			ct, err := s.cat.Publish(ctx, e.ServiceID, []byte(in.Manifest), "service:"+e.ServiceID)
			switch {
			case errors.Is(err, catalog.ErrPendingExists):
				res.Warnings = append(res.Warnings, fmt.Sprintf("contract version %d wasn't published: an earlier version still waits for approval", manifest.Version))
			case err != nil:
				return Result{}, err
			default:
				res.ContractStatus = ct.Status
			}
		}
	}
	if svc, err = s.cat.GetService(ctx, e.ServiceID); err != nil {
		return Result{}, err
	}
	res.ContractVersion = svc.ActiveVersion

	// 4. The sign-in client for the service's address.
	if base != "" {
		id, warn, err := s.webClient(ctx, svc, e, base, actor)
		if err != nil {
			return Result{}, err
		}
		res.WebClientID = id
		if warn != "" {
			res.Warnings = append(res.Warnings, warn)
		}
	}

	// 5. The API credential: new with the registration, else rotated. Last, so
	// nothing above can fail after the old secret stopped working.
	if !res.Registered {
		secret, err := s.clients.RotateSecret(ctx, svc.ClientID, actor)
		if err != nil {
			return Result{}, err
		}
		res.ClientID, res.ClientSecret = svc.ClientID, secret
	}

	// 6. Whoever set the service up can run it. Only when the code registered
	// it: re-enrolling never hands back a role someone took away.
	if registered && in.AdminRole != "" {
		admin := maker.ID
		if in.AdminEmail != "" {
			u, _, err := s.ids.Invite(ctx, in.AdminEmail, "", actor)
			if err != nil {
				return Result{}, err
			}
			admin = u.ID
		}
		if warn := s.grantAdmin(ctx, admin, in.AdminRole, actor); warn != "" {
			res.Warnings = append(res.Warnings, warn)
		}
	}

	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE service_enrollments SET used_at = now(), claimed_until = NULL, web_client_id = $2
			WHERE id = $1`, e.ID, res.WebClientID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, s.store.Region, actor, "service.enrolled", "service:"+e.ServiceID,
			map[string]any{"enrollment": e.ID, "registered": res.Registered, "web_client": res.WebClientID, "contract_version": res.ContractVersion})
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// Preview is what a code would do, for a setup page to show before it is spent.
type Preview struct {
	ServiceID   string
	DisplayName string
	// Registers says redeeming it registers the service; otherwise it rotates
	// the service's API secret.
	Registers bool
	MadeBy    identity.User
	ExpiresAt time.Time
}

// Inspect reads a code without spending it. It refuses a code that couldn't
// be redeemed, for the same reasons Redeem would.
func (s *Service) Inspect(ctx context.Context, code string) (Preview, error) {
	code = strings.TrimSpace(code)
	if !strings.HasPrefix(code, Prefix) {
		return Preview{}, ErrUnknown
	}
	e, err := scan(s.store.Pool.QueryRow(ctx, `SELECT `+columns+` FROM service_enrollments WHERE code_hash = $1`, crypto.HashToken(code)))
	if errors.Is(err, store.ErrNotFound) {
		return Preview{}, ErrUnknown
	}
	if err != nil {
		return Preview{}, err
	}
	if err := statusErr(e.Status(s.now())); err != nil {
		return Preview{}, err
	}
	maker, err := s.ids.GetUser(ctx, e.CreatedBy)
	if err != nil {
		return Preview{}, err
	}
	p := Preview{ServiceID: e.ServiceID, DisplayName: firstNonEmpty(e.DisplayName, e.ServiceID), MadeBy: maker, ExpiresAt: e.ExpiresAt}
	svc, err := s.cat.GetService(ctx, e.ServiceID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		p.Registers = true
	case err != nil:
		return Preview{}, err
	default:
		p.DisplayName = svc.DisplayName
	}
	return p, nil
}

func statusErr(status string) error {
	switch status {
	case StatusUsed:
		return ErrUsed
	case StatusRevoked:
		return ErrRevoked
	case StatusExpired:
		return ErrExpired
	}
	return nil
}

// claim takes the code for one redemption, or says why it can't be had.
func (s *Service) claim(ctx context.Context, code string) (Enrollment, error) {
	if !strings.HasPrefix(code, Prefix) {
		return Enrollment{}, ErrUnknown
	}
	var e Enrollment
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		e, err = scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM service_enrollments WHERE code_hash = $1 FOR UPDATE`, crypto.HashToken(code)))
		if errors.Is(err, store.ErrNotFound) {
			return ErrUnknown
		}
		if err != nil {
			return err
		}
		now := s.now()
		if err := statusErr(e.Status(now)); err != nil {
			return err
		}
		if e.ClaimedUntil != nil && now.Before(*e.ClaimedUntil) {
			return ErrBusy
		}
		_, err = tx.Exec(ctx, `UPDATE service_enrollments SET claimed_until = $2 WHERE id = $1`, e.ID, now.Add(claimFor))
		return err
	})
	return e, err
}

// release gives a code back after a redemption failed. It runs on its own
// context: the request's may be the reason it failed.
func (s *Service) release(id uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.store.Pool.Exec(ctx, `UPDATE service_enrollments SET claimed_until = NULL WHERE id = $1 AND used_at IS NULL`, id)
}

// webClient keeps one public sign-in client with base's callback: one that
// already has it, else the one an earlier enrollment made (the service moved,
// so its new address is added), else a new one.
func (s *Service) webClient(ctx context.Context, svc catalog.ServiceInfo, e Enrollment, base, actor string) (string, string, error) {
	redirect, logout := base+"/callback", []string{base, base + "/"}
	list, err := s.clients.ForService(ctx, svc.ID)
	if err != nil {
		return "", "", err
	}
	disabled := func(c *hoidc.Client) string {
		if c.DisabledAt != nil {
			return fmt.Sprintf("the sign-in client %s is turned off; turn it on in Hayat's console to sign in", c.ID)
		}
		return ""
	}
	for _, c := range list {
		if c.ID != svc.ClientID && c.Kind == hoidc.KindPublic && slices.Contains(c.Redirects, redirect) {
			return c.ID, disabled(c), nil
		}
	}
	var prev string
	err = s.store.Pool.QueryRow(ctx, `SELECT web_client_id FROM service_enrollments WHERE service_id = $1 AND used_at IS NOT NULL
		AND web_client_id <> '' ORDER BY used_at DESC LIMIT 1`, svc.ID).Scan(&prev)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	for _, c := range list {
		if c.ID == prev && c.Kind == hoidc.KindPublic && !c.Builtin {
			updated, err := s.clients.Update(ctx, c.ID, hoidc.ClientInput{Name: c.Name, Redirects: appendNew(c.Redirects, redirect),
				PostLogoutRedirects: appendNew(c.PostLogoutRedirects, logout...), Grants: c.Grants}, actor)
			if err != nil {
				return "", "", err
			}
			return updated.ID, disabled(updated), nil
		}
	}
	name := firstNonEmpty(svc.DisplayName, e.DisplayName, svc.ID)
	c, _, err := s.clients.Create(ctx, hoidc.ClientInput{ServiceID: svc.ID, Name: name + " (web)", Kind: hoidc.KindPublic,
		Redirects: []string{redirect}, PostLogoutRedirects: logout, Grants: []string{"authorization_code", "refresh_token"}}, actor)
	if err != nil {
		return "", "", err
	}
	return c.ID, "", nil
}

// grantAdmin grants the service's admin role at the root. A role the contract
// doesn't define (yet) is a warning, not a failure: the credentials matter more.
func (s *Service) grantAdmin(ctx context.Context, userID uuid.UUID, slug, actor string) string {
	snap, err := s.cat.Snapshot(ctx, true)
	if err != nil {
		return "the admin role wasn't granted: " + err.Error()
	}
	for id, r := range snap.Roles {
		if r.Slug != slug {
			continue
		}
		_, _, err := s.tree.AddGrant(ctx, tree.GrantInput{NodeID: catalog.RootID, PrincipalType: tree.PrincipalUser, PrincipalID: userID,
			RoleID: id, Inherit: true}, actor)
		if err != nil && !errors.Is(err, tree.ErrGrantExists) {
			return fmt.Sprintf("the role %s wasn't granted: %v", slug, err)
		}
		return ""
	}
	return fmt.Sprintf("the role %s wasn't granted: no active contract defines it yet; grant it in Hayat's console", slug)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func appendNew(list []string, add ...string) []string {
	out := slices.Clone(list)
	for _, a := range add {
		if !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out
}
