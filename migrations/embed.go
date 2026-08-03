package migrations

import "embed"

//go:embed mysql/*.sql
var MySQL embed.FS

//go:embed sqlite/*.sql
var SQLite embed.FS
