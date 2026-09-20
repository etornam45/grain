package schema

import (
	"fmt"
)

type IndexDef struct {
	Name      string
	Cols      []string
	Unique    bool
	Predicate string
}

type ConstraintDef struct {
	Name string
	Expr string
}

type TableDef struct {
	Name        string
	colsByName  map[string]*ColumnDef
	columns     []*ColumnDef
	indices     []IndexDef
	uniques     []IndexDef
	constraints []ConstraintDef
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
func (t *TableDef) GetUnique() []IndexDef  { return t.uniques }
func (t *TableDef) GetConstraints() []ConstraintDef {
	return t.constraints
}
func (t *TableDef) TableName() string { return t.Name }

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

func (t *TableDef) IndexUnique(name string, cols ...string) *TableDef {
	t.indices = append(t.indices, IndexDef{Name: name, Cols: cols, Unique: true})
	return t
}

// IndexPartial creates an index restricted to rows matching the predicate,
// e.g. IndexPartial("idx_users_active", []string{"status"}, "status = 'active'").
func (t *TableDef) IndexPartial(name string, cols []string, predicate string) *TableDef {
	t.indices = append(t.indices, IndexDef{Name: name, Cols: cols, Predicate: predicate})
	return t
}

func (t *TableDef) Unique(name string, cols ...string) *TableDef {
	t.uniques = append(t.uniques, IndexDef{Name: name, Cols: cols})
	return t
}

func (t *TableDef) Constraint(name, expr string) *TableDef {
	t.constraints = append(t.constraints, ConstraintDef{Name: name, Expr: expr})
	return t
}

var Registry []*TableDef
