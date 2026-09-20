package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/etornam45/grain/db"
)

type DeleteBuilder struct {
	table     string
	where     Condition
	orderBy   []OrderBy
	limitN    *int
	returning []string
}

func Delete(table namedTable) *DeleteBuilder {
	return &DeleteBuilder{table: table.TableName()}
}

func (d *DeleteBuilder) Where(c Condition) *DeleteBuilder { d.where = c; return d }

func (d *DeleteBuilder) OrderBy(cols []string, dir OrderDir) *DeleteBuilder {
	d.orderBy = append(d.orderBy, OrderBy{dir: dir, cols: cols})
	return d
}

func (d *DeleteBuilder) OrderByNulls(cols []string, dir OrderDir, nulls NullsOrder) *DeleteBuilder {
	d.orderBy = append(d.orderBy, OrderBy{dir: dir, cols: cols, nulls: nulls})
	return d
}

// Limit caps how many rows the delete removes (PostgreSQL DELETE ... LIMIT n).
func (d *DeleteBuilder) Limit(n int) *DeleteBuilder { d.limitN = &n; return d }

func (d *DeleteBuilder) Returning(cols ...string) *DeleteBuilder {
	d.returning = cols
	return d
}

func (d *DeleteBuilder) SQL() (string, []any) {
	sql := fmt.Sprintf("DELETE FROM %s", d.table)
	var args []any
	if d.where != nil {
		whereSQL, whereArgs := d.where.SQL(1)
		sql += " WHERE " + whereSQL
		args = whereArgs
	}
	if len(d.orderBy) > 0 {
		sql += " ORDER BY "
		parts := make([]string, 0, len(d.orderBy))
		for _, o := range d.orderBy {
			clause := strings.Join(o.cols, ", ") + " " + string(o.dir)
			if o.nulls != "" {
				clause += " " + string(o.nulls)
			}
			parts = append(parts, clause)
		}
		sql += strings.Join(parts, ", ")
	}
	if d.limitN != nil {
		sql += fmt.Sprintf(" LIMIT %d", *d.limitN)
	}
	if len(d.returning) > 0 {
		sql += " RETURNING "
		for i, c := range d.returning {
			if i > 0 {
				sql += ", "
			}
			sql += c
		}
	}
	return sql, args
}

func (d *DeleteBuilder) Run(ctx context.Context, exec db.Executor) (int64, error) {
	sqlStr, args := d.SQL()
	res, err := exec.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DeleteBuilder) Scan(ctx context.Context, exec db.Executor, dest ...any) error {
	sqlStr, args := d.SQL()
	return exec.QueryRowContext(ctx, sqlStr, args...).Scan(dest...)
}
