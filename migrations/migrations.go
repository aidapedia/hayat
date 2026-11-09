// Package migrations embeds Hayat's SQL migrations, one history per engine.
package migrations

import "embed"

//go:embed postgres/*.sql yugabyte/*.sql
var FS embed.FS
