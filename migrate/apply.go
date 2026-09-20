package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type MigrationFile struct {
	Version     int64
	Name        string
	UpSQL       string
	DownSQL     string
	NoTxUpSQL   string
	NoTxDownSQL string
	Path        string
	Checksum    string
}

type MigrationStatus struct {
	MigrationFile
	Applied          bool
	ChecksumVerified bool
	Orphaned         bool
}

type migrationExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

const migrationLockID int64 = 675212221069

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
		digest := sha256.Sum256(raw)
		up, down, noTxUp, noTxDown := splitMigration(string(raw))
		files = append(files, MigrationFile{
			Version: version, Name: m[2], UpSQL: up, DownSQL: down,
			NoTxUpSQL: noTxUp, NoTxDownSQL: noTxDown,
			Path:     filepath.Join(dir, e.Name()),
			Checksum: hex.EncodeToString(digest[:]),
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	return files, nil
}

func splitMigration(content string) (up, down, noTxUp, noTxDown string) {
	sections := map[string]*strings.Builder{
		"up": {}, "down": {},
		"no-tx-up": {}, "no-tx-down": {},
	}
	current := ""
	sawDirective := false
	for _, line := range strings.SplitAfter(content, "\n") {
		if section, ok := migrationSection(line); ok {
			current = section
			sawDirective = true
			continue
		}
		if current != "" {
			sections[current].WriteString(line)
		}
	}
	if !sawDirective {
		return strings.TrimSpace(content), "", "", ""
	}
	return strings.TrimSpace(sections["up"].String()),
		strings.TrimSpace(sections["down"].String()),
		strings.TrimSpace(sections["no-tx-up"].String()),
		strings.TrimSpace(sections["no-tx-down"].String())
}

func migrationSection(line string) (string, bool) {
	normalized := strings.ToLower(strings.Join(strings.Fields(line), ""))
	switch normalized {
	case "--+migrateup":
		return "up", true
	case "--+migratedown":
		return "down", true
	case "--+requiresnotxup":
		return "no-tx-up", true
	case "--+requiresnotxdown":
		return "no-tx-down", true
	default:
		return "", false
	}
}

func ensureMigrationsTable(ctx context.Context, dbConn migrationExecutor) error {
	_, err := dbConn.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS grain;
		CREATE TABLE IF NOT EXISTS grain.schema_migrations (
			version    BIGINT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			checksum   TEXT NOT NULL DEFAULT ''
		);
		ALTER TABLE grain.schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT NOT NULL DEFAULT ''`)
	return err
}

func appliedChecksums(ctx context.Context, dbConn migrationExecutor) (map[int64]string, error) {
	rows, err := dbConn.QueryContext(ctx, `SELECT version, checksum FROM grain.schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[int64]string{}
	for rows.Next() {
		var v int64
		var checksum string
		if err := rows.Scan(&v, &checksum); err != nil {
			return nil, err
		}
		applied[v] = checksum
	}
	return applied, rows.Err()
}

// Init ensures the migrations directory and the grain.schema_migrations
// tracking table exist, without applying anything.
func Init(ctx context.Context, dbConn *sql.DB, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return withMigrationLock(ctx, dbConn, func(conn *sql.Conn) error {
		return ensureMigrationsTable(ctx, conn)
	})
}

func Apply(ctx context.Context, dbConn *sql.DB, dir string) error {
	files, err := LoadMigrations(dir)
	if err != nil {
		return err
	}
	return withMigrationLock(ctx, dbConn, func(conn *sql.Conn) error {
		if err := ensureMigrationsTable(ctx, conn); err != nil {
			return err
		}
		applied, err := appliedChecksums(ctx, conn)
		if err != nil {
			return err
		}
		if err := verifyAndBackfillChecksums(ctx, conn, files, applied); err != nil {
			return err
		}
		if orphans := orphanedApplied(applied, files); len(orphans) > 0 {
			return fmt.Errorf("applied migration(s) %s are not on disk anymore; restore them before continuing", intsToString(orphans))
		}
		for _, f := range files {
			if _, ok := applied[f.Version]; ok {
				continue
			}
			// PostgreSQL requires some statements (notably ALTER TYPE ... ADD VALUE)
			// to be committed before the transactional migration can use their result.
			if err := runOutsideTx(ctx, conn, f.NoTxUpSQL); err != nil {
				return fmt.Errorf("migration %d_%s no-transaction up failed: %w", f.Version, f.Name, err)
			}
			if err := runInTx(ctx, conn, f.UpSQL, f.Version, f.Name, f.Checksum); err != nil {
				return fmt.Errorf("migration %d_%s failed: %w", f.Version, f.Name, err)
			}
			fmt.Printf("applied %d_%s\n", f.Version, f.Name)
		}
		return nil
	})
}

func Status(ctx context.Context, dbConn *sql.DB, dir string) ([]MigrationStatus, error) {
	files, err := LoadMigrations(dir)
	if err != nil {
		return nil, err
	}
	var statuses []MigrationStatus
	err = withMigrationLock(ctx, dbConn, func(conn *sql.Conn) error {
		if err := ensureMigrationsTable(ctx, conn); err != nil {
			return err
		}
		applied, err := appliedChecksums(ctx, conn)
		if err != nil {
			return err
		}
		for _, file := range files {
			recorded, isApplied := applied[file.Version]
			statuses = append(statuses, MigrationStatus{
				MigrationFile:    file,
				Applied:          isApplied,
				ChecksumVerified: !isApplied || recorded == file.Checksum,
			})
		}
		for _, v := range orphanedApplied(applied, files) {
			statuses = append(statuses, MigrationStatus{
				MigrationFile: MigrationFile{Version: v},
				Applied:       true,
				Orphaned:      true,
			})
		}
		return nil
	})
	return statuses, err
}

// orphanedApplied returns applied migration versions that have no matching
// file on disk (deleted or renamed since they were applied).
func orphanedApplied(applied map[int64]string, files []MigrationFile) []int64 {
	onDisk := map[int64]bool{}
	for _, f := range files {
		onDisk[f.Version] = true
	}
	var orphans []int64
	for v := range applied {
		if !onDisk[v] {
			orphans = append(orphans, v)
		}
	}
	sort.Slice(orphans, func(i, j int) bool { return orphans[i] < orphans[j] })
	return orphans
}

func intsToString(vals []int64) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.FormatInt(v, 10)
	}
	return strings.Join(parts, ", ")
}

func runOutsideTx(ctx context.Context, dbConn migrationExecutor, sqlText string) error {
	if !hasExecutableSQL(sqlText) {
		return nil
	}
	_, err := dbConn.ExecContext(ctx, sqlText)
	return err
}

func runInTx(ctx context.Context, dbConn migrationExecutor, sqlText string, version int64, name, checksum string) error {
	tx, err := dbConn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if hasExecutableSQL(sqlText) {
		if _, err := tx.ExecContext(ctx, sqlText); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO grain.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`, version, name, checksum,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// RollbackLast rolls back the single most recent applied migration.
func RollbackLast(ctx context.Context, dbConn *sql.DB, dir string) error {
	return RollbackN(ctx, dbConn, dir, 1)
}

// RollbackN rolls back the n most recent applied migrations (or all of them if
// fewer than n are applied).
func RollbackN(ctx context.Context, dbConn *sql.DB, dir string, n int) error {
	if n <= 0 {
		return fmt.Errorf("rollback count must be positive, got %d", n)
	}
	files, err := LoadMigrations(dir)
	if err != nil {
		return err
	}
	return withMigrationLock(ctx, dbConn, func(conn *sql.Conn) error {
		if err := ensureMigrationsTable(ctx, conn); err != nil {
			return err
		}
		applied, err := appliedChecksums(ctx, conn)
		if err != nil {
			return err
		}
		if orphans := orphanedApplied(applied, files); len(orphans) > 0 {
			return fmt.Errorf("applied migration(s) %s are not on disk anymore; restore them before rolling back", intsToString(orphans))
		}
		if err := verifyAndBackfillChecksums(ctx, conn, files, applied); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			handled, err := rollbackOne(ctx, conn, files, applied)
			if err != nil {
				return err
			}
			if !handled {
				return nil
			}
		}
		return nil
	})
}

// rollbackOne down-migrates the last applied migration in files and removes its
// tracking row. It returns false when there is nothing left to roll back.
func rollbackOne(ctx context.Context, conn *sql.Conn, files []MigrationFile, applied map[int64]string) (bool, error) {
	var last *MigrationFile
	for i := len(files) - 1; i >= 0; i-- {
		if _, ok := applied[files[i].Version]; ok {
			last = &files[i]
			break
		}
	}
	if last == nil {
		return false, nil
	}
	if !hasExecutableSQL(last.DownSQL) && !hasExecutableSQL(last.NoTxDownSQL) {
		return true, fmt.Errorf("migration %d_%s is irreversible: no executable down migration", last.Version, last.Name)
	}
	if err := runOutsideTx(ctx, conn, last.NoTxDownSQL); err != nil {
		return true, fmt.Errorf("migration %d_%s no-transaction down failed: %w", last.Version, last.Name, err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return true, err
	}
	if hasExecutableSQL(last.DownSQL) {
		if _, err := tx.ExecContext(ctx, last.DownSQL); err != nil {
			_ = tx.Rollback()
			return true, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM grain.schema_migrations WHERE version = $1`, last.Version); err != nil {
		_ = tx.Rollback()
		return true, err
	}
	if err := tx.Commit(); err != nil {
		return true, err
	}
	delete(applied, last.Version)
	fmt.Printf("rolled back %d_%s\n", last.Version, last.Name)
	return true, nil
}

func withMigrationLock(ctx context.Context, dbConn *sql.DB, fn func(*sql.Conn) error) error {
	conn, err := dbConn.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockID)
	return fn(conn)
}

func verifyAndBackfillChecksums(ctx context.Context, dbConn migrationExecutor, files []MigrationFile, applied map[int64]string) error {
	for _, file := range files {
		recorded, ok := applied[file.Version]
		if !ok {
			continue
		}
		if recorded == "" {
			if _, err := dbConn.ExecContext(ctx, `UPDATE grain.schema_migrations SET checksum = $2 WHERE version = $1 AND checksum = ''`, file.Version, file.Checksum); err != nil {
				return err
			}
			continue
		}
		if recorded != file.Checksum {
			return fmt.Errorf("migration %d_%s checksum does not match the applied migration", file.Version, file.Name)
		}
	}
	return nil
}

func hasExecutableSQL(sqlText string) bool {
	for _, line := range strings.Split(sqlText, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "--") {
			return true
		}
	}
	return false
}
