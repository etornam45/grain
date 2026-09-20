package query

import (
	"reflect"
	"testing"
)

func TestNilConditionValues(t *testing.T) {
	users, _ := localTables()

	sql, args := Select[struct{}](users.Col("id")).
		From(users).
		Where(And(
			Eq(users.Col("name"), nil),
			Neq(users.Col("name"), nil),
			Gt(users.Col("age"), nil),
		)).
		SQL()

	want := `SELECT u2_users.id AS "u2_users.id" FROM u2_users ` +
		`WHERE (u2_users.name IS NULL AND u2_users.name IS NOT NULL AND u2_users.age > NULL)`
	if sql != want {
		t.Fatalf("got %q\nwant %q", sql, want)
	}
	if len(args) != 0 {
		t.Fatalf("nil conditions should bind no arguments, got %v", args)
	}
}

func TestInAndBetweenWithColumnRefs(t *testing.T) {
	users, orders := localTables()

	sql, args := Select[struct{}](users.Col("id")).
		From(users).
		Where(And(
			In(users.Col("id"), orders.Col("user_id"), 5, 7),
			NotIn(users.Col("id"), orders.Col("user_id")),
			Between(users.Col("age"), 18, users.Col("age")),
			NotBetween(users.Col("age"), orders.Col("user_id"), 60),
		)).
		SQL()

	want := `SELECT u2_users.id AS "u2_users.id" FROM u2_users WHERE (` +
		`u2_users.id IN (u2_orders.user_id, $1, $2) AND ` +
		`u2_users.id NOT IN (u2_orders.user_id) AND ` +
		`u2_users.age BETWEEN $3 AND u2_users.age AND ` +
		`u2_users.age NOT BETWEEN u2_orders.user_id AND $4)`
	if sql != want {
		t.Fatalf("got %q\nwant %q", sql, want)
	}
	if !reflect.DeepEqual(args, []any{5, 7, 18, 60}) {
		t.Fatalf("got args %v, want [5 7 18 60]", args)
	}
}

func TestUpdateSetOverwritesRepeatedKey(t *testing.T) {
	users, _ := localTables()

	sql, args := Update(users).
		Set(map[string]any{"name": "a"}).
		Set(map[string]any{"name": "b", "age": 1}).
		SQL()

	want := "UPDATE u2_users SET age = $1, name = $2"
	if sql != want {
		t.Fatalf("got %q\nwant %q", sql, want)
	}
	if !reflect.DeepEqual(args, []any{1, "b"}) {
		t.Fatalf("got args %v, want [1 b]", args)
	}
}

func TestRenumberPlaceholdersSkipsQuotedText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"name = '$5' AND age = $1", "name = '$5' AND age = $7"},
		{"x = 'it''s $3' AND y = ?", "x = 'it''s $3' AND y = $7"},
		{`price = $1 AND note = "a$2" AND more = ?`, `price = $7 AND note = "a$2" AND more = $8`},
		{"x = $$l$$ AND y = $1", "x = $$l$$ AND y = $7"},
		{"x = $tag$ amount: $5 $tag$ AND y = $1", "x = $tag$ amount: $5 $tag$ AND y = $7"},
	}
	for _, tc := range cases {
		if got := renumberPlaceholders(tc.in, 7); got != tc.want {
			t.Errorf("renumberPlaceholders(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRawRenumbersDollarAndQuestionMarks(t *testing.T) {
	users, _ := localTables()

	sql, args := Select[struct{}](users.Col("id")).
		From(users).
		Where(And(
			Eq(users.Col("name"), "joe"),
			Raw("age > $1 AND status = ? AND label = '$$10'", 21, "active"),
		)).
		SQL()

	if !contains(sql, "age > $2 AND status = $3 AND label = '$$10'") {
		t.Fatalf("got SQL %q, want numbered $2/$3 and untouched literal", sql)
	}
	if !reflect.DeepEqual(args, []any{"joe", 21, "active"}) {
		t.Fatalf("got args %v, want [joe 21 active]", args)
	}
}

type embedBaseT struct {
	CreatedAt string `db:"created_at"`
}

func TestExtractTagsHandlesPointerEmbedded(t *testing.T) {
	type P struct {
		*embedBaseT
		ID string `db:"users.id"`
	}
	got := extractTags(reflect.TypeOf(P{}))
	want := []string{"created_at", "users.id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got tags %v, want %v", got, want)
	}
}

func TestSelectPointerEmbeddedBuildsSQL(t *testing.T) {
	type P struct {
		*embedBaseT
		ID string `db:"users.id"`
	}
	users, _ := localTables()

	sql, _ := Select[P](users.Col("id")).From(users).SQL()
	if !contains(sql, `u2_users.id AS "u2_users.id"`) {
		t.Fatalf("got SQL %q", sql)
	}
}

func TestSelectRawColumnNotAliased(t *testing.T) {
	users, orders := localTables()

	sub := Sub(Select[struct{}](orders.Col("id")).From(orders).Limit(1))

	sql, _ := Select[struct{}](users.Col("id"), sub.As("latest")).
		From(users).
		SQL()

	if !contains(sql, "(SELECT u2_orders.id AS \"u2_orders.id\" FROM u2_orders LIMIT 1) AS latest") {
		t.Fatalf("subquery-as-column should be emitted verbatim, got %q", sql)
	}
}

func TestFormatSelectColUsesColRefType(t *testing.T) {
	if got := formatSelectCol(selectedCol{name: "users.name"}); got != `users.name AS "users.name"` {
		t.Fatalf("plain column should be aliased, got %q", got)
	}
	if got := formatSelectCol(selectedCol{name: "count(*)", expr: true}); got != "count(*)" {
		t.Fatalf("expression column should be verbatim, got %q", got)
	}
}