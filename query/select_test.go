package query

import (
	"reflect"
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

func TestSelectJoinsAndDistinct(t *testing.T) {
	users := getTestTable()
	type otherCols struct{ ID *schema.ColumnDef }
	other := schema.Table[otherCols]("other", schema.Column("id", schema.Int()))

	sql, _ := Select[struct{}](users.Cols.ID).
		From(users).
		Distinct().
		RightJoin(other, Eq(users.Cols.ID, other.Cols.ID)).
		FullJoin(other, Eq(users.Cols.ID, other.Cols.ID)).
		CrossJoin(other).
		SQL()

	wantJoinSQL := "SELECT DISTINCT users.id FROM users " +
		"RIGHT JOIN other ON users.id = other.id " +
		"FULL JOIN other ON users.id = other.id " +
		"CROSS JOIN other"
	if sql != wantJoinSQL {
		t.Errorf("got join SQL %q, want %q", sql, wantJoinSQL)
	}
}

func TestComplexSelectQuery(t *testing.T) {
	users := getTestTable()

	type orderCols struct {
		ID     *schema.ColumnDef
		UserID *schema.ColumnDef
		Total  *schema.ColumnDef
	}
	orders := schema.Table[orderCols]("orders",
		schema.Column("id", schema.Int()).PrimaryKey(),
		schema.Column("user_id", schema.Int()),
		schema.Column("total", schema.Int()),
	)

	sql, args := Select[struct{}](users.Cols.ID, users.Cols.Name).
		From(users).
		InnerJoin(orders, Eq(users.Cols.ID, orders.Cols.UserID)).
		Where(And(
			In(users.Cols.Age, 20, 30, 40),
			Gte(orders.Cols.Total, 100),
			IsNotNull(users.Cols.Email),
			Raw("users.name != ?", "Excluded"),
		)).
		GroupBy(users.Cols.ID.String(), users.Cols.Name.String()).
		Having(Gt(orders.Cols.Total, 50)).
		OrderBy([]string{users.Cols.Name.String()}, Asc).
		Limit(10).
		Offset(5).
		SQL()

	wantSQL := "SELECT users.id, users.name FROM users " +
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

