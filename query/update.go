package query

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"grain/db"
)

type UpdateBuilder struct {
	table     string
	setCols   []string
	setVals   []any
	where     Condition
	returning []string
}

func Update(table namedTable) *UpdateBuilder {
	return &UpdateBuilder{table: table.TableName()}
}

func (u *UpdateBuilder) Set(vals map[string]any) *UpdateBuilder {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		u.setCols = append(u.setCols, k)
		u.setVals = append(u.setVals, vals[k])
	}
	return u
}

func (u *UpdateBuilder) Where(c Condition) *UpdateBuilder { u.where = c; return u }

func (u *UpdateBuilder) Returning(cols ...string) *UpdateBuilder {
	u.returning = cols
	return u
}

func (u *UpdateBuilder) SQL() (string, []any) {
	sets := make([]string, len(u.setCols))
	args := make([]any, 0, len(u.setVals))
	for i, col := range u.setCols {
		sets[i] = fmt.Sprintf("%s = $%d", col, i+1)
		args = append(args, u.setVals[i])
	}

	sql := fmt.Sprintf("UPDATE %s SET %s", u.table, strings.Join(sets, ", "))

	if u.where != nil {
		whereSQL, whereArgs := u.where.SQL(len(args) + 1)
		sql += " WHERE " + whereSQL
		args = append(args, whereArgs...)
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
