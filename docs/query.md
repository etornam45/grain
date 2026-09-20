# Query

`github.com/etornam45/grain/query` is a fluent, type-safe SQL builder. It never talks to PostgreSQL
directly, and builders work against anything with a `TableName() string` method
(tables) and anything with a `String() string` method (columns), so
`schema.ColumnDef` values plug straight in. (The one exception is table
aliasing, `query.Alias`, which needs a real `*schema.TableDef` to resolve
column names.)

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
    schema.Users.Col("id"),
    schema.Users.Col("name"),
    schema.Users.Col("email"),
    schema.Users.Col("status"),
).
    From(schema.Users).
    Where(query.Eq(schema.Users.Col("status"), "active")).
    OrderBy([]string{schema.Users.Col("name").Str()}, query.Asc).
    Limit(10).
    All(ctx, conn)
```

`Select[T](cols ...colRef)` is generic over the row type `T`. If `cols` is
empty, `SELECT` is filled from the struct's `db` tags (the same tags `scan`
uses), so you don't repeat the column list. Choose the columns explicitly for
joined queries or when you want expressions in the list.

### Table aliases

`query.Alias(t *schema.TableDef, "u")` returns an aliased view whose `.Col("field")`
renders as `u.field` and whose `From`/join clause renders `schema AS alias`:

```go
u := query.Alias(schema.Users, "u")
rows, err := query.Select[userRow](u.Col("id"), u.Col("name")).
    From(u).
    Where(query.Eq(u.Col("status"), "active")).
    All(ctx, conn)
```

### Where clauses

`Where(c Condition)` sets/overrides the filter; see [Conditions](#conditions).

### Joins

```go
rows, err := query.Select[userOrderRow](
    schema.Users.Col("name"),
    schema.Orders.Col("total"),
).
    From(schema.Users).
    InnerJoin(schema.Orders, query.Eq(schema.Users.Col("id"), schema.Orders.Col("user_id"))).
    Where(query.Gt(schema.Orders.Col("total"), 100)).
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
query.Select[countRow](schema.Users.Col("status")).
    From(schema.Users).
    GroupBy(schema.Users.Col("status").String()).
    Having(query.Gt(schema.Users.Col("id"), 5)).
    All(ctx, conn)
```

- `GroupBy(cols ...string)` — raw column strings (use `.String()`).
- `Having(c Condition)` — same condition API as `Where`.

### Ordering and pagination

```go
query.Select[User](...).
    From(schema.Users).
    OrderBy([]string{schema.Users.Col("name").String()}, query.Asc).
    OrderBy([]string{schema.Users.Col("id").String()}, query.Desc).
    Limit(10).
    Offset(20).
    All(ctx, conn)
```

- `OrderBy(cols []string, dir OrderDir)` — `dir` is `query.Asc` or `query.Desc`.
  Multiple `OrderBy` calls accumulate into an ordered list.
- `Limit(n int)` / `Offset(n int)`.

### Distinct

```go
query.Select[User](schema.Users.Col("name")).Distinct().From(schema.Users)
```

### Ordering with NULLS placement

`OrderByNulls(cols []string, dir OrderDir, nulls NullsOrder)` appends
`NULLS FIRST` / `NULLS LAST` to the clause:

```go
query.Select[User](...).
    From(schema.Users).
    OrderByNulls([]string{schema.Users.Col("age").String()}, query.Desc, query.NullsLast)
```

### Common table expressions

`With(name string, sub Subquery)` prepends `WITH name AS (SELECT ...)`. CTE
arguments are bound before the main query's, so placeholders stay correct:

```go
active := query.Sub(
    query.Select[struct{}](schema.Users.Col("id")).
        From(schema.Users).
        Where(query.Eq(schema.Users.Col("status"), "active")),
)

rows, err := query.Select[User](...).
    With("active", active).
    From(schema.Users).
    All(ctx, conn)
```

### Set operations

`Union`, `UnionAll`, `Intersect` and `Except` combine another `Subquery`'s
result with this one. The outer builder keeps its `ORDER BY`/`LIMIT`, which
apply to the whole combined result:

```go
adults := query.Sub(query.Select[...](...).From(schema.Users).Where(query.Gt(schema.Users.Col("age"), 18)))
teens  := query.Sub(query.Select[...](...).From(schema.Users).Where(query.Between(schema.Users.Col("age"), 13, 17)))

rows, err := query.Select[...](...).From(schema.Users).
    Union(adults).
    UnionAll(teens).
    OrderBy([]string{schema.Users.Col("name").String()}, query.Asc).
    All(ctx, conn)
```

### Row-level locking

`ForUpdate()` / `ForShare()` append the lock, and `NoWait()` / `SkipLocked()`
pick the contention behavior:

```go
row, err := query.Select[Job](schema.Jobs.Col("id")).
    From(schema.Jobs).
    Where(query.Eq(schema.Jobs.Col("state"), "pending")).
    ForUpdate().
    SkipLocked().
    First(ctx, conn)
```

### Executing

| Method | Result                                        |
| ------ | --------------------------------------------- |
| `All(ctx, exec)` | All rows scanned into `[]T` (zero rows → empty slice) |
| `First(ctx, exec)` | First row as `*T`; `(nil, nil)` when there are no rows (adds `LIMIT 1`) |
| `Count(ctx, exec)` | `int64` count of the whole query via `SELECT COUNT(*) FROM (<query>)` |
| `CountDistinct(ctx, exec, col)` | `SELECT COUNT(DISTINCT col) FROM (<query>)` |
| `SQL()` | `(string, []any)` — the SQL and its args |

> `Count` and `CountDistinct` wrap the query in a subquery
> (`SELECT COUNT(*) FROM (...) AS _count_subquery`), so they work with joins,
> grouping and ordering in place.

### Subqueries

`Sub(selectBuilder)` captures any `SelectBuilder[T]` as a reusable `Subquery`.
Use it in conditions and CTEs (above). Its placeholders are re-numbered to
wherever it nests:

```go
bigOrders := query.Sub(
    query.Select[struct{}](schema.Orders.Col("user_id")).
        From(schema.Orders).
        Where(query.Gt(schema.Orders.Col("total"), 1000)),
)

query.Select[userRow](...).
    From(schema.Users).
    Where(query.And(
        query.In(schema.Users.Col("id"), bigOrders),
        query.Exists(
            query.Sub(query.Select[struct{}](schema.Orders.Col("id")).
                From(schema.Orders).
                Where(query.Raw("orders.user_id = users.id"))),
        ),
        query.EqAny(schema.Users.Col("id"), bigOrders),
        query.NeqAll(schema.Users.Col("id"), bigOrders),
    ))
```

`Subquery.As(alias)` renders a subquery as a selected column
(`(SELECT ...) AS total`); only subqueries **without bound arguments** may be
used this way (argument values can't thread through the select list yet).

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
| `Between(col, low, high)` | `col BETWEEN $n AND $n+1` |
| `NotBetween(col, low, high)` | `col NOT BETWEEN $n AND $n+1` |
| `EqAny(col, sub)`   | `col = ANY (SELECT ...)`    |
| `NeqAll(col, sub)`  | `col <> ALL (SELECT ...)`   |
| `Exists(sub)`       | `EXISTS (SELECT ...)`       |
| `Raw(sql, args...)` | Custom SQL expr (`?` bound) |

> `In` and `NotIn` accept either variadic values (`query.In(col, 1, 2, 3)`),
> Go slices (`query.In(col, ids)`), or a single `Subquery`
> (`query.In(col, query.Sub(...))` → `col IN (SELECT ...)`).
> Passing an empty slice safely renders `1 = 0` for `In` and `1 = 1` for `NotIn`.

### JSON / JSONB operators

These bind the operand as a placeholder (`$n`):

| Function            | SQL              |
| ------------------- | ---------------- |
| `Contains(col, v)`  | `col @> $n`      |
| `ContainedBy(col, v)`| `col <@ $n`     |
| `KeyExists(col, k)` | `col ? $n`       |

```go
query.Select[...](...).
    From(schema.Users).
    Where(query.Contains(schema.Users.Col("profile"), map[string]any{"plan": "pro"}))
```

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
        query.Eq(schema.Users.Col("status"), "active"),
        query.Eq(schema.Users.Col("status"), "suspended"),
    ),
    query.Gt(schema.Orders.Col("total"), 0),
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
    Returning(schema.Users.Col("id").String()).
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
    Where(query.Eq(schema.Users.Col("email"), "ama@example.com")).
    Run(ctx, conn)
```

| Method | Result |
| ------ | ------ |
| `Set(vals map[string]any)` | Columns to set (keys sorted alphabetically). Values may be **literals** (bound as placeholders), **column references** (`users.age` renders as-is → `SET x = users.age`), or **subqueries** (`query.Sub(...)` renders `SET x = (SELECT ...)`) |
| `Where(c Condition)` | Filter; omit to update every row |
| `OrderBy(cols []string, dir OrderDir)` | `ORDER BY` (PostgreSQL `UPDATE ... ORDER BY`) |
| `OrderByNulls(cols, dir, nulls)` | `OrderBy` with `NULLS FIRST/LAST` |
| `Limit(n int)` | Cap updated rows (PostgreSQL `UPDATE ... LIMIT n`) |
| `Returning(cols ...string)` | Appends `RETURNING <cols>` |
| `Run(ctx, exec)` | Executes; returns `(rowsAffected int64, error)` |
| `Scan(ctx, exec, dest ...any)` | Executes and scans the single `RETURNING` row into `dest` |
| `SQL()` | `(string, []any)` |

## Delete

```go
n, err := query.Delete(schema.Orders).
    Where(query.Eq(schema.Orders.Col("user_id"), id)).
    Run(ctx, conn)
```

| Method | Result |
| ------ | ------ |
| `Where(c Condition)` | Filter; omit to delete every row |
| `OrderBy(cols []string, dir OrderDir)` | `ORDER BY` (PostgreSQL `DELETE ... ORDER BY`) |
| `OrderByNulls(cols, dir, nulls)` | `OrderBy` with `NULLS FIRST/LAST` |
| `Limit(n int)` | Cap removed rows (PostgreSQL `DELETE ... LIMIT n`) |
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