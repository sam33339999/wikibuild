//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxMigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	stdlib "github.com/jackc/pgx/v5/stdlib"
	"github.com/sam33339999/wikibuild/db"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func StartPostgres(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()

	container, err := tcpg.Run(ctx, "postgres:16-alpine",
		tcpg.WithDatabase("wikibuild"),
		tcpg.WithUsername("wikibuild"),
		tcpg.WithPassword("wikibuild"),
		tc.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	return pool, dsn
}

// ApplyMigrations runs the embedded golang-migrate up files against the pool's
// database, leaving the schema ready for repository tests. It fails the test
// on any error other than ErrNoChange (re-applying an already-migrated DB).
func ApplyMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	src, err := iofs.New(db.Migrations, "migrations")
	require.NoError(t, err)

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	drv, err := pgxMigrate.WithInstance(sqlDB, &pgxMigrate.Config{DatabaseName: "wikibuild"})
	require.NoError(t, err)

	m, err := migrate.NewWithInstance("iofs", src, "wikibuild", drv)
	require.NoError(t, err)
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		require.NoError(t, err)
	}
}

// NewTestRepo starts Postgres, applies migrations, and returns a Repository
// ready for integration testing. It is the standard entry point for L4 tests.
func NewTestRepo(t *testing.T) *Repository {
	t.Helper()
	pool, _ := StartPostgres(t)
	ApplyMigrations(t, pool)
	return New(pool)
}
