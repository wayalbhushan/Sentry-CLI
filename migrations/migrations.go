// Package migrations contains embedded database migration SQL files.
package migrations

import "embed"

// FS embeds all .sql migration files in filename order.
//go:embed *.sql
var FS embed.FS
