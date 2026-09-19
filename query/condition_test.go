package query

import (
	"reflect"
	"testing"
)

func TestInAndNotInConditions(t *testing.T) {
	users := getTestTable()

	// In with variadic values
	cond := In(users.Cols.ID, 1, 2, 3)
	sql, args := cond.SQL(1)
	if sql != "users.id IN ($1, $2, $3)" {
		t.Errorf("got sql %q, want %q", sql, "users.id IN ($1, $2, $3)")
	}
	if !reflect.DeepEqual(args, []any{1, 2, 3}) {
		t.Errorf("got args %v, want %v", args, []any{1, 2, 3})
	}

	// In with slice
	condSlice := In(users.Cols.Name, []string{"Alice", "Bob"})
	sql, args = condSlice.SQL(3)
	if sql != "users.name IN ($3, $4)" {
		t.Errorf("got sql %q, want %q", sql, "users.name IN ($3, $4)")
	}
	if !reflect.DeepEqual(args, []any{"Alice", "Bob"}) {
		t.Errorf("got args %v, want %v", args, []any{"Alice", "Bob"})
	}

	// In with empty slice -> 1 = 0
	condEmpty := In(users.Cols.ID, []int{})
	sql, args = condEmpty.SQL(1)
	if sql != "1 = 0" {
		t.Errorf("got sql %q, want %q", sql, "1 = 0")
	}
	if len(args) != 0 {
		t.Errorf("got args %v, want empty", args)
	}

	// NotIn with variadic
	condNotIn := NotIn(users.Cols.ID, 4, 5)
	sql, args = condNotIn.SQL(1)
	if sql != "users.id NOT IN ($1, $2)" {
		t.Errorf("got sql %q, want %q", sql, "users.id NOT IN ($1, $2)")
	}
	if !reflect.DeepEqual(args, []any{4, 5}) {
		t.Errorf("got args %v, want %v", args, []any{4, 5})
	}

	// NotIn with empty slice -> 1 = 1
	condNotInEmpty := NotIn(users.Cols.ID, []int{})
	sql, args = condNotInEmpty.SQL(1)
	if sql != "1 = 1" {
		t.Errorf("got sql %q, want %q", sql, "1 = 1")
	}
	if len(args) != 0 {
		t.Errorf("got args %v, want empty", args)
	}
}

func TestRawCondition(t *testing.T) {
	// Raw without args
	rawNoArgs := Raw("created_at > NOW() - INTERVAL '1 day'")
	sql, args := rawNoArgs.SQL(1)
	if sql != "created_at > NOW() - INTERVAL '1 day'" {
		t.Errorf("got sql %q", sql)
	}
	if len(args) != 0 {
		t.Errorf("got args %v, want empty", args)
	}

	// Raw with placeholders and offset
	rawWithArgs := Raw("users.age >= ? AND users.name LIKE ?", 18, "A%")
	sql, args = rawWithArgs.SQL(5)
	if sql != "users.age >= $5 AND users.name LIKE $6" {
		t.Errorf("got sql %q, want %q", sql, "users.age >= $5 AND users.name LIKE $6")
	}
	if !reflect.DeepEqual(args, []any{18, "A%"}) {
		t.Errorf("got args %v, want %v", args, []any{18, "A%"})
	}
}

func TestComparisonAndLogicalConditions(t *testing.T) {
	users := getTestTable()

	// Test Neq, Lt, Lte, Like, ILike, IsNull, Or, Not
	cond := Or(
		Neq(users.Cols.Name, "Ama"),
		Lt(users.Cols.Age, 18),
		Lte(users.Cols.Age, 65),
		Like(users.Cols.Email, "%@gmail.com"),
		ILike(users.Cols.Name, "a%"),
		IsNull(users.Cols.Age),
		Not(Eq(users.Cols.ID, 99)),
	)
	sqlCond, args := cond.SQL(1)
	wantCondSQL := "(users.name <> $1 OR users.age < $2 OR users.age <= $3 OR users.email LIKE $4 OR users.name ILIKE $5 OR users.age IS NULL OR NOT (users.id = $6))"
	if sqlCond != wantCondSQL {
		t.Errorf("got cond SQL %q, want %q", sqlCond, wantCondSQL)
	}
	if len(args) != 6 {
		t.Errorf("got %d args, want 6", len(args))
	}
}

