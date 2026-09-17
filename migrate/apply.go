package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type MigrationFile struct {
	Version int64
	Name    string
	UpSQL   string
	DownSQL string
	Path    string
}

var filenamePattern = regexp.MustCompile(`^(\d+)_(.+)\.sql$`)

func LoadMigrations(dir string) ([]MigrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []MigrationFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := filenamePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		version, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		up, down := splitMigration(string(raw))
		files = append(files, MigrationFile{
			Version: version, Name: m[2], UpSQL: up, DownSQL: down,
			Path: filepath.Join(dir, e.Name()),
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	return files, nil
}

func splitMigration(content string) (up, down string) {
	const upMarker = "-- +migrate Up"
	const downMarker = "-- +migrate Down"
	upIdx := strings.Index(content, upMarker)
	downIdx := strings.Index(content, downMarker)
	if upIdx == -1 || downIdx == -1 {
		return content, ""
	}
	up = strings.TrimSpace(content[upIdx+len(upMarker) : downIdx])
	down = strings.TrimSpace(content[downIdx+len(downMarker):])
	return up, down
}

func ensureMigrationsTable(ctx context.Context, dbConn *sql.DB) error {
	_, err := dbConn.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS grain;
		CREATE TABLE IF NOT EXISTS grain.schema_migrations (
			version    BIGINT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

func appliedVersions(ctx context.Context, dbConn *sql.DB) (map[int64]bool, error) {
	rows, err := dbConn.QueryContext(ctx, `SELECT version FROM grain.schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[int64]bool{}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func Apply(ctx context.Context, dbConn *sql.DB, dir string) error {
	if err := ensureMigrationsTable(ctx, dbConn); err != nil {
		return err
	}
	files, err := LoadMigrations(dir)
	if err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, dbConn)
	if err != nil {
		return err
	}

	for _, f := range files {
		if applied[f.Version] {
			continue
		}
		if err := runInTx(ctx, dbConn, f.UpSQL, f.Version, f.Name); err != nil {
			return fmt.Errorf("migration %d_%s failed: %w", f.Version, f.Name, err)
		}
		fmt.Printf("applied %d_%s\n", f.Version, f.Name)
	}
	return nil
}

func runInTx(ctx context.Context, dbConn *sql.DB, sqlText string, version int64, name string) error {
	tx, err := dbConn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO grain.schema_migrations (version, name) VALUES ($1, $2)`, version, name,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func RollbackLast(ctx context.Context, dbConn *sql.DB, dir string) error {
	files, err := LoadMigrations(dir)
	if err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, dbConn)
	if err != nil {
		return err
	}

	var last *MigrationFile
	for i := len(files) - 1; i >= 0; i-- {
		if applied[files[i].Version] {
			last = &files[i]
			break
		}
	}
	if last == nil {
		return fmt.Errorf("no applied migrations to roll back")
	}

	tx, err := dbConn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, last.DownSQL); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM grain.schema_migrations WHERE version = $1`, last.Version,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
