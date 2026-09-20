package query

import (
	"fmt"

	"github.com/etornam45/grain/schema"
)

// aliasedTable is an aliased view of a schema table. Columns resolved through
// it render as "alias.col", and From/join clauses render "schema AS alias".
//
// The query package otherwise stays decoupled from schema, but table aliasing
// needs column metadata, so Alias requires a *schema.TableDef.
type aliasedTable struct {
	table *schema.TableDef
	alias string
}

// Alias returns an aliased view of table. Use its Col() values anywhere a
// column is accepted and pass the result of Alias itself to From/joins:
//
//	u := query.Alias(schema.Users, "u")
//	query.Select[T](u.Col("id")).From(u).Where(query.Eq(u.Col("status"), "active"))
func Alias(table *schema.TableDef, alias string) *aliasedTable {
	if table == nil {
		panic("grain: Alias: table must not be nil")
	}
	if alias == "" {
		panic("grain: Alias: alias must not be empty")
	}
	return &aliasedTable{table: table, alias: alias}
}

func (a *aliasedTable) TableName() string { return a.table.TableName() }
func (a *aliasedTable) Alias() string     { return a.alias }

// Col returns a column reference qualified with this table's alias.
func (a *aliasedTable) Col(name string) *schema.ColumnDef {
	c := a.table.Col(name)
	cp := *c
	cp.Table = a.alias
	return &cp
}

func aliasDDL(t namedTable) string {
	if a, ok := t.(interface{ Alias() string }); ok {
		return fmt.Sprintf("%s AS %s", t.TableName(), a.Alias())
	}
	return t.TableName()
}