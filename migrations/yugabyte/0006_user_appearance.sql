-- Hayat 0006 (YugabyteDB): how every app looks for a person. One theme per
-- account, read by Hayat's console and by every service that signs people in
-- with Hayat (DirectoryService.GetUserAppearance). NULL means "never set": each
-- app shows its own defaults. The shape is internal/identity's Appearance.
ALTER TABLE users ADD COLUMN appearance jsonb;
