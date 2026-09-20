package db

import (
	"context"
	"database/sql"
)

type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type DB struct {
	*sql.DB
}

func Open(driverName, dsn string) (*DB, error) {
	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	return &DB{sqlDB}, nil
}

// Connect opens a database and verifies it is reachable with a ping. It never
// returns a half-open DB: on failure sqlDB is closed before returning the error.
func Connect(ctx context.Context, driverName, dsn string) (*DB, error) {
	conn, err := Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

type Tx struct {
	*sql.Tx
}

// TransactionOpts mirrors database/sql TxOptions. A nil *TransactionOpts means
// the database driver's default isolation level and read-write access.
type TransactionOpts struct {
	Isolation sql.IsolationLevel
	ReadOnly  bool
}

func (d *DB) Transaction(ctx context.Context, fn func(tx *Tx) error) error {
	return d.TransactionOpts(ctx, nil, fn)
}

// TransactionOpts runs fn inside a transaction configured per opts.
func (d *DB) TransactionOpts(ctx context.Context, opts *TransactionOpts, fn func(tx *Tx) error) error {
	var sqlOpts *sql.TxOptions
	if opts != nil {
		sqlOpts = &sql.TxOptions{Isolation: opts.Isolation, ReadOnly: opts.ReadOnly}
	}
	sqlTx, err := d.DB.BeginTx(ctx, sqlOpts)
	if err != nil {
		return err
	}
	if err := fn(&Tx{sqlTx}); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	return sqlTx.Commit()
}