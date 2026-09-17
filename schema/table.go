package schema

import (
	"fmt"
	"reflect"
	"strings"
)

type tableCore struct {
	Name       string
	colsByName map[string]*ColumnDef
	columns    []*ColumnDef
}

// TODO: Add support for indexing

func (t *tableCore) Col(name string) *ColumnDef {
	c, ok := t.colsByName[name]
	if !ok {
		panic(fmt.Sprintf("grain: no column %q on table %q", name, t.Name))
	}
	return c
}

func (t *tableCore) Columns() []*ColumnDef { return t.columns }
func (t *tableCore) TableName() string     { return t.Name }

type TableDef[T any] struct {
	tableCore
	Cols T
}

func Table[T any](name string, columns ...*ColumnDef) *TableDef[T] {
	t := &TableDef[T]{
		tableCore: tableCore{Name: name, colsByName: map[string]*ColumnDef{}},
	}
	for _, c := range columns {
		c.Table = name
		t.colsByName[c.Name] = c
		t.columns = append(t.columns, c)
	}
	t.Cols = Bind[T](&t.tableCore)
	Registry = append(Registry, &t.tableCore)
	return t
}

func Bind[T any](t *tableCore) T {
	var out T
	v := reflect.ValueOf(&out).Elem()
	typ := v.Type()

	if typ.Kind() != reflect.Struct {
		panic(fmt.Sprintf("grain: Bind[%s]: T must be a struct of *ColumnDef fields", typ))
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		colName := toSnakeCase(field.Name)
		col, ok := t.colsByName[colName]
		if !ok {
			panic(fmt.Sprintf(
				"grain: Bind: table %q has no column %q for field %s.%s",
				t.Name, colName, typ.Name(), field.Name,
			))
		}
		v.Field(i).Set(reflect.ValueOf(col))
	}
	return out
}

func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

var Registry []*tableCore
