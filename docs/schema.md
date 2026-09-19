# Schema

`github.com/etornam45/grain/schema` lets you describe your database schema in Go, once. Every table
and column is a typed value that the rest of Grain (queries, migrations,
scanning) consumes.

A schema file is a normal Go file — usually grouped under a `schema` package so
the migration generator can load it.

## Tables

A table is declared with `schema.Table(name, columns...)`. It returns a
`*TableDef` whose columns can be accessed by name via `.Col("name")`. The
binding is done **by name**: the column's declared name is what you use
everywhere else (queries, conditions, foreign keys).

```go
import "github.com/etornam45/grain/schema"

var Users = schema.Table("users",
    schema.Column("id", schema.UUID()).PrimaryKey().Default("gen_random_uuid()"),
    schema.Column("name", schema.Varchar(255)).NotNull(),
    schema.Column("email", schema.Varchar(255)).NotNull().Unique(),
    schema.Column("age", schema.Int()),
    schema.Column("status", schema.Text()).NotNull().Default("active"),
)
```

Each column is a `*schema.ColumnDef` that you can use in queries and
migrations. Columns are accessible by name:

```go
Users.Col("id")    // *schema.ColumnDef — panics if name not found
Users.Col("email") // used for foreign keys, conditions, etc.
```

A column whose name doesn't exist panics at startup — this catches typos
between your Go view and your table definition immediately.

## Column types

`Column(name string, t *ColumnType) *ColumnDef` creates a column. The type is
one of the `schema` constructors:

| Constructor            | SQL                              |
| ---------------------- | -------------------------------- |
| `schema.Serial()`      | `SERIAL`                         |
| `schema.BigSerial()`   | `BIGSERIAL`                      |
| `schema.Int()`         | `INTEGER`                        |
| `schema.BigInt()`      | `BIGINT`                         |
| `schema.Bool()`        | `BOOLEAN`                        |
| `schema.Text()`        | `TEXT`                           |
| `schema.UUID()`        | `UUID`                           |
| `schema.Timestamp()`   | `TIMESTAMP`                      |
| `schema.TimestampTZ()` | `TIMESTAMPTZ`                    |
| `schema.JSONB()`       | `JSONB`                          |
| `schema.Varchar(n)`    | `VARCHAR(n)`                     |
| `schema.Numeric(p, s)` | `NUMERIC(p,s)`                   |
| `schema.Enum(name, values...)` | PostgreSQL enum type `name` |

### Enums

Enums are registered globally so migration generation can diff them:

```go
var UserStatus = schema.Enum("user_status", "active", "suspended", "banned")

// use it like any other column type:
schema.Column("status", UserStatus).NotNull().Default("active")
```

The enum's name and values are stored in `schema.EnumRegistry` (a
`map[string]*ColumnType`), which `migrate` reads when building snapshots.

## Column modifiers

Every modifier returns the same `*ColumnDef`, so the calls chain in any order:

```go
schema.Column("email", schema.Varchar(255)).
    NotNull().
    Unique()
```

| Method                                   | Effect                                        |
| ---------------------------------------- | --------------------------------------------- |
| `.PrimaryKey()`                          | Marks the column as the primary key           |
| `.NotNull()`                             | Adds `NOT NULL`                               |
| `.Unique()`                              | Adds `UNIQUE`                                 |
| `.Default(v any)`                        | Sets a default **value** (quoted as needed)   |
| `.DefaultExpr(expr string)`              | Sets a default **expression** (used verbatim) |
| `.References(col *ColumnDef)`            | Foreign key → the referenced column           |
| `.OnDelete(a ReferentialAction)`         | FK `ON DELETE` action (requires `References`) |
| `.OnUpdate(a ReferentialAction)`         | FK `ON UPDATE` action (requires `References`) |
| `.Index()`                               | Creates a single-column index named `idx_<table>_<col>` |

### Defaults

`Default` supplies a literal value:

```go
schema.Column("status", schema.Text()).Default("active") // DEFAULT 'active'
schema.Column("price", schema.Numeric(10, 2)).Default(0) // DEFAULT 0
```

`DefaultExpr` supplies raw SQL (no quoting):

```go
schema.Column("id", schema.UUID()).DefaultExpr("gen_random_uuid()")
schema.Column("created_at", schema.TimestampTZ()).DefaultExpr("now()")
```

A string passed to `Default` that contains a `(` is also emitted verbatim (to
allow function-call defaults), while any other string is quoted.

### Referential actions

FK actions come from `schema`:

```go
const (
    Cascade  ReferentialAction = "CASCADE"
    SetNull  ReferentialAction = "SET NULL"
    Restrict ReferentialAction = "RESTRICT"
    NoAction ReferentialAction = "NO ACTION"
)
```

```go
schema.Column("user_id", schema.UUID()).
    NotNull().
    References(Users.Col("id")).
    OnDelete(schema.Cascade)
```

## Indexes

The `.Index()` column modifier creates an index `idx_<table>_<column>`. For
composite indexes, use the table-level `Index`:

```go
var Product = schema.Table("product",
    schema.Column("id", schema.UUID()).PrimaryKey().NotNull(),
    schema.Column("name", schema.Varchar(225)).NotNull().Index(),
    schema.Column("price", schema.Numeric(10, 2)).NotNull(),
    schema.Column("sku", schema.Varchar(15)).NotNull().Unique(),
    schema.Column("barcode", schema.Int()).Index(),
).
    Index("idx_product_name_sku", "name", "sku")
```

```go
func (t *TableDef) Index(name string, cols ...string) *TableDef
```

## Inspecting tables

`*ColumnDef` exposes its definition and a renderable reference:

- `String() string` → `"name"` when not bound to a table yet, or `"table.name"`
  once the column is registered on a table.
- `Str() string` → alias of `String()`.

`*TableDef` exposes:

- `Col(name string) *ColumnDef` — lookup by column name, panics if missing.
- `Columns() []*ColumnDef` — columns in declaration order.
- `GetIndices() []IndexDef` — declared indexes (`IndexDef{Name, Cols}`).
- `TableName() string` — the table name.

`*TableDef` implements `namedTable` (a `TableName() string` method), which
is all the `query` package needs, so `schema.Table`s plug straight into
`Select`, `Insert`, `Update`, and `Delete`.

## The registry

Each call to `schema.Table(...)` appends the table to the package-level
`Registry` (a `[]*TableDef`). `migrate.BuildSnapshot()` walks `Registry` and
`EnumRegistry` to produce the schema snapshot used by the migration generator —
so every table you want tracked must be declared at package `init` time (i.e.
as a package-level `var`) in a package the generator can import.