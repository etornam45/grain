package schema

type ColumnDef struct {
	Table          string //NOTE: set by Table() once the column is attached
	Name           string
	Type           *ColumnType
	IsPK           bool
	IsNotNull      bool
	IsUnique       bool
	DefaultVal     any
	DefaultExprStr string
	RefCol         *ColumnDef
	OnDeleteAction ReferentialAction
	OnUpdateAction ReferentialAction
	HasIndex       bool
}

func Column(name string, t *ColumnType) *ColumnDef {
	return &ColumnDef{Name: name, Type: t}
}

func (c *ColumnDef) PrimaryKey() *ColumnDef                  { c.IsPK = true; return c }
func (c *ColumnDef) NotNull() *ColumnDef                     { c.IsNotNull = true; return c }
func (c *ColumnDef) Unique() *ColumnDef                      { c.IsUnique = true; return c }
func (c *ColumnDef) Default(v any) *ColumnDef                { c.DefaultVal = v; return c }
func (c *ColumnDef) DefaultExpr(expr string) *ColumnDef      { c.DefaultExprStr = expr; return c }
func (c *ColumnDef) References(col *ColumnDef) *ColumnDef    { c.RefCol = col; return c }
func (c *ColumnDef) OnDelete(a ReferentialAction) *ColumnDef { c.OnDeleteAction = a; return c }
func (c *ColumnDef) OnUpdate(a ReferentialAction) *ColumnDef { c.OnUpdateAction = a; return c }
func (c *ColumnDef) Index() *ColumnDef                       { c.HasIndex = true; return c }

func (c *ColumnDef) String() string {
	if c.Table == "" {
		return c.Name
	}
	return c.Table + "." + c.Name
}


func (c *ColumnDef) Str() string {
	return c.String()
}