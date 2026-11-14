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
  services file there. A group is a folder and a list of people at once, and
  groups nest like an org chart: whoever is on Payment (under Engineering) is
  in Engineering too, so a grant to Engineering reaches them. Belonging goes
  up, access goes down: a grant on Payment reaches nothing above it, and a team
  never opens what a sibling team filed. A team lead is a role granted on the
  team's node.
- **Roles and grants**: a role is a set of permissions; a grant gives one to a
  person or group on a node, reaching everything below unless it says otherwise.
  Grants can expire.
- **Decisions**: ownership, inheriting grants, placement under a group, tenant
  membership. Every answer can say why.
- **Service contracts**: each service declares its resource types, permissions
  and suggested roles in a YAML manifest. Versions only add, and go live when
  published — unless they place types under another service's types, which
  waits for that service's owners to approve.
- **Service API** (gRPC, gRPC-Web and Connect on one port): Check, BatchCheck,
  PermissionsAt, LookupResources, LookupSubjects, resource writes, user
  profiles, a user's tenants, a tenant's roster and what each member was granted, a Watch stream of policy
  changes, contract publishing.
- **Import** (`hayat import`): moves another system's people and access in —
  tabletsgo writes the bundle when it hands identity over to Hayat.
- **SDKs**: Go (`sdk/go/hayat`) and Node (`sdk/node`), with client credentials,
  a decision cache emptied by Watch, read-your-writes and token verification.
- **Console**: services and contract approval; roles (each one in plain words
  — what it lets someone do, grouped by service, where it can be granted and
  who holds it — a By service view that sets every permission against every
  role, and a full-page editor that starts from a copy, previews the role live
  and warns when a permission can't take effect where the role can be
  granted); and a Resource Tree page
  (every tenant, its groups and resources; right-click to add a group or move a
  node) with one access list per node (a row per role with a count and faces;
  it opens a modal of everyone it reaches and how, searchable and paged, that
  marks who can be ungranted there and ungrants them together) and rosters
  (a group's own people, then everyone in the groups inside it, "via Payment"). A
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
| `alice@hayat.test` | owns tenant Acme |
| `bob@hayat.test` | head of Engineering: on its roster, Group manager there (so over every team under it) |
| `paula@hayat.test` | Team lead of Engineering › Payment |
| `pete@hayat.test`, `priya@hayat.test` | members of Engineering › Payment |
| `leo@hayat.test` | Team lead of Engineering › Promo |
| `mia@hayat.test` | member of Engineering › Promo |
| `carol@hayat.test` | member of Engineering › Platform, no grants |
| `mona@hayat.test` | Team lead of Marketing |
| `dave@hayat.test` | owns tenant Globex (group Sales) |

Acme is a small org chart. Teams nest under their department, so Payment's,
Promo's and Platform's people are all in Engineering too: open Engineering in
the Resource Tree and they're listed "via Payment" and so on. "Team lead" is a
custom role the seed makes (manage the team's roster, grant on what it files)
and grants on each team; reshaping the tree stays with Group managers.

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

Every OAuth client belongs to a service (see below).

1. In the console, open the service → **Clients → Add sign-in client**. A
   server is *confidential* (gets a secret); a browser, mobile or command-line
   app is *public* (uses PKCE). The service's owners manage its clients.
2. Point the application's OIDC library at the issuer
   (`<issuer>/.well-known/openid-configuration`) with the client id.
3. Verify access tokens offline against `<issuer>/keys`. They carry `sub` and
   `sid` (the Hayat session) and never carry permissions. Their `aud` names
   the client and its service, so a service checking its own namespace
   accepts any of its sign-in clients.

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
       # Placement: everyone in the group a ledger is filed in (sub-teams included),
       # and anyone directly on a group further up, may view it.
       access: { permission: ledger.view, via_placement: true, requires_membership: true }
   permissions:
     - { key: ledger.view, scope: ledger, label: View ledgers, description: See entries and balances }
     - { key: ledger.post, scope: ledger, label: Post entries, description: Post journal entries }
   default_roles:
     - { slug: accountant, name: Accountant, permissions: [ledger.view, ledger.post] }
   ```

   Short names are qualified: `billing/ledger`, `billing.ledger.view`. `label` is
   the short name admins read on the Roles pages (without one they see the key).
   A later version may only add; labels and descriptions are display text and
   may change. A version goes live as soon as it is published. Placing your
   types under another service's types waits until that service's owners
   approve it on their service's page.
3. **Mirror resources and ask for decisions:**

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
   `verifyAccessToken` (or the bundled middleware), passing your namespace
   (`billing`) as the audience, then ask `Check`. Tokens never carry permissions.

## Move a service's own accounts in

A service that kept its own users, tenants and grants hands them over with a
bundle (format `hayat-import`, version 1 — tabletsgo writes one with
`npm run hayat:bundle`, and on its first start after the switch):

```sh
hayat import --file bundle.json --app-url https://db.example.com --out result.json
```

It registers the service if needed (printing its API credentials once),
publishes the bundle's contract, registers a public sign-in client of the service
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

CI (`.github/workflows/quality.yml`) runs on every push and PR: Go vet + tests,
the console's typecheck and build, the SDK's typecheck, and a check that
generated code (`make generate`, `sdk/node/dist`) is committed up to date.

## Releases

1. **Actions → Release → Run workflow** with a version (or
   `gh workflow run release.yml -f version=0.3.0`). It bumps
   `console/package.json` on a `release/v0.3.0` branch and opens a PR into the
   major's release line (`v0.x.x`, created from `main` on the first release). Set a
   `RELEASE_PAT` secret to have it open the PR; without one it links to it.
2. **Merge the PR.** `release-publish.yml` tags the merged commit `v0.3.0`, builds
   `ghcr.io/aidapedia/hayat` and `ghcr.io/aidapedia/hayat-console` for amd64 and
   arm64 (tags `0.3.0`, `0.3`, `latest`) and creates the GitHub Release.

Pushing a `v*.*.*` tag by hand builds and publishes the same way. The binary's
version comes from the tag (`hayat version`); the Node SDK keeps its own.
