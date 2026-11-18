-- Hayat 0010 (YugabyteDB): agents. A member kind of their own, never rows in users: an agent has
-- no email, password, session or admin role, lives in exactly one tenant and
-- always answers to a person there (its sponsor). It never signs in; a service
-- it works in asks for a short-lived agent token on its behalf. internal/agents
--
-- Agents are kept out of every people table on purpose. group_members stays
-- people only, so no member list, count or access list has to remember to skip
-- them; an agent's place in groups is agent_groups.

-- Global: find an agent by id alone, and keep handles unique in a tenant.
CREATE TABLE agent_refs (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL,
  handle_lower text NOT NULL,
  region       text NOT NULL,
  UNIQUE (tenant_id, handle_lower)
);

CREATE TABLE agents (
  region          text NOT NULL,
  id              uuid NOT NULL,
  tenant_id       uuid NOT NULL,
  handle          text NOT NULL,
  name            text NOT NULL,
  sponsor_user_id uuid NOT NULL,
  status          text NOT NULL CHECK (status IN ('active', 'paused', 'disabled')),
  -- Why it isn't active: 'sponsor_left', 'sponsor_disabled', or '' when a person paused it.
  status_reason   text NOT NULL DEFAULT '',
  -- The services (namespaces) that may ask for its tokens.
  services        text[] NOT NULL DEFAULT '{}',
  created_by      text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, id)
);
CREATE INDEX agents_tenant ON agents (tenant_id);
CREATE INDEX agents_sponsor ON agents (sponsor_user_id);

-- An agent's groups. Its tenant's roster is implied by agents.tenant_id.
CREATE TABLE agent_groups (
  region     text NOT NULL,
  group_id   uuid NOT NULL,
  tenant_id  uuid NOT NULL,
  agent_id   uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (region, group_id, agent_id)
);
CREATE INDEX agent_groups_agent ON agent_groups (agent_id);

-- Grants name an agent like they name a person.
ALTER TABLE grants DROP CONSTRAINT grants_principal_type_check;
ALTER TABLE grants ADD CONSTRAINT grants_principal_type_check CHECK (principal_type IN ('user', 'group', 'agent'));

-- A permission only people may hold (approving, administering). Hayat refuses
-- to grant a role with one to an agent, and never answers yes for an agent.
ALTER TABLE permissions ADD COLUMN people_only boolean NOT NULL DEFAULT false;

-- Tenant admins manage their tenant's agents. Built-in roles get new platform
-- permissions here, once; an admin's later edits to the role stay.
INSERT INTO role_permissions (role_id, permission)
VALUES ('00000000-0000-7000-8000-000000000101', 'hayat.agents.manage') ON CONFLICT DO NOTHING;
