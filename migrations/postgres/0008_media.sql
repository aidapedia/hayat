-- Hayat 0008: uploaded images — avatars, and the logos of tenants,
-- services and login pages. internal/media owns this table. It records what
-- each image is and who owns it; the bytes live outside the database, in a
-- directory or an S3-compatible bucket (HAYAT_STORAGE), and `backend` says
-- which one each image went to.
--
-- The login page logos 0007 kept in login_page_assets are moved into the
-- store by the admin role at boot (media.AdoptLoginPageAssets), keeping their
-- ids; SQL can't write files. login_page_assets is dead after that.

CREATE TABLE media (
  id           uuid PRIMARY KEY,
  kind         text NOT NULL CHECK (kind IN ('avatar', 'logo')),
  -- user:<id> | tenant:<id> | service:<id> | login_page:<service>
  owner        text NOT NULL,
  backend      text NOT NULL,
  object_key   text NOT NULL,
  content_type text NOT NULL,
  bytes        integer NOT NULL,
  width        integer NOT NULL DEFAULT 0,
  height       integer NOT NULL DEFAULT 0,
  created_by   text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX media_owner ON media (owner);

-- A tenant's and a service's logo: a media id, empty for none. A person's
-- avatar stays users.avatar_url (it is the OIDC picture claim), set to
-- <issuer>/media/<id> when they upload one.
ALTER TABLE tenants ADD COLUMN logo text NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN logo text NOT NULL DEFAULT '';
