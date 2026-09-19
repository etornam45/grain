package query

import (
	"context"
	"reflect"
	"testing"

	"github.com/etornam45/grain/schema"
)

func TestInsertBatch(t *testing.T) {
	users := getTestTable()

	row1 := map[string]any{"name": "Alice", "age": 30}
	row2 := map[string]any{"name": "Bob", "age": 25}

	sql, args := Insert(users).Values(row1, row2).Returning("id").SQL()

	wantSQL := "INSERT INTO users (age, name) VALUES ($1, $2), ($3, $4) RETURNING id"
	if sql != wantSQL {
		t.Fatalf("got SQL %q, want %q", sql, wantSQL)
	}
	wantArgs := []any{30, "Alice", 25, "Bob"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("got args %v, want %v", args, wantArgs)
	}
}

func TestInsertSortMapKeys(t *testing.T) {
	users := schema.Table("query_map_users", schema.Column("id", schema.Int()))

	insertSQL, insertArgs := Insert(users).Values(map[string]any{"z": 1, "a": 2}).SQL()
	if insertSQL != "INSERT INTO query_map_users (a, z) VALUES ($1, $2)" {
		t.Fatalf("insert SQL = %q", insertSQL)
	}
	if insertArgs[0] != 2 || insertArgs[1] != 1 {
		t.Fatalf("insert args = %v", insertArgs)
	}
}

func TestInsertOnConflict(t *testing.T) {
	users := getTestTable()

	// OnConflictDoNothing without target
	sql, args := Insert(users).
		Values(map[string]any{"email": "test@example.com"}).
		OnConflictDoNothing().
		SQL()

	if sql != "INSERT INTO users (email) VALUES ($1) ON CONFLICT DO NOTHING" {
		t.Fatalf("got SQL %q", sql)
	}
	if !reflect.DeepEqual(args, []any{"test@example.com"}) {
		t.Fatalf("got args %v", args)
	}

	// OnConflictDoNothing with target
	sql, _ = Insert(users).
		Values(map[string]any{"email": "test@example.com"}).
		OnConflictDoNothing("email").
		SQL()

	if sql != "INSERT INTO users (email) VALUES ($1) ON CONFLICT (email) DO NOTHING" {
		t.Fatalf("got SQL %q", sql)
	}

	// OnConflictDoUpdate
	sql, args = Insert(users).
		Values(map[string]any{"email": "test@example.com", "name": "Test"}).
		OnConflictDoUpdate([]string{"email"}, map[string]any{"name": "Updated"}).
		SQL()

	wantSQL := "INSERT INTO users (email, name) VALUES ($1, $2) ON CONFLICT (email) DO UPDATE SET name = $3"
	if sql != wantSQL {
		t.Fatalf("got SQL %q, want %q", sql, wantSQL)
	}
	if !reflect.DeepEqual(args, []any{"test@example.com", "Test", "Updated"}) {
		t.Fatalf("got args %v", args)
	}

	// OnConflictExcluded
	sql, args = Insert(users).
		Values(map[string]any{"email": "test@example.com", "name": "Test"}).
		OnConflictExcluded([]string{"email"}, "name").
		SQL()

	wantSQL = "INSERT INTO users (email, name) VALUES ($1, $2) ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name"
	if sql != wantSQL {
		t.Fatalf("got SQL %q, want %q", sql, wantSQL)
	}
}

func TestInsertRun(t *testing.T) {
	ctx := context.Background()
	users := getTestTable()
	exec := &dummyExecutor{}

	err := Insert(users).Values(map[string]any{"name": "Ama"}).Run(ctx, exec)
	if err != nil {
		t.Fatalf("Insert.Run err: %v", err)
	}
	if exec.lastQuery != "INSERT INTO users (name) VALUES ($1)" {
		t.Errorf("got insert query %q", exec.lastQuery)
	}
}
