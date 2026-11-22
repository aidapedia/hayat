<div align="center">

<img src="console/public/hayat.png" width="80" height="80" alt="Hayat logo" />

# Hayat

**Self-hosted sign-in and tree-based access control for every service you run.**
One account per person, tenants and nested groups, and one answer to "may this person do this, here?"

[![License](https://img.shields.io/badge/license-Apache--2.0-3FAE86)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](go.mod)
[![Docker](https://img.shields.io/badge/image-ghcr.io%2Faidapedia%2Fhayat-2496ED?logo=docker&logoColor=white)](#quick-start)
[![OpenID Connect](https://img.shields.io/badge/OpenID-Connect-F78C40?logo=openid&logoColor=white)](#use-it-from-a-service)

[Documentation](https://github.com/aidapedia/aidapedia/tree/HEAD/apps/hayat/src/content/docs) ·
[Contributing](CONTRIBUTING.md) ·
[Security](SECURITY.md)

</div>

Hayat signs people in (it is an OpenID Connect provider), keeps the tenants and
groups they belong to, and answers "may this subject do this, here?" over gRPC
for every service that publishes a contract.

## Features

- **Single sign-on**: an OpenID Connect provider with PKCE, rotating refresh
  tokens, client credentials and RP-initiated logout.
- **Sign-in methods**: passwords, Google, GitHub and company SSO (Okta, Entra
  ID, Keycloak…), with two-step verification and passkeys.
- **Tenants and nested groups**: organisations shared by every service. Groups
  nest like an org chart: belonging goes up, access goes down.
- **Roles and grants**: a role is a set of permissions, granted to a person or
  group on a node of the resource tree. Every decision can say why.
- **Service contracts**: each service declares its resource types, permissions
  and suggested roles in YAML. Versions only add.
- **Service API and SDKs**: Check, Lookup, resource writes and a Watch stream
  over gRPC / Connect, with Go and Node SDKs that cache decisions.
- **Branded login pages and themes**: each service designs its own sign-in
  page; each person picks a theme that every app follows.
- **Console**: services, roles, the resource tree and who can reach what.
  Tenant owners run their own tenant without being instance admins.

Not yet: multi-region on YugabyteDB, a shared Redis decision cache.

## Quick start

```sh
cd deploy/compose
cp .env.example .env    # set HAYAT_MASTER_KEY: openssl rand -base64 32
docker compose --profile console up -d
docker compose logs hayat-auth | grep setup   # open the link to create the first admin
```

The console is on <http://localhost:3000>, and mail goes to Mailpit at
<http://localhost:8025>. Every setting is listed in
[`deploy/compose/.env.example`](deploy/compose/.env.example).

## Use it from a service

Register the service in the console (**Services → Enroll service**), publish
its contract, then ask for decisions:

```yaml
service: billing
version: 1
resource_types:
  - name: ledger
    parents: [hayat/tenant, hayat/group]
permissions:
  - { key: ledger.view, scope: ledger, label: View ledgers }
default_roles:
  - { slug: accountant, name: Accountant, permissions: [ledger.view] }
```

```ts
const hayat = await Hayat.create({ apiUrl, issuer, clientId, clientSecret })
void hayat.watch()
await hayat.check(userId, 'billing.ledger.view', ref('billing/ledger', id))
```

The documentation covers
[connecting an app](https://github.com/aidapedia/aidapedia/tree/HEAD/apps/hayat/src/content/docs/integrate),
contracts, the SDKs, importing another system's accounts, and running Hayat in
production.

## Develop

Needs Go 1.27, Node 22 with pnpm, and Docker.

```sh
make up     # Postgres, every Hayat role and the console on http://localhost:3000
make test   # Go tests on an embedded Postgres, plus the console typecheck
```

`make up` seeds demo accounts (password `hayat-dev-password`, `make seed` lists
them). [CONTRIBUTING.md](CONTRIBUTING.md) covers the full setup, the rules that
break things silently, and how to open a PR. Everyone taking part follows the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Security

Please report vulnerabilities privately, as described in
[SECURITY.md](SECURITY.md), rather than in a public issue.

## Sponsor

Hayat is free and open source, with nothing behind a paywall. If it helps you,
please consider [sponsoring its development](https://github.com/sponsors/aidapedia).

## License

[Apache License 2.0](LICENSE)
