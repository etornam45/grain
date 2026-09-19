package query

import (
	"testing"
)

type testEmbeddedBase struct {
	ID string `db:"users.id"`
}

type testRefUser struct {
	testEmbeddedBase
	Name    string `db:"users.name"`
	Email   string `db:"users.email"`
	NoTag   string
	Age     int `db:"users.age"`
}

func TestRefSuccess(t *testing.T) {
	var u testRefUser

	colName := Ref(&u, &u.Name)
	if colName.String() != "users.name" {
		t.Errorf("got %q, want users.name", colName.String())
	}
	if colName.Str() != "users.name" {
		t.Errorf("got %q, want users.name", colName.Str())
	}

	colAge := ref(&u, &u.Age)
	if colAge.String() != "users.age" {
		t.Errorf("got %q, want users.age", colAge.String())
	}

	// Embedded struct field
	colID := Ref(&u, &u.ID)
	if colID.String() != "users.id" {
		t.Errorf("got %q, want users.id", colID.String())
	}
}

func TestRefNoTagPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for field without db tag")
		}
	}()

	var u testRefUser
	Ref(&u, &u.NoTag)
}

func TestRefUnrelatedPointerPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for pointer to unrelated instance")
		}
	}()

	var u1, u2 testRefUser
	Ref(&u1, &u2.Name)
}

func TestRefNilStructPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil struct pointer")
		}
	}()

	var dummy string
	Ref((*testRefUser)(nil), &dummy)
}

