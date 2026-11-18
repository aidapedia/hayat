-- Hayat 0009: signing in with another account. internal/federation
-- owns these tables.
--
-- The instance sets up Google, GitHub and one company OpenID Connect provider
-- for everyone (tenant_id NULL); a tenant may connect its own company provider and
-- prove the email domains it may sign people in for.

-- A provider Hayat signs people in through. Global: read on every sign-in.
CREATE TABLE identity_providers (
  id            uuid PRIMARY KEY,
  tenant_id     uuid,                 -- NULL: the instance's own (Google, GitHub)
  kind          text NOT NULL CHECK (kind IN ('google', 'github', 'oidc')),
  name          text NOT NULL,        -- on the button: "Acme SSO"
  issuer        text NOT NULL DEFAULT '',   -- oidc: discovery comes from here
  client_id     text NOT NULL,
  secret_sealed bytea NOT NULL,       -- AES-256-GCM under HAYAT_MASTER_KEY
  enabled       boolean NOT NULL DEFAULT true,
  -- A tenant's provider may create an account for someone new at one of the
  -- tenant's verified domains, and add them to the tenant's roster.
  jit           boolean NOT NULL DEFAULT false,
  created_by    text NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
-- One of each kind for the instance: Google, GitHub, company OIDC.
CREATE UNIQUE INDEX identity_providers_instance_kind ON identity_providers (kind) WHERE tenant_id IS NULL;
CREATE INDEX identity_providers_tenant ON identity_providers (tenant_id);

-- An outside account linked to a Hayat account. Regional: personal data.
CREATE TABLE user_identities (
  region       text NOT NULL,
  id           uuid NOT NULL,
  user_id      uuid NOT NULL,
  provider_id  uuid NOT NULL,
  subject      text NOT NULL,         -- the provider's id for the person, never their email
  email        text NOT NULL DEFAULT '',
  last_used_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id)
);
CREATE INDEX user_identities_user ON user_identities (region, user_id);

-- (provider, subject) → the account, in any region. Global, like user_refs.
CREATE TABLE identity_refs (
  provider_id uuid NOT NULL,
  subject     text NOT NULL,
  user_id     uuid NOT NULL,
  region      text NOT NULL,
  identity_id uuid NOT NULL,
  PRIMARY KEY (provider_id, subject)
);

-- An email domain a tenant claims. Only once a DNS TXT record proves it may
-- the tenant's provider sign people in at it, or force them to.
CREATE TABLE tenant_domains (
  domain       text PRIMARY KEY,      -- lower-cased; one tenant per domain
  tenant_id    uuid NOT NULL,
  region       text NOT NULL,
  verify_token text NOT NULL,
  verified_at  timestamptz,
  provider_id  uuid,                  -- the tenant's provider for this domain
  -- offer: company sign-in alongside the password; enforce: company sign-in only
  policy       text NOT NULL DEFAULT 'offer' CHECK (policy IN ('offer', 'enforce')),
  created_by   text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tenant_domains_tenant ON tenant_domains (tenant_id);

-- One trip to a provider and back: ten minutes, then the worker forgets it.
CREATE TABLE sso_states (
  state_hash      bytea PRIMARY KEY,
  provider_id     uuid NOT NULL,
  auth_request_id text NOT NULL DEFAULT '',  -- the sign-in it finishes, if any
  link_user_id    uuid,                      -- set when linking from Account
  verifier_sealed bytea NOT NULL,            -- PKCE
  expires_at      timestamptz NOT NULL
);
