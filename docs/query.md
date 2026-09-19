# Query

`github.com/etornam45/grain/query` is a fluent, type-safe SQL builder. It never talks to PostgreSQL
directly *and* it never needs to import `github.com/etornam45/grain/schema` — builders work against
anything with a `TableName() string` method (tables) and anything with a
`String() string` method (columns), so `schema.ColumnDef` values plug straight
in.

Every builder has an `SQL() (string, []any)` method that renders the final SQL
with `$1, $2, ...` placeholders ready for pgx, and one or more execution
methods.

## Select

```go
type activeUser struct {
    ID     string `db:"users.id"`
    Name   string `db:"users.name"`
    Email  string `db:"users.email"`
    Status string `db:"users.status"`
}

users, err := query.Select[activeUser](
    schema.Users.Cols.ID,
    schema.Users.Cols.Name,
    schema.Users.Cols.Email,
    schema.Users.Cols.Status,
).
    From(schema.Users).
    Where(query.Eq(schema.Users.Cols.Status, "active")).
    OrderBy([]string{schema.Users.Cols.Name.Str()}, query.Asc).
    Limit(10).
    All(ctx, conn)
```

`Select[T](cols ...colRef)` is generic over the row type `T`. If `cols` is
empty the rendered SQL selects `*` (choose the columns explicitly for joined
queries).

### Where clauses

`Where(c Condition)` sets/overrides the filter; see [Conditions](#conditions).

### Joins

```go
rows, err := query.Select[userOrderRow](
    schema.Users.Cols.Name,
    schema.Orders.Cols.Total,
).
    From(schema.Users).
    InnerJoin(schema.Orders, query.Eq(schema.Users.Cols.ID, schema.Orders.Cols.UserID)).
    Where(query.Gt(schema.Orders.Cols.Total, 100)).
    All(ctx, conn)
```

| Method                            | SQL                       |
| --------------------------------- | ------------------------- |
| `InnerJoin(t, on Condition)`      | `INNER JOIN t ON <on>`    |
| `LeftJoin(t, on Condition)`       | `LEFT JOIN t ON <on>`     |
| `RightJoin(t, on Condition)`      | `RIGHT JOIN t ON <on>`    |
| `FullJoin(t, on Condition)`       | `FULL JOIN t ON <on>`     |
| `CrossJoin(t)`                    | `CROSS JOIN t`            |

Column references are rendered fully qualified (`users.id`), and scans match
result columns against struct `db:` tags — see [scan](db-and-scan.md).

### Grouping and filtering groups

```go
query.Select[countRow](schema.Users.Cols.Status).
    From(schema.Users).
    GroupBy(schema.Users.Cols.Status.String()).
    Having(query.Gt(schema.Users.Cols.ID, 5)).
    All(ctx, conn)
```

- `GroupBy(cols ...string)` — raw column strings (use `.String()`).
- `Having(c Condition)` — same condition API as `Where`.

### Ordering and pagination

```go
query.Select[User](...).
    From(schema.Users).
    OrderBy([]string{schema.Users.Cols.Name.String()}, query.Asc).
    OrderBy([]string{schema.Users.Cols.ID.String()}, query.Desc).
    Limit(10).
    Offset(20).
    All(ctx, conn)
```

- `OrderBy(cols []string, dir OrderDir)` — `dir` is `query.Asc` or `query.Desc`.
  Multiple `OrderBy` calls accumulate into an ordered list.
- `Limit(n int)` / `Offset(n int)`.

### Distinct

```go
query.Select[User](schema.Users.Cols.Name).Distinct().From(schema.Users)
```

### Executing

| Method | Result                                        |
| ------ | --------------------------------------------- |
| `All(ctx, exec)` | All rows scanned into `[]T` (zero rows → empty slice) |
| `First(ctx, exec)` | First row as `*T`; `(nil, nil)` when there are no rows (adds `LIMIT 1`) |
| `Count(ctx, exec)` | `int64` count of the whole query via `SELECT COUNT(*) FROM (<query>)` |
| `SQL()` | `(string, []any)` — the SQL and its args |

> `Count` wraps the query in a subquery (`SELECT COUNT(*) FROM (...) AS _count_subquery`),
> so it works with joins, grouping and ordering in place.

## Conditions

A `Condition` renders itself to a SQL fragment starting placeholder numbering
at a given offset, so conditions compose anywhere in a query.

### Comparison

| Function            | SQL                        |
| ------------------- | -------------------------- |
| `Eq(col, v)`        | `col = $n`                 |
| `Neq(col, v)`       | `col <> $n`                |
| `Gt(col, v)`        | `col > $n`                 |
| `Gte(col, v)`       | `col >= $n`                |
| `Lt(col, v)`        | `col < $n`                 |
| `Lte(col, v)`       | `col <= $n`                |
| `Like(col, pattern)`| `col LIKE $n`              |
| `ILike(col, pattern)`| `col ILIKE $n`            |
| `In(col, vals...)`  | `col IN ($n, $n+1, ...)`   |
| `NotIn(col, vals...)`| `col NOT IN ($n, ...)`    |
| `Raw(sql, args...)` | Custom SQL expr (`?` bound) |

> `In` and `NotIn` accept either variadic values (`query.In(col, 1, 2, 3)`) or Go slices (`query.In(col, ids)`).
> Passing an empty slice safely renders `1 = 0` for `In` and `1 = 1` for `NotIn`.

### Null checks

| Function            | SQL            |
| ------------------- | -------------- |
| `IsNull(col)`       | `col IS NULL`  |
| `IsNotNull(col)`    | `col IS NOT NULL` |

### Composition

| Function       | Renders as                 |
| -------------- | --------------------------- |
| `And(a, b, ...)` | `(a AND b AND ...)`       |
| `Or(a, b, ...)`  | `(a OR b OR ...)`         |
| `Not(c)`       | `NOT (c)`                  |

Conditions can be nested arbitrarily:

```go
query.And(
    query.Or(
        query.Eq(schema.Users.Cols.Status, "active"),
        query.Eq(schema.Users.Cols.Status, "suspended"),
    ),
    query.Gt(schema.Orders.Cols.Total, 0),
)
```

## Insert

```go
err := query.Insert(schema.Users).
    Values(map[string]any{
        "name":   "Ama",
        "email":  "ama@example.com",
        "status": "active",
    }).
    Run(ctx, conn)
```

Map keys become the column list; they are **sorted alphabetically** so the
generated SQL is deterministic and the placeholders line up with the values.

| Method | Result |
| ------ | ------ |
| `Values(rows ...map[string]any)` | Set one or more rows (batch inserts) |
| `OnConflictDoNothing(targets ...string)` | `ON CONFLICT (targets) DO NOTHING` |
| `OnConflictDoUpdate(targets []string, vals map[string]any)` | `ON CONFLICT (targets) DO UPDATE SET ...` |
| `OnConflictExcluded(targets []string, cols ...string)` | `ON CONFLICT (targets) DO UPDATE SET col = EXCLUDED.col` |
| `Returning(cols ...string)` | Appends `RETURNING <cols>` |
| `Run(ctx, exec)` | Executes; returns `error` |
| `Scan(ctx, exec, dest ...any)` | Executes and scans the `RETURNING` row into `dest` via `QueryRow` |
| `SQL()` | `(string, []any)` |

Capture a generated key:

```go
var id string
err := query.Insert(schema.Users).
    Values(map[string]any{"name": "Ama"}).
    Returning(schema.Users.Cols.ID.String()).
    Scan(ctx, conn, &id)
```

Batch insert multiple rows:

```go
err := query.Insert(schema.Users).
    Values(
        map[string]any{"name": "Ama", "email": "ama@example.com"},
        map[string]any{"name": "Kofi", "email": "kofi@example.com"},
    ).
    Run(ctx, conn)
```

Upsert with `ON CONFLICT`:

```go
err := query.Insert(schema.Users).
    Values(map[string]any{"email": "ama@example.com", "name": "Ama"}).
    OnConflictDoUpdate([]string{"email"}, map[string]any{"name": "Ama Updated"}).
    Run(ctx, conn)
```

## Update

```go
n, err := query.Update(schema.Users).
    Set(map[string]any{"status": "banned"}).
    Where(query.Eq(schema.Users.Cols.Email, "ama@example.com")).
    Run(ctx, conn)
```

| Method | Result |
| ------ | ------ |
| `Set(vals map[string]any)` | Columns to set (keys sorted alphabetically) |
| `Where(c Condition)` | Filter; omit to update every row |
| `Returning(cols ...string)` | Appends `RETURNING <cols>` |
| `Run(ctx, exec)` | Executes; returns `(rowsAffected int64, error)` |
| `Scan(ctx, exec, dest ...any)` | Executes and scans the single `RETURNING` row into `dest` |
| `SQL()` | `(string, []any)` |

## Delete

```go
n, err := query.Delete(schema.Orders).
    Where(query.Eq(schema.Orders.Cols.UserID, id)).
    Run(ctx, conn)
```

| Method | Result |
| ------ | ------ |
| `Where(c Condition)` | Filter; omit to delete every row |
| `Returning(cols ...string)` | Appends `RETURNING <cols>` |
| `Run(ctx, exec)` | Executes; returns `(rowsAffected int64, error)` |
| `Scan(ctx, exec, dest ...any)` | Executes and scans the single `RETURNING` row into `dest` |
| `SQL()` | `(string, []any)` |

## Running against connections and transactions

Every execution method takes *any* `db.Executor` — that's the `*db.DB` from
`db.Open(...)`, a `*db.Tx` from `conn.Transaction(...)`, or anything else that
implements the interface. Passing the wrong thing is impossible: it's a
compile-time `error`, not a runtime panic.

```go
err = conn.Transaction(ctx, func(tx *db.Tx) error {
    _, err := query.Delete(schema.Orders).Where(...).Run(ctx, tx)
    if err != nil {
        return err
    }
    _, err = query.Delete(schema.Users).Where(...).Run(ctx, tx)
    return err
})
```

See [Transactions](db-and-scan.md#transactions).