package migrate

import (
	"fmt"
	"sort"
	"strings"
)

func Diff(old, new Snapshot, prompt PromptFunc) ([]Change, error) {
	var changes []Change
	enumChanges, err := diffEnums(old, new, prompt)
	if err != nil {
		return nil, err
	}
	changes = append(changes, enumChanges...)

	tableChanges, err := diffTables(old, new, prompt)
	if err != nil {
		return nil, err
	}
	changes = append(changes, tableChanges...)
	return changes, nil
}

func diffEnums(old, new Snapshot, prompt PromptFunc) ([]Change, error) {
	oldByName := map[string]EnumSnapshot{}
	for _, e := range old.Enums {
		oldByName[e.Name] = e
	}
	newByName := map[string]EnumSnapshot{}
	for _, e := range new.Enums {
		newByName[e.Name] = e
	}

	var changes []Change

	// NOTE: Resolve removed/renamed enums first so a rename target isn't also
	// emitted as a fresh CREATE TYPE below.
	unmatchedOld, unmatchedNew := findUnmatched(oldByName, newByName)
	for _, oldName := range unmatchedOld {
		res, err := prompt(PromptContext{Kind: AmbiguousEnum, OldName: oldName, Candidates: unmatchedNew})
		if err != nil {
			return nil, err
		}
		switch res.Action {
		case "rename":
			changes = append(changes, Change{
				Kind:    RenameEnum,
				UpSQL:   fmt.Sprintf("ALTER TYPE %s RENAME TO %s;", oldName, res.Target),
				DownSQL: fmt.Sprintf("ALTER TYPE %s RENAME TO %s;", res.Target, oldName),
			})
			unmatchedNew = removeString(unmatchedNew, res.Target)
		case "delete":
			changes = append(changes, Change{
				Kind:    DropEnum,
				UpSQL:   fmt.Sprintf("DROP TYPE %s;", oldName),
				DownSQL: fmt.Sprintf("-- cannot auto-generate: %s's original values are gone once dropped", oldName),
			})
		default: // "ignore"
		}
	}

	// Create enums that are genuinely new (every other new name was either
	// matched to an existing enum or already consumed by a rename above).
	for _, name := range sortedStringSlice(unmatchedNew) {
		e := newByName[name]
		changes = append(changes, Change{
			Kind:    CreateEnum,
			UpSQL:   fmt.Sprintf("CREATE TYPE %s AS ENUM (%s);", e.Name, quotedList(e.Values)),
			DownSQL: fmt.Sprintf("DROP TYPE %s;", e.Name),
		})
	}

	// Add newly introduced values to enums that exist on both sides.
	for _, e := range new.Enums {
		existing, ok := oldByName[e.Name]
		if !ok {
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
		// Removed values can't be generated: Postgres has no ALTER TYPE ...
		// DROP VALUE. When values disappear the snapshot simply stops tracking
		// them; recycling a removed name still requires a manual migration
		// (recreate the type + backfill).
	}

	return changes, nil
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

	var renames []Change
	var alters []Change

	// Existing matched tables: compute their column/index/unique/constraint
	// changes but defer emission
	for _, name := range sortedKeys(newTables) {
		newT := newTables[name]
		oldT, ok := oldTables[name]
		if !ok {
			continue
		}
		tableChanges, err := tableAlters(name, oldT, newT, prompt)
		if err != nil {
			return nil, err
		}
		alters = append(alters, tableChanges...)
	}

	unmatchedOld, unmatchedNew := findUnmatched(oldTables, newTables)

	// Renames/deletes must run before creates so that a renamed table is
	// referenced under its new name; their column diffs are deferred to alters.
	for _, oldName := range unmatchedOld {
		res, err := prompt(PromptContext{Kind: AmbiguousTable, OldName: oldName, Candidates: unmatchedNew})
		if err != nil {
			return nil, err
		}
		switch res.Action {
		case "rename":
			renames = append(renames, Change{
				Kind:    RenameTable,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", oldName, res.Target),
				DownSQL: fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", res.Target, oldName),
			})
			unmatchedNew = removeString(unmatchedNew, res.Target)
			tableChanges, err := tableAlters(res.Target, oldTables[oldName], newTables[res.Target], prompt)
			if err != nil {
				return nil, err
			}
			alters = append(alters, tableChanges...)
		case "delete":
			renames = append(renames, Change{
				Kind:    DropTable,
				UpSQL:   fmt.Sprintf("DROP TABLE %s;", oldName),
				DownSQL: fmt.Sprintf("-- cannot auto-generate: %s's original definition is gone once dropped", oldName),
			})
		default: // "ignore"
		}
	}

	var creates []Change
	newTablesInOrder, err := topoSortByFK(newTables, unmatchedNew)
	if err != nil {
		return nil, err
	}
	for _, t := range newTablesInOrder {
		creates = append(creates, createTableChange(t))
		idxChanges, err := diffIndexes(t.Name, TableSnapshot{}, t)
		if err != nil {
			return nil, err
		}
		creates = append(creates, idxChanges...)
	}

	var changes []Change
	changes = append(changes, renames...)
	changes = append(changes, creates...)
	changes = append(changes, alters...)
	return changes, nil
}

func tableAlters(table string, oldT, newT TableSnapshot, prompt PromptFunc) ([]Change, error) {
	colChanges, err := diffColumns(table, oldT, newT, prompt)
	if err != nil {
		return nil, err
	}
	idxChanges, err := diffIndexes(table, oldT, newT)
	if err != nil {
		return nil, err
	}
	uniqueChanges, err := diffUniques(table, oldT, newT)
	if err != nil {
		return nil, err
	}
	constraintChanges, err := diffConstraints(table, oldT, newT)
	if err != nil {
		return nil, err
	}

	var changes []Change
	changes = append(changes, orderColumnChanges(colChanges)...)
	changes = append(changes, idxChanges...)
	changes = append(changes, uniqueChanges...)
	changes = append(changes, constraintChanges...)
	return changes, nil
}

// orderColumnChanges moves constraint-drop changes ahead of column drops.
// PostgreSQL refuses to DROP COLUMN while the column is still referenced by a
// PRIMARY KEY, UNIQUE, FOREIGN KEY or CHECK constraint, so the constraint must
// go first.
func orderColumnChanges(changes []Change) []Change {
	leadKind := map[ChangeKind]bool{
		DropPrimaryKey:       true,
		DropForeignKey:       true,
		DropColumnUnique:     true,
		DropUniqueConstraint: true,
		DropConstraint:       true,
	}
	var leads, rest []Change
	for _, c := range changes {
		if leadKind[c.Kind] {
			leads = append(leads, c)
		} else {
			rest = append(rest, c)
		}
	}
	if len(leads) == 0 {
		return changes
	}
	return append(leads, rest...)
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

	for _, name := range sortedKeys(newCols) {
		newC := newCols[name]
		oldC, ok := oldCols[name]
		if !ok {
			continue
		}
		matchedOld[name] = true
		matchedNew[name] = true
		changes = append(changes, diffColumnChanges(table, oldC, newC)...)
	}

	var unmatchedOld, unmatchedNew []string
	for _, name := range sortedKeys(oldCols) {
		if !matchedOld[name] {
			unmatchedOld = append(unmatchedOld, name)
		}
	}
	for _, name := range sortedKeys(newCols) {
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
			changes = append(changes, diffColumnChanges(table, oldCols[oldName], newCols[res.Target])...)
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
			UpSQL:       fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", table, columnDDLWithRef(c)),
			DownSQL:     fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", table, name),
			Destructive: c.NotNull && c.Default == nil && c.DefaultExpr == "" && !c.Identity,
			Note:        addColumnNote(c),
		})
	}

	changes = append(changes, diffPrimaryKey(table, oldT, newT)...)
	changes = append(changes, diffForeignKeys(table, oldT, newT)...)

	return changes, nil
}

// diffColumnChanges emits changes for every attribute difference between two
// same-named columns: type/collation, identity, generated expression, check,
// nullability, uniqueness and default.
func diffColumnChanges(table string, oldC, newC ColumnSnapshot) []Change {
	var changes []Change

	if oldC.Type != newC.Type || oldC.Collation != newC.Collation {
		oldType := columnTypeWithCollation(oldC)
		newType := columnTypeWithCollation(newC)
		changes = append(changes, Change{
			Kind:        AlterColumnType,
			UpSQL:       fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", table, newC.Name, newType),
			DownSQL:     fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", table, oldC.Name, oldType),
			Destructive: true,
			Note: fmt.Sprintf(
				"%s.%s changed type from %s to %s. Whether this cast is safe depends "+
					"on existing data — review and add a USING clause if needed.",
				table, oldC.Name, oldC.Type, newC.Type,
			),
		})
	}

	if oldC.Identity != newC.Identity {
		if newC.Identity {
			changes = append(changes, Change{
				Kind:    AlterColumnIdentity,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s ADD GENERATED ALWAYS AS IDENTITY;", table, newC.Name),
				DownSQL: fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP IDENTITY;", table, newC.Name),
			})
		} else {
			changes = append(changes, Change{
				Kind:    AlterColumnIdentity,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP IDENTITY;", table, newC.Name),
				DownSQL: fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s ADD GENERATED ALWAYS AS IDENTITY;", table, newC.Name),
			})
		}
	}

	if oldC.Generated != newC.Generated {
		drop := Change{
			Kind:        AlterColumnGenerated,
			UpSQL:       fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP EXPRESSION;", table, oldC.Name),
			DownSQL:     fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s ADD GENERATED ALWAYS AS (%s) STORED;", table, oldC.Name, oldC.Generated),
			Destructive: true,
			Note:        fmt.Sprintf("%s.%s changed its generated-expression definition; review existing rows.", table, oldC.Name),
		}
		if newC.Generated != "" {
			drop.DownSQL = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP EXPRESSION;", table, newC.Name)
		}
		changes = append(changes, drop)
		if newC.Generated != "" {
			changes = append(changes, Change{
				Kind:        AlterColumnGenerated,
				UpSQL:       fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s ADD GENERATED ALWAYS AS (%s) STORED;", table, newC.Name, newC.Generated),
				DownSQL:     fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP EXPRESSION;", table, newC.Name),
				Destructive: true,
				Note:        fmt.Sprintf("%s.%s is a newly generated column; verify the expression on existing rows.", table, newC.Name),
			})
		}
	}

	if oldC.Check != newC.Check {
		if oldC.Check != "" {
			changes = append(changes, Change{
				Kind:    AlterColumnCheck,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, checkConstraintName(table, oldC.Name)),
				DownSQL: fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s);", table, checkConstraintName(table, oldC.Name), oldC.Check),
			})
		}
		if newC.Check != "" {
			changes = append(changes, Change{
				Kind:    AlterColumnCheck,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s);", table, checkConstraintName(table, newC.Name), newC.Check),
				DownSQL: fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, checkConstraintName(table, newC.Name)),
			})
		}
	}

	if oldC.NotNull != newC.NotNull {
		if newC.NotNull {
			changes = append(changes, Change{
				Kind:        AlterColumnNullability,
				UpSQL:       fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", table, newC.Name),
				DownSQL:     fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", table, newC.Name),
				Destructive: true,
				Note: fmt.Sprintf(
					"%s.%s becomes NOT NULL. This fails on existing NULL rows — "+
						"backfill them or add a default first.", table, newC.Name,
				),
			})
		} else {
			changes = append(changes, Change{
				Kind:    AlterColumnNullability,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", table, newC.Name),
				DownSQL: fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", table, newC.Name),
			})
		}
	}

	if oldC.Unique != newC.Unique {
		con := uniqueConstraintName(table, newC.Name)
		if newC.Unique {
			changes = append(changes, Change{
				Kind:    AddColumnUnique,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", table, con, newC.Name),
				DownSQL: fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, con),
			})
		} else {
			changes = append(changes, Change{
				Kind:    DropColumnUnique,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, con),
				DownSQL: fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", table, con, newC.Name),
			})
		}
	}

	oldDef, newDef := defaultClause(oldC), defaultClause(newC)
	if oldDef != newDef {
		if newDef == "" {
			changes = append(changes, Change{
				Kind:    AlterColumnDefault,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", table, newC.Name),
				DownSQL: fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", table, newC.Name, oldDef),
			})
		} else {
			changes = append(changes, Change{
				Kind:    AlterColumnDefault,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", table, newC.Name, newDef),
				DownSQL: fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", table, newC.Name, oldDef),
			})
		}
	}

	return changes
}

func columnTypeWithCollation(c ColumnSnapshot) string {
	if c.Collation != "" {
		return c.Type + " COLLATE " + c.Collation
	}
	return c.Type
}

// diffForeignKeys emits drop+add changes for FK attributes that changed on an
// existing column, plus add/drop for columns that gained/lost a reference.
func diffForeignKeys(table string, oldT, newT TableSnapshot) []Change {
	oldCols := map[string]ColumnSnapshot{}
	for _, c := range oldT.Columns {
		oldCols[c.Name] = c
	}
	newCols := map[string]ColumnSnapshot{}
	for _, c := range newT.Columns {
		newCols[c.Name] = c
	}

	var changes []Change
	for _, name := range sortedKeys(newCols) {
		oldC, ok := oldCols[name]
		if !ok {
			continue
		}
		newC := newCols[name]
		oldFK := fkClause(table, oldC)
		newFK := fkClause(table, newC)
		if oldFK == "" && newFK == "" {
			continue
		}
		if oldFK != "" && newFK == "" {
			changes = append(changes, Change{
				Kind:    DropForeignKey,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, fkConstraintName(table, name)),
				DownSQL: fmt.Sprintf("ALTER TABLE %s ADD %s;", table, oldFK),
			})
			continue
		}
		if oldFK == "" && newFK != "" {
			changes = append(changes, Change{
				Kind:    AddForeignKey,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s ADD %s;", table, newFK),
				DownSQL: fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, fkConstraintName(table, name)),
			})
			continue
		}
		if oldFK != newFK {
			changes = append(changes, Change{
				Kind:    DropForeignKey,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, fkConstraintName(table, name)),
				DownSQL: fmt.Sprintf("ALTER TABLE %s ADD %s;", table, oldFK),
			})
			changes = append(changes, Change{
				Kind:    AddForeignKey,
				UpSQL:   fmt.Sprintf("ALTER TABLE %s ADD %s;", table, newFK),
				DownSQL: fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, fkConstraintName(table, name)),
			})
		}
	}
	return changes
}

func diffPrimaryKey(table string, oldT, newT TableSnapshot) []Change {
	oldPks := pkCols(oldT)
	newPks := pkCols(newT)
	if sameColumns(oldPks, newPks) {
		return nil
	}
	var changes []Change
	con := table + "_pkey"
	if len(oldPks) > 0 {
		changes = append(changes, Change{
			Kind:    DropPrimaryKey,
			UpSQL:   fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, con),
			DownSQL: fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s);", table, strings.Join(oldPks, ", ")),
		})
	}
	if len(newPks) > 0 {
		changes = append(changes, Change{
			Kind:    AddPrimaryKey,
			UpSQL:   fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s);", table, strings.Join(newPks, ", ")),
			DownSQL: fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, con),
		})
	}
	return changes
}

func pkCols(t TableSnapshot) []string {
	var cols []string
	for _, c := range t.Columns {
		if c.PK {
			cols = append(cols, c.Name)
		}
	}
	return cols
}

func fkClause(table string, c ColumnSnapshot) string {
	if c.RefTable == "" || c.RefColumn == "" {
		return ""
	}
	fk := fmt.Sprintf("CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s)",
		fkConstraintName(table, c.Name), c.Name, c.RefTable, c.RefColumn)
	if c.OnDelete != "" {
		fk += " ON DELETE " + c.OnDelete
	}
	if c.OnUpdate != "" {
		fk += " ON UPDATE " + c.OnUpdate
	}
	return fk
}

func fkConstraintName(table, col string) string {
	return fmt.Sprintf("%s_%s_fkey", table, col)
}

func uniqueConstraintName(table, col string) string {
	return fmt.Sprintf("%s_%s_key", table, col)
}

func checkConstraintName(table, col string) string {
	return fmt.Sprintf("%s_%s_check", table, col)
}

func defaultClause(c ColumnSnapshot) string {
	switch {
	case c.DefaultExpr != "":
		return c.DefaultExpr
	case c.Default != nil:
		return formatDefault(c.Default)
	}
	return ""
}

func diffIndexes(table string, oldT, newT TableSnapshot) ([]Change, error) {
	oldIdx := map[string]IndexSnapshot{}
	for _, i := range oldT.Indexes {
		oldIdx[i.Name] = i
	}
	newIdx := map[string]IndexSnapshot{}
	for _, i := range newT.Indexes {
		newIdx[i.Name] = i
	}

	var changes []Change
	unmatchedOld, unmatchedNew := findUnmatched(oldIdx, newIdx)
	for _, name := range sortedKeys(oldIdx) {
		oldIndex := oldIdx[name]
		newIndex, existsInNew := newIdx[name]
		if !existsInNew || sameIndex(oldIndex, newIndex) {
			continue
		}
		unmatchedOld = append(unmatchedOld, name)
		unmatchedNew = append(unmatchedNew, name)
	}
	sort.Strings(unmatchedOld)
	sort.Strings(unmatchedNew)

	for _, oldName := range unmatchedOld {
		changes = append(changes, Change{
			Kind:    DropIndex,
			UpSQL:   fmt.Sprintf("DROP INDEX IF EXISTS %s;", oldName),
			DownSQL: indexDDL(table, oldIdx[oldName]),
		})
	}

	for _, newName := range unmatchedNew {
		changes = append(changes, Change{
			Kind:    AddIndex,
			UpSQL:   indexDDL(table, newIdx[newName]),
			DownSQL: fmt.Sprintf("DROP INDEX IF EXISTS %s;", newName),
		})
	}

	return changes, nil
}

func diffUniques(table string, oldT, newT TableSnapshot) ([]Change, error) {
	oldU := map[string]IndexSnapshot{}
	for _, u := range oldT.Uniques {
		oldU[u.Name] = u
	}
	newU := map[string]IndexSnapshot{}
	for _, u := range newT.Uniques {
		newU[u.Name] = u
	}

	var changes []Change
	unmatchedOld, unmatchedNew := findUnmatched(oldU, newU)
	for _, name := range sortedKeys(oldU) {
		oldItem := oldU[name]
		newItem, existsInNew := newU[name]
		if !existsInNew || sameColumns2(oldItem, newItem) {
			continue
		}
		unmatchedOld = append(unmatchedOld, name)
		unmatchedNew = append(unmatchedNew, name)
	}
	sort.Strings(unmatchedOld)
	sort.Strings(unmatchedNew)

	for _, oldName := range unmatchedOld {
		changes = append(changes, Change{
			Kind:    DropUniqueConstraint,
			UpSQL:   fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, oldName),
			DownSQL: fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", table, oldName, strings.Join(oldU[oldName].Cols, ", ")),
		})
	}
	for _, newName := range unmatchedNew {
		changes = append(changes, Change{
			Kind:    AddUniqueConstraint,
			UpSQL:   fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", table, newName, strings.Join(newU[newName].Cols, ", ")),
			DownSQL: fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, newName),
		})
	}
	return changes, nil
}

func diffConstraints(table string, oldT, newT TableSnapshot) ([]Change, error) {
	oldC := map[string]ConstraintSnapshot{}
	for _, c := range oldT.Constraints {
		oldC[c.Name] = c
	}
	newC := map[string]ConstraintSnapshot{}
	for _, c := range newT.Constraints {
		newC[c.Name] = c
	}

	var changes []Change
	unmatchedOld, unmatchedNew := findUnmatched(oldC, newC)
	for _, name := range sortedKeys(oldC) {
		oldItem := oldC[name]
		newItem, existsInNew := newC[name]
		if !existsInNew || oldItem.Expr == newItem.Expr {
			continue
		}
		unmatchedOld = append(unmatchedOld, name)
		unmatchedNew = append(unmatchedNew, name)
	}
	sort.Strings(unmatchedOld)
	sort.Strings(unmatchedNew)

	for _, oldName := range unmatchedOld {
		changes = append(changes, Change{
			Kind:    DropConstraint,
			UpSQL:   fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, oldName),
			DownSQL: fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s);", table, oldName, oldC[oldName].Expr),
		})
	}
	for _, newName := range unmatchedNew {
		changes = append(changes, Change{
			Kind:    AddConstraint,
			UpSQL:   fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s);", table, newName, newC[newName].Expr),
			DownSQL: fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", table, newName),
		})
	}
	return changes, nil
}

func sameColumns(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sameIndex(left, right IndexSnapshot) bool {
	return left.Unique == right.Unique && left.Predicate == right.Predicate && sameColumns(left.Cols, right.Cols)
}

func sameColumns2(u IndexSnapshot, n IndexSnapshot) bool {
	return sameColumns(u.Cols, n.Cols)
}

func indexDDL(table string, idx IndexSnapshot) string {
	kind := "INDEX"
	if idx.Unique {
		kind = "UNIQUE INDEX"
	}
	ddl := fmt.Sprintf("CREATE %s %s ON %s (%s);", kind, idx.Name, table, strings.Join(idx.Cols, ", "))
	if idx.Predicate != "" {
		ddl = fmt.Sprintf("CREATE %s %s ON %s (%s) WHERE %s;", kind, idx.Name, table, strings.Join(idx.Cols, ", "), idx.Predicate)
	}
	return ddl
}

func findUnmatched[T any](old, new map[string]T) ([]string, []string) {
	matchedOld := map[string]bool{}
	matchedNew := map[string]bool{}
	for name := range new {
		_, ok := old[name]
		if !ok {
			continue
		}

		matchedOld[name] = true
		matchedNew[name] = true
	}

	var unmatchedOld, unmatchedNew []string
	for _, name := range sortedKeys(old) {
		if !matchedOld[name] {
			unmatchedOld = append(unmatchedOld, name)
		}
	}
	for _, name := range sortedKeys(new) {
		if !matchedNew[name] {
			unmatchedNew = append(unmatchedNew, name)
		}
	}
	return unmatchedOld, unmatchedNew
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func topoSortByFK(all map[string]TableSnapshot, names []string) ([]TableSnapshot, error) {
	visited := map[string]bool{}
	visiting := map[string]bool{}
	var order []TableSnapshot
	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("foreign-key cycle involving new table %q; create the tables first and add the constraint in a separate migration", name)
		}
		t, ok := all[name]
		if !ok {
			return nil
		}
		visiting[name] = true
		for _, c := range t.Columns {
			// Self-referencing FKs are legal and must not be treated as a cycle.
			if c.RefTable != "" && c.RefTable != name && contains(names, c.RefTable) {
				if err := visit(c.RefTable); err != nil {
					return err
				}
			}
		}
		visiting[name] = false
		visited[name] = true
		order = append(order, t)
		return nil
	}
	for _, name := range sortedStringSlice(names) {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func sortedStringSlice(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
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
	for _, u := range t.Uniques {
		colDefs = append(colDefs, fmt.Sprintf("CONSTRAINT %s UNIQUE (%s)", u.Name, strings.Join(u.Cols, ", ")))
	}
	for _, c := range t.Constraints {
		colDefs = append(colDefs, fmt.Sprintf("CONSTRAINT %s CHECK (%s)", c.Name, c.Expr))
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

// columnDDL renders `COL_NAME COL_TYPE [COLLATE name] [NOT NULL] [UNIQUE]
// [DEFAULT value] [GENERATED ALWAYS AS IDENTITY] [CHECK (expr)]
// [GENERATED ALWAYS AS (expr) STORED]`.
func columnDDL(c ColumnSnapshot) string {
	parts := []string{c.Name, c.Type}
	if c.Collation != "" {
		parts = append(parts, "COLLATE "+c.Collation)
	}
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
	if c.Identity {
		parts = append(parts, "GENERATED ALWAYS AS IDENTITY")
	}
	if c.Check != "" {
		parts = append(parts, "CHECK ("+c.Check+")")
	}
	if c.Generated != "" {
		parts = append(parts, "GENERATED ALWAYS AS ("+c.Generated+") STORED")
	}
	return strings.Join(parts, " ")
}

// columnDDLWithRef renders columnDDL plus an inline REFERENCES clause for new
// columns that carry a foreign key. ALTER TABLE ... ADD COLUMN accepts a
// REFERENCES clause and creates the constraint directly.
func columnDDLWithRef(c ColumnSnapshot) string {
	ddl := columnDDL(c)
	if c.RefTable != "" && c.RefColumn != "" {
		ddl += " REFERENCES " + c.RefTable + "(" + c.RefColumn + ")"
		if c.OnDelete != "" {
			ddl += " ON DELETE " + c.OnDelete
		}
		if c.OnUpdate != "" {
			ddl += " ON UPDATE " + c.OnUpdate
		}
	}
	return ddl
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
	if c.NotNull && c.Default == nil && c.DefaultExpr == "" && !c.Identity {
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
