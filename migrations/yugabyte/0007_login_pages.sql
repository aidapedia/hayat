-- Hayat 0007 (YugabyteDB): each service's login page. Its owners design it in the
-- console (Services → a service → Login page) and Hayat's own sign-in pages
-- render it, so single sign-on and password managers keep working. A design
-- is data, never markup: internal/loginpage's Design is its shape.
--
-- Global like services: read on every sign-in, written rarely.

-- The draft being edited and the published design, one row per service.
-- service_id is 'hayat' for the console's own sign-in page.
CREATE TABLE login_pages (
  service_id   text PRIMARY KEY,
  draft        jsonb NOT NULL DEFAULT '{}',
  published    jsonb,
  version      integer NOT NULL DEFAULT 0,
  published_by text NOT NULL DEFAULT '',
  published_at timestamptz,
  updated_at   timestamptz NOT NULL DEFAULT now()
);

-- Every design ever published, so an owner can bring an old one back.
CREATE TABLE login_page_versions (
  service_id   text NOT NULL,
  version      integer NOT NULL,
  design       jsonb NOT NULL,
  published_by text NOT NULL,
  published_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (service_id, version)
);

-- Logos. Raster images only (PNG, JPEG, WebP), at most 256 KB, served from
-- Hayat's own origin so the sign-in pages load nothing from elsewhere.
CREATE TABLE login_page_assets (
  id           uuid PRIMARY KEY,
  service_id   text NOT NULL,
  content_type text NOT NULL,
  data         bytea NOT NULL,
  created_by   text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX login_page_assets_service ON login_page_assets (service_id);
