package rss2go

import "embed"

//go:embed migrations/*.sql
var MigrationsFS embed.FS
