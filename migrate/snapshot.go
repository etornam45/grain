package migrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/etornam45/grain/schema"
)

type ColumnSnapshot struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	NotNull     bool   `json:"not_null,omitempty"`
	Unique      bool   `json:"unique,omitempty"`
	PK          bool   `json:"pk,omitempty"`
	Identity    bool   `json:"identity,omitempty"`
	Default     any    `json:"default,omitempty"`
	DefaultExpr string `json:"default_expr,omitempty"`
	Generated   string `json:"generated,omitempty"`
	Check       string `json:"check,omitempty"`
	Collation   string `json:"collation,omitempty"`
	RefTable    string `json:"ref_table,omitempty"`
	RefColumn   string `json:"ref_column,omitempty"`
	OnDelete    string `json:"on_delete,omitempty"`
	OnUpdate    string `json:"on_update,omitempty"`
}

type IndexSnapshot struct {
	Name      string   `json:"name"`
	Cols      []string `json:"cols"`
	Unique    bool     `json:"unique,omitempty"`
	Predicate string   `json:"predicate,omitempty"`
}

type ConstraintSnapshot struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
}

type TableSnapshot struct {
	Name        string               `json:"name"`
	Columns     []ColumnSnapshot     `json:"columns"`
	Indexes     []IndexSnapshot      `json:"indexes"`
	Uniques     []IndexSnapshot      `json:"uniques"`
	Constraints []ConstraintSnapshot `json:"constraints"`
}

type EnumSnapshot struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

type Snapshot struct {
	FormatVersion int             `json:"format_version"`
	Tables        []TableSnapshot `json:"tables"`
	Enums         []EnumSnapshot  `json:"enums"`
}

// snapshotFormatVersion must be bumped whenever the Snapshot shape changes so
// the dynamically generated loader and cached journals fail loudly on
// mismatches instead of silently diffing garbage.
const snapshotFormatVersion = 2

// BuildSnapshot walks schema.Registry and schema.EnumRegistry to produce the
// full schema state. It is also called from the dynamically generated loader
// program in LoadSnapshotFromPackage, so don't change the Snapshot shape
// without bumping snapshotFormatVersion.
func BuildSnapshot() Snapshot {
	var tables []TableSnapshot
	for _, t := range schema.Registry {
		var cols []ColumnSnapshot
		for _, c := range t.Columns() {
			cs := ColumnSnapshot{
				Name: c.Name, Type: c.Type.SQLType,
				NotNull: c.IsNotNull, Unique: c.IsUnique, PK: c.IsPK,
				Identity: c.IsIdentity, Default: c.DefaultVal,
				DefaultExpr: c.DefaultExprStr, Generated: c.GeneratedExpr,
				Check: c.CheckExpr, Collation: c.Collation,
			}
			if c.RefCol != nil {
				cs.RefTable = c.RefCol.Table
				cs.RefColumn = c.RefCol.Name
				cs.OnDelete = string(c.OnDeleteAction)
				cs.OnUpdate = string(c.OnUpdateAction)
			}
			cols = append(cols, cs)
		}
		var idx []IndexSnapshot
		for _, i := range t.GetIndices() {
			idx = append(idx, IndexSnapshot{Name: i.Name, Cols: i.Cols, Unique: i.Unique, Predicate: i.Predicate})
		}
		var uniques []IndexSnapshot
		for _, u := range t.GetUnique() {
			uniques = append(uniques, IndexSnapshot{Name: u.Name, Cols: u.Cols})
		}
		var constraints []ConstraintSnapshot
		for _, c := range t.GetConstraints() {
			constraints = append(constraints, ConstraintSnapshot{Name: c.Name, Expr: c.Expr})
		}
		tables = append(tables, TableSnapshot{
			Name: t.TableName(), Columns: cols, Indexes: idx,
			Uniques: uniques, Constraints: constraints,
		})
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].Name < tables[j].Name })

	var enums []EnumSnapshot
	for name, ct := range schema.EnumRegistry {
		enums = append(enums, EnumSnapshot{Name: name, Values: ct.EnumValues})
	}
	sort.Slice(enums, func(i, j int) bool { return enums[i].Name < enums[j].Name })

	return Snapshot{FormatVersion: snapshotFormatVersion, Tables: tables, Enums: enums}
}

type journalEntry struct {
	Version string `json:"version"`
	Name    string `json:"name"`
}

type journal struct {
	Entries []journalEntry `json:"entries"`
}

// snapshotVersion is the format version baked into every generated snapshot
// file so that journal/snapshot mismatches are loud rather than silent.
func checkFormatVersion(snap *Snapshot) error {
	if snap.FormatVersion != 0 && snap.FormatVersion != snapshotFormatVersion {
		return fmt.Errorf(
			"snapshot format version %d is not supported (this build understands version %d); "+
				"regenerate your snapshots or upgrade/downgrade grain",
			snap.FormatVersion, snapshotFormatVersion,
		)
	}
	return nil
}

func LoadLatestSnapshot(dir string) (Snapshot, error) {
	journalPath := filepath.Join(dir, "meta", "_journal.json")
	data, err := os.ReadFile(journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, err
	}

	var j journal
	if err := json.Unmarshal(data, &j); err != nil {
		return Snapshot{}, fmt.Errorf("parse journal: %w", err)
	}
	if len(j.Entries) == 0 {
		return Snapshot{}, nil
	}

	last := j.Entries[len(j.Entries)-1]
	snapPath := filepath.Join(dir, "meta", last.Version+"_snapshot.json")
	snapData, err := os.ReadFile(snapPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read snapshot %s: %w", last.Version, err)
	}

	var snap Snapshot
	if err := json.Unmarshal(snapData, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("parse snapshot %s: %w", last.Version, err)
	}
	if err := checkFormatVersion(&snap); err != nil {
		return Snapshot{}, fmt.Errorf("snapshot %s: %w", last.Version, err)
	}
	return snap, nil
}

func SaveSnapshot(dir, version, name string, snap Snapshot) error {
	metaDir := filepath.Join(dir, "meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}

	snapData, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(metaDir, version+"_snapshot.json"), snapData, 0o644); err != nil {
		return err
	}

	journalPath := filepath.Join(metaDir, "_journal.json")
	var j journal
	if data, err := os.ReadFile(journalPath); err == nil {
		_ = json.Unmarshal(data, &j) // best-effort; a corrupt journal shouldn't block writing this snapshot
	}
	j.Entries = append(j.Entries, journalEntry{Version: version, Name: name})

	jData, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(journalPath, jData, 0o644)
}