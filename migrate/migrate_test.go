package migrate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ignorePrompt(PromptContext) (Resolution, error) { return Resolution{Action: "ignore"}, nil }

func TestDiffReplacesIndexWhenColumnsChange(t *testing.T) {
	old := Snapshot{Tables: []TableSnapshot{{Name: "users", Indexes: []IndexSnapshot{{Name: "idx_users_lookup", Cols: []string{"email"}}}}}}
	new := Snapshot{Tables: []TableSnapshot{{Name: "users", Indexes: []IndexSnapshot{{Name: "idx_users_lookup", Cols: []string{"email", "status"}}}}}}

	changes, err := Diff(old, new, ignorePrompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("got %d changes, want drop and recreate: %#v", len(changes), changes)
	}
	if changes[0].Kind != DropIndex || changes[1].Kind != AddIndex {
		t.Fatalf("unexpected changes: %#v", changes)
	}
}

func TestDiffUpdatesIndexesAfterTableRename(t *testing.T) {
	old := Snapshot{Tables: []TableSnapshot{{Name: "old_users", Indexes: []IndexSnapshot{{Name: "idx_old_users_email", Cols: []string{"email"}}}}}}
	new := Snapshot{Tables: []TableSnapshot{{Name: "users", Indexes: []IndexSnapshot{{Name: "idx_users_email", Cols: []string{"email"}}}}}}
	prompt := func(ctx PromptContext) (Resolution, error) {
		if ctx.Kind == AmbiguousTable && ctx.OldName == "old_users" {
			return Resolution{Action: "rename", Target: "users"}, nil
		}
		return Resolution{Action: "ignore"}, nil
	}

	changes, err := Diff(old, new, prompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0].Kind != RenameTable || changes[1].Kind != DropIndex || changes[2].Kind != AddIndex {
		t.Fatalf("unexpected changes after table rename: %#v", changes)
	}
}

func TestDiffRejectsNewTableForeignKeyCycle(t *testing.T) {
	snap := Snapshot{Tables: []TableSnapshot{
		{Name: "a", Columns: []ColumnSnapshot{{Name: "b_id", Type: "INTEGER", RefTable: "b", RefColumn: "id"}}},
		{Name: "b", Columns: []ColumnSnapshot{{Name: "a_id", Type: "INTEGER", RefTable: "a", RefColumn: "id"}}},
	}}
	_, err := Diff(Snapshot{}, snap, ignorePrompt)
	if err == nil || !strings.Contains(err.Error(), "foreign-key cycle") {
		t.Fatalf("Diff() error = %v, want foreign-key-cycle error", err)
	}
}

func TestGenerateReversesDownDependencyOrder(t *testing.T) {
	dir := t.TempDir()
	snap := Snapshot{
		Enums:  []EnumSnapshot{{Name: "user_status", Values: []string{"active"}}},
		Tables: []TableSnapshot{{Name: "users", Columns: []ColumnSnapshot{{Name: "status", Type: "user_status"}}}},
	}
	path, err := GenerateFromSnapshot(snap, dir, "init", ignorePrompt)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	down := string(content[strings.Index(string(content), "-- +migrate Down"):])
	if strings.Index(down, "DROP TABLE users") > strings.Index(down, "DROP TYPE user_status") {
		t.Fatalf("down migration drops enum before dependent table:\n%s", down)
	}
}

func TestGenerateDoesNotWriteOrAdvanceSnapshotForUnsafeChange(t *testing.T) {
	dir := t.TempDir()
	baseline := Snapshot{Tables: []TableSnapshot{{Name: "users", Columns: []ColumnSnapshot{{Name: "id", Type: "INTEGER"}}}}}
	if err := SaveSnapshot(dir, "1", "baseline", baseline); err != nil {
		t.Fatal(err)
	}
	unsafe := Snapshot{Tables: []TableSnapshot{{Name: "users", Columns: []ColumnSnapshot{
		{Name: "id", Type: "INTEGER"},
		{Name: "required_name", Type: "TEXT", NotNull: true},
	}}}}

	_, err := GenerateFromSnapshot(unsafe, dir, "unsafe", ignorePrompt)
	var reviewErr *ManualReviewRequiredError
	if !errors.As(err, &reviewErr) {
		t.Fatalf("GenerateFromSnapshot error = %v, want ManualReviewRequiredError", err)
	}
	if len(reviewErr.Changes) == 0 {
		t.Fatal("review error did not expose the changes requiring review")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			t.Fatalf("unsafe generation wrote migration %q", filepath.Join(dir, entry.Name()))
		}
	}
	loaded, err := LoadLatestSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Tables[0].Columns) != 1 {
		t.Fatalf("snapshot advanced despite unsafe generation: %#v", loaded)
	}
}

func TestGenerateWritesAndLoadMigrationsParsesNoTransactionSections(t *testing.T) {
	dir := t.TempDir()
	baseline := Snapshot{Enums: []EnumSnapshot{{Name: "status", Values: []string{"active"}}}}
	if err := SaveSnapshot(dir, "1", "baseline", baseline); err != nil {
		t.Fatal(err)
	}
	next := Snapshot{Enums: []EnumSnapshot{{Name: "status", Values: []string{"active", "disabled"}}}}
	path, err := GenerateFromSnapshot(next, dir, "add status", ignorePrompt)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "-- +RequiresNoTx Up\nALTER TYPE status ADD VALUE IF NOT EXISTS 'disabled';") {
		t.Fatalf("generated migration is missing no-transaction up section:\n%s", content)
	}

	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d migration files, want 1", len(files))
	}
	if files[0].NoTxUpSQL != "ALTER TYPE status ADD VALUE IF NOT EXISTS 'disabled';" {
		t.Fatalf("NoTxUpSQL = %q", files[0].NoTxUpSQL)
	}
}

func TestSplitMigrationAcceptsSpacedNoTransactionMarker(t *testing.T) {
	content := "-- +migrate Up\nCREATE TABLE users ();\n-- + RequiresNoTx down\nDROP INDEX IF EXISTS idx_users_email;\n-- +migrate Down\nDROP TABLE users;\n"
	up, down, noTxUp, noTxDown := splitMigration(content)
	if up != "CREATE TABLE users ();" || down != "DROP TABLE users;" || noTxUp != "" || noTxDown != "DROP INDEX IF EXISTS idx_users_email;" {
		t.Fatalf("unexpected sections: up=%q down=%q noTxUp=%q noTxDown=%q", up, down, noTxUp, noTxDown)
	}
}

func TestSplitMigrationTreatsUndirectedSQLAsUp(t *testing.T) {
	up, down, noTxUp, noTxDown := splitMigration("CREATE TABLE users ();\n")
	if up != "CREATE TABLE users ();" || down != "" || noTxUp != "" || noTxDown != "" {
		t.Fatalf("unexpected sections: up=%q down=%q noTxUp=%q noTxDown=%q", up, down, noTxUp, noTxDown)
	}
}

func TestNextMigrationVersionAvoidsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, time.September, 18, 12, 0, 0, 0, time.UTC)
	base := now.Format("20060102150405000")
	if err := os.WriteFile(filepath.Join(dir, base+"_first.sql"), []byte("-- +migrate Up"), 0o644); err != nil {
		t.Fatal(err)
	}
	version, err := nextMigrationVersion(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if version != base+"01" {
		t.Fatalf("version = %q, want %q", version, base+"01")
	}
}

func TestHasExecutableSQL(t *testing.T) {
	if hasExecutableSQL("-- cannot restore this table") {
		t.Fatal("comment-only down migration should be irreversible")
	}
	if !hasExecutableSQL("-- note\nDROP TABLE users;") {
		t.Fatal("SQL down migration should be executable")
	}
}
