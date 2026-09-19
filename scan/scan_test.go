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

