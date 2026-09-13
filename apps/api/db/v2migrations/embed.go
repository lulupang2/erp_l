package v2migrations

import "embed"

// Files is used by the explicit v2 migration CLI, never by server startup.
//
//go:embed *.sql
var Files embed.FS
