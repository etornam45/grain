### Welcome to Grain

Grain is an Object-Relational Mapping (ORM) for Go 

### Documentation

Full library reference and tutorials live in [docs/](docs/README.md):

- [Schema](docs/schema.md) — tables, columns, types, enums, foreign keys, indexes
- [Query](docs/query.md) — select/insert/update/delete, conditions, joins, aggregation
- [db & scan](docs/db-and-scan.md) — connections, transactions, row mapping
- [Migrations & CLI](docs/migrations.md) — the `grain` CLI, generation, apply/rollback/status
- [Tutorials](docs/tutorials/) — getting started, relationships

### Usage Example

```go
import "grain/schema"

var UserStatus = schema.Enum("user_status", "active", "suspended", "banned")

type usersColumns struct {
	ID, Name, Email, Age, Status *schema.ColumnDef
}

var Users = schema.Table[usersColumns]("users",
	schema.Column("id", schema.UUID()).PrimaryKey().Default("gen_random_uuid()"),
	schema.Column("name", schema.Varchar(255)).NotNull(),
	schema.Column("email", schema.Varchar(255)).NotNull().Unique(),
	schema.Column("age", schema.Int()),
	schema.Column("status", UserStatus).NotNull().Default("active"),
)
```



### Migration

You can generate a migration for go using 

```bash
grain generate -schema path/to/model "create user table" 
```

Command usage

```bash
usage:
  grain generate -schema <dir> [name]
  grain migrate up                       (requires DATABASE_URL)
  grain migrate down                     (requires DATABASE_URL)
```



### Querying

1. **Inserting**

```go
var newID string
err = query.Insert(model.Users).
	Values(map[string]any{
		"name":   "Ama",
		"email":  "ama@example.com",
		"status": "active",
	}).
	Returning(schema.Users.Cols.ID.String()).
	Scan(ctx, conn, &newID)
```

1. **Selecting**

```go
type User struct {
	ID     string `db:"users.id"`
	Name   string `db:"users.name"`
	Email  string `db:"users.email"`
	Status string `db:"users.status"`
}

users, err := query.Select[User](schema.Users.Cols.ID, schema.Users.Cols.Name, schema.Users.Cols.Email, schema.Users.Cols.Status).
	From(schema.Users).
	Where(query.Eq(schema.Users.Cols.Status, "active")).
	OrderBy([]string {schema.Users.Cols.Name.Str()}, query.Asc).
	Limit(10).
	All(ctx, conn)
```

1. **Joins**

```go
rows, err := query.Select[UserOrderRow](schema.Users.Cols.Name, schema.Orders.Cols.Total).
	From(schema.Users).
	InnerJoin(schema.Orders, query.Eq(schema.Users.Cols.ID, schema.Orders.Cols.UserID)).
	Where(query.Gt(schema.Orders.Cols.Total, 100)).
	All(ctx, conn)
```

1. **Deleting**

```go
_, err = query.Delete(schema.Users).Where(query.Eq(schema.Users.Cols.ID, newID)).Run(ctx, tx)
```

1. **Transactions**

```go
err = conn.Transaction(ctx, func(tx *db.Tx) error {
	_, err := query.Delete(schema.Orders)
		.Where(query.Eq(schema.Orders.Cols.UserID, newID))
		.Run(ctx, tx)
	if err != nil {
		return err
	}
	_, err = query.Delete(schema.Users)
		.Where(query.Eq(schema.Users.Cols.ID, newID))
		.Run(ctx, tx)
	return err
})
```

> NOTE: this project is in it's early development and may have API changes before a major release

