package query

import (
	"reflect"
	"testing"

	"github.com/etornam45/grain/schema"
)

func TestSelectOrderByRendersValidSeparatedClauses(t *testing.T) {
	users := schema.Table("query_order_users",
		schema.Column("id", schema.Int()),
		schema.Column("name", schema.Text()),
	)

	sql, args := Select[struct{}](users.Col("id")).
		From(users).
		OrderBy([]string{users.Col("name").String()}, Asc).
		OrderBy([]string{users.Col("id").String()}, Desc).
		SQL()

	want := `SELECT query_order_users.id AS "query_order_users.id" FROM query_order_users ORDER BY query_order_users.name ASC, query_order_users.id DESC`
	if sql != want {
		t.Fatalf("SQL() = %q, want %q", sql, want)
	}
	if len(args) != 0 {
		t.Fatalf("SQL() args = %v, want none", args)
	}
}

func TestSelectJoinsAndDistinct(t *testing.T) {
	users := getTestTable()
	other := schema.Table("other", schema.Column("id", schema.Int()))

	sql, _ := Select[struct{}](users.Col("id")).
		From(users).
		Distinct().
		RightJoin(other, Eq(users.Col("id"), other.Col("id"))).
		FullJoin(other, Eq(users.Col("id"), other.Col("id"))).
		CrossJoin(other).
		SQL()

	wantJoinSQL := `SELECT DISTINCT users.id AS "users.id" FROM users ` +
		"RIGHT JOIN other ON users.id = other.id " +
		"FULL JOIN other ON users.id = other.id " +
		"CROSS JOIN other"
	if sql != wantJoinSQL {
		t.Errorf("got join SQL %q, want %q", sql, wantJoinSQL)
	}
}

func TestComplexSelectQuery(t *testing.T) {
	users := getTestTable()

	orders := schema.Table("orders",
		schema.Column("id", schema.Int()).PrimaryKey(),
		schema.Column("user_id", schema.Int()),
		schema.Column("total", schema.Int()),
	)

	sql, args := Select[struct{}](users.Col("id"), users.Col("name")).
		From(users).
		InnerJoin(orders, Eq(users.Col("id"), orders.Col("user_id"))).
		Where(And(
			In(users.Col("age"), 20, 30, 40),
			Gte(orders.Col("total"), 100),
			IsNotNull(users.Col("email")),
			Raw("users.name != ?", "Excluded"),
		)).
		GroupBy(users.Col("id").String(), users.Col("name").String()).
		Having(Gt(orders.Col("total"), 50)).
		OrderBy([]string{users.Col("name").String()}, Asc).
		Limit(10).
		Offset(5).
		SQL()

	wantSQL := `SELECT users.id AS "users.id", users.name AS "users.name" FROM users ` +
		"INNER JOIN orders ON users.id = orders.user_id " +
		"WHERE (users.age IN ($1, $2, $3) AND orders.total >= $4 AND users.email IS NOT NULL AND users.name != $5) " +
		"GROUP BY users.id, users.name " +
		"HAVING orders.total > $6 " +
		"ORDER BY users.name ASC " +
		"LIMIT 10 OFFSET 5"

	if sql != wantSQL {
		t.Fatalf("got SQL %q\nwant %q", sql, wantSQL)
	}

	wantArgs := []any{20, 30, 40, 100, "Excluded", 50}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("got args %v, want %v", args, wantArgs)
	}
}

func TestSelectAutoColumnsFromTags(t *testing.T) {
	type User struct {
		ID     string `db:"users.id"`
		Name   string `db:"users.name"`
		Status string `db:"users.status"`
	}

	users := schema.Table("users",
		schema.Column("id", schema.UUID()),
		schema.Column("name", schema.Text()),
		schema.Column("status", schema.Text()),
	)

	sql, args := Select[User]().
		From(users).
		SQL()

	wantSQL := `SELECT users.id AS "users.id", users.name AS "users.name", users.status AS "users.status" FROM users`
	if sql != wantSQL {
		t.Fatalf("got SQL %q\nwant %q", sql, wantSQL)
	}
	if len(args) != 0 {
		t.Fatalf("got args %v, want none", args)
	}
}

func TestSelectRefCondition(t *testing.T) {
	type User struct {
		ID     string `db:"users.id"`
		Status string `db:"users.status"`
	}

	var u User
	users := schema.Table("users",
		schema.Column("id", schema.UUID()),
		schema.Column("status", schema.Text()),
	)

	sql, args := Select[User]().
		From(users).
		Where(Eq(Ref(&u, &u.Status), "active")).
		SQL()

	wantSQL := `SELECT users.id AS "users.id", users.status AS "users.status" FROM users WHERE users.status = $1`
	if sql != wantSQL {
		t.Fatalf("got SQL %q\nwant %q", sql, wantSQL)
	}
	if !reflect.DeepEqual(args, []any{"active"}) {
		t.Fatalf("got args %v", args)
	}
}

func TestSelectJoinAmbiguousColumnAliasing(t *testing.T) {
	type UserOrderRow struct {
		UserID  string `db:"users.id"`
		OrderID string `db:"orders.id"`
	}

	users := schema.Table("users", schema.Column("id", schema.UUID()))
	orders := schema.Table("orders", schema.Column("id", schema.Int()))

	sql, _ := Select[UserOrderRow]().
		From(users).
		InnerJoin(orders, Eq(users.Col("id"), orders.Col("id"))).
		SQL()

	wantSQL := `SELECT users.id AS "users.id", orders.id AS "orders.id" FROM users INNER JOIN orders ON users.id = orders.id`
	if sql != wantSQL {
		t.Fatalf("got SQL %q\nwant %q", sql, wantSQL)
	}
}
