-- Hayat 0010: account security — second factors, passkeys, recovery codes,
-- lockout and step-up. internal/identity owns these tables, except
-- tenants.require_mfa (internal/tenancy).

-- A second factor or passkey a person has set up. Regional: personal data.
--   totp:    secret_sealed is the shared secret, AES-256-GCM under HAYAT_MASTER_KEY;
--            last_step is the newest 30-second step accepted, so a code is never
--            accepted twice.
--   passkey: credential is the WebAuthn credential (public key, flags, sign
--            count) as JSON; credential_id is its raw id.
CREATE TABLE user_factors (
  region        text NOT NULL,
  id            uuid NOT NULL,
  user_id       uuid NOT NULL,
  kind          text NOT NULL CHECK (kind IN ('totp', 'passkey')),
  name          text NOT NULL DEFAULT '',
  secret_sealed bytea NOT NULL DEFAULT '\x',
  last_step     bigint NOT NULL DEFAULT 0,
  credential_id bytea NOT NULL DEFAULT '\x',
  credential    jsonb NOT NULL DEFAULT '{}',
  confirmed     boolean NOT NULL DEFAULT false,  -- a TOTP secret counts once a code from it was entered
  last_used_at  timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id)
);
CREATE INDEX user_factors_user ON user_factors (region, user_id);

-- A passkey's credential id → its account, in any region. Global: a passkey
-- sign-in starts with nothing but the credential id.
CREATE TABLE passkey_refs (
  credential_id bytea PRIMARY KEY,
  user_id       uuid NOT NULL,
  region        text NOT NULL,
  factor_id     uuid NOT NULL
);

-- One-time recovery codes, for when the second factor is lost. Only hashes
-- are kept; a fresh set replaces the old one.
CREATE TABLE recovery_codes (
  region    text NOT NULL,
  id        uuid NOT NULL,
  user_id   uuid NOT NULL,
  code_hash bytea NOT NULL,
  used_at   timestamptz,
  PRIMARY KEY (region, id)
);
CREATE INDEX recovery_codes_user ON recovery_codes (region, user_id);

-- A sign-in half done, held between two pages: the password was right and
-- the second factor is next (mfa), a factor must be set up first (enroll),
-- or a WebAuthn ceremony is under way (passkey). Minutes long; the worker
-- forgets expired ones.
CREATE TABLE sign_in_challenges (
  region          text NOT NULL,
  id              uuid NOT NULL,
  token_hash      bytea NOT NULL,
  kind            text NOT NULL CHECK (kind IN ('mfa', 'enroll', 'passkey')),
  user_id         uuid,                 -- NULL for a passkey sign-in, until the passkey names its owner
  amr             text[] NOT NULL DEFAULT '{}',
  auth_request_id text NOT NULL DEFAULT '',
  data            jsonb NOT NULL DEFAULT '{}',
  attempts        integer NOT NULL DEFAULT 0,
  expires_at      timestamptz NOT NULL,
  PRIMARY KEY (region, id),
  UNIQUE (region, token_hash)
);

-- Lockout: consecutive wrong passwords, and until when the account refuses
-- its password after too many.
ALTER TABLE users ADD COLUMN failed_logins integer NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN locked_until timestamptz;

-- Step-up: when the person last proved it was them in this session (sign-in,
-- or confirming again before a sensitive change).
ALTER TABLE sessions ADD COLUMN verified_at timestamptz;

-- A tenant can require every member to sign in with a second factor.
ALTER TABLE tenants ADD COLUMN require_mfa boolean NOT NULL DEFAULT false;
