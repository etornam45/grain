package query

import (
	"fmt"
	"reflect"
	"strings"
)

type Condition interface {
	SQL(argOffset int) (string, []any)
}

type colRef interface {
	String() string
}

type simpleCond struct {
	left string
	op   string
	val  any
}

func (c simpleCond) SQL(argOffset int) (string, []any) {
	if r, ok := c.val.(colRef); ok {
		return fmt.Sprintf("%s %s %s", c.left, c.op, r.String()), nil
	}
	return fmt.Sprintf("%s %s $%d", c.left, c.op, argOffset), []any{c.val}
}

func Eq(col colRef, val any) Condition           { return simpleCond{col.String(), "=", val} }
func Neq(col colRef, val any) Condition          { return simpleCond{col.String(), "<>", val} }
func Gt(col colRef, val any) Condition           { return simpleCond{col.String(), ">", val} }
func Gte(col colRef, val any) Condition          { return simpleCond{col.String(), ">=", val} }
func Lt(col colRef, val any) Condition           { return simpleCond{col.String(), "<", val} }
func Lte(col colRef, val any) Condition          { return simpleCond{col.String(), "<=", val} }
func Like(col colRef, pattern string) Condition  { return simpleCond{col.String(), "LIKE", pattern} }
func ILike(col colRef, pattern string) Condition { return simpleCond{col.String(), "ILIKE", pattern} }

type nullCond struct {
	col string
	not bool
}

func (c nullCond) SQL(argOffset int) (string, []any) {
	if c.not {
		return c.col + " IS NOT NULL", nil
	}
	return c.col + " IS NULL", nil
}

func IsNull(col colRef) Condition    { return nullCond{col.String(), false} }
func IsNotNull(col colRef) Condition { return nullCond{col.String(), true} }

type boolCond struct {
	joiner string
	conds  []Condition
}

func (c boolCond) SQL(argOffset int) (string, []any) {
	var parts []string
	var args []any
	for _, cond := range c.conds {
		s, a := cond.SQL(argOffset + len(args))
		parts = append(parts, s)
		args = append(args, a...)
	}
	return "(" + strings.Join(parts, " "+c.joiner+" ") + ")", args
}

func And(conds ...Condition) Condition { return boolCond{"AND", conds} }
func Or(conds ...Condition) Condition  { return boolCond{"OR", conds} }

type notCond struct{ inner Condition }

func (c notCond) SQL(argOffset int) (string, []any) {
	s, args := c.inner.SQL(argOffset)
	return "NOT (" + s + ")", args
}

func Not(cond Condition) Condition { return notCond{cond} }

type inCond struct {
	col  string
	vals []any
	not  bool
}

func (c inCond) SQL(argOffset int) (string, []any) {
	if len(c.vals) == 0 {
		if c.not {
			return "1 = 1", nil
		}
		return "1 = 0", nil
	}
	placeholders := make([]string, len(c.vals))
	for i := range c.vals {
		placeholders[i] = fmt.Sprintf("$%d", argOffset+i)
	}
	op := "IN"
	if c.not {
		op = "NOT IN"
	}
	return fmt.Sprintf("%s %s (%s)", c.col, op, strings.Join(placeholders, ", ")), c.vals
}

func flattenValues(vals []any) []any {
	if len(vals) == 1 && vals[0] != nil {
		rv := reflect.ValueOf(vals[0])
		if rv.Kind() == reflect.Slice {
			res := make([]any, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				res[i] = rv.Index(i).Interface()
			}
			return res
		}
	}
	return vals
}

func In(col colRef, vals ...any) Condition {
	if len(vals) == 1 {
		if sub, ok := vals[0].(Subquery); ok {
			return subqueryCond{col: col.String(), op: "IN", sub: sub}
		}
	}
	return inCond{col: col.String(), vals: flattenValues(vals), not: false}
}

func NotIn(col colRef, vals ...any) Condition {
	if len(vals) == 1 {
		if sub, ok := vals[0].(Subquery); ok {
			return subqueryCond{col: col.String(), op: "NOT IN", sub: sub}
		}
	}
	return inCond{col: col.String(), vals: flattenValues(vals), not: true}
}

type rawCond struct {
	expr string
	args []any
}

func (c rawCond) SQL(argOffset int) (string, []any) {
	if len(c.args) == 0 {
		return c.expr, nil
	}
	var b strings.Builder
	argIdx := argOffset
	for _, r := range c.expr {
		if r == '?' {
			fmt.Fprintf(&b, "$%d", argIdx)
			argIdx++
		} else {
			b.WriteRune(r)
		}
	}
	return b.String(), c.args
}

func Raw(expr string, args ...any) Condition {
	return rawCond{expr: expr, args: args}
}

type betweenCond struct {
	col   string
	low   any
	high  any
	not   bool
}

func (c betweenCond) SQL(argOffset int) (string, []any) {
	op := "BETWEEN"
	if c.not {
		op = "NOT BETWEEN"
	}
	return fmt.Sprintf("%s %s $%d AND $%d", c.col, op, argOffset, argOffset+1), []any{c.low, c.high}
}

// Between renders `col BETWEEN $n AND $n+1`.
func Between(col colRef, low, high any) Condition {
	return betweenCond{col: col.String(), low: low, high: high}
}

// NotBetween renders `col NOT BETWEEN $n AND $n+1`.
func NotBetween(col colRef, low, high any) Condition {
	return betweenCond{col: col.String(), low: low, high: high, not: true}
}

// jsonCond renders PostgreSQL JSON/JSONB operators against a bound value.
type jsonCond struct {
	col  string
	op   string
	val  any
}

func (c jsonCond) SQL(argOffset int) (string, []any) {
	return fmt.Sprintf("%s %s $%d", c.col, c.op, argOffset), []any{c.val}
}

// Contains renders `col @> $n` (the JSON value contains the operand).
func Contains(col colRef, val any) Condition { return jsonCond{col: col.String(), op: "@>", val: val} }

// ContainedBy renders `col <@ $n` (the JSON value is contained by the operand).
func ContainedBy(col colRef, val any) Condition { return jsonCond{col: col.String(), op: "<@", val: val} }

// KeyExists renders `col ? $n` (the JSON value has the key).
func KeyExists(col colRef, key any) Condition { return jsonCond{col: col.String(), op: "?", val: key} }

// subqueryCond renders a nested SELECT against a column or as EXISTS.
type subqueryCond struct {
	col string
	op  string // "IN", "= ANY", "<> ALL", or "" for EXISTS
	sub Subquery
}

func (c subqueryCond) SQL(argOffset int) (string, []any) {
	sql := c.sub.sqlAt(argOffset)
	if c.col == "" {
		return "EXISTS (" + sql + ")", c.sub.args
	}
	return fmt.Sprintf("%s %s (%s)", c.col, c.op, sql), c.sub.args
}

// Exists renders `EXISTS (SELECT ...)`.
func Exists(sub Subquery) Condition { return subqueryCond{sub: sub} }

// EqAny renders `col = ANY (SELECT ...)`.
func EqAny(col colRef, sub Subquery) Condition {
	return subqueryCond{col: col.String(), op: "= ANY", sub: sub}
}

// NeqAll renders `col <> ALL (SELECT ...)`.
func NeqAll(col colRef, sub Subquery) Condition {
	return subqueryCond{col: col.String(), op: "<> ALL", sub: sub}
}
