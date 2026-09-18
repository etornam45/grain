package migrate

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestMigrationLifecycleWithNoTransactionSection(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL integration tests")
	}
	dbConn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer dbConn.Close()
	ctx := context.Background()
	cleanup := "DROP SCHEMA IF EXISTS grain CASCADE; DROP TABLE IF EXISTS migration_test_records; DROP TYPE IF EXISTS migration_test_state"
	if _, err := dbConn.ExecContext(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = dbConn.ExecContext(ctx, cleanup) })

	dir := t.TempDir()
	first := "-- +migrate Up\nCREATE TYPE migration_test_state AS ENUM ('active');\n-- +migrate Down\nDROP TYPE migration_test_state;"
	second := "-- +RequiresNoTx Up\nALTER TYPE migration_test_state ADD VALUE IF NOT EXISTS 'disabled';\n-- +migrate Up\nCREATE TABLE migration_test_records (state migration_test_state NOT NULL);\n-- +migrate Down\nDROP TABLE migration_test_records;"
	if err := os.WriteFile(filepath.Join(dir, "1_create_type.sql"), []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2_add_value.sql"), []byte(second), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, dbConn, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := dbConn.ExecContext(ctx, "INSERT INTO migration_test_records (state) VALUES ('disabled')"); err != nil {
		t.Fatalf("no-transaction enum value was not usable after migration: %v", err)
	}
	if err := RollbackLast(ctx, dbConn, dir); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := dbConn.QueryRowContext(ctx, "SELECT to_regclass('migration_test_records') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("rollback did not remove migration_test_records")
	}
}
