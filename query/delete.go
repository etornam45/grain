package query

import (
	"context"
	"fmt"
	"grainorm/grain/db"
)

type DeleteBuilder struct {
	table     string
	where     Condition
	returning []string
}

func Delete(table namedTable) *DeleteBuilder {
	return &DeleteBuilder{table: table.TableName()}
}

func (d *DeleteBuilder) Where(c Condition) *DeleteBuilder { d.where = c; return d }

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
