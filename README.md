# Grain

**Grain** is a type-safe Object-Relational Mapping (ORM) for Go, built on
[pgx](https://github.com/jackc/pgx). It targets PostgreSQL only.

- **Type-safe schema** — tables and columns defined once in Go; column binds
  are checked at startup, not on first SQL error.
- **Snapshot-driven migrations** — edit your schema, then `grain generate`
  diffs it against the last applied snapshot and writes the SQL for you.
- **Fluent query builder** — `SELECT`/`INSERT`/`UPDATE`/`DELETE` with joins,
  conditions, grouping and pagination — plus subqueries, CTEs, set operations
  (`UNION`/`INTERSECT`/`EXCEPT`), row-level locking (`FOR UPDATE ... SKIP
  LOCKED`), JSON/JSONB operators and table aliases; parameterized everywhere.
- **Transactions** — one method that begins, commits and rolls back for you,
  with optional isolation level / read-only control.
- **Reflection-based scanning** — rows map into plain structs via `db:` tags,
  including embedded structs, pointer fields and `db:"-"` skips.

## Contents

- [Installation](#installation)
- [Usage example](#usage-example)
- [Migrations](#migrations)
- [Querying](#querying)
- [Documentation](#documentation)
- [Requirements](#requirements)
- [License](#license)

## Installation

Add the library and the `grain` CLI to your project:

```bash
go get github.com/etornam45/grain@latest
go install github.com/etornam45/grain/cmd/grain@latest   # CLI -> $(go env GOPATH)/bin/grain
```

The CLI is a separate binary from the library — you only need it if you manage
migrations with `grain generate` / `grain migrate`.

A runnable version of everything below lives in [`examples/basic`](examples/basic).

### Usage Example

```go
import "github.com/etornam45/grain/schema"

var UserStatus = schema.Enum("user_status", "active", "suspended", "banned")

var Users = schema.Table("users",
	schema.Column("id", schema.UUID()).PrimaryKey().Default("gen_random_uuid()"),
	schema.Column("name", schema.Varchar(255)).NotNull(),
	schema.Column("email", schema.Varchar(255)).NotNull().Unique(),
	schema.Column("age", schema.Int()),
	schema.Column("status", UserStatus).NotNull().Default("active"),
)
```



### Migrations

Generate a migration from your schema code, then apply it:

```bash
grain generate -schema path/to/model "create user table"
DATABASE_URL=postgres://user:pass@localhost:5432/app grain migrate up
```

Command usage

```bash
usage:
  grain generate -schema <dir> [-force] [-yes] [name]
  grain migrate init                      (requires DATABASE_URL)
  grain migrate up                        (requires DATABASE_URL)
  grain migrate down [n]                  (requires DATABASE_URL)
  grain migrate redo                      (requires DATABASE_URL)
  grain migrate status                    (requires DATABASE_URL)
```



### Querying

These snippets assume a table defined as in the [Usage example](#usage-example),
plus `ctx`, `conn`, `tx` and `newID` in scope. `query` is
`github.com/etornam45/grain/query` and `db` is
`github.com/etornam45/grain/db`.

1. **Inserting**

```go
var newID string
err = query.Insert(schema.Users).
	Values(map[string]any{
		"name":   "Ama",
		"email":  "ama@example.com",
		"status": "active",
	}).
	Returning(schema.Users.Col("id").String()).
	Scan(ctx, conn, &newID)
```

2. **Selecting**

```go
type User struct {
	ID     string `db:"users.id"`
	Name   string `db:"users.name"`
	Email  string `db:"users.email"`
	Status string `db:"users.status"`
}

users, err := query.Select[User](schema.Users.Col("id"), schema.Users.Col("name"), schema.Users.Col("email"), schema.Users.Col("status")).
	From(schema.Users).
	Where(query.Eq(schema.Users.Col("status"), "active")).
	OrderBy([]string{schema.Users.Col("name").Str()}, query.Asc).
	Limit(10).
	All(ctx, conn)
```

3. **Joins**

```go
rows, err := query.Select[UserOrderRow](schema.Users.Col("name"), schema.Orders.Col("total")).
	From(schema.Users).
	InnerJoin(schema.Orders, query.Eq(schema.Users.Col("id"), schema.Orders.Col("user_id"))).
	Where(query.Gt(schema.Orders.Col("total"), 100)).
	All(ctx, conn)
```

4. **Deleting**

```go
_, err = query.Delete(schema.Users).Where(query.Eq(schema.Users.Col("id"), newID)).Run(ctx, tx)
```

5. **Transactions**

```go
err = conn.Transaction(ctx, func(tx *db.Tx) error {
	_, err := query.Delete(schema.Orders)
		.Where(query.Eq(schema.Orders.Col("user_id"), newID))
		.Run(ctx, tx)
	if err != nil {
		return err
	}
	_, err = query.Delete(schema.Users)
		.Where(query.Eq(schema.Users.Col("id"), newID))
		.Run(ctx, tx)
	return err
})
```

### Documentation

Full library reference and tutorials live in [docs/](docs/README.md):

- [Schema](docs/schema.md) — tables, columns, types, enums, foreign keys, indexes
- [Query](docs/query.md) — select/insert/update/delete, conditions, joins, aggregation
- [db & scan](docs/db-and-scan.md) — connections, transactions, row mapping
- [Migrations & CLI](docs/migrations.md) — the `grain` CLI, generation, apply/rollback/status
- [Tutorials](docs/tutorials/) — getting started, relationships

### Requirements

- Go 1.25+
- PostgreSQL 12+

### License

Grain is released under the Apache License 2.0. See [`LICENSE`](LICENSE) for details.

