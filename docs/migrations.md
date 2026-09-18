# Migrations & the grain CLI

Grain ships a CLI (`cmd/grain`) that generates and applies migrations from your
Go schema definitions. Instead of hand-writing SQL for every change, you edit
your `schema` package and Grain diffs it against the last applied snapshot.

```bash
> grain --help
usage:
  grain generate -schema <dir> [name]
  grain migrate up                       (requires DATABASE_URL)
  grain migrate down                     (requires DATABASE_URL)
  grain migrate status                   (requires DATABASE_URL)
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
| `create_enum`       | `CREATE TYPE ... AS ENUM (...)`      |
| `add_enum_value`    | `ALTER TYPE ... ADD VALUE ...`       |
| `add_index`         | `CREATE INDEX ...`                   |
| `drop_index`        | `DROP INDEX IF EXISTS ...`           |

New tables are created in foreign-key dependency order; a cycle among new
tables is reported so you can split the migration.

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

The prompt is pluggable: `migrate.PromptFunc` can be replaced with an
automated resolver for CI (`migrate.CLIPrompt` is the default interactive one).

### Destructive changes

Some generated changes can lose data. Grain marks these and **refuses to
generate** them, returning a `*ManualReviewRequiredError` listing what needs
manual review instead:

- `alter_column_type` — casting existing data may fail or be lossy; add a
  `USING` clause yourself.
- Adding a `NOT NULL` column with **no default** to a table that may have rows.
- Dropping a table or column (Grain can't reconstruct the dropped definition,
  so the `down` migration is a `-- cannot auto-generate` comment).

Write these by hand (or backfill data first), then re-run `generate`.

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

Removing enum values and renaming enums are not yet wired up (PostgreSQL has no
`DROP VALUE`).

## Snapshot journal

Every successful `generate` records the new snapshot under
`db/migrations/meta/`:

- `meta/<version>_snapshot.json` — one per migration, the full schema state.
- `meta/_journal.json` — the ordered list of `{version, name}` entries; the
  **last** entry is what the next `generate` diffs against.

Snapshot files are for diffing, not for rewriting your database — the source of
truth is `db/migrations/*.sql`.

## migrate

All migrate commands require `DATABASE_URL` and connect directly to PostgreSQL.

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
DATABASE_URL=... grain migrate down
```

Rolls back the **most recent applied** migration only:

1. Verifies checksums (so you never roll back an edited migration).
2. Runs `-- +RequiresNoTx Down` outside a transaction.
3. Runs `-- +migrate Down` inside a transaction, then deletes the tracking row.

Migrations whose `down` section is only comments (e.g. a generated table/column
drop) are reported as irreversible and left untouched.

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

Everything under the hood is exported from `grain/migrate`:

```go
migrate.LoadMigrations(dir)                 // []MigrationFile{Version, Name, UpSQL, DownSQL, NoTxUpSQL, NoTxDownSQL, Path, Checksum}
migrate.Apply(ctx, dbConn, dir)             // apply pending migrations (*sql.DB)
migrate.RollbackLast(ctx, dbConn, dir)      // roll back the last applied one
migrate.Status(ctx, dbConn, dir)            // []MigrationStatus{Applied, ChecksumVerified, ...}
migrate.GenerateFromSnapshot(snap, dir, name, prompt) // write a migration file
migrate.BuildSnapshot()                     // Snapshot of schema.Registry + EnumRegistry
migrate.Diff(old, new snap, prompt)         // []Change from two snapshots
```

See [migrations in the repo](../migrate) for the full source.