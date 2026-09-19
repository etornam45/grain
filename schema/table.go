package schema

import (
	"fmt"
)

type IndexDef struct {
	Name string
	Cols []string
}

type TableDef struct {
	Name       string
	colsByName map[string]*ColumnDef
	columns    []*ColumnDef
	indices    []IndexDef
}

func (t *TableDef) Col(name string) *ColumnDef {
	c, ok := t.colsByName[name]
	if !ok {
		panic(fmt.Sprintf("grain: no column %q on table %q", name, t.Name))
	}
	return c
}

func (t *TableDef) Columns() []*ColumnDef  { return t.columns }
func (t *TableDef) GetIndices() []IndexDef { return t.indices }
func (t *TableDef) TableName() string      { return t.Name }

func Table(name string, columns ...*ColumnDef) *TableDef {
	t := &TableDef{
		Name:       name,
		colsByName: map[string]*ColumnDef{},
	}
	for _, c := range columns {
		c.Table = name
		t.colsByName[c.Name] = c
		t.columns = append(t.columns, c)
		if c.HasIndex {
			t.indices = append(t.indices, IndexDef{
				Name: fmt.Sprintf("idx_%s_%s", name, c.Name),
				Cols: []string{c.Name},
			})
		}
	}
	Registry = append(Registry, t)
	return t
}

func (t *TableDef) Index(name string, cols ...string) *TableDef {
	t.indices = append(t.indices, IndexDef{Name: name, Cols: cols})
	return t
}

// func (t *TableDef[T]) Unique(name string, cols ...string) *TableDef[T]
// func (t *TableDef[T]) Constraint(name, expr string) *TableDef[T]

var Registry []*TableDef
