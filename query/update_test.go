package query

import (
	"context"
	"reflect"
	"testing"

	"github.com/etornam45/grain/schema"
)

func TestUpdateAndReturningSQL(t *testing.T) {
	users := getTestTable()

	sql, args := Update(users).
		Set(map[string]any{"name": "UpdatedName"}).
		Where(Eq(users.Cols.ID, 1)).
		Returning("id", "name").
		SQL()

	wantSQL := "UPDATE users SET name = $1 WHERE users.id = $2 RETURNING id, name"
	if sql != wantSQL {
		t.Fatalf("got SQL %q, want %q", sql, wantSQL)
	}
	if !reflect.DeepEqual(args, []any{"UpdatedName", 1}) {
		t.Fatalf("got args %v", args)
	}
}

func TestUpdateSortMapKeys(t *testing.T) {
	type columns struct{ ID *schema.ColumnDef }
	users := schema.Table[columns]("query_map_users", schema.Column("id", schema.Int()))

	updateSQL, updateArgs := Update(users).Set(map[string]any{"z": 1, "a": 2}).SQL()
	if updateSQL != "UPDATE query_map_users SET a = $1, z = $2" {
		t.Fatalf("update SQL = %q", updateSQL)
	}
	if updateArgs[0] != 2 || updateArgs[1] != 1 {
		t.Fatalf("update args = %v", updateArgs)
	}
}

func TestUpdateRun(t *testing.T) {
	ctx := context.Background()
	users := getTestTable()
	exec := &dummyExecutor{}

	rows, err := Update(users).Set(map[string]any{"name": "Ama"}).Where(Eq(users.Cols.ID, 1)).Run(ctx, exec)
	if err != nil || rows != 1 {
		t.Fatalf("Update.Run err: %v, rows: %d", err, rows)
	}
	if exec.lastQuery != "UPDATE users SET name = $1 WHERE users.id = $2" {
		t.Errorf("got update query %q", exec.lastQuery)
	}
}

