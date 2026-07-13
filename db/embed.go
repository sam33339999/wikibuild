// Package db embeds the migration files so they ship in the binary and are
// available to the postgres integration tests (via golang-migrate's iofs
// source) regardless of the test working directory.
package db

import "embed"

// Migrations holds the golang-migrate .sql files under db/migrations.
//
//go:embed migrations/*.sql
var Migrations embed.FS
