package query

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/etornam45/grain/db"
	"github.com/etornam45/grain/scan"
)

type OrderDir string

const (
	Asc  OrderDir = "ASC"
	Desc OrderDir = "DESC"
)

type NullsOrder string

const (
	NullsFirst NullsOrder = "NULLS FIRST"
	NullsLast  NullsOrder = "NULLS LAST"
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
	cols  []string
	dir   OrderDir
	nulls NullsOrder
}

type cte struct {
	name string
	sub  Subquery
}

type setOp struct {
	kind string // "UNION", "UNION ALL", "INTERSECT", "EXCEPT"
	sub  Subquery
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
	ctes     []cte
	setOps   []setOp
	lockMode string // "FOR UPDATE" | "FOR SHARE"
	lockOpt  string // "", "NOWAIT", "SKIP LOCKED"
}

func Select[T any](cols ...colRef) *SelectBuilder[T] {
	var names []string
	if len(cols) > 0 {
		for _, c := range cols {
			names = append(names, c.String())
		}
	} else {
		var zero T
		names = extractTags(reflect.TypeOf(zero))
	}
	return &SelectBuilder[T]{columns: names}
}

func extractTags(typ reflect.Type) []string {
	if typ == nil {
		return nil
	}
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	var tags []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			tags = append(tags, extractTags(f.Type)...)
			continue
		}
		tag := f.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.IndexByte(tag, ','); idx != -1 {
			tag = tag[:idx]
		}
		tags = append(tags, tag)
	}
	return tags
}

func formatSelectCol(col string) string {
	if col == "*" || strings.Contains(strings.ToUpper(col), " AS ") || strings.Contains(col, "(") {
		return col
	}
	return fmt.Sprintf(`%s AS "%s"`, col, col)
}

func (q *SelectBuilder[T]) Distinct() *SelectBuilder[T] { q.distinct = true; return q }

func (q *SelectBuilder[T]) From(table namedTable) *SelectBuilder[T] {
	q.table = aliasDDL(table)
	return q
}

func (q *SelectBuilder[T]) join(kind JoinKind, table namedTable, on Condition) *SelectBuilder[T] {
	q.joins = append(q.joins, joinClause{kind: kind, table: aliasDDL(table), on: on})
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
	q.joins = append(q.joins, joinClause{kind: CROSS, table: aliasDDL(table)})
	return q
}

func (q *SelectBuilder[T]) Where(c Condition) *SelectBuilder[T]      { q.where = c; return q }
func (q *SelectBuilder[T]) GroupBy(cols ...string) *SelectBuilder[T] { q.groupBy = cols; return q }
func (q *SelectBuilder[T]) Having(c Condition) *SelectBuilder[T]     { q.having = c; return q }

func (q *SelectBuilder[T]) OrderBy(cols []string, dir OrderDir) *SelectBuilder[T] {
	q.orderBy = append(q.orderBy, OrderBy{dir: dir, cols: cols})
	return q
}

// OrderByNulls is OrderBy with an explicit NULLS FIRST / NULLS LAST placement.
func (q *SelectBuilder[T]) OrderByNulls(cols []string, dir OrderDir, nulls NullsOrder) *SelectBuilder[T] {
	q.orderBy = append(q.orderBy, OrderBy{dir: dir, cols: cols, nulls: nulls})
	return q
}

func (q *SelectBuilder[T]) Limit(n int) *SelectBuilder[T]  { q.limitN = &n; return q }
func (q *SelectBuilder[T]) Offset(n int) *SelectBuilder[T] { q.offsetN = &n; return q }

// With prepends a common table expression: `WITH name AS (SELECT ...)`.
func (q *SelectBuilder[T]) With(name string, sub Subquery) *SelectBuilder[T] {
	q.ctes = append(q.ctes, cte{name: name, sub: sub})
	return q
}

// setOp appends a set operation against another query's result.
func (q *SelectBuilder[T]) setOp(kind string, sub Subquery) *SelectBuilder[T] {
	q.setOps = append(q.setOps, setOp{kind: kind, sub: sub})
	return q
}

func (q *SelectBuilder[T]) Union(sub Subquery) *SelectBuilder[T]        { return q.setOp("UNION", sub) }
func (q *SelectBuilder[T]) UnionAll(sub Subquery) *SelectBuilder[T]     { return q.setOp("UNION ALL", sub) }
func (q *SelectBuilder[T]) Intersect(sub Subquery) *SelectBuilder[T]    { return q.setOp("INTERSECT", sub) }
func (q *SelectBuilder[T]) Except(sub Subquery) *SelectBuilder[T]       { return q.setOp("EXCEPT", sub) }

// ForUpdate adds `FOR UPDATE` (row-level lock).
func (q *SelectBuilder[T]) ForUpdate() *SelectBuilder[T] { q.lockMode = "FOR UPDATE"; return q }

// ForShare adds `FOR SHARE` (row-level shared lock).
func (q *SelectBuilder[T]) ForShare() *SelectBuilder[T] { q.lockMode = "FOR SHARE"; return q }

// NoWait makes a locking clause fail immediately if rows are locked.
func (q *SelectBuilder[T]) NoWait() *SelectBuilder[T] { q.lockOpt = "NOWAIT"; return q }

// SkipLocked skips rows locked by other transactions (SELECT ... FOR UPDATE SKIP LOCKED).
func (q *SelectBuilder[T]) SkipLocked() *SelectBuilder[T] { q.lockOpt = "SKIP LOCKED"; return q }

func (q *SelectBuilder[T]) SQL() (string, []any) {
	return q.render(1)
}

// render builds the SQL with placeholders starting at base. CTE bodies consume
// the first args, then the main query continues from the next placeholder.
func (q *SelectBuilder[T]) render(base int) (string, []any) {
	cols := "*"
	if len(q.columns) > 0 {
		formatted := make([]string, len(q.columns))
		for i, c := range q.columns {
			formatted[i] = formatSelectCol(c)
		}
		cols = strings.Join(formatted, ", ")
	}

	var b strings.Builder
	args := []any{}
	if len(q.ctes) > 0 {
		b.WriteString("WITH ")
		cteParts := make([]string, len(q.ctes))
		for i, c := range q.ctes {
			cteParts[i] = fmt.Sprintf("%s AS (%s)", c.name, c.sub.sqlAt(base+len(args)))
			args = append(args, c.sub.args...)
		}
		b.WriteString(strings.Join(cteParts, ", "))
		b.WriteString(" ")
	}

	b.WriteString("SELECT ")
	if q.distinct {
		b.WriteString("DISTINCT ")
	}
	b.WriteString(cols)
	fmt.Fprintf(&b, " FROM %s", q.table)

	for _, j := range q.joins {
		if j.kind == "CROSS" {
			fmt.Fprintf(&b, " CROSS JOIN %s", j.table)
			continue
		}
		onSQL, onArgs := j.on.SQL(len(args) + base)
		fmt.Fprintf(&b, " %s JOIN %s ON %s", j.kind, j.table, onSQL)
		args = append(args, onArgs...)
	}

	if q.where != nil {
		whereSQL, whereArgs := q.where.SQL(len(args) + base)
		b.WriteString(" WHERE ")
		b.WriteString(whereSQL)
		args = append(args, whereArgs...)
	}

	if len(q.groupBy) > 0 {
		b.WriteString(" GROUP BY ")
		b.WriteString(strings.Join(q.groupBy, ", "))
	}

	if q.having != nil {
		havingSQL, havingArgs := q.having.SQL(len(args) + base)
		b.WriteString(" HAVING ")
		b.WriteString(havingSQL)
		args = append(args, havingArgs...)
	}

	for _, op := range q.setOps {
		opSQL := op.sub.sqlAt(len(args) + base)
		fmt.Fprintf(&b, " %s (%s)", op.kind, opSQL)
		args = append(args, op.sub.args...)
	}

	if len(q.orderBy) > 0 {
		b.WriteString(" ORDER BY ")
		parts := make([]string, 0, len(q.orderBy))
		for _, o := range q.orderBy {
			clause := strings.Join(o.cols, ", ") + " " + string(o.dir)
			if o.nulls != "" {
				clause += " " + string(o.nulls)
			}
			parts = append(parts, clause)
		}
		b.WriteString(strings.Join(parts, ", "))
	}
	if q.limitN != nil {
		fmt.Fprintf(&b, " LIMIT %d", *q.limitN)
	}
	if q.offsetN != nil {
		fmt.Fprintf(&b, " OFFSET %d", *q.offsetN)
	}

	lockSQL := q.lockMode
	if lockSQL != "" && q.lockOpt != "" {
		lockSQL += " " + q.lockOpt
	}
	if lockSQL != "" {
		b.WriteString(" " + lockSQL)
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
	countSQL := "SELECT COUNT(*) FROM (" + sqlStr + ") AS _count_subquery"
	var n int64
	err := exec.QueryRowContext(ctx, countSQL, args...).Scan(&n)
	return n, err
}

// CountDistinct counts the distinct values of col across the query result:
// `SELECT COUNT(DISTINCT col) FROM (<query>)`.
func (q *SelectBuilder[T]) CountDistinct(ctx context.Context, exec db.Executor, col colRef) (int64, error) {
	sqlStr, args := q.SQL()
	countSQL := "SELECT COUNT(DISTINCT " + col.String() + ") FROM (" + sqlStr + ") AS _count_subquery"
	var n int64
	err := exec.QueryRowContext(ctx, countSQL, args...).Scan(&n)
	return n, err
}