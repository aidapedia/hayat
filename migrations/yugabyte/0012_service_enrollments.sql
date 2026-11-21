-- Hayat 0012 (YugabyteDB): service enrollment codes. internal/enrollment
--
-- A one-time code an owner (or, for a service not registered yet, an instance
-- admin) hands to a service's deployment. The service redeems it once on the
-- api role and receives its credentials, so nobody copies a client secret.
-- Redeeming for a service that already exists rotates its API secret.
--
-- Global, like services: rare, and looked up by the code's hash alone. Only the
-- code's SHA-256 is kept.
CREATE TABLE service_enrollments (
  id            uuid PRIMARY KEY,
  service_id    text NOT NULL,
  -- The name a new service registers under; empty keeps the namespace.
  display_name  text NOT NULL DEFAULT '',
  code_hash     bytea NOT NULL,
  -- The person who made it: a new service's first owner, and whose access is
  -- checked again when the code is redeemed.
  created_by    uuid NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  expires_at    timestamptz NOT NULL,
  -- A redemption in progress holds the code until then; a crash frees it.
  claimed_until timestamptz,
  used_at       timestamptz,
  revoked_at    timestamptz,
  -- Set on a successful redemption: whether it registered the service, and the
  -- sign-in client it made or kept for the service's address.
  registered    boolean NOT NULL DEFAULT false,
  web_client_id text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX service_enrollments_code ON service_enrollments (code_hash);
CREATE INDEX service_enrollments_service ON service_enrollments (service_id, created_at);
