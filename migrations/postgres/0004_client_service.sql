-- Hayat 0004: every OAuth client belongs to a service. A service's API
-- credential belongs to that service; any client registered before this
-- belongs to the platform itself (`hayat`), where an instance admin manages it.
-- The default only fills the existing rows: from here on the code names the
-- service of every client it creates.
ALTER TABLE oauth_clients ADD COLUMN service_id text NOT NULL DEFAULT 'hayat';
UPDATE oauth_clients c SET service_id = s.id FROM services s WHERE s.client_id = c.id;
ALTER TABLE oauth_clients ALTER COLUMN service_id DROP DEFAULT;
CREATE INDEX oauth_clients_service ON oauth_clients (service_id);
