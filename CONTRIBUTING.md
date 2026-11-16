# Contributing to Hayat

Thanks for taking the time to contribute! This guide covers how to get a dev
environment running, the rules the codebase holds itself to, and how to submit
a change.

By participating, you agree to follow our [Code of Conduct](CODE_OF_CONDUCT.md).

---

## Getting started

### Prerequisites

- Go 1.27
- Node.js 22 with pnpm (for the console and the Node SDK)
- Docker (for the local PostgreSQL)
- OpenSSL (`make` generates the dev master key with it)

### Local development

```sh
git clone https://github.com/aidapedia/hayat.git
cd hayat
make up
```

`make up` starts PostgreSQL in Docker on `127.0.0.1:55432`, builds the `hayat`
binary, migrates, seeds a demo org chart, and runs every role (auth `:8443`,
admin `:8080`, api `:7443`) with the console on <http://localhost:3000>. Emails
are written to the log. Ctrl+C stops it. The seeded accounts all use the
password `hayat-dev-password`; `make seed` prints them.

Local runs use the same binary and commands as production (`migrate`, then
`serve`); only `HAYAT_DATABASE_URL`, `HAYAT_MASTER_KEY` (kept in
`.dev/master.key`) and `HAYAT_DEV=true` differ.

Other useful targets:

```sh
make dev        # the backend only
make console    # the console only (run `make dev` first)
make db-reset   # wipe the local Postgres; the next `make up` starts from scratch
make test       # go vet + Go tests (embedded Postgres) + console typecheck
make generate   # after editing proto/: Go and TypeScript code (needs buf + plugins)
make images     # build both Docker images locally
```

### Tests

Go tests need nothing but Go: they run on an embedded PostgreSQL
(`internal/testdb`). To run them against a server of your own, set
`HAYAT_TEST_DATABASE_URL`, and `HAYAT_TEST_DATABASE_ENGINE=yugabyte` for
YugabyteDB (the compose file's `yugabyte` profile starts one).

---

## Project structure

| Path | What it is |
| --- | --- |
| `cmd/hayat/` | The one binary: `serve`, `migrate`, `setup-link`, `rotate-keys`, `seed`, `import`, `version` |
| `internal/` | One package per module; each owns its tables (`identity`, `tenancy`, `tree`, `authz`, `catalog`, `oidc`, …) |
| `internal/config/` | Every environment variable, read once at start-up |
| `migrations/postgres`, `migrations/yugabyte` | The migration history, one per engine |
| `proto/` → `gen/` | The service and admin APIs, and the code generated from them |
| `sdk/go/hayat`, `sdk/node` | The client SDKs for services |
| `console/` | The React console (its own image) |
| `deploy/compose/` | The Docker Compose deployment and `.env.example` |

[`CLAUDE.md`](CLAUDE.md) holds the full list of invariants with the reasoning
behind each. Read it before changing anything in `authz`, `tree`, `catalog`,
`oidc` or the migrations.

---

## Development standards

- Prioritize simplicity and readability over clever solutions. Start with
  minimal functionality, verify it works, then grow it.
- Test with real inputs. Every module has tests; the sign-in flow is tested end
  to end in `internal/authserver` and `internal/app`. A change to behaviour
  comes with a test that would have caught it.
- A module owns its tables. Routes and RPC handlers read as: authorize →
  validate → call a module → respond.
- Match the existing style rather than introducing a new one in the file
  you're touching.

### Rules that break things silently

These are the ones a reviewer will ask about first. `CLAUDE.md` has the rest.

- **Only `internal/config` reads the environment.** A new setting goes there,
  in `deploy/compose/.env.example`, and in the README's configuration notes.
- **YugabyteDB compatibility, from day one.** Ids are UUIDv7
  (`store.NewID`), every transaction goes through `store.Tx`, and there are no
  sequences on busy tables, no advisory locks, no triggers or extensions, and
  no `LISTEN/NOTIFY`-only paths.
- **Migrations are append-only, one history per engine.** Add a new file to
  both `migrations/postgres` and `migrations/yugabyte`; never edit a shipped
  one (the runner refuses to start if you do).
- **Nil Go slices become SQL NULL.** Every array column is `NOT NULL`; pass
  `[]string{}`.
- **The decision rule lives in one place**, `internal/authz`. Union only, no
  deny rules; lookups confirm every candidate with the same rule as Check.
- **Every policy write bumps a revision** in the same transaction, so caches
  and the Watch stream stay honest.
- **A permission exists because something enforces it.** Hayat's own
  (`hayat.*`) are code in `internal/catalog`; a service's come from its
  contract, and contracts only ever add.
- **Tokens never carry permissions.**
- **User-facing errors are `errs.New`.** Anything else is hidden from API
  callers.

### Changing an API

Edit the `.proto` file under `proto/`, run `make generate`, and commit the
generated Go and TypeScript with your change. The service API (`hayat.authz.v1`)
is what other teams build on: add fields and RPCs, never rename or remove them.
If the Node SDK's public surface changes, rebuild it (`cd sdk/node && pnpm
build`) and commit `sdk/node/dist`; CI checks both are up to date.

---

## Versioning

Releases are cut by the **Release** workflow, which sets the version for the
binary, both images and `console/package.json`. Don't bump versions in a
feature PR. The Node SDK keeps its own version in `sdk/node/package.json`;
bump it in the PR that changes the SDK.

---

## Commit & PR workflow

1. Fork the repo and create a branch off `main`.
2. Make your change, following the standards above.
3. Run `make test` and exercise the change locally with `make up` before
   opening a PR. If it touches sign-in, sign in through the console.
4. Update `CLAUDE.md`, `deploy/compose/.env.example` and the README if your
   change affects an invariant, a setting or how Hayat is run.
5. Open a PR against `main` with a clear description of the *why*, not just
   the *what*.
6. Keep PRs focused: one feature or fix per PR is easier to review than a
   bundle of unrelated changes.

CI (`.github/workflows/quality.yml`) runs on every push and PR: Go vet and
tests, the console's typecheck and build, the SDK's typecheck, and a check that
generated code is committed up to date.

### Commit messages

Keep them short and descriptive of intent (see `git log` for examples), e.g.:

```
feat: add tenant and authz
fix: input password with show hide
```

---

## Documentation

The public docs at `apps/hayat/src/content/docs` in
[aidapedia/aidapedia](https://github.com/aidapedia/aidapedia) describe what
this code does. When a change alters what an operator sets or runs, or what a
service calls, open a matching PR there too.

---

## Reporting bugs & requesting features

Please use GitHub Issues. Include:

- What you expected vs. what happened.
- Steps to reproduce, and which role (auth, admin, api, worker) and database
  engine (PostgreSQL or YugabyteDB) were involved.
- Relevant logs, with tokens, secrets and personal data removed.

For security issues, see [SECURITY.md](SECURITY.md) instead of opening a
public issue.

---

## Questions

Open a GitHub Discussion or Issue — happy to help you get oriented.
