# Migrations & the grain CLI

Grain ships a CLI (`cmd/grain`) that generates and applies migrations from your
Go schema definitions. Instead of hand-writing SQL for every change, you edit
your `schema` package and Grain diffs it against the last applied snapshot.

## Installing the CLI

```bash
go install github.com/etornam45/grain/cmd/grain@latest
```

This installs `grain` into `$(go env GOPATH)/bin` — add that directory to your
`PATH` if it isn't already. (If you're developing Grain itself, you can run it
with `go run ./cmd/grain` instead.) The CLI is a separate binary from the
library: users of your application don't need it unless they manage migrations.

```bash
> grain --help
usage:
  grain generate -schema <dir> [-force] [-yes] [name]
  grain migrate init                      (requires DATABASE_URL)
  grain migrate up                        (requires DATABASE_URL)
  grain migrate down [n]                  (requires DATABASE_URL)
  grain migrate redo                      (requires DATABASE_URL)
  grain migrate status                    (requires DATABASE_URL)
```

## The workflow

1. Write your schema in Go (see [schema.md](schema.md)); tables must live at
   package level (`var`) so the generator can load them.
2. Generate a migration:
   ```bash
   grain generate -schema internal/schema "add orders table"
   ```
3. Review the generated `.sql` file, then apply it:
   ```bash
   DATABASE_URL=postgres://user:pass@localhost:5432/app grain migrate up
   ```

Changes accumulate as numbered files in `db/migrations/`, e.g.
`20260917113909_add_orders_table.sql`.

## generate

```
grain generate -schema <dir> [name]
```

- `-schema <dir>` — the directory containing your `schema.Table(...)` /
  `schema.Enum(...)` definitions (a package import path under your module).
- `[name]` — an optional migration name; defaults to `change`. Spaces are
  replaced with underscores in the filename.
- `-yes` — run non-interactively: rename/delete ambiguity prompts resolve
  automatically (rename to the only candidate, otherwise ignore).
- `-force` — generate destructive changes instead of failing with
  `ManualReviewRequiredError` (type casts without `USING`, `SET NOT NULL`, ...).
  Use only when you've verified the data.

Generation loads the **current** schema from your code, diffing it against the
**latest snapshot** recorded from the previous run. The output is a migration
file like:

```sql
-- +migrate Up
ALTER TABLE users ADD COLUMN age INTEGER;

-- +migrate Down
ALTER TABLE users DROP COLUMN age;
```

If there are no differences you get `no schema changes detected`, and nothing
is written.

### How the diff works

The loader (`migrate.LoadSnapshotFromDir`) writes a throwaway Go program that
imports your schema package and dumps `migrate.BuildSnapshot()` as JSON — a
snapshot of all tables (columns, types, defaults, PK/unique/not-null, foreign
keys, indexes) and enums.

Grain then compares old vs new snapshots and emits changes:

| Change kind         | Example SQL                          |
| ------------------- | ------------------------------------ |
| `create_table`      | `CREATE TABLE ...`                   |
| `drop_table`        | `DROP TABLE ...`                     |
| `rename_table`      | `ALTER TABLE ... RENAME TO ...`      |
| `add_column`        | `ALTER TABLE ... ADD COLUMN ...`     |
| `drop_column`       | `ALTER TABLE ... DROP COLUMN ...`    |
| `rename_column`     | `ALTER TABLE ... RENAME COLUMN ...`  |
| `alter_column_type` | `ALTER TABLE ... ALTER COLUMN ... TYPE ...` |
| `alter_column_identity` | `ADD/DROP GENERATED ALWAYS AS IDENTITY` |
| `alter_column_default` | `SET/DROP DEFAULT`                |
| `alter_column_nullability` | `SET/DROP NOT NULL`             |
| `alter_column_generated` | `ADD/DROP GENERATED ALWAYS AS (...)` |
| `alter_column_check` | `ADD/DROP CONSTRAINT ... CHECK (...)` |
| `add_column_unique` / `drop_column_unique` | add/remove `UNIQUE` on a column |
| `add_primary_key` / `drop_primary_key` | add/remove the table primary key |
| `add_foreign_key` / `drop_foreign_key` | add/remove a foreign key |
| `add_unique_constraint` / `drop_unique_constraint` | table-level `UNIQUE` |
| `add_constraint` / `drop_constraint` | table-level `CHECK` |
| `create_enum`       | `CREATE TYPE ... AS ENUM (...)`      |
| `add_enum_value`    | `ALTER TYPE ... ADD VALUE ...`       |
| `rename_enum`       | `ALTER TYPE ... RENAME TO ...`       |
| `drop_enum`         | `DROP TYPE ...`                      |
| `add_index`         | `CREATE INDEX ...`                   |
| `drop_index`        | `DROP INDEX IF EXISTS ...`           |

The diff understands column attribute changes (type, collation via
`COLLATE`), identity, generated expressions, check constraints, nullability,
uniqueness and defaults, and emits the matching `ALTER TABLE` statements.
Table-level unique/check constraints and partial/unique indexes are diffed the
same way.

New tables are created in foreign-key dependency order; a cycle among new
tables is reported so you can split the migration.

The `down` migrations are generated to reverse every `up`, except for drops
(table/column/type) where old definitions can't be reconstructed — those get a
`-- cannot auto-generate` comment and are flagged **irreversible** by the apply
step.

### Ambiguity prompts

When the old snapshot has a table/column that no longer exists in your code,
Grain asks what happened:

```
Table "users" is in the last migration but not in your schema code.
  [1] renamed to "accounts"
  [d] deleted — generate a DROP for it
  [i] ignore — leave it in the database untouched for now
> 
```

- A **rename** generates `ALTER TABLE/RENAME` (and then diffs columns).
- A **delete** generates a `DROP`.
- **ignore** leaves the database alone but stops tracking it in the snapshot.

Rename prompts apply to **tables, columns, and enums**. `migrate.PromptFunc`
is pluggable: `migrate.CLIPrompt` is the interactive default, and
`migrate.AutoResolvePrompt` (used by `generate -yes`) renames to the only
candidate or ignores when ambiguous, for CI.

> When a rename target is chosen, Grain does **not** also `CREATE`/`ADD` a fresh
> object with that name — the rename and the rest of the diff compose into one
> correct migration.

### Destructive changes

Some generated changes can lose data. Grain marks these and **refuses to
generate** them, returning a `*ManualReviewRequiredError` listing what needs
manual review instead:

- `alter_column_type` — casting existing data may fail or be lossy; add a
  `USING` clause yourself.
- `alter_column_nullability` (`SET NOT NULL`) — fails on existing NULL rows;
  backfill first.
- `alter_column_generated` — expression changes on existing rows.
- Adding a `NOT NULL` column with **no default** to a table that may have rows.

Write these by hand (or backfill data first), then re-run `generate`, or pass
`-force` once you're sure (`grain generate -schema ... -force ...`).

### Enum values

`ALTER TYPE ... ADD VALUE` cannot always run inside a transaction, so on enum
addition Grain emits it in a special section that the apply step runs outside
the migration transaction:

```sql
-- +RequiresNoTx Up
ALTER TYPE user_status ADD VALUE IF NOT EXISTS 'pending';

-- +migrate Up
...

-- +migrate Down
...

-- +RequiresNoTx Down
...
```

Removing enum values still has no generator path (PostgreSQL has no
`DROP VALUE`) — when values disappear the snapshot simply stops tracking them.
Use `generate -yes` or the rename prompt to handle enum renames; a removed
enum can be dropped via the same delete prompt.

## Snapshot journal

Every successful `generate` records the new snapshot under
`db/migrations/meta/`:

- `meta/<version>_snapshot.json` — one per migration, the full schema state.
- `meta/_journal.json` — the ordered list of `{version, name}` entries; the
  **last** entry is what the next `generate` diffs against.

Snapshot files are for diffing, not for rewriting your database — the source of
truth is `db/migrations/*.sql`.

Snapshots are self-describing: each file carries a `format_version` (currently
`2`). `LoadLatestSnapshot` refuses to diff snapshots from an incompatible
version, so a schema-format change fails loudly instead of silently generating
garbage migrations.

## migrate

All migrate commands require `DATABASE_URL` and connect directly to PostgreSQL.
The CLI verifies the connection (`db.Connect`) before touching anything.

### init

```bash
DATABASE_URL=... grain migrate init
```

Creates `db/migrations/` plus the `grain` schema and
`grain.schema_migrations` tracking table — without applying anything. Idempotent.

### up

```bash
DATABASE_URL=... grain migrate up
```

- Creates the `grain` schema and `grain.schema_migrations` tracking table
  (idempotent).
- Recomputes a **SHA-256 checksum** of each migration file and verifies it
  against what's recorded; an edited, already-applied migration aborts with a
  checksum mismatch.
- Runs each not-yet-applied migration, **in a transaction**: the `-- +migrate
  Up` SQL plus the version/name/checksum row commit together; the
  `-- +RequiresNoTx Up` section runs outside the transaction first.
- A migration lock (`pg_advisory_lock`) serializes concurrent runs.

Migration files may omit the directives entirely — a `.sql` file with no
`-- +migrate` markers is treated as Up-only.

The tracking table:

```
grain.schema_migrations (
  version    BIGINT PRIMARY KEY,
  name       TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  checksum   TEXT NOT NULL DEFAULT ''
)
```

### down

```bash
DATABASE_URL=... grain migrate down       # roll back the latest
DATABASE_URL=... grain migrate down 3     # roll back the latest 3
```

Rolls back applied migrations, newest first:

1. Verifies checksums (so you never roll back an edited migration).
2. Runs `-- +RequiresNoTx Down` outside a transaction.
3. Runs `-- +migrate Down` inside a transaction, then deletes the tracking row.

If the count is larger than the number of applied migrations, all of them are
rolled back. Migrations whose `down` section is only comments (e.g. a
generated table/column drop) are reported as irreversible and the run stops.

### redo

```bash
DATABASE_URL=... grain migrate redo
```

Rolls back the latest applied migration and applies it again (a full
`down 1 && up`). Handy while iterating on a migration you just ran.

### status

```bash
DATABASE_URL=... grain migrate status
```

Prints every migration file with its state:

```
applied 20260917112541_init
pending 20260917113909_create_product_table
applied 20260917115012_add_user.age (checksum mismatch)
```

Files are matched by their `^(\d+)_.*\.sql$` filename pattern.

## Using migrate as a library

Everything under the hood is exported from `github.com/etornam45/grain/migrate`:

```go
migrate.LoadMigrations(dir)                 // []MigrationFile{Version, Name, UpSQL, DownSQL, NoTxUpSQL, NoTxDownSQL, Path, Checksum}
migrate.Init(ctx, dbConn, dir)              // create dir + tracking table
migrate.Apply(ctx, dbConn, dir)             // apply pending migrations (*sql.DB)
migrate.RollbackN(ctx, dbConn, dir, n)      // roll back the latest n applied
migrate.RollbackLast(ctx, dbConn, dir)      // roll back the latest (RollbackN(1))
migrate.Status(ctx, dbConn, dir)            // []MigrationStatus{Applied, ChecksumVerified, Orphaned, ...}
migrate.GenerateFromSnapshot(snap, dir, name, prompt)                          // write a migration file
migrate.GenerateFromSnapshotOpts(snap, dir, name, prompt, opts)                // write, honoring GenerateOptions{Force}
migrate.BuildSnapshot()                     // Snapshot of schema.Registry + EnumRegistry
migrate.LoadLatestSnapshot(dir)             // Snapshot from the last journal entry
migrate.Diff(old, new snap, prompt)         // []Change from two snapshots
migrate.CLIPrompt / migrate.AutoResolvePrompt // built-in PromptFuncs (interactive / CI)
```

See [migrations in the repo](../migrate) for the full source.