package query

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/etornam45/grain/db"
	"github.com/etornam45/grain/scan"
)

type UpdateBuilder struct {
	table     string
	set       map[string]any
	where     Condition
	orderBy   []OrderBy
	limitN    *int
	returning []string
}

func Update(table namedTable) *UpdateBuilder {
	return &UpdateBuilder{table: table.TableName()}
}

// Set assigns columns. Later calls overwrite earlier values for the same
// column instead of appending a duplicate SET clause. Values may be:
//   - literals (bound as placeholders): Set(map[string]any{"name": "foo"})
//   - column references (rendered as-is): Set(map[string]any{"count": seq})
//   - subqueries (rendered as `col = (SELECT ...)`): Set(map[string]any{"total": sub})
func (u *UpdateBuilder) Set(vals map[string]any) *UpdateBuilder {
	if u.set == nil {
		u.set = map[string]any{}
	}
	for k, v := range vals {
		u.set[k] = v
	}
	return u
}

func (u *UpdateBuilder) Where(c Condition) *UpdateBuilder { u.where = c; return u }

func (u *UpdateBuilder) OrderBy(cols []string, dir OrderDir) *UpdateBuilder {
	u.orderBy = append(u.orderBy, OrderBy{dir: dir, cols: cols})
	return u
}

func (u *UpdateBuilder) OrderByNulls(cols []string, dir OrderDir, nulls NullsOrder) *UpdateBuilder {
	u.orderBy = append(u.orderBy, OrderBy{dir: dir, cols: cols, nulls: nulls})
	return u
}

// Limit caps how many rows the update touches (PostgreSQL UPDATE ... LIMIT n).
func (u *UpdateBuilder) Limit(n int) *UpdateBuilder { u.limitN = &n; return u }

func (u *UpdateBuilder) Returning(cols ...string) *UpdateBuilder {
	u.returning = cols
	return u
}

func (u *UpdateBuilder) SQL() (string, []any) {
	keys := make([]string, 0, len(u.set))
	for k := range u.set {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	sets := make([]string, len(keys))
	args := make([]any, 0, len(keys))
	for i, col := range keys {
		setVal := u.set[col]
		if ref, ok := asColRef(setVal); ok {
			sets[i] = fmt.Sprintf("%s = %s", col, ref)
			continue
		}
		switch v := setVal.(type) {
		case Subquery:
			sets[i] = fmt.Sprintf("%s = (%s)", col, v.sqlAt(len(args)+1))
			args = append(args, v.args...)
		default:
			sets[i] = fmt.Sprintf("%s = $%d", col, len(args)+1)
			args = append(args, v)
		}
	}

	sql := fmt.Sprintf("UPDATE %s SET %s", u.table, strings.Join(sets, ", "))

	if u.where != nil {
		whereSQL, whereArgs := u.where.SQL(len(args) + 1)
		sql += " WHERE " + whereSQL
		args = append(args, whereArgs...)
	}

	if len(u.orderBy) > 0 {
		sql += " ORDER BY "
		parts := make([]string, 0, len(u.orderBy))
		for _, o := range u.orderBy {
			clause := strings.Join(o.cols, ", ") + " " + string(o.dir)
			if o.nulls != "" {
				clause += " " + string(o.nulls)
			}
			parts = append(parts, clause)
		}
		sql += strings.Join(parts, ", ")
	}
	if u.limitN != nil {
		sql += fmt.Sprintf(" LIMIT %d", *u.limitN)
	}
	if len(u.returning) > 0 {
		sql += " RETURNING " + strings.Join(u.returning, ", ")
	}
	return sql, args
}

func (u *UpdateBuilder) Run(ctx context.Context, exec db.Executor) (int64, error) {
	sqlStr, args := u.SQL()
	res, err := exec.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (u *UpdateBuilder) Scan(ctx context.Context, exec db.Executor, dest ...any) error {
	sqlStr, args := u.SQL()
	return exec.QueryRowContext(ctx, sqlStr, args...).Scan(dest...)
}

func (u *UpdateBuilder) ScanInto[T any](ctx context.Context, exec db.Executor) (T, error) {
	sqlStr, args := u.SQL()
	return scan.One[T](exec.QueryRowContext(ctx, sqlStr, args...), u.returning)
}
