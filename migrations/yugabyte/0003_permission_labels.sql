-- Hayat 0003 (YugabyteDB): a permission's short, human name ("Manage members"),
-- shown wherever an admin picks or reads permissions. Empty means the console
-- falls back to the key. Platform labels are code; a service's come from its contract.
ALTER TABLE permissions ADD COLUMN label text NOT NULL DEFAULT '';
