-- Hayat 0013: a theme of its own for one app. users.appearance stays the
-- account's theme, which every app follows; this maps a service id ("hayat"
-- for Hayat's console) to the theme that app shows instead. A key that is
-- present is an app with its own theme ('{}' is "this app's defaults"); an
-- absent key follows the account. NULL is no app with its own. The shape of
-- each value is internal/identity's Appearance.
ALTER TABLE users ADD COLUMN app_appearances jsonb;
