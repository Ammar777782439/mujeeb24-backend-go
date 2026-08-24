package migrations

import "embed"

// Files contains the versioned SQL migrations shipped with the Mujeeb 24 binary.
// Migration files are forward-only in production; a new correction gets a new version.
//
//go:embed *.up.sql
var Files embed.FS
