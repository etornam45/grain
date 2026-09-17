package query

import (
	"fmt"
	"strings"
)

// Condition renders itself to a SQL fragment plus its own args, starting its
// placeholder numbering at argOffset so it composes correctly no matter where
// it lands in a larger query (after N earlier args).
type Condition interface {
	SQL(argOffset int) (string, []any)
}

// colRef is satisfied by *schema.ColumnDef (via its String() method), so the
// query package never needs to import schema directly.
type colRef interface {
	String() string
}

type simpleCond struct {
	left string
	op   string
	val  any
}

func (c simpleCond) SQL(argOffset int) (string, []any) {
	return fmt.Sprintf("%s %s $%d", c.left, c.op, argOffset), []any{c.val}
}

func Eq(col colRef, val any) Condition   { return simpleCond{col.String(), "=", val} }
func Neq(col colRef, val any) Condition  { return simpleCond{col.String(), "<>", val} }
func Gt(col colRef, val any) Condition   { return simpleCond{col.String(), ">", val} }
func Gte(col colRef, val any) Condition  { return simpleCond{col.String(), ">=", val} }
func Lt(col colRef, val any) Condition   { return simpleCond{col.String(), "<", val} }
func Lte(col colRef, val any) Condition  { return simpleCond{col.String(), "<=", val} }
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
