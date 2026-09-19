package query

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/etornam45/grain/db"
)

type conflictClause struct {
	targets      []string
	doNothing    bool
	setCols      []string
	setVals      []any
	excludedCols []string
}

type InsertBuilder struct {
	table     string
	cols      []string
	vals      []any
	rowCount  int
	conflict  *conflictClause
	returning []string
}

func Insert(table namedTable) *InsertBuilder {
	return &InsertBuilder{table: table.TableName()}
}

func (i *InsertBuilder) Values(rows ...map[string]any) *InsertBuilder {
	if len(rows) == 0 {
		return i
	}
	keySet := make(map[string]struct{})
	for _, r := range rows {
		for k := range r {
			keySet[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(keySet))
	for k := range keySet {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	i.cols = cols
	i.vals = make([]any, 0, len(rows)*len(cols))
	i.rowCount = len(rows)

	for _, r := range rows {
		for _, c := range cols {
			i.vals = append(i.vals, r[c])
		}
	}
	return i
}

func (i *InsertBuilder) OnConflictDoNothing(targets ...string) *InsertBuilder {
	i.conflict = &conflictClause{
		targets:   targets,
		doNothing: true,
	}
	return i
}

func (i *InsertBuilder) OnConflictDoUpdate(targets []string, updateVals map[string]any) *InsertBuilder {
	keys := make([]string, 0, len(updateVals))
	for k := range updateVals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	setCols := make([]string, len(keys))
	setVals := make([]any, len(keys))
	for idx, k := range keys {
		setCols[idx] = k
		setVals[idx] = updateVals[k]
	}
	i.conflict = &conflictClause{
		targets: targets,
		setCols: setCols,
		setVals: setVals,
	}
	return i
}

func (i *InsertBuilder) OnConflictExcluded(targets []string, cols ...string) *InsertBuilder {
	i.conflict = &conflictClause{
		targets:      targets,
		excludedCols: cols,
	}
	return i
}

func (i *InsertBuilder) Returning(cols ...string) *InsertBuilder {
	i.returning = cols
	return i
}

func (i *InsertBuilder) SQL() (string, []any) {
	if len(i.cols) == 0 {
		return "", nil
	}
	numCols := len(i.cols)
	rowPlaceholders := make([]string, i.rowCount)
	paramIdx := 1
	for r := 0; r < i.rowCount; r++ {
		colPhs := make([]string, numCols)
		for c := 0; c < numCols; c++ {
			colPhs[c] = fmt.Sprintf("$%d", paramIdx)
			paramIdx++
		}
		rowPlaceholders[r] = "(" + strings.Join(colPhs, ", ") + ")"
	}

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		i.table, strings.Join(i.cols, ", "), strings.Join(rowPlaceholders, ", "),
	)

	args := append([]any(nil), i.vals...)

	if i.conflict != nil {
		sql += " ON CONFLICT"
		if len(i.conflict.targets) > 0 {
			sql += " (" + strings.Join(i.conflict.targets, ", ") + ")"
		}
		if i.conflict.doNothing {
			sql += " DO NOTHING"
		} else if len(i.conflict.setCols) > 0 || len(i.conflict.excludedCols) > 0 {
			sql += " DO UPDATE SET "
			var updates []string
			for idx, col := range i.conflict.setCols {
				updates = append(updates, fmt.Sprintf("%s = $%d", col, len(args)+1))
				args = append(args, i.conflict.setVals[idx])
			}
			for _, col := range i.conflict.excludedCols {
				updates = append(updates, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
			}
			sql += strings.Join(updates, ", ")
		}
	}

	if len(i.returning) > 0 {
		sql += " RETURNING " + strings.Join(i.returning, ", ")
	}
	return sql, args
}

func (i *InsertBuilder) Run(ctx context.Context, exec db.Executor) error {
	sqlStr, args := i.SQL()
	_, err := exec.ExecContext(ctx, sqlStr, args...)
	return err
}

func (i *InsertBuilder) Scan(ctx context.Context, exec db.Executor, dest ...any) error {
	sqlStr, args := i.SQL()
	return exec.QueryRowContext(ctx, sqlStr, args...).Scan(dest...)
}
