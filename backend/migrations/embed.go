// Package migrations embeds the goose SQL migrations so the API binary can
// apply them at startup without shipping loose files.
package migrations

import "embed"

// FS contains every *.sql migration in this directory.
//
//go:embed *.sql
var FS embed.FS
