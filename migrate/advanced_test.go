package migrate

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestDiffColumnAttributeChanges(t *testing.T) {
	oldT := Snapshot{Tables: []TableSnapshot{{
		Name: "users",
		Columns: []ColumnSnapshot{
			{Name: "email", Type: "TEXT", Default: "active"},
		},
	}}}
	newT := Snapshot{Tables: []TableSnapshot{{
		Name: "users",
		Columns: []ColumnSnapshot{
			{Name: "email", Type: "VARCHAR(20)", NotNull: true, Unique: true, DefaultExpr: "now()"},
		},
	}}}

	changes, err := Diff(oldT, newT, ignorePrompt)
	if err != nil {
		t.Fatal(err)
	}

	wantKinds := []ChangeKind{AlterColumnType, AlterColumnNullability, AddColumnUnique, AlterColumnDefault}
	if len(changes) != len(wantKinds) {
		t.Fatalf("got %d changes %#v, want %d", len(changes), changes, len(wantKinds))
	}
	for i, kind := range wantKinds {
		if changes[i].Kind != kind {
			t.Fatalf("changes[%d].Kind = %s, want %s (all: %#v)", i, changes[i].Kind, kind, changes)
		}
	}

	if changes[0].UpSQL != "ALTER TABLE users ALTER COLUMN email TYPE VARCHAR(20);" {
		t.Fatalf("type change UpSQL = %q", changes[0].UpSQL)
	}
	if !changes[0].Destructive {
		t.Fatal("type change should be flagged destructive")
	}
	if changes[1].UpSQL != "ALTER TABLE users ALTER COLUMN email SET NOT NULL;" {
		t.Fatalf("not-null UpSQL = %q", changes[1].UpSQL)
	}
	if !changes[1].Destructive {
		t.Fatal("SET NOT NULL should be flagged destructive")
	}
	if changes[2].UpSQL != "ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);" {
		t.Fatalf("unique UpSQL = %q", changes[2].UpSQL)
	}
	if changes[3].UpSQL != "ALTER TABLE users ALTER COLUMN email SET DEFAULT now();" {
		t.Fatalf("default UpSQL = %q", changes[3].UpSQL)
	}
	if changes[3].DownSQL != "ALTER TABLE users ALTER COLUMN email SET DEFAULT 'active';" {
		t.Fatalf("default DownSQL = %q", changes[3].DownSQL)
	}
}

func TestDiffIdentityGeneratedAndCheck(t *testing.T) {
	oldT := Snapshot{Tables: []TableSnapshot{{
		Name: "users",
		Columns: []ColumnSnapshot{
			{Name: "id", Type: "INTEGER"},
			{Name: "price", Type: "NUMERIC(10,2)"},
		},
	}}}
	newT := Snapshot{Tables: []TableSnapshot{{
		Name: "users",
		Columns: []ColumnSnapshot{
			{Name: "id", Type: "INTEGER", Identity: true},
			{Name: "price", Type: "NUMERIC(10,2)", Generated: "amount * rate", Check: "price > 0"},
		},
	}}}

	changes, err := Diff(oldT, newT, ignorePrompt)
	if err != nil {
		t.Fatal(err)
	}

	var kinds []ChangeKind
	for _, c := range changes {
		kinds = append(kinds, c.Kind)
	}
	want := []ChangeKind{AlterColumnIdentity, AlterColumnGenerated, AlterColumnGenerated, AlterColumnCheck}
	if !equalKinds(kinds, want) {
		t.Fatalf("got kinds %v, want %v\nchanges: %#v", kinds, want, changes)
	}

	if changes[0].UpSQL != "ALTER TABLE users ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY;" {
		t.Fatalf("identity UpSQL = %q", changes[0].UpSQL)
	}
	if changes[3].UpSQL != "ALTER TABLE users ADD CONSTRAINT users_price_check CHECK (price > 0);" {
		t.Fatalf("check UpSQL = %q", changes[3].UpSQL)
	}
	hasGeneratedAdd := false
	for _, c := range changes {
		if strings.Contains(c.UpSQL, "ADD GENERATED ALWAYS AS (amount * rate) STORED;") {
			hasGeneratedAdd = true
		}
	}
	if !hasGeneratedAdd {
		t.Fatal("expected a generated-column ADD change")
	}
}

func equalKinds(a, b []ChangeKind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDiffTableLevelUniquesAndConstraints(t *testing.T) {
	oldT := Snapshot{Tables: []TableSnapshot{{
		Name: "accounts",
		Columns: []ColumnSnapshot{
			{Name: "id", Type: "INTEGER"},
			{Name: "org_id", Type: "INTEGER"},
			{Name: "balance", Type: "NUMERIC(12,2)"},
		},
		Uniques:     []IndexSnapshot{{Name: "uq_accounts_org", Cols: []string{"org_id"}}},
		Constraints: []ConstraintSnapshot{{Name: "chk_balance", Expr: "balance >= 0"}},
	}}}
	newT := Snapshot{Tables: []TableSnapshot{{
		Name: "accounts",
		Columns: []ColumnSnapshot{
			{Name: "id", Type: "INTEGER"},
			{Name: "org_id", Type: "INTEGER"},
			{Name: "balance", Type: "NUMERIC(12,2)"},
		},
		Uniques: []IndexSnapshot{
			{Name: "uq_accounts_org", Cols: []string{"org_id", "email"}},
			{Name: "uq_accounts_tenant", Cols: []string{"tenant_id"}},
		},
		Constraints: []ConstraintSnapshot{{Name: "chk_balance", Expr: "balance != 0"}},
	}}}

	changes, err := Diff(oldT, newT, ignorePrompt)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []ChangeKind
	for _, c := range changes {
		kinds = append(kinds, c.Kind)
	}
	want := []ChangeKind{
		DropUniqueConstraint, AddUniqueConstraint, AddUniqueConstraint,
		DropConstraint, AddConstraint,
	}
	if !equalKinds(kinds, want) {
		t.Fatalf("got kinds %v, want %v\nchanges: %#v", kinds, want, changes)
	}
}

func TestDiffUniqueConstraintUnchangedIsNoop(t *testing.T) {
	snap := Snapshot{Tables: []TableSnapshot{{
		Name: "accounts",
		Uniques:     []IndexSnapshot{{Name: "uq_a", Cols: []string{"org_id"}}},
		Constraints: []ConstraintSnapshot{{Name: "chk_a", Expr: "x > 0"}},
	}}}
	changes, err := Diff(snap, snap, ignorePrompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("identical snapshots produced %d changes: %#v", len(changes), changes)
	}
}

func TestDiffEnumRenameAndDelete(t *testing.T) {
	oldS := Snapshot{Enums: []EnumSnapshot{{Name: "status", Values: []string{"active", "disabled"}}}}
	newS := Snapshot{Enums: []EnumSnapshot{{Name: "state", Values: []string{"active", "disabled"}}}}

	renamePrompt := func(ctx PromptContext) (Resolution, error) {
		if ctx.Kind != AmbiguousEnum || ctx.OldName != "status" {
			t.Fatalf("unexpected prompt context: %#v", ctx)
		}
		return Resolution{Action: "rename", Target: "state"}, nil
	}
	changes, err := Diff(oldS, newS, renamePrompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != RenameEnum ||
		changes[0].UpSQL != "ALTER TYPE status RENAME TO state;" {
		t.Fatalf("rename changes: %#v", changes)
	}

	deletePrompt := func(ctx PromptContext) (Resolution, error) {
		return Resolution{Action: "delete"}, nil
	}
	changes, err = Diff(oldS, Snapshot{}, deletePrompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != DropEnum ||
		changes[0].UpSQL != "DROP TYPE status;" {
		t.Fatalf("delete changes: %#v", changes)
	}
}

func TestCreateTableIncludesConstraints(t *testing.T) {
	changes, err := Diff(Snapshot{}, Snapshot{Tables: []TableSnapshot{{
		Name: "accounts",
		Columns: []ColumnSnapshot{
			{Name: "org_id", Type: "INTEGER", NotNull: true},
			{Name: "balance", Type: "NUMERIC(12,2)", RefTable: "orgs", RefColumn: "id"},
		},
		Uniques:     []IndexSnapshot{{Name: "uq_accounts_org", Cols: []string{"org_id"}}},
		Constraints: []ConstraintSnapshot{{Name: "chk_balance", Expr: "balance >= 0"}},
	}}}, ignorePrompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != CreateTable {
		t.Fatalf("changes: %#v", changes)
	}
	up := changes[0].UpSQL
	for _, want := range []string{
		"CONSTRAINT uq_accounts_org UNIQUE (org_id)",
		"CONSTRAINT chk_balance CHECK (balance >= 0)",
		"FOREIGN KEY (balance) REFERENCES orgs(id)",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("create table missing %q:\n%s", want, up)
		}
	}
}

func TestGenerateForceBypassesDestructiveGuard(t *testing.T) {
	dir := t.TempDir()
	baseline := Snapshot{Tables: []TableSnapshot{{
		Name:    "users",
		Columns: []ColumnSnapshot{{Name: "email", Type: "TEXT"}},
	}}}
	if err := SaveSnapshot(dir, "1", "baseline", baseline); err != nil {
		t.Fatal(err)
	}
	unsafe := Snapshot{Tables: []TableSnapshot{{
		Name:    "users",
		Columns: []ColumnSnapshot{{Name: "email", Type: "VARCHAR(20)"}},
	}}}

	_, err := GenerateFromSnapshot(unsafe, dir, "tighten email", ignorePrompt)
	if !errors.As(err, new(*ManualReviewRequiredError)) {
		t.Fatalf("expected ManualReviewRequiredError, got %v", err)
	}

	path, err := GenerateFromSnapshotOpts(unsafe, dir, "tighten email", ignorePrompt, GenerateOptions{Force: true})
	if err != nil {
		t.Fatalf("forced generation: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "ALTER TABLE users ALTER COLUMN email TYPE VARCHAR(20);") {
		t.Fatalf("forced migration content:\n%s", content)
	}
	loaded, err := LoadLatestSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tables[0].Columns[0].Type != "VARCHAR(20)" {
		t.Fatalf("snapshot did not advance after forced generation: %#v", loaded)
	}
}

func TestLoadRejectsUnknownFormatVersion(t *testing.T) {
	dir := t.TempDir()
	if err := SaveSnapshot(dir, "1", "weird", Snapshot{FormatVersion: 99}); err != nil {
		t.Fatal(err)
	}
	_, err := LoadLatestSnapshot(dir)
	if err == nil || !strings.Contains(err.Error(), "format version 99") {
		t.Fatalf("LoadLatestSnapshot error = %v, want format-version error", err)
	}
}

func TestDiffCollationChange(t *testing.T) {
	oldS := Snapshot{Tables: []TableSnapshot{{
		Name: "users",
		Columns: []ColumnSnapshot{{Name: "name", Type: "TEXT", Collation: "en_US", Default: "anon"}},
	}}}
	newS := Snapshot{Tables: []TableSnapshot{{
		Name: "users",
		Columns: []ColumnSnapshot{{Name: "name", Type: "TEXT"}},
	}}}
	changes, err := Diff(oldS, newS, ignorePrompt)
	if err != nil {
		t.Fatal(err)
	}
	// Dropping a collation is a real type change (collate clause removal) and
	// so flows through the destructive AlterColumnType path.
	if len(changes) != 2 || changes[0].Kind != AlterColumnType || changes[1].Kind != AlterColumnDefault {
		t.Fatalf("collation removal changes: %#v", changes)
	}
	if changes[0].UpSQL != "ALTER TABLE users ALTER COLUMN name TYPE TEXT;" {
		t.Fatalf("type UpSQL = %q", changes[0].UpSQL)
	}
	if !strings.Contains(changes[0].DownSQL, "COLLATE en_US") {
		t.Fatalf("type DownSQL should restore collation: %q", changes[0].DownSQL)
	}
}