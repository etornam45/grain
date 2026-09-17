package schema

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

type tableCore struct {
	Name       string
	colsByName map[string]*ColumnDef
	columns    []*ColumnDef
	indices    []IndexDef
}

// TODO: Add support for indexing

func (t *tableCore) Col(name string) *ColumnDef {
	c, ok := t.colsByName[name]
	if !ok {
		panic(fmt.Sprintf("grain: no column %q on table %q", name, t.Name))
	}
	return c
}

func (t *tableCore) Columns() []*ColumnDef  { return t.columns }
func (t *tableCore) GetIndices() []IndexDef { return t.indices }
func (t *tableCore) TableName() string      { return t.Name }

type IndexDef struct {
	Name string
	Cols []string
}

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
	runes := []rune(s)
	n := len(runes)
	var b strings.Builder

	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				nextIsLower := i+1 < n && unicode.IsLower(runes[i+1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) || nextIsLower {
					b.WriteByte('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (t *TableDef[T]) Index(name string, cols ...string) *TableDef[T] {
	t.indices = append(t.indices, IndexDef{Name: name, Cols: cols})
	return t
}

// func (t *TableDef[T]) Unique(name string, cols ...string) *TableDef[T]
// func (t *TableDef[T]) Constraint(name, expr string) *TableDef[T]

var Registry []*tableCore
