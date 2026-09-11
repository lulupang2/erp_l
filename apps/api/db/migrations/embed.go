package migrations

import "embed"

// Files is used by the explicit migration CLI, never by server startup.
//
//go:embed *.sql
var Files embed.FS
