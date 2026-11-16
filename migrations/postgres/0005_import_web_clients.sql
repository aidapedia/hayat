-- Hayat 0005: `hayat import` registered a service's sign-in client as
-- "<display name> (web)" before clients belonged to services, so 0004 left it
-- with the platform. Give each such client back to its service. A service
-- renamed since keeps its client with the platform, where an admin sees it.
UPDATE oauth_clients c SET service_id = s.id, updated_at = now()
FROM services s
WHERE c.service_id = 'hayat' AND NOT c.builtin AND c.name = s.display_name || ' (web)';
