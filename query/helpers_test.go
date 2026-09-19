package query

import (
	"context"
	"database/sql"

	"github.com/etornam45/grain/schema"
)

type testCols struct {
	ID    *schema.ColumnDef
	Name  *schema.ColumnDef
	Email *schema.ColumnDef
	Age   *schema.ColumnDef
}

func getTestTable() *schema.TableDef[testCols] {
	return schema.Table[testCols]("users",
		schema.Column("id", schema.Int()).PrimaryKey(),
		schema.Column("name", schema.Varchar(255)),
		schema.Column("email", schema.Varchar(255)),
		schema.Column("age", schema.Int()),
	)
}

type dummyResult struct {
	rows int64
}

func (d dummyResult) LastInsertId() (int64, error) { return 0, nil }
func (d dummyResult) RowsAffected() (int64, error) { return d.rows, nil }

type dummyExecutor struct {
	lastQuery string
	lastArgs  []any
}

func (d *dummyExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	d.lastQuery = query
	d.lastArgs = args
	return dummyResult{rows: 1}, nil
}

func (d *dummyExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	d.lastQuery = query
	d.lastArgs = args
	return nil, nil
}

func (d *dummyExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	d.lastQuery = query
	d.lastArgs = args
	return nil
}

