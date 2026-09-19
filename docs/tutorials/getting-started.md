# Getting Started

This tutorial walks through building a small Grain application end to end:
defining a schema, generating and applying migrations, and running CRUD queries
against PostgreSQL. It mirrors the runnable [`examples/basic`](../../examples/basic)
program.

## Prerequisites

- Go 1.22+ (Grain is built on a recent pgx)
- A PostgreSQL server (local or remote)

## 1. New project and dependency

```bash
mkdir shop && cd shop
go mod init shop
go get github.com/etornam45/grain@latest
go install github.com/etornam45/grain/cmd/grain@latest
```

(`go install` puts the `grain` CLI in `$(go env GOPATH)/bin` — make sure that's
on your `PATH` so the `grain generate`/`grain migrate` commands below work.
With just `go get` you have the library but no command.)

This adds Grain (and its `github.com/jackc/pgx/v5` dependency) to your
`go.mod` as a normal dependency — no `replace` needed since it's published.
A minimal `go.mod` looks like:

```
module shop

go 1.25.0

require github.com/etornam45/grain v0.1.0
```

Run `go mod tidy` to wire up the indirect dependencies.

## 2. Define the schema

Create a schema package whose tables live at package level. Grain needs that so
the migration generator can load them.

`internal/schema/schema.go`:

```go
package schema

import "github.com/etornam45/grain/schema"

var UserStatus = schema.Enum("user_status", "active", "suspended", "banned")

var Users = schema.Table("users",
    schema.Column("id", schema.UUID()).PrimaryKey().DefaultExpr("gen_random_uuid()"),
    schema.Column("name", schema.Varchar(255)).NotNull(),
    schema.Column("email", schema.Varchar(255)).NotNull().Unique(),
    schema.Column("age", schema.Int()),
    schema.Column("status", UserStatus).NotNull().Default("active"),
)
```

Key points:

- `Table("users", ...)` binds whatever columns you pass; refer to them by name
  with `Users.Col("email")` wherever a column is needed.
- `.DefaultExpr("gen_random_uuid()")` emits the expression verbatim (no
  quoting); `.Default("active")` emits a quoted literal.
- `schema.Enum` registers `user_status` so migrations can create the type.

## 3. Generate and apply the initial migration

```bash
grain generate -schema internal/schema "create users table"
```

Grain writes `db/migrations/<timestamp>_create_users_table.sql` and the
snapshot journal under `db/migrations/meta/`. The generated file looks like:

```sql
-- +migrate Up
CREATE TYPE user_status AS ENUM ('active', 'suspended', 'banned');
CREATE TABLE users (
  id UUID DEFAULT gen_random_uuid(),
  name VARCHAR(255) NOT NULL,
  email VARCHAR(255) NOT NULL UNIQUE,
  age INTEGER,
  status user_status NOT NULL DEFAULT 'active',
  PRIMARY KEY (id)
);

-- +migrate Down
DROP TYPE user_status;
DROP TABLE users;
```

Apply it:

```bash
export DATABASE_URL=postgres://user:pass@localhost:5432/shop?sslmode=disable
grain migrate up
```

`grain.schema_migrations` now records the applied version and its checksum.
Check the state at any time with `grain migrate status`.

## 4. Connect

`github.com/etornam45/grain/db` wraps `database/sql`. Registering the pgx stdlib driver lets you
open with `db.Open("pgx", dsn)`:

```go
package main

import (
    "context"
    "fmt"
    "log"

    _ "github.com/jackc/pgx/v5/stdlib"

    "github.com/etornam45/grain/db"
    "github.com/etornam45/grain/query"
    "shop/internal/schema"
)

func main() {
    ctx := context.Background()

    conn, err := db.Open("pgx", "postgres://user:pass@localhost:5432/shop?sslmode=disable")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    _ = ctx
    _ = schema.Users
    _ = query.Eq
    fmt.Println("connected")
}
```

## 5. Insert

`query.Insert(...).Values(...)` takes a `map[string]any` keyed by column name.
Add `.Returning(...)` and `.Scan(...)` to capture generated values:

```go
var newID string
err := query.Insert(schema.Users).
    Values(map[string]any{
        "name":   "Ama",
        "email":  "ama@example.com",
        "status": "active",
    }).
    Returning(schema.Users.Col("id").String()).
    Scan(ctx, conn, &newID)
if err != nil {
    log.Fatal(err)
}
fmt.Println("inserted user:", newID)
```

## 6. Select with a non-empty result

Define a plain struct whose fields are tagged with the fully-qualified column
names, then use the generic `Select`:

```go
type User struct {
    ID     string `db:"users.id"`
    Name   string `db:"users.name"`
    Email  string `db:"users.email"`
    Status string `db:"users.status"`
}

users, err := query.Select[User](
    schema.Users.Col("id"),
    schema.Users.Col("name"),
    schema.Users.Col("email"),
    schema.Users.Col("status"),
).
    From(schema.Users).
    Where(query.Eq(schema.Users.Col("status"), "active")).
    OrderBy([]string{schema.Users.Col("name").String()}, query.Asc).
    Limit(10).
    All(ctx, conn)
if err != nil {
    log.Fatal(err)
}
for _, u := range users {
    fmt.Printf("%s <%s> is %s\n", u.Name, u.Email, u.Status)
}
```

## 7. Update and delete

`Update` returns the affected row count; `Delete` too:

```go
n, err := query.Update(schema.Users).
    Set(map[string]any{"status": "suspended"}).
    Where(query.Eq(schema.Users.Col("id"), newID)).
    Run(ctx, conn)
if err != nil {
    log.Fatal(err)
}
fmt.Println("updated", n, "user(s)")

n, err = query.Delete(schema.Users).
    Where(query.Eq(schema.Users.Col("id"), newID)).
    Run(ctx, conn)
if err != nil {
    log.Fatal(err)
}
fmt.Println("deleted", n, "user(s)")
```

## 8. Evolve the schema

Migrations are generated, not hand-written. Add a column — just add its
`schema.Column` to the table, then regenerate:

```go
var Users = schema.Table("users",
    schema.Column("id", schema.UUID()).PrimaryKey().DefaultExpr("gen_random_uuid()"),
    schema.Column("name", schema.Varchar(255)).NotNull(),
    schema.Column("email", schema.Varchar(255)).NotNull().Unique(),
    schema.Column("age", schema.Int()),
    schema.Column("status", UserStatus).NotNull().Default("active"),
    schema.Column("last_seen", schema.TimestampTZ()),
)
```

```bash
grain generate -schema internal/schema "add last_seen to users"
cat db/migrations/$(ls -t db/migrations | head -1)
grain migrate up
```

Grain detected `users.last_seen` was missing from the last snapshot and
generated `ALTER TABLE users ADD COLUMN last_seen ...`.

## Next steps

- [Relationships](relationships.md) — foreign keys, joins, and transactions.
- [Query API reference](../query.md)
- [Schema API reference](../schema.md)
- [Migration system reference](../migrations.md)