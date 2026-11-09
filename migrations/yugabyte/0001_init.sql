-- Hayat 0001: identity core (YugabyteDB).
--
-- Differs from postgres/0001 only in index sharding: hashed tenant first, range on path.
--
-- Append-only: never edit this file once shipped. The yugabyte/ history holds
-- the same tables; keep the two in step, changing only what must differ there
-- (index sharding, partitions, tablespaces).
--
-- Regional tables carry `region` first in their primary key. v1 runs one region
-- ("default"). Lookups by id alone go through the small global *_refs tables,
-- because once regional tables are partitioned by region a unique index on them
-- must include region.

-- ---------- Global ----------

CREATE TABLE instance_settings (
  key        text PRIMARY KEY,
  value      jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_refs (
  id          uuid PRIMARY KEY,
  email_lower text NOT NULL UNIQUE,
  region      text NOT NULL
);

CREATE TABLE tenant_refs (
  id     uuid PRIMARY KEY,
  slug   text NOT NULL UNIQUE,
  region text NOT NULL
);

CREATE TABLE node_refs (
  type        text NOT NULL,
  external_id text NOT NULL,
  id          uuid NOT NULL,
  region      text NOT NULL,
  PRIMARY KEY (type, external_id)
);

CREATE TABLE oauth_clients (
  id                        text PRIMARY KEY,
  name                      text NOT NULL,
  kind                      text NOT NULL CHECK (kind IN ('public', 'confidential')),
  secret_hash               bytea,
  redirect_uris             text[] NOT NULL DEFAULT '{}',
  post_logout_redirect_uris text[] NOT NULL DEFAULT '{}',
  grant_types               text[] NOT NULL DEFAULT '{authorization_code,refresh_token}',
  builtin                   boolean NOT NULL DEFAULT false,
  disabled_at               timestamptz,
  created_at                timestamptz NOT NULL DEFAULT now(),
  updated_at                timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE signing_keys (
  kid             text PRIMARY KEY,
  alg             text NOT NULL,
  private_key_enc bytea NOT NULL,
  status          text NOT NULL CHECK (status IN ('next', 'active', 'retired')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  activated_at    timestamptz,
  retired_at      timestamptz
);

CREATE TABLE oidc_code_refs (
  code_hash  bytea PRIMARY KEY,
  region     text NOT NULL,
  request_id uuid NOT NULL,
  expires_at timestamptz NOT NULL
);
CREATE INDEX oidc_code_refs_request ON oidc_code_refs (request_id);

CREATE TABLE worker_leases (
  name       text PRIMARY KEY,
  holder     text NOT NULL,
  expires_at timestamptz NOT NULL
);

-- ---------- Identity (regional by the user's home region) ----------

CREATE TABLE users (
  region            text NOT NULL,
  id                uuid NOT NULL,
  email             text NOT NULL,
  email_verified_at timestamptz,
  name              text NOT NULL DEFAULT '',
  avatar_url        text NOT NULL DEFAULT '',
  locale            text NOT NULL DEFAULT '',
  zoneinfo          text NOT NULL DEFAULT '',
  phone             text NOT NULL DEFAULT '',
  status            text NOT NULL CHECK (status IN ('pending', 'active', 'disabled')),
  system_role       text NOT NULL DEFAULT '' CHECK (system_role IN ('', 'admin')),
  password_hash     text NOT NULL DEFAULT '',
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id)
);
CREATE INDEX users_created ON users (created_at DESC);

CREATE TABLE email_tokens (
  region     text NOT NULL,
  id         uuid NOT NULL,
  user_id    uuid NOT NULL,
  purpose    text NOT NULL CHECK (purpose IN ('verify', 'reset')),
  token_hash bytea NOT NULL,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id),
  UNIQUE (region, token_hash)
);

CREATE TABLE sessions (
  region       text NOT NULL,
  id           uuid NOT NULL,
  user_id      uuid NOT NULL,
  token_hash   bytea NOT NULL,
  amr          text[] NOT NULL DEFAULT '{}',
  ip           text NOT NULL DEFAULT '',
  user_agent   text NOT NULL DEFAULT '',
  auth_time    timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  expires_at   timestamptz NOT NULL,
  revoked_at   timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id),
  UNIQUE (region, token_hash)
);
CREATE INDEX sessions_user ON sessions (user_id);

-- ---------- OIDC (regional) ----------

CREATE TABLE oidc_auth_requests (
  region                text NOT NULL,
  id                    uuid NOT NULL,
  client_id             text NOT NULL,
  redirect_uri          text NOT NULL,
  scopes                text[] NOT NULL DEFAULT '{}',
  state                 text NOT NULL DEFAULT '',
  nonce                 text NOT NULL DEFAULT '',
  response_type         text NOT NULL,
  response_mode         text NOT NULL DEFAULT '',
  code_challenge        text NOT NULL DEFAULT '',
  code_challenge_method text NOT NULL DEFAULT '',
  prompt                text[] NOT NULL DEFAULT '{}',
  max_age               integer,
  login_hint            text NOT NULL DEFAULT '',
  user_id               uuid,
  session_id            uuid,
  auth_time             timestamptz,
  amr                   text[] NOT NULL DEFAULT '{}',
  done                  boolean NOT NULL DEFAULT false,
  created_at            timestamptz NOT NULL DEFAULT now(),
  expires_at            timestamptz NOT NULL,
  PRIMARY KEY (region, id)
);

CREATE TABLE refresh_tokens (
  region     text NOT NULL,
  id         uuid NOT NULL,
  family_id  uuid NOT NULL,
  token_hash bytea NOT NULL,
  client_id  text NOT NULL,
  user_id    uuid NOT NULL,
  session_id uuid,
  scopes     text[] NOT NULL DEFAULT '{}',
  audience   text[] NOT NULL DEFAULT '{}',
  amr        text[] NOT NULL DEFAULT '{}',
  auth_time  timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  rotated_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id),
  UNIQUE (region, token_hash)
);
CREATE INDEX refresh_tokens_family ON refresh_tokens (family_id);
CREATE INDEX refresh_tokens_user ON refresh_tokens (user_id);

-- ---------- Tenants and the tree (regional by the tenant's home region) ----------

CREATE TABLE tenants (
  region     text NOT NULL,
  id         uuid NOT NULL,
  slug       text NOT NULL,
  name       text NOT NULL,
  policy     jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id)
);

CREATE TABLE nodes (
  region      text NOT NULL,
  id          uuid NOT NULL,
  tenant_id   uuid,
  parent_id   uuid,
  type        text NOT NULL,
  external_id text,
  name        text NOT NULL,
  owner_id    uuid,
  path        text NOT NULL,
  depth       smallint NOT NULL CHECK (depth <= 16),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id)
);
CREATE INDEX nodes_subtree ON nodes (tenant_id HASH, path ASC);
CREATE INDEX nodes_parent ON nodes (parent_id);

-- The application root. Global in meaning; it lives in the first region's rows
-- until the tree is geo-partitioned (phase 4).
INSERT INTO nodes (region, id, type, name, path, depth)
VALUES ('default', '00000000-0000-0000-0000-000000000001', 'hayat/root', 'Application', '/root/', 0);

CREATE TABLE group_members (
  region     text NOT NULL,
  group_id   uuid NOT NULL,
  tenant_id  uuid NOT NULL,
  user_id    uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, group_id, user_id)
);
CREATE INDEX group_members_user ON group_members (user_id);
CREATE INDEX group_members_tenant ON group_members (tenant_id HASH, user_id ASC);

CREATE TABLE tenant_revisions (
  region    text NOT NULL,
  tenant_id uuid NOT NULL,
  rev       bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (region, tenant_id)
);

-- ---------- Mail and audit (regional) ----------

CREATE TABLE email_outbox (
  region     text NOT NULL,
  id         uuid NOT NULL,
  to_addr    text NOT NULL,
  subject    text NOT NULL,
  body_text  text NOT NULL,
  attempts   integer NOT NULL DEFAULT 0,
  last_error text NOT NULL DEFAULT '',
  send_after timestamptz NOT NULL DEFAULT now(),
  sent_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id)
);
CREATE INDEX email_outbox_pending ON email_outbox (sent_at, send_after);

CREATE TABLE audit_log (
  region text NOT NULL,
  id     uuid NOT NULL,
  at     timestamptz NOT NULL DEFAULT now(),
  actor  text NOT NULL,
  action text NOT NULL,
  target text NOT NULL DEFAULT '',
  detail jsonb NOT NULL DEFAULT '{}',
  PRIMARY KEY (region, id)
);
CREATE INDEX audit_log_at ON audit_log (at DESC);
