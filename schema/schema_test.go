package schema

import (
	"testing"
)

type userCols struct {
	ID        *ColumnDef
	FirstName *ColumnDef
	Email     *ColumnDef
	Role      *ColumnDef
}

func TestTableAndColumns(t *testing.T) {
	statusEnum := Enum("user_role", "admin", "member")

	users := Table[userCols]("users",
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
	if users.Cols.ID == nil || users.Cols.ID.Name != "id" {
		t.Errorf("users.Cols.ID not bound properly")
	}
	if !users.Cols.ID.IsPK {
		t.Errorf("expected ID to be primary key")
	}
	if users.Cols.FirstName == nil || users.Cols.FirstName.Name != "first_name" {
		t.Errorf("users.Cols.FirstName not bound properly (snake_case conversion)")
	}
	if !users.Cols.FirstName.IsNotNull {
		t.Errorf("expected first_name to be not null")
	}
	if users.Cols.Email == nil || !users.Cols.Email.IsUnique {
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

