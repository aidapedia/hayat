# Hayat

Identity and tree-based access control for many services. Hayat signs people in
(it is an OpenID Connect provider), keeps the tenants and groups they belong to,
and — from phase 2 — answers "may this subject do this, here?" over gRPC for every
service that publishes a contract.

Design: <https://claude.ai/artifact/AceR8JGbXjPdNE2mz8WHck>

## What's built

**Phase 1, identity core**
- **OpenID provider**: authorization code with PKCE (S256, required for public
  clients), refresh tokens rotated on every use with reuse detection, client
  credentials, discovery, JWKS, userinfo, introspection, revocation and
  RP-initiated logout. ES256 signing keys rotate on a schedule.
- **Accounts**: sign-up where the password is chosen from the emailed link,
  password reset, sessions with single sign-on across every application, and a
  first-admin setup page guarded by a token printed in the log.
- **Tenants**: organisations shared by every service, with members and an owner.

**Phase 2, authorization**
- **Resource tree**: tenants hold groups; groups hold groups and the resources
  services file there. A group is a folder and a list of people at once.
- **Roles and grants**: a role is a set of permissions; a grant gives one to a
  person or group on a node, reaching everything below unless it says otherwise.
  Grants can expire.
- **Decisions**: ownership, inheriting grants, placement under a group, tenant
  membership. Every answer can say why.
- **Service contracts**: each service declares its resource types, permissions
  and suggested roles in a YAML manifest. Versions only add, and go live once an
  owner of every namespace they touch approves.
- **Service API** (gRPC, gRPC-Web and Connect on one port): Check, BatchCheck,
  PermissionsAt, LookupResources, LookupSubjects, resource writes, user
  profiles, a user's tenants and a tenant's roster, a Watch stream of policy
  changes, contract publishing.
- **Import** (`hayat import`): moves another system's people and access in —
  tabletsgo writes the bundle when it hands identity over to Hayat.
- **SDKs**: Go (`sdk/go/hayat`) and Node (`sdk/node`), with client credentials,
  a decision cache emptied by Watch, read-your-writes and token verification.
- **Console**: services and contract approval, roles, and a Resource Tree page
  (every tenant, its groups and resources; right-click to add a group or move a
  node) with grants, rosters, "who has access" and "check a person". A
  tenant's owner signs in to the same console and runs their tenant (members,
  rename, its Resource Tree) without being an instance admin; members see their
  tenants read-only unless granted more.

Not yet: Google/GitHub and enterprise SSO, MFA (phase 3), multi-region on
YugabyteDB (phase 4), a shared Redis decision cache (each api pod caches in
memory today).

## Run it locally

Needs Go 1.27, Node 22 with pnpm, and Docker (for PostgreSQL). Local runs use the
same `hayat` binary and commands as production: `migrate`, then `serve`.

```sh
make up         # everything: Postgres in Docker on 127.0.0.1:55432, auth :8443,
                # admin :8080, api :7443, console http://localhost:3000, emails in
                # the log. Ctrl+C stops hayat and the console; `make db-down` stops Postgres.
                # On macOS it starts Docker Desktop first if it isn't running.
```

Or one piece per terminal: `make dev` (the backend) and `make console`. The master
key is generated once into `.dev/master.key`.

`make up` (and `make dev`) seeds the empty database once with these accounts,
all with the password `hayat-dev-password`:

| Account | Who |
| --- | --- |
| `admin@hayat.test` | instance admin |
| `alice@hayat.test` | owns tenant Acme (groups Engineering › Backend, Frontend; Marketing) |
| `bob@hayat.test` | Acme member, group manager of Engineering |
| `carol@hayat.test` | Acme member on the Backend roster, no grants |
| `dave@hayat.test` | owns tenant Globex (group Sales) |

`make seed` prints them again. `make db-reset` wipes the local Postgres so the
next `make up` starts from scratch. `hayat seed` refuses to run without
`HAYAT_DEV=true`. On an unseeded instance the first start logs a setup link
instead (`hayat setup-link` prints a new one).

Go tests still need nothing but Go: they run on an embedded PostgreSQL.

## Run it with Docker

```sh
cd deploy/compose
cp .env.example .env    # set HAYAT_MASTER_KEY: openssl rand -base64 32
docker compose up -d                       # without the console
docker compose --profile console up -d     # with it, on :3000
docker compose logs hayat-auth | grep setup
```

Mail goes to Mailpit at <http://localhost:8025>.

## Images and roles

| Image | Role | Port | Does |
| --- | --- | --- | --- |
| `hayat` | `serve --role=auth` | 8443 | OIDC endpoints and the hosted sign-in pages. Public. |
| `hayat` | `serve --role=admin` | 8080 | The admin API the console calls. Keep it internal. |
| `hayat` | `serve --role=api` | 7443 | The service API: decisions, resources, Watch. Service network only. |
| `hayat` | `serve --role=worker` | — | Sends email, cleans up, rotates keys. One active per region. |
| `hayat` | `migrate` | — | Applies migrations; run once before each release. |
| `hayat-console` | nginx | 80 | The optional console; proxies the admin API. |

Every role also serves `/healthz` and `/readyz` on 9090. `serve` without
`--role` runs all four in one process.

## Connect an application

1. In the console, **OAuth clients → Register client**. A server is
   *confidential* (gets a secret); a browser or mobile app is *public* (uses PKCE).
2. Point the application's OIDC library at the issuer
   (`<issuer>/.well-known/openid-configuration`) with the client id.
3. Verify access tokens offline against `<issuer>/keys`. They carry `sub` and
   `sid` (the Hayat session) and never carry permissions.

## Plug a service in

1. **Register it** (instance admin): console → **Services → Register service**.
   You get a namespace (`billing`), API credentials, and a first owner.
2. **Publish its contract**, from CI with the SDK or in the console:

   ```yaml
   service: billing
   version: 1
   resource_types:
     - name: ledger
       parents: [hayat/tenant, hayat/group]
       # Placement: everyone on the roster of a group above a ledger may view it.
       access: { permission: ledger.view, via_placement: true, requires_membership: true }
   permissions:
     - { key: ledger.view, scope: ledger, description: See entries and balances }
     - { key: ledger.post, scope: ledger, description: Post journal entries }
   default_roles:
     - { slug: accountant, name: Accountant, permissions: [ledger.view, ledger.post] }
   ```

   Short names are qualified: `billing/ledger`, `billing.ledger.view`. A later
   version may only add. Placing your types under another service's types needs
   that service's owners to approve too.
3. **An owner approves it** on the service's page.
4. **Mirror resources and ask for decisions:**

   ```go
   hz, _ := hayat.New(ctx, hayat.Config{APIURL: "http://hayat-api:7443", Issuer: issuer,
       ClientID: os.Getenv("HAYAT_CLIENT_ID"), ClientSecret: os.Getenv("HAYAT_CLIENT_SECRET")})
   go hz.Watch(ctx)
   hz.UpsertResource(ctx, hayat.Ref("billing/ledger", id), "Ledger 2026", hayat.Resource{NodeID: groupID}, ownerID)
   ok, _ := hz.Check(ctx, userID, "billing.ledger.post", hayat.Ref("billing/ledger", id))
   ids, _ := hz.Lookup(ctx, userID, "billing.ledger.view", "billing/ledger", tenantID)
   ```

   ```ts
   const hayat = await Hayat.create({ apiUrl, issuer, clientId, clientSecret })
   void hayat.watch()
   await hayat.check(userId, 'billing.ledger.view', ref('billing/ledger', id))
   ```

   Verify your users' Hayat access tokens offline with `VerifyAccessToken` /
   `verifyAccessToken` (or the bundled middleware), then ask `Check`. Tokens
   never carry permissions.

## Move a service's own accounts in

A service that kept its own users, tenants and grants hands them over with a
bundle (format `hayat-import`, version 1 — tabletsgo writes one with
`npm run hayat:bundle`, and on its first start after the switch):

```sh
hayat import --file bundle.json --app-url https://db.example.com --out result.json
```

It registers the service if needed (printing its API credentials once),
publishes and approves the bundle's contract, registers a public sign-in client
for `<app-url>/callback`, then writes people, tenants, groups, rosters, resources,
roles and grants. People and tenants keep the ids they had, so the service's rows
still point at them. A password hash in the old system's unsalted SHA-256 form
(`$sha256$<hex>`) verifies once and is replaced with argon2id on that sign-in.
An address Hayat already knows is matched, not duplicated: `result.json`'s
`idMap` lists the ids that changed, for the service to rewrite. The bundle's
admins become Hayat instance admins only if Hayat has none yet. Running it again
changes nothing.

## Configuration

All settings are environment variables, read in `internal/config/config.go`.
The full list with defaults is in `deploy/compose/.env.example`. Required:
`HAYAT_DATABASE_URL`, `HAYAT_MASTER_KEY`, and an `https://` `HAYAT_ISSUER`
outside development.

## Develop

```sh
make test       # Go tests on an embedded Postgres, plus the console typecheck
make generate   # after editing proto/: Go and TypeScript code (needs buf + plugins)
```

Set `HAYAT_TEST_DATABASE_URL` (and `HAYAT_TEST_DATABASE_ENGINE=yugabyte`) to run
the tests against a server of your own, for example the compose `yugabyte`
profile.
