package query

import (
	"context"
	"reflect"
	"testing"
)

func TestDeleteAndReturningSQL(t *testing.T) {
	users := getTestTable()

	sql, args := Delete(users).
		Where(Eq(users.Cols.ID, 1)).
		Returning("id").
		SQL()

	wantSQL := "DELETE FROM users WHERE users.id = $1 RETURNING id"
	if sql != wantSQL {
		t.Fatalf("got SQL %q, want %q", sql, wantSQL)
	}
	if !reflect.DeepEqual(args, []any{1}) {
		t.Fatalf("got args %v, want %v", args, []any{1})
	}
}

func TestDeleteRun(t *testing.T) {
	ctx := context.Background()
	users := getTestTable()
	exec := &dummyExecutor{}

	rows, err := Delete(users).Where(Eq(users.Cols.ID, 1)).Run(ctx, exec)
	if err != nil || rows != 1 {
		t.Fatalf("Delete.Run err: %v, rows: %d", err, rows)
	}
	if exec.lastQuery != "DELETE FROM users WHERE users.id = $1" {
		t.Errorf("got delete query %q", exec.lastQuery)
	}
}

