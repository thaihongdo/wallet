//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newUUIDv7 generates a time-ordered UUIDv7, matching production's
// IDGenerator. It panics on error since uuid.NewV7 only fails if the
// system's random source is broken.
func newUUIDv7() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id
}

// runMigrations executes migrations/000001_init.up.sql directly against
// pool, so integration tests don't depend on the golang-migrate CLI
// being installed.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	path, err := migrationUpPath()
	if err != nil {
		return err
	}
	sql, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, string(sql))
	return err
}

// migrationUpPath locates migrations/000001_init.up.sql relative to
// this source file, so tests work regardless of the working directory
// `go test` is invoked from.
func migrationUpPath() (string, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(root, "migrations", "000001_init.up.sql"), nil
}

// truncateAll clears every table between tests so each test starts from
// a clean, empty database.
func truncateAll(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `TRUNCATE TABLE outbox, ledger_entries, operations, wallets RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}
