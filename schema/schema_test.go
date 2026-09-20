package schema

import (
	"testing"
)

func TestTableAndColumns(t *testing.T) {
	statusEnum := Enum("user_role", "admin", "member")

	users := Table("users",
		Column("id", UUID()).PrimaryKey().Default("gen_random_uuid()"),
		Column("first_name", Varchar(100)).NotNull(),
		Column("email", Text()).Unique().Index(),
		Column("role", statusEnum).Default("member"),
	)

	if users.TableName() != "users" {
		t.Errorf("got TableName %q, want 'users'", users.TableName())
	}
	if len(users.Columns()) != 4 {
		t.Errorf("got %d columns, want 4", len(users.Columns()))
	}
	if users.Col("id") == nil || users.Col("id").Name != "id" {
		t.Errorf("users.Col(\"id\") not found or invalid")
	}
	if !users.Col("id").IsPK {
		t.Errorf("expected ID to be primary key")
	}
	if users.Col("first_name") == nil || users.Col("first_name").Name != "first_name" {
		t.Errorf("users.Col(\"first_name\") not found or invalid")
	}
	if !users.Col("first_name").IsNotNull {
		t.Errorf("expected first_name to be not null")
	}
	if users.Col("email") == nil || !users.Col("email").IsUnique {
		t.Errorf("expected email to be unique")
	}
	if len(users.GetIndices()) != 1 {
		t.Errorf("expected 1 index, got %d", len(users.GetIndices()))
	} else if users.GetIndices()[0].Name != "idx_users_email" {
		t.Errorf("expected index idx_users_email, got %s", users.GetIndices()[0].Name)
	}

	col := users.Col("first_name")
	if col.String() != "users.first_name" {
		t.Errorf("got %q, want users.first_name", col.String())
	}
}

func TestColumnReferences(t *testing.T) {
	pkCol := Column("id", Int()).PrimaryKey()
	fkCol := Column("user_id", Int()).
		References(pkCol).
		OnDelete(Cascade).
		OnUpdate(SetNull)

	if fkCol.RefCol != pkCol {
		t.Errorf("expected fkCol to reference pkCol")
	}
	if fkCol.OnDeleteAction != Cascade {
		t.Errorf("expected Cascade on delete")
	}
	if fkCol.OnUpdateAction != SetNull {
		t.Errorf("expected SetNull on update")
	}
}

func TestNewColumnTypes(t *testing.T) {
	tests := []struct {
		name string
		typ  *ColumnType
		want string
	}{
		{"smallserial", SmallSerial(), "SMALLSERIAL"},
		{"smallint", SmallInt(), "SMALLINT"},
		{"real", Real(), "REAL"},
		{"double", Double(), "DOUBLE PRECISION"},
		{"bytea", Bytea(), "BYTEA"},
		{"date", Date(), "DATE"},
		{"time", Time(), "TIME"},
		{"timetz", Timetz(), "TIMETZ"},
		{"interval", Interval(), "INTERVAL"},
		{"money", Money(), "MONEY"},
		{"char", Char(8), "CHAR(8)"},
		{"array", Array(Int()), "INTEGER[]"},
		{"array_text", Array(Varchar(10)), "VARCHAR(10)[]"},
		{"varchar", Varchar(80), "VARCHAR(80)"},
		{"numeric", Numeric(10, 2), "NUMERIC(10,2)"},
	}
	for _, tt := range tests {
		if tt.typ.SQLType != tt.want {
			t.Errorf("%s: got %s, want %s", tt.name, tt.typ.SQLType, tt.want)
		}
	}
}

func TestColumnModifiers(t *testing.T) {
	col := Column("price", Numeric(12, 2)).
		Identity().
		GeneratedAs("(amount * rate)").
		Check("price > 0").
		Collate("C")

	if !col.IsIdentity {
		t.Error("expected IsIdentity")
	}
	if col.GeneratedExpr != "(amount * rate)" {
		t.Errorf("got GeneratedExpr %q", col.GeneratedExpr)
	}
	if col.CheckExpr != "price > 0" {
		t.Errorf("got CheckExpr %q", col.CheckExpr)
	}
	if col.Collation != "C" {
		t.Errorf("got Collation %q", col.Collation)
	}
}

func TestTableLevelConstraints(t *testing.T) {
	accounts := Table("accounts",
		Column("id", Int()).PrimaryKey(),
		Column("org_id", Int()),
		Column("email", Text()).NotNull(),
		Column("balance", Numeric(12, 2)),
	).
		IndexUnique("uniq_org_email", "org_id", "email").
		IndexPartial("idx_active_accounts", []string{"org_id"}, "status = 'active'").
		Unique("uq_accounts_org", "org_id").
		Constraint("chk_balance_positive", "balance >= 0")

	if len(accounts.GetIndices()) != 2 {
		t.Fatalf("got %d indices, want 2", len(accounts.GetIndices()))
	}
	uniq := accounts.GetIndices()[0]
	if !uniq.Unique || uniq.Name != "uniq_org_email" {
		t.Errorf("got index %+v, want unique named uniq_org_email", uniq)
	}
	partial := accounts.GetIndices()[1]
	if partial.Predicate != "status = 'active'" {
		t.Errorf("got predicate %q", partial.Predicate)
	}

	if len(accounts.GetUnique()) != 1 || accounts.GetUnique()[0].Name != "uq_accounts_org" {
		t.Errorf("got uniques %+v, want [uq_accounts_org]", accounts.GetUnique())
	}
	if len(accounts.GetConstraints()) != 1 ||
		accounts.GetConstraints()[0].Name != "chk_balance_positive" ||
		accounts.GetConstraints()[0].Expr != "balance >= 0" {
		t.Errorf("got constraints %+v", accounts.GetConstraints())
	}
}

func TestArrayType(t *testing.T) {
	arr := Array(Varchar(20))
	if arr.SQLType != "VARCHAR(20)[]" {
		t.Errorf("got %q, want VARCHAR(20)[]", arr.SQLType)
	}
}
