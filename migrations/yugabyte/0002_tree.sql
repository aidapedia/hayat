-- Hayat 0002 (YugabyteDB): the resource tree's policy half: service namespaces, contracts,
-- the catalog they publish, roles, grants and policy events.
--
-- Append-only. The yugabyte/ history holds the same tables.

-- ---------- Global: services and the catalog ----------

-- A service is a namespace in the catalog ("billing") plus the confidential
-- client it calls the API with. Its hayat/service node is where owner grants live.
CREATE TABLE services (
  id             text PRIMARY KEY,
  display_name   text NOT NULL,
  description    text NOT NULL DEFAULT '',
  node_id        uuid NOT NULL,
  client_id      text NOT NULL,
  active_version integer NOT NULL DEFAULT 0,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX services_client ON services (client_id);

CREATE TABLE contract_versions (
  service_id   text NOT NULL,
  version      integer NOT NULL,
  manifest     jsonb NOT NULL,
  status       text NOT NULL CHECK (status IN ('pending', 'active', 'superseded', 'rejected')),
  -- Every namespace the contract touches, its own first. Each needs an owner's approval.
  namespaces   text[] NOT NULL DEFAULT '{}',
  summary      jsonb NOT NULL DEFAULT '{}',
  submitted_by text NOT NULL,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  decided_by   text NOT NULL DEFAULT '',
  decided_at   timestamptz,
  reason       text NOT NULL DEFAULT '',
  PRIMARY KEY (service_id, version)
);

CREATE TABLE contract_approvals (
  service_id  text NOT NULL,
  version     integer NOT NULL,
  namespace   text NOT NULL,
  approved_by uuid NOT NULL,
  approved_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (service_id, version, namespace)
);

CREATE TABLE node_types (
  key                 text PRIMARY KEY,
  service_id          text NOT NULL,
  parents             text[] NOT NULL DEFAULT '{}',
  access_permission   text NOT NULL DEFAULT '',
  via_placement       boolean NOT NULL DEFAULT false,
  requires_membership boolean NOT NULL DEFAULT false,
  degradable          boolean NOT NULL DEFAULT false,
  description         text NOT NULL DEFAULT '',
  created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE permissions (
  key         text PRIMARY KEY,
  service_id  text NOT NULL,
  scope       text NOT NULL,
  description text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- Roles are data. service_id says who owns one: 'hayat' for the built-ins, a
-- service for its contract's default roles, '' for roles an admin made.
CREATE TABLE roles (
  id          uuid PRIMARY KEY,
  slug        text NOT NULL UNIQUE,
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  service_id  text NOT NULL DEFAULT '',
  -- Node types the role may be granted on; empty means any.
  applies_to  text[] NOT NULL DEFAULT '{}',
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE role_permissions (
  role_id    uuid NOT NULL,
  permission text NOT NULL,
  PRIMARY KEY (role_id, permission)
);

-- ---------- Regional: grants and policy events ----------

CREATE TABLE grants (
  region         text NOT NULL,
  id             uuid NOT NULL,
  tenant_id      uuid,
  node_id        uuid NOT NULL,
  principal_type text NOT NULL CHECK (principal_type IN ('user', 'group')),
  principal_id   uuid NOT NULL,
  role_id        uuid NOT NULL,
  inherit        boolean NOT NULL DEFAULT true,
  expires_at     timestamptz,
  created_by     text NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id),
  UNIQUE (region, node_id, principal_type, principal_id, role_id)
);
CREATE INDEX grants_node ON grants (node_id);
CREATE INDEX grants_principal ON grants (principal_id);
CREATE INDEX grants_role ON grants (role_id);

-- One row per policy change, read by Watch and by every region's caches.
CREATE TABLE policy_events (
  region     text NOT NULL,
  id         uuid NOT NULL,
  tenant_id  uuid NOT NULL,
  rev        bigint NOT NULL,
  kind       text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id)
);
CREATE INDEX policy_events_id ON policy_events (id);

-- Every node is findable by id alone through the global ref table: groups are
-- recorded with their own id as external_id.
CREATE UNIQUE INDEX node_refs_id ON node_refs (id);

CREATE INDEX nodes_owner ON nodes (owner_id);
CREATE INDEX nodes_tenant_type ON nodes (tenant_id HASH, type ASC);

-- The global revision: catalog, roles, and grants outside any tenant.
INSERT INTO tenant_revisions (region, tenant_id, rev) VALUES ('default', '00000000-0000-0000-0000-000000000000', 1);
