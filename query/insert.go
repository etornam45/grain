package query

import (
	"context"
	"fmt"
	"grainorm/grain/db"
	"strings"
)

type InsertBuilder struct {
	table     string
	cols      []string
	vals      []any
	returning []string
}

func Insert(table namedTable) *InsertBuilder {
	return &InsertBuilder{table: table.TableName()}
}

func (i *InsertBuilder) Values(row map[string]any) *InsertBuilder {
	for k, v := range row {
		i.cols = append(i.cols, k)
		i.vals = append(i.vals, v)
	}
	return i
}

func (i *InsertBuilder) Returning(cols ...string) *InsertBuilder {
	i.returning = cols
	return i
}

func (i *InsertBuilder) SQL() (string, []any) {
	placeholders := make([]string, len(i.vals))
	for idx := range i.vals {
		placeholders[idx] = fmt.Sprintf("$%d", idx+1)
	}
	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		i.table, strings.Join(i.cols, ", "), strings.Join(placeholders, ", "),
	)
	if len(i.returning) > 0 {
		sql += " RETURNING " + strings.Join(i.returning, ", ")
	}
	return sql, i.vals
}

func (i *InsertBuilder) Run(ctx context.Context, exec db.Executor) error {
	sqlStr, args := i.SQL()
	_, err := exec.ExecContext(ctx, sqlStr, args...)
	return err
}


//	var id string
//	query.Insert(Users).Values(row).Returning("id").Scan(ctx, conn, &id)
func (i *InsertBuilder) Scan(ctx context.Context, exec db.Executor, dest ...any) error {
	sqlStr, args := i.SQL()
	return exec.QueryRowContext(ctx, sqlStr, args...).Scan(dest...)
}
