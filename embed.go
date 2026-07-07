package rss2go

import "embed"

// MigrationsFS embeds the migrations directory for use in database migration steps.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
