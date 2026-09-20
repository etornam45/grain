package scan

import (
	"testing"
)

type userRow struct {
	ID    int    `db:"users.id"`
	Name  string `db:"users.name"`
	Email string `db:"email"`
}

func TestDestinations(t *testing.T) {
	var u userRow

	// Test exact match and suffix match
	cols := []string{"users.id", "name", "email"}
	ptrs, err := destinations(&u, cols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ptrs) != 3 {
		t.Fatalf("expected 3 ptrs, got %d", len(ptrs))
	}

	// Test missing column error
	_, err = destinations(&u, []string{"nonexistent_column"})
	if err == nil {
		t.Fatalf("expected error for nonexistent_column, got nil")
	}
}

type baseFields struct {
	CreatedAt string `db:"created_at"`
}

type embeddedRow struct {
	baseFields
	ID   int    `db:"users.id"`
	Name string `db:"users.name"`
}

func TestDestinationsFlattensEmbeddedStruct(t *testing.T) {
	var r embeddedRow
	ptrs, err := destinations(&r, []string{"users.id", "users.name", "created_at"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ptrs) != 3 {
		t.Fatalf("expected 3 ptrs, got %d", len(ptrs))
	}

	*(ptrs[0].(*int)) = 7
	*(ptrs[2].(*string)) = "ts"
	if r.ID != 7 || r.CreatedAt != "ts" {
		t.Fatalf("scan targets did not reach embedded fields: %+v", r)
	}
}

type skippedRow struct {
	ID    int    `db:"users.id"`
	Skip  string `db:"-"`
	Other string `db:"other"`
}

func TestDestinationsSkipsDashedField(t *testing.T) {
	var s skippedRow
	ptrs, err := destinations(&s, []string{"users.id", "other"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ptrs) != 2 {
		t.Fatalf("expected 2 ptrs, got %d", len(ptrs))
	}
}

type pointerRow struct {
	ID   int     `db:"users.id"`
	Name *string `db:"users.name"`
	Bad  string  `db:"bad"`
}

func TestDestinationsAllocatesPointerFields(t *testing.T) {
	var p pointerRow
	ptrs, err := destinations(&p, []string{"users.id", "users.name", "bad"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	namePtr, ok := ptrs[1].(**string)
	if !ok {
		t.Fatalf("pointer field dest should be **string, got %T", ptrs[1])
	}
	name := "alice"
	*namePtr = &name
	if p.Name == nil || *p.Name != "alice" {
		t.Fatalf("pointer field not populated: %v", p.Name)
	}
	if p.Bad != "" {
		t.Fatalf("unexpected value for uninitialized dest %v", p.Bad)
	}
}

type dupTagRow struct {
	A string `db:"users.name"`
	B string `db:"users.name"`
}

func TestDestinationsRejectsDuplicateTags(t *testing.T) {
	var d dupTagRow
	if _, err := destinations(&d, []string{"users.name"}); err == nil {
		t.Fatal("expected duplicate-tag error")
	}
}

type ambiguousRow struct {
	UserID  string `db:"users.id"`
	OrderID string `db:"orders.id"`
}

func TestDestinationsRejectsAmbiguousBareColumn(t *testing.T) {
	var a ambiguousRow
	if _, err := destinations(&a, []string{"id"}); err == nil {
		t.Fatal("expected ambiguity error for bare column matching two tags")
	}
}

func TestDestinationsAllocatesPointerRow(t *testing.T) {
	var p *userRow
	ptrs, err := destinations(&p, []string{"users.id", "users.name", "email"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("nil pointer row was not allocated")
	}
	*(ptrs[0].(*int)) = 3
	*(ptrs[1].(*string)) = "bob"
	*(ptrs[2].(*string)) = "bob@example.com"
	if p.ID != 3 || p.Name != "bob" || p.Email != "bob@example.com" {
		t.Fatalf("scan targets did not reach pointer row: %+v", p)
	}
}

type BaseRow struct {
	ID    int    `db:"users.id"`
	Name  string `db:"users.name"`
	Email string `db:"email"`
}

type ptrEmbeddedRow struct {
	*BaseRow
	Extra string `db:"extra"`
}

func TestDestinationsFlattensPointerEmbeddedStruct(t *testing.T) {
	var p *ptrEmbeddedRow
	ptrs, err := destinations(&p, []string{"users.id", "users.name", "email", "extra"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil || p.BaseRow == nil {
		t.Fatalf("embedded pointer not allocated: %+v", p)
	}
	if len(ptrs) != 4 {
		t.Fatalf("expected 4 ptrs, got %d", len(ptrs))
	}
	*(ptrs[0].(*int)) = 9
	if p.ID != 9 {
		t.Fatalf("scan target did not reach embedded pointer row: %+v", p)
	}
}
