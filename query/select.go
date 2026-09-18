package query

import (
	"context"
	"fmt"
	"github.com/etornam45/grain/db"
	"github.com/etornam45/grain/scan"
	"strings"
)

type OrderDir string

const (
	Asc  OrderDir = "ASC"
	Desc OrderDir = "DESC"
)

type namedTable interface {
	TableName() string
}

type JoinKind string

const (
	INNER JoinKind = "INNER"
	LEFT  JoinKind = "LEFT"
	RIGHT JoinKind = "RIGHT"
	FULL  JoinKind = "FULL"
	CROSS JoinKind = "CROSS"
)

type joinClause struct {
	kind  JoinKind
	table string
	on    Condition
}

type OrderBy struct {
	cols []string
	dir  OrderDir
}

type SelectBuilder[T any] struct {
	columns  []string
	distinct bool
	table    string
	joins    []joinClause
	where    Condition
	groupBy  []string
	having   Condition
	orderBy  []OrderBy
	limitN   *int
	offsetN  *int
}

func Select[T any](cols ...colRef) *SelectBuilder[T] {
	var names []string
	for _, c := range cols {
		names = append(names, c.String())
	}
	return &SelectBuilder[T]{columns: names}
}

func (q *SelectBuilder[T]) Distinct() *SelectBuilder[T] { q.distinct = true; return q }

func (q *SelectBuilder[T]) From(table namedTable) *SelectBuilder[T] {
	q.table = table.TableName()
	return q
}

func (q *SelectBuilder[T]) join(kind JoinKind, table namedTable, on Condition) *SelectBuilder[T] {
	q.joins = append(q.joins, joinClause{kind: kind, table: table.TableName(), on: on})
	return q
}

func (q *SelectBuilder[T]) InnerJoin(table namedTable, on Condition) *SelectBuilder[T] {
	return q.join(INNER, table, on)
}
func (q *SelectBuilder[T]) LeftJoin(table namedTable, on Condition) *SelectBuilder[T] {
	return q.join(LEFT, table, on)
}
func (q *SelectBuilder[T]) RightJoin(table namedTable, on Condition) *SelectBuilder[T] {
	return q.join(RIGHT, table, on)
}
func (q *SelectBuilder[T]) FullJoin(table namedTable, on Condition) *SelectBuilder[T] {
	return q.join(FULL, table, on)
}
func (q *SelectBuilder[T]) CrossJoin(table namedTable) *SelectBuilder[T] {
	q.joins = append(q.joins, joinClause{kind: CROSS, table: table.TableName()})
	return q
}

func (q *SelectBuilder[T]) Where(c Condition) *SelectBuilder[T]      { q.where = c; return q }
func (q *SelectBuilder[T]) GroupBy(cols ...string) *SelectBuilder[T] { q.groupBy = cols; return q }
func (q *SelectBuilder[T]) Having(c Condition) *SelectBuilder[T]     { q.having = c; return q }

func (q *SelectBuilder[T]) OrderBy(cols []string, dir OrderDir) *SelectBuilder[T] {
	q.orderBy = append(q.orderBy, OrderBy{dir: dir, cols: cols})
	return q
}

func (q *SelectBuilder[T]) Limit(n int) *SelectBuilder[T]  { q.limitN = &n; return q }
func (q *SelectBuilder[T]) Offset(n int) *SelectBuilder[T] { q.offsetN = &n; return q }

func (q *SelectBuilder[T]) SQL() (string, []any) {
	cols := "*"
	if len(q.columns) > 0 {
		cols = strings.Join(q.columns, ", ")
	}

	var b strings.Builder
	b.WriteString("SELECT ")
	if q.distinct {
		b.WriteString("DISTINCT ")
	}
	b.WriteString(cols)
	fmt.Fprintf(&b, " FROM %s", q.table)

	var args []any
	for _, j := range q.joins {
		if j.kind == "CROSS" {
			fmt.Fprintf(&b, " CROSS JOIN %s", j.table)
			continue
		}
		onSQL, onArgs := j.on.SQL(len(args) + 1)
		fmt.Fprintf(&b, " %s JOIN %s ON %s", j.kind, j.table, onSQL)
		args = append(args, onArgs...)
	}

	if q.where != nil {
		whereSQL, whereArgs := q.where.SQL(len(args) + 1)
		b.WriteString(" WHERE ")
		b.WriteString(whereSQL)
		args = append(args, whereArgs...)
	}

	if len(q.groupBy) > 0 {
		b.WriteString(" GROUP BY ")
		b.WriteString(strings.Join(q.groupBy, ", "))
	}

	if q.having != nil {
		havingSQL, havingArgs := q.having.SQL(len(args) + 1)
		b.WriteString(" HAVING ")
		b.WriteString(havingSQL)
		args = append(args, havingArgs...)
	}

	if len(q.orderBy) > 0 {
		b.WriteString(" ORDER BY ")
		parts := make([]string, 0, len(q.orderBy))
		for _, o := range q.orderBy {
			parts = append(parts, strings.Join(o.cols, ", ")+" "+string(o.dir))
		}
		b.WriteString(strings.Join(parts, ", "))
	}
	if q.limitN != nil {
		fmt.Fprintf(&b, " LIMIT %d", *q.limitN)
	}
	if q.offsetN != nil {
		fmt.Fprintf(&b, " OFFSET %d", *q.offsetN)
	}

	return b.String(), args
}

func (q *SelectBuilder[T]) All(ctx context.Context, exec db.Executor) ([]T, error) {
	sqlStr, args := q.SQL()
	rows, err := exec.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scan.All[T](rows)
}

func (q *SelectBuilder[T]) First(ctx context.Context, exec db.Executor) (*T, error) {
	limited := *q
	one := 1
	limited.limitN = &one
	results, err := limited.All(ctx, exec)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

func (q *SelectBuilder[T]) Count(ctx context.Context, exec db.Executor) (int64, error) {
	sqlStr, args := q.SQL()
	// FIXME: verify it this works in complex queries
	countSQL := "SELECT COUNT(*) FROM (" + sqlStr + ") AS _count_subquery"
	var n int64
	err := exec.QueryRowContext(ctx, countSQL, args...).Scan(&n)
	return n, err
}
