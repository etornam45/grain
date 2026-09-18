package query

import (
	"testing"

	"github.com/etornam45/grain/schema"
)

func TestSelectOrderByRendersValidSeparatedClauses(t *testing.T) {
	type columns struct {
		ID   *schema.ColumnDef
		Name *schema.ColumnDef
	}
	users := schema.Table[columns]("query_order_users",
		schema.Column("id", schema.Int()),
		schema.Column("name", schema.Text()),
	)

	sql, args := Select[struct{}](users.Cols.ID).
		From(users).
		OrderBy([]string{users.Cols.Name.String()}, Asc).
		OrderBy([]string{users.Cols.ID.String()}, Desc).
		SQL()

	want := "SELECT query_order_users.id FROM query_order_users ORDER BY query_order_users.name ASC, query_order_users.id DESC"
	if sql != want {
		t.Fatalf("SQL() = %q, want %q", sql, want)
	}
	if len(args) != 0 {
		t.Fatalf("SQL() args = %v, want none", args)
	}
}

func TestInsertAndUpdateSortMapKeys(t *testing.T) {
	type columns struct{ ID *schema.ColumnDef }
	users := schema.Table[columns]("query_map_users", schema.Column("id", schema.Int()))

	insertSQL, insertArgs := Insert(users).Values(map[string]any{"z": 1, "a": 2}).SQL()
	if insertSQL != "INSERT INTO query_map_users (a, z) VALUES ($1, $2)" {
		t.Fatalf("insert SQL = %q", insertSQL)
	}
	if insertArgs[0] != 2 || insertArgs[1] != 1 {
		t.Fatalf("insert args = %v", insertArgs)
	}

	updateSQL, updateArgs := Update(users).Set(map[string]any{"z": 1, "a": 2}).SQL()
	if updateSQL != "UPDATE query_map_users SET a = $1, z = $2" {
		t.Fatalf("update SQL = %q", updateSQL)
	}
	if updateArgs[0] != 2 || updateArgs[1] != 1 {
		t.Fatalf("update args = %v", updateArgs)
	}
}
