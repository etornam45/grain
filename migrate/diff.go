package migrate

import (
	"fmt"
	"strings"
)

func Diff(old, new Snapshot, prompt PromptFunc) ([]Change, error) {
	var changes []Change
	changes = append(changes, diffEnums(old, new)...)

	tableChanges, err := diffTables(old, new, prompt)
	if err != nil {
		return nil, err
	}
	changes = append(changes, tableChanges...)
	return changes, nil
}

func diffEnums(old, new Snapshot) []Change {
	oldByName := map[string]EnumSnapshot{}
	for _, e := range old.Enums {
		oldByName[e.Name] = e
	}

	var changes []Change
	for _, e := range new.Enums {
		existing, ok := oldByName[e.Name]
		if !ok {
			changes = append(changes, Change{
				Kind:    CreateEnum,
				UpSQL:   fmt.Sprintf("CREATE TYPE %s AS ENUM (%s);", e.Name, quotedList(e.Values)),
				DownSQL: fmt.Sprintf("DROP TYPE %s;", e.Name),
			})
			continue
		}
		existingVals := toSet(existing.Values)
		for _, v := range e.Values {
			if existingVals[v] {
				continue
			}
			changes = append(changes, Change{
				Kind:         AddEnumValue,
				UpSQL:        fmt.Sprintf("ALTER TYPE %s ADD VALUE IF NOT EXISTS '%s';", e.Name, v),
				RequiresNoTx: true,
				Note: "Postgres can require ALTER TYPE ... ADD VALUE to run outside a " +
					"transaction, and a newly added value can't always be used in the " +
					"same transaction that added it. Run this alone.",
			})
		}
		// Enum value removal and enum renames aren't wired up yet: Postgres
		// has no DROP VALUE, and a removed enum name is the same rename-vs-
		// delete ambiguity as tables/columns below, just not routed through
		// the prompt for enums specifically.
	}
	return changes
}

func diffTables(old, new Snapshot, prompt PromptFunc) ([]Change, error) {
	oldTables := map[string]TableSnapshot{}
	for _, t := range old.Tables {
		oldTables[t.Name] = t
	}
	newTables := map[string]TableSnapshot{}
	for _, t := range new.Tables {
		newTables[t.Name] = t
	}

	var changes []Change
	matchedOld := map[string]bool{}
	matchedNew := map[string]bool{}

	for name, newT := range newTables {
		oldT, ok := oldTables[name]
		if !ok {
			continue
		}
		matchedOld[name] = true
		matchedNew[name] = true
		colChanges, err := diffColumns(name, oldT, newT, prompt)
		idxChanges, err2 := diffIndexes(name, oldT, newT)
		if err != nil || err2 != nil {
			return nil, err
		}
		changes = append(changes, colChanges...)
		changes = append(changes, idxChanges...)
	}

	unmatchedOld, unmatchedNew := findUnmatched(oldTables, newTables)

	for _, oldName := range unmatchedOld {
		res, err := prompt(PromptContext{Kind: AmbiguousTable, OldName: oldName, Candidates: unmatchedNew})
		if err != nil {
			return nil, err
		}
		switch res.Action {
		case "rename":
			changes = append(changes, Change{
				Kind:    RenameTable,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", oldName, res.Target),
				DownSQL: fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", res.Target, oldName),
			})
			unmatchedNew = removeString(unmatchedNew, res.Target)
			colChanges, err := diffColumns(res.Target, oldTables[oldName], newTables[res.Target], prompt)
			if err != nil {
				return nil, err
			}
			changes = append(changes, colChanges...)
		case "delete":
			changes = append(changes, Change{
				Kind:    DropTable,
				UpSQL:   fmt.Sprintf("DROP TABLE %s;", oldName),
				DownSQL: fmt.Sprintf("-- cannot auto-generate: %s's original definition is gone once dropped", oldName),
			})
		default: // "ignore"
		}
	}

	for _, t := range topoSortByFK(newTables, unmatchedNew) {
		changes = append(changes, createTableChange(t))
		idxChanges, err := diffIndexes(t.Name, TableSnapshot{}, t)
		if err != nil {
			return nil, err
		}
		changes = append(changes, idxChanges...)
	}

	return changes, nil
}

func diffColumns(table string, oldT, newT TableSnapshot, prompt PromptFunc) ([]Change, error) {
	oldCols := map[string]ColumnSnapshot{}
	for _, c := range oldT.Columns {
		oldCols[c.Name] = c
	}
	newCols := map[string]ColumnSnapshot{}
	for _, c := range newT.Columns {
		newCols[c.Name] = c
	}

	var changes []Change
	matchedOld := map[string]bool{}
	matchedNew := map[string]bool{}

	for name, newC := range newCols {
		oldC, ok := oldCols[name]
		if !ok {
			continue
		}
		matchedOld[name] = true
		matchedNew[name] = true
		if oldC.Type != newC.Type {
			changes = append(changes, Change{
				Kind:        AlterColumnType,
				UpSQL:       fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", table, name, newC.Type),
				DownSQL:     fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", table, name, oldC.Type),
				Destructive: true,
				Note: fmt.Sprintf(
					"%s.%s changed type from %s to %s. Whether this cast is safe depends "+
						"on existing data — review and add a USING clause if needed.",
					table, name, oldC.Type, newC.Type,
				),
			})
		}
	}

	var unmatchedOld, unmatchedNew []string
	for name := range oldCols {
		if !matchedOld[name] {
			unmatchedOld = append(unmatchedOld, name)
		}
	}
	for name := range newCols {
		if !matchedNew[name] {
			unmatchedNew = append(unmatchedNew, name)
		}
	}

	for _, oldName := range unmatchedOld {
		res, err := prompt(PromptContext{Kind: AmbiguousColumn, Table: table, OldName: oldName, Candidates: unmatchedNew})
		if err != nil {
			return nil, err
		}
		switch res.Action {
		case "rename":
			changes = append(changes, Change{
				Kind:    RenameColumn,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;", table, oldName, res.Target),
				DownSQL: fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;", table, res.Target, oldName),
			})
			unmatchedNew = removeString(unmatchedNew, res.Target)
		case "delete":
			changes = append(changes, Change{
				Kind:    DropColumn,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", table, oldName),
				DownSQL: fmt.Sprintf("-- cannot auto-generate: %s.%s's original definition is gone once dropped", table, oldName),
			})
		default: // "ignore"
		}
	}

	for _, name := range unmatchedNew {
		c := newCols[name]
		changes = append(changes, Change{
			Kind:        AddColumn,
			UpSQL:       fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", table, columnDDL(c)),
			DownSQL:     fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", table, name),
			Destructive: c.NotNull && c.Default == nil && c.DefaultExpr == "",
			Note:        addColumnNote(c),
		})
	}

	return changes, nil
}

func diffIndexes(table string, oldT, newT TableSnapshot) ([]Change, error) {
	oldIdx := map[string]IndexSnapshort{}
	for _, i := range oldT.Indexes {
		oldIdx[i.Name] = i
	}
	newIdx := map[string]IndexSnapshort{}
	for _, i := range newT.Indexes {
		newIdx[i.Name] = i
	}

	var changes []Change
	unmatchedOld, unmatchedNew := findUnmatched(oldIdx, newIdx)

	for _, oldName := range unmatchedOld {
		changes = append(changes, Change{
			Kind:    DropIndex,
			UpSQL:   fmt.Sprintf("DROP INDEX CONCURRENTLY IF EXISTS %s;", oldName),
			DownSQL: fmt.Sprintf("CREATE INDEX CONCURRENTLY %s ON %s (%s);", oldName, table, strings.Join(oldIdx[oldName].Cols, ", ")),
		})
	}

	for _, newName := range unmatchedNew {
		changes = append(changes, Change{
			Kind:    AddIndex,
			UpSQL:   fmt.Sprintf("CREATE INDEX CONCURRENTLY %s ON %s (%s);", newName, table, strings.Join(newIdx[newName].Cols, ", ")),
			DownSQL: fmt.Sprintf("DROP INDEX CONCURRENTLY IF EXISTS %s;", newName),
		})
	}

	return changes, nil
}

func findUnmatched[T any](old, new map[string]T) ([]string, []string) {
	matchedOld := map[string]bool{}
	matchedNew := map[string]bool{}
	for name, _ := range new {
		_, ok := old[name]
		if !ok {
			continue
		}

		matchedOld[name] = true
		matchedNew[name] = true
	}

	var unmatchedOld, unmatchedNew []string
	for name := range old {
		if !matchedOld[name] {
			unmatchedOld = append(unmatchedOld, name)
		}
	}
	for name := range new {
		if !matchedNew[name] {
			unmatchedNew = append(unmatchedNew, name)
		}
	}
	return unmatchedOld, unmatchedNew
}

// FIXME: It doesn't detect cycles — a circular FK between two brand-new tables still
// needs a manual migration (create both, then ALTER TABLE ADD CONSTRAINT).
func topoSortByFK(all map[string]TableSnapshot, names []string) []TableSnapshot {
	visited := map[string]bool{}
	var order []TableSnapshot
	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		t, ok := all[name]
		if !ok {
			return
		}
		for _, c := range t.Columns {
			if c.RefTable != "" && contains(names, c.RefTable) {
				visit(c.RefTable)
			}
		}
		order = append(order, t)
	}
	for _, name := range names {
		visit(name)
	}
	return order
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func removeString(list []string, s string) []string {
	out := list[:0]
	for _, v := range list {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

func createTableChange(t TableSnapshot) Change {
	var colDefs, pk []string
	for _, c := range t.Columns {
		colDefs = append(colDefs, columnDDL(c))
		if c.PK {
			pk = append(pk, c.Name)
		}
	}
	if len(pk) > 0 {
		colDefs = append(colDefs, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(pk, ", ")))
	}
	for _, c := range t.Columns {
		if c.RefTable == "" {
			continue
		}
		fk := fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s(%s)", c.Name, c.RefTable, c.RefColumn)
		if c.OnDelete != "" {
			fk += " ON DELETE " + c.OnDelete
		}
		if c.OnUpdate != "" {
			fk += " ON UPDATE " + c.OnUpdate
		}
		colDefs = append(colDefs, fk)
	}

	up := fmt.Sprintf("CREATE TABLE %s (\n  %s\n);", t.Name, strings.Join(colDefs, ",\n  "))
	down := fmt.Sprintf("DROP TABLE %s;", t.Name)
	return Change{Kind: CreateTable, UpSQL: up, DownSQL: down}
}

// NOTE: ColumnDDL returns `COL_NAME COL_TYPE NOT NULL UNIQUE DEFAULT VALUE`
func columnDDL(c ColumnSnapshot) string {
	parts := []string{c.Name, c.Type}
	if c.NotNull {
		parts = append(parts, "NOT NULL")
	}
	if c.Unique {
		parts = append(parts, "UNIQUE")
	}
	switch {
	case c.DefaultExpr != "":
		parts = append(parts, "DEFAULT "+c.DefaultExpr)
	case c.Default != nil:
		parts = append(parts, "DEFAULT "+formatDefault(c.Default))
	}
	return strings.Join(parts, " ")
}

func formatDefault(v any) string {
	if s, ok := v.(string); ok {
		if strings.Contains(s, "(") {
			return s
		}
		return "'" + s + "'"
	}
	return fmt.Sprintf("%v", v)
}

func addColumnNote(c ColumnSnapshot) string {
	if c.NotNull && c.Default == nil && c.DefaultExpr == "" {
		return fmt.Sprintf(
			"Adding NOT NULL column %q with no default will fail on a table with "+
				"existing rows. Add a Default()/DefaultExpr() or backfill manually first.",
			c.Name,
		)
	}
	return ""
}

func quotedList(vals []string) string {
	q := make([]string, len(vals))
	for i, v := range vals {
		q[i] = "'" + v + "'"
	}
	return strings.Join(q, ", ")
}

func toSet(vals []string) map[string]bool {
	s := map[string]bool{}
	for _, v := range vals {
		s[v] = true
	}
	return s
}
