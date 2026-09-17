package db

import (
	"context"
	"database/sql"
)

// Executor is satisfied by both *sql.DB and *sql.Tx, so every query builder
// in the query/ package can run against either without knowing which.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type DB struct {
	*sql.DB
}

// Open wraps database/sql.Open. This package deliberately does not import a
// Postgres driver — import one with a blank import in your own main package
// (e.g. `_ "github.com/jackc/pgx/v5/stdlib"` and driverName "pgx", or
// `_ "github.com/lib/pq"` and driverName "postgres"), then pass its
// registered name here. Keeps grain's core dependency-free.
func Open(driverName, dsn string) (*DB, error) {
	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	return &DB{sqlDB}, nil
}

type Tx struct {
	*sql.Tx
}

func (d *DB) Transaction(ctx context.Context, fn func(tx *Tx) error) error {
	sqlTx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Tx{sqlTx}); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	return sqlTx.Commit()
}
