# Hayat — agent notes

Identity provider + tree-based RBAC shared by many services. Design and roadmap:
https://claude.ai/artifact/AceR8JGbXjPdNE2mz8WHck. Phases 1 (identity) and 2
(resource tree, contracts, service API, SDKs) are built; phase 3 (social and
enterprise SSO, MFA) is next.

## Standards
- Simple and readable over clever. Start minimal, verify, then grow.
- Test with real inputs: Go tests run on an embedded PostgreSQL (`internal/testdb`).
  Every module has tests; the sign-in flow is tested end to end in
  `internal/authserver` and `internal/app`.
- A module owns its tables. Routes and RPC handlers read as: authorize → validate →
  call a module → respond.

## Invariants
- **Only `internal/config` reads the environment.**
- **YugabyteDB compatibility, from day one.** Regional tables have `region` first in
  their primary key; lookups by id alone go through the global `*_refs` tables;
  ids are UUIDv7 (`store.NewID`); no sequences on busy tables; every transaction
  goes through `store.Tx` (retries 40001); no LISTEN/NOTIFY-only paths, no
  advisory locks (use a row lock or lease row), no triggers or extensions.
- **Migrations are append-only, one history per engine** (`migrations/postgres`,
  `migrations/yugabyte`), edited together; they differ only in index sharding,
  partitions and tablespaces. The runner refuses an edited shipped file.
- **Nil Go slices become SQL NULL.** Every array column is `NOT NULL`; pass `[]string{}`.
- **User-facing errors are `errs.New`.** APIs pass their message through and hide
  everything else.
- **Account rules**: a self-registered account is pending until its mailbox
  answers, and the password is chosen from the emailed link; sign-up and reset
  never reveal whether an address exists; disabling or resetting ends every
  session, and a refresh token dies with its session; the last instance admin
  can't be demoted or disabled.
- **A person's theme is the account's** (`users.appearance`, `identity.Appearance`):
  one per account, edited only through AccountService, read by services through
  `DirectoryService.GetUserAppearance`, never cached by them. Every field is
  optional — empty means "the app's own default". `console/src/shared/lib/appearance.ts`
  and `features/account/AppearanceEditor.tsx` are kept identical with tabletsgo's
  console copies; change both.
- **Tokens never carry permissions.** Access tokens are ES256 JWTs with `sub` and `sid`.
- **Refresh tokens rotate; a reused one revokes its family.** Never let two code
  paths refresh concurrently (the console dedupes `signinSilent`).
- **A tenant is a `hayat/tenant` node; its roster is `group_members`.** The owner is
  the node's `owner_id`, always a member, never removable. Membership changes bump
  `tenant_revisions`. A tenant is deleted only when nothing is filed under it.
- **The decision rule lives in one place**, `internal/authz`: owner short-circuit,
  grants (inherit decides reach below), placement through `hayat/group` ancestors
  only (never the tenant), membership gate for types that require it, instance
  admins get platform (`hayat.*`) permissions only, unknown keys dropped. Union
  only, no deny. Lookups must confirm every candidate with `At`, so they can't
  disagree with Check.
- **Groups nest like an org chart: belonging goes up, access goes down.** Whoever
  is on Payment's roster counts in Engineering above it (`tree.Membership`), so a
  grant *to* Engineering reaches them; a grant *on* Payment reaches nothing above
  it. Placement counts nesting only at the nearest group above a resource (plus
  direct rosters further up), so sibling teams never open each other's things.
  Rosters themselves stay flat (`group_members` is direct only); a team lead is a
  role granted on the team's node, not a roster column. Moving a group needs
  `resources.grant` where it lands.
- **Every policy write bumps a revision** (`revision.Bump`, in the same
  transaction): the tenant's, or the global one for the catalog, roles, root and
  service grants, and account status/role. Caches key on revisions; Watch reads
  `policy_events`.
- **Catalog is code for `hayat.*`, contracts for everything else.** A permission
  exists because a route enforces it. Contracts are additive; a version goes
  live when published, unless it touches another service's namespace — then it
  waits until an owner of each such namespace approves
  (`hayat.contracts.approve` on that service's node). A service's own namespace
  never needs an approval: only its owners, or the service, can publish for it.
- **Every OAuth client belongs to a service** (`oauth_clients.service_id`,
  required). The service's owners manage its sign-in clients through
  RegistryService; its API credential is its own client. The console's client
  belongs to the built-in `hayat` service, which instance admins manage. A
  sign-in token's `aud` names the client and its service.
- **A grant is valid only where it means something**: the role applies to the
  node's type, every permission's scope is at or below the node, a user grantee
  is on the tenant roster, a group grantee is a group of the same tenant or the
  tenant itself ("everyone"). Leaving a tenant removes the person's grants and
  group memberships in it.
- **A service writes only its own types** (`catalog.ServiceOf(type) == service`).
  Mirrored nodes are created, renamed, moved and deleted by their service only.
- **Admin policy calls are allowed by permission, not by `is_admin`**: handlers
  call `s.require(ctx, perm, node)`. Instance admins pass because they hold
  every platform permission. That includes `TenantService`, so a tenant's owner
  runs their tenant and its tree from the console; only creating or deleting a
  tenant needs an instance admin, and ownership is only handed over by the
  owner (or an admin), never taken with a Tenant admin grant. The console
  mirrors it: tenant pages open to members, every action shows only when
  `my_permissions` holds it.
- **The console is optional.** Sign-in pages live in the binary
  (`internal/authserver/templates`), never in the console.
- **The admin API accepts only console tokens** and checks the session live on every call.

## Where things are
```
cmd/hayat/            the one binary: serve --role=auth|admin|api|worker, migrate, setup-link, rotate-keys,
                      seed (dev accounts + sample tenants, HAYAT_DEV only; `make up` runs it),
                      import (another system's bundle — internal/app/import.go)
                      (local runs use it too: `make dev` = compose Postgres + migrate + serve)
internal/config       every env var
internal/store        pgx pool, Tx with retry, UUIDv7, ErrNotFound
internal/migrate      append-only runner over migrations/<engine>
internal/identity     users, emailed links, sessions
internal/tenancy      tenants, the tenant node, members
internal/oidc         zitadel/oidc storage: clients, keys, auth requests, tokens
internal/revision     per-tenant revisions + policy events (Watch)
internal/catalog      node types, permissions, roles, services, contracts
internal/tree         nodes, groups, rosters, grants, resource upserts
internal/authz        the decision engine: At, Check, LookupResources, LookupSubjects
internal/apiserver    the api role: gRPC/Connect for services (proto/hayat/authz/v1)
internal/authserver   the auth role: provider + hosted pages
internal/adminserver  the admin role: Connect handlers for proto/hayat/admin/v1
internal/worker       outbox mail, cleanup, key rotation (leased)
internal/app          wires modules into roles
proto/ → gen/go, console/src/shared/api/gen   (make generate; output is committed)
sdk/go/hayat          Go client: token, cached Check, Watch, VerifyAccessToken
sdk/node              Node client (same features), built with tsc
console/              React + Vite + shadcn/ui, feature-sliced like tabletsgo
deploy/compose/       docker-compose + .env.example
.github/workflows/    quality (go, web, generated-code-is-current), release → release-publish → docker-release (both images)
```

## Console conventions (after tabletsgo)
- `src/app` wiring, `src/shared` (shadcn `ui/`, `lib/`, `hooks/`, `api/`),
  `src/features/<name>` pages and pieces. Cross-feature imports go through a barrel.
- Every page: `PageHeader` (22px title, 13px description), content in the layout's
  one container (max 1280px); `Section` for titled blocks; `Narrow` for forms.
- Sidebar: pinned row + titled groups (10px uppercase headers), 13px rows, 16px
  lucide icons, collapses to a 56px rail. Buttons `text-xs` semibold.
- Anything a user can look at has an address (`/users/:id`, `/services/:id/clients/:clientId`).
- Times go through `shared/lib/datetime` (viewer's account time zone).
- Every table pages, with tabletsgo's footer: `shared/ui/table/Pagination` inside the
  table card (`overflow-hidden rounded-xl border`, pager `className="border-t"`).
  `usePagination(rows)` for a list already loaded, `useServerPagination(fetchPage, deps)`
  for an API paged by `page_token` (no total: it offers the pages reached so far).
- Calls go through `useClient(Service)`; show `errorMessage(err)` in a toast.
- Every "pick one" list is `Combobox` (`shared/ui/combobox`), from Person/Group to
  the time zones: options are `{ value, label, hint?, icon? }`, the search box
  appears at 8+ options, `variant="ghost"` for a toolbar or table footer. There is
  no `Select` — don't add shadcn's back.
