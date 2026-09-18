# db & scan

Grain's database connections live in `grain/db`; row mapping lives in
`grain/scan`.

## Connecting

`db.Open` wraps `database/sql` for any driver:

```go
import (
    _ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
    "grain/db"
)

conn, err := db.Open("pgx", "postgres://user:pass@localhost:5432/app?sslmode=disable")
if err != nil {
    panic(err)
}
defer conn.Close()
```

Any `database/sql` driver works, so `github.com/lib/pq` (`db.Open("postgres",
dsn)`) is a drop-in alternative. The `pgx` stdlib driver is used throughout
this documentation and by the CLI.

`*db.DB` embeds `*sql.DB`, so all standard `database/sql` methods (pool
settings, `PingContext`, `Close`, ...) are available too.

## The Executor interface

The single seam between connections, transactions, and the query builder:

```go
type Executor interface {
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
```

Both `*db.DB` and `*db.Tx` satisfy it, so the same query code runs against a
plain connection or inside a transaction. The query builders (`Select`,
`Insert`, `Update`, `Delete`) accept an `Executor` as their execution target —
see [query.md](query.md).

## Transactions

```go
err := conn.Transaction(ctx, func(tx *db.Tx) error {
    _, err := query.Delete(schema.Orders).
        Where(query.Eq(schema.Orders.Cols.UserID, id)).
        Run(ctx, tx)
    if err != nil {
        return err
    }
    _, err = query.Delete(schema.Users).
        Where(query.Eq(schema.Users.Cols.ID, id)).
        Run(ctx, tx)
    return err
})
if err != nil {
    // the whole transaction was rolled back
}
```

Semantics:

- `Transaction(ctx, fn)` begins a transaction, runs `fn` with a `*db.Tx`, and
  **commits** when `fn` returns `nil`.
- If `fn` returns an error, the transaction is **rolled back** and that error is
  returned as-is.
- Return an error from `fn` to abort; the rollback is automatic — you don't
  need to call `Rollback()`.

## Scan

`grain/scan` maps query results into structs using reflection. Columns are
matched to struct fields by `db` tag.

### Tags

A struct field's tag may be the fully-qualified column (`table.name`) or the
bare column name (`name`). Fully-qualified tags matter when a result contains
columns from several tables.

```go
type UserOrder struct {
    ID    string `db:"users.id"`
    Name  string `db:"users.name"`
    Total string `db:"orders.total"`
}
```

### Matching rules

The scanner takes the result columns from the driver (e.g. `users.name`,
`orders.total`) and resolves each one:

1. A `db` tag equal to the column name is an exact match.
2. Otherwise it falls back to a **suffix match** — a tag like `users.name`
   still matches a result column `name` (and `users.name`.)
3. If no field matches a result column, scanning fails with an error naming the
   offending column — you won't silently drop data.

This is why single-table queries can use `db:"name"` while joined queries should
use `db:"users.name"`.

### Usage

The query builders call `scan` for you:

```go
// via query.Select
users, err := query.Select[User](...).From(schema.Users).All(ctx, conn)

// standalone against raw sql.Rows
rows, err := conn.QueryContext(ctx, "SELECT * FROM users")
if err != nil {
    return err
}
defer rows.Close()
users, err := scan.All[User](rows)
```

Exported API:

```go
func All[T any](rows *sql.Rows) ([]T, error)            // all rows
func One[T any](row *sql.Row, cols []string) (T, error) // single row; returns sql.ErrNoRows if none
```

`scan.All` is what powers `SelectBuilder.All`; `First` layers on `All` with a
`LIMIT 1` and translates "no rows" into `(nil, nil)`.