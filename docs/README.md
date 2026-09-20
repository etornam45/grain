# Grain Documentation

Grain is an Object-Relational Mapping (ORM) for Go, built on top of
[pgx](https://github.com/jackc/pgx). It targets PostgreSQL only.

Grain is split into a few small, composable packages:


| Package         | Purpose                                                                 |
| --------------- | ----------------------------------------------------------------------- |
| `github.com/etornam45/grain/schema`  | Type-safe table, column, enum, type, constraint and index definitions   |
| `github.com/etornam45/grain/query`   | Fluent SQL query builder for `SELECT`, `INSERT`, `UPDATE`, `DELETE`     |
| `github.com/etornam45/grain/db`      | Connection wrapper (`Open`, `Connect`, `TransactionOpts`) and the `Executor` interface |
| `github.com/etornam45/grain/scan`    | Reflection-based scanning of rows into Go structs via `db:` tags        |
| `github.com/etornam45/grain/migrate` | Snapshot-driven migration generation, apply, rollback and status        |
| `grain` CLI     | `generate`, `migrate init / up / down [n] / redo / status`              |


> NOTE: this project is in its early development and may have API changes
> before a major release.



## Documentation index



### Reference

- [Schema](schema.md) — column types, `ColumnDef` modifiers, tables, binds,
  enums, foreign keys, identity/generated/check columns and indexes.
- [Query](query.md) — select/insert/update/delete builders, conditions, joins,
  ordering, grouping, pagination — plus subqueries, CTEs, set operations,
  row-level locking and table aliases.
- [db & scan](db-and-scan.md) — connecting, transactions (with isolation /
  read-only options), and mapping rows into structs (embedded structs,
  pointer fields, `db:"-"`).
- [Migrations & CLI](migrations.md) — the `grain` CLI, snapshot/diff workflow,
  migration file format, applied-state tracking (checksums), rename/destructive
  prompts, and safety notes.



### Tutorials

- [Getting Started](tutorials/getting-started.md) — end to end: define a schema,
generate and apply migrations, then insert/select/update/delete.
- [Relationships](tutorials/relationships.md) — foreign keys, join queries,
grouped counts, and transactional deletes.



## Quick start

```go
import (
    "context"
    "fmt"

    _ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
    "github.com/etornam45/grain/db"
    "github.com/etornam45/grain/examples/basic/schema"
    "github.com/etornam45/grain/query"
)

func main() {
    conn, err := db.Open("pgx", "postgres://user:pass@localhost:5432/app?sslmode=disable")
    if err != nil {
        panic(err)
    }
    defer conn.Close()

    ctx := context.Background()

    var newID string
    err = query.Insert(schema.Users).
        Values(map[string]any{
            "name":   "Ama",
            "email":  "ama@example.com",
            "status": "active",
        }).
        Returning(schema.Users.Col("id").String()).
        Scan(ctx, conn, &newID)
    if err != nil {
        panic(err)
    }
    fmt.Println("inserted user:", newID)
}
```

See [tutorials/getting-started.md](tutorials/getting-started.md) for the full
walkthrough, and the `[examples/basic](../examples/basic)` directory for a
complete runnable program plus its schema.