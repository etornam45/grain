package query

import (
	"reflect"
	"testing"

	"github.com/etornam45/grain/schema"
)

func localTables() (*schema.TableDef, *schema.TableDef) {
	users := schema.Table("u2_users",
		schema.Column("id", schema.Int()).PrimaryKey(),
		schema.Column("name", schema.Varchar(50)),
		schema.Column("email", schema.Varchar(50)),
		schema.Column("age", schema.Int()),
		schema.Column("profile", schema.JSONB()),
	)
	orders := schema.Table("u2_orders",
		schema.Column("id", schema.Int()).PrimaryKey(),
		schema.Column("user_id", schema.Int()),
		schema.Column("total", schema.Int()),
	)
	return users, orders
}

func TestSelectWithCTE(t *testing.T) {
	users, _ := localTables()
	active := Sub(Select[struct{}](users.Col("id"), users.Col("name")).
		From(users).
		Where(Eq(users.Col("name"), "joe")))

	sql, args := Select[struct{}](users.Col("name")).
		With("active", active).
		From(users).
		Where(Raw("active_id IS NOT NULL")).
		SQL()

	wantSQL := "WITH active AS (SELECT u2_users.id AS \"u2_users.id\", u2_users.name AS \"u2_users.name\" FROM u2_users WHERE u2_users.name = $1) " +
		`SELECT u2_users.name AS "u2_users.name" FROM u2_users WHERE active_id IS NOT NULL`
	if sql != wantSQL {
		t.Fatalf("got SQL %q\nwant %q", sql, wantSQL)
	}
	if !reflect.DeepEqual(args, []any{"joe"}) {
		t.Fatalf("got args %v, want ['joe']", args)
	}
}

func TestSelectSetOps_ArgumentRenumbering(t *testing.T) {
	users, _ := localTables()

	a := Sub(Select[struct{}](users.Col("id")).From(users).Where(Gt(users.Col("age"), 18)))
	b := Sub(Select[struct{}](users.Col("id")).From(users).Where(Lt(users.Col("age"), 65)))

	sql, args := Select[struct{}](users.Col("id")).
		From(users).
		Where(Eq(users.Col("id"), 7)).
		Union(a).
		Intersect(b).
		OrderBy([]string{users.Col("id").String()}, Desc).
		Limit(5).
		SQL()

	wantSQL := "SELECT u2_users.id AS \"u2_users.id\" FROM u2_users WHERE u2_users.id = $1 " +
		"UNION (SELECT u2_users.id AS \"u2_users.id\" FROM u2_users WHERE u2_users.age > $2) " +
		"INTERSECT (SELECT u2_users.id AS \"u2_users.id\" FROM u2_users WHERE u2_users.age < $3) " +
		"ORDER BY \"u2_users.id\" DESC LIMIT 5"
	if sql != wantSQL {
		t.Fatalf("got SQL %q\nwant %q", sql, wantSQL)
	}
	if !reflect.DeepEqual(args, []any{7, 18, 65}) {
		t.Fatalf("got args %v, want [7 18 65]", args)
	}
}

func TestSelectLocksAndNullsOrder(t *testing.T) {
	users, _ := localTables()

	sql, _ := Select[struct{}](users.Col("id")).
		From(users).
		OrderByNulls([]string{users.Col("age").String()}, Desc, NullsLast).
		ForUpdate().
		SkipLocked().
		SQL()

	wantSQL := "SELECT u2_users.id AS \"u2_users.id\" FROM u2_users ORDER BY u2_users.age DESC NULLS LAST FOR UPDATE SKIP LOCKED"
	if sql != wantSQL {
		t.Fatalf("got SQL %q\nwant %q", sql, wantSQL)
	}
}

func TestSelectForShareNoWait(t *testing.T) {
	users, _ := localTables()
	sql, _ := Select[struct{}](users.Col("id")).From(users).ForShare().NoWait().SQL()
	wantSQL := "SELECT u2_users.id AS \"u2_users.id\" FROM u2_users FOR SHARE NOWAIT"
	if sql != wantSQL {
		t.Fatalf("got SQL %q\nwant %q", sql, wantSQL)
	}
}

func TestConditions_BetweenJSONSubquery(t *testing.T) {
	users, orders := localTables()
	big := Sub(Select[struct{}](orders.Col("id")).From(orders).Where(Gt(orders.Col("total"), 1000)))

	sql, args := Select[struct{}](users.Col("id")).
		From(users).
		Where(And(
			Between(users.Col("age"), 18, 30),
			NotBetween(users.Col("age"), 60, 99),
			Contains(users.Col("profile"), map[string]any{"plan": "pro"}),
			KeyExists(users.Col("profile"), "admin"),
			In(users.Col("id"), big),
			EqAny(users.Col("id"), big),
			NeqAll(users.Col("id"), big),
			Exists(Sub(Select[struct{}](orders.Col("id")).From(orders).Where(Raw("u2_orders.user_id = u2_users.id")))),
		)).
		SQL()

	wantSQL := "SELECT u2_users.id AS \"u2_users.id\" FROM u2_users " +
		"WHERE (u2_users.age BETWEEN $1 AND $2 AND u2_users.age NOT BETWEEN $3 AND $4 AND " +
		"u2_users.profile @> $5 AND u2_users.profile ? $6 AND u2_users.id IN (SELECT u2_orders.id AS \"u2_orders.id\" FROM u2_orders WHERE u2_orders.total > $7) AND " +
		"u2_users.id = ANY (SELECT u2_orders.id AS \"u2_orders.id\" FROM u2_orders WHERE u2_orders.total > $8) AND " +
		"u2_users.id <> ALL (SELECT u2_orders.id AS \"u2_orders.id\" FROM u2_orders WHERE u2_orders.total > $9) AND " +
		"EXISTS (SELECT u2_orders.id AS \"u2_orders.id\" FROM u2_orders WHERE u2_orders.user_id = u2_users.id))"
	if sql != wantSQL {
		t.Fatalf("got SQL %q\nwant %q", sql, wantSQL)
	}

	plan := map[string]any{"plan": "pro"}
	wantArgs := []any{18, 30, 60, 99, plan, "admin", 1000, 1000, 1000}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("got args %v\nwant %v", args, wantArgs)
	}
}

func TestSubqueryAsColumn(t *testing.T) {
	users, orders := localTables()
	sub := Sub(Select[struct{}](orders.Col("id")).From(orders).Limit(1))

	sql, _ := Select[struct{}](users.Col("id")).
		From(users).
		Where(NotIn(users.Col("id"), sub)).
		SQL()

	want := "IN (SELECT u2_orders.id AS \"u2_orders.id\" FROM u2_orders LIMIT 1)"
	if !contains(sql, want) {
		t.Fatalf("got SQL %q, want it to contain %q", sql, want)
	}
}

func TestSubqueryAsWithArgsPanics(t *testing.T) {
	users, _ := localTables()
	sub := Sub(Select[struct{}](users.Col("id")).From(users).Where(Eq(users.Col("id"), 1)))

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for argument-carrying subquery column")
		}
	}()
	_ = sub.As("latest")
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestAliasTableRendersAliasQualifier(t *testing.T) {
	users, _ := localTables()
	u := Alias(users, "u")

	sql, _ := Select[struct{}](u.Col("id"), u.Col("name")).
		From(u).
		Where(Eq(u.Col("age"), 21)).
		SQL()

	wantSQL := `SELECT u.id AS "u.id", u.name AS "u.name" FROM u2_users AS u WHERE u.age = $1`
	if sql != wantSQL {
		t.Fatalf("got SQL %q\nwant %q", sql, wantSQL)
	}
}

func TestUpdateSetColumnAndSubquery(t *testing.T) {
	users, orders := localTables()
	total := Sub(Select[struct{}](orders.Col("id")).From(orders).Where(Eq(orders.Col("user_id"), 1)))

	sql, args := Update(users).
		Set(map[string]any{"name": users.Col("email"), "age": 30, "id": total}).
		Where(Eq(users.Col("name"), "bob")).
		SQL()

	// Set sorts keys: age (literal $1), id (subquery $2), name (column-to-column).
	if !contains(sql, `age = $1`) {
		t.Errorf("expected placeholder for literal assignment, got %q", sql)
	}
	if !contains(sql, `id = (SELECT u2_orders.id AS "u2_orders.id" FROM u2_orders WHERE u2_orders.user_id = $2)`) {
		t.Errorf("expected subquery assignment, got %q", sql)
	}
	if !contains(sql, `name = u2_users.email`) {
		t.Errorf("expected column-to-column assignment, got %q", sql)
	}
	if !reflect.DeepEqual(args, []any{30, 1, "bob"}) {
		t.Fatalf("got args %v, want [30 1 bob]", args)
	}
}

func TestUpdateOrderByLimit(t *testing.T) {
	users, _ := localTables()
	sql, _ := Update(users).
		Set(map[string]any{"age": 25}).
		Where(Lt(users.Col("age"), 18)).
		OrderBy([]string{users.Col("id").String()}, Asc).
		Limit(10).
		Returning("id").
		SQL()

	want := "UPDATE u2_users SET age = $1 WHERE u2_users.age < $2 ORDER BY u2_users.id ASC LIMIT 10 RETURNING id"
	if sql != want {
		t.Fatalf("got SQL %q\nwant %q", sql, want)
	}
}

func TestDeleteOrderByLimit(t *testing.T) {
	users, _ := localTables()
	sql, _ := Delete(users).
		Where(IsNull(users.Col("name"))).
		OrderByNulls([]string{users.Col("id").String()}, Desc, NullsFirst).
		Limit(100).
		SQL()

	want := "DELETE FROM u2_users WHERE u2_users.name IS NULL ORDER BY u2_users.id DESC NULLS FIRST LIMIT 100"
	if sql != want {
		t.Fatalf("got SQL %q\nwant %q", sql, want)
	}
}