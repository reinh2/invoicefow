package platform

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationLockID int64 = 904211743

// Migrate applies lexically ordered forward-only .sql files. A session advisory
// lock prevents concurrent API and worker startup from racing migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockID) }()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name text PRIMARY KEY,
		checksum bytea NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	applied, err := appliedMigrations(ctx, conn.Conn())
	if err != nil {
		return err
	}
	if err := validateMigrationOrder(files, applied); err != nil {
		return err
	}
	for _, file := range files {
		if err := applyMigration(ctx, conn.Conn(), file); err != nil {
			return err
		}
	}
	return nil
}

func appliedMigrations(ctx context.Context, conn *pgx.Conn) (map[string]struct{}, error) {
	rows, err := conn.Query(ctx, "SELECT name FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read migration ledger: %w", err)
	}
	defer rows.Close()
	applied := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan migration ledger: %w", err)
		}
		applied[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration ledger: %w", err)
	}
	return applied, nil
}

// validateMigrationOrder keeps the migration history append-only. Applying a
// previously unseen migration before an already recorded one would make fresh
// databases diverge from existing environments.
func validateMigrationOrder(files []string, applied map[string]struct{}) error {
	latestApplied := ""
	for name := range applied {
		if name > latestApplied {
			latestApplied = name
		}
	}
	for _, file := range files {
		name := filepath.Base(file)
		if _, exists := applied[name]; !exists && latestApplied != "" && name < latestApplied {
			return fmt.Errorf("migration %q is earlier than already applied migration %q", name, latestApplied)
		}
	}
	return nil
}

func migrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

func applyMigration(ctx context.Context, conn *pgx.Conn, file string) error {
	sql, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read migration %q: %w", file, err)
	}
	name := filepath.Base(file)
	checksum := sha256.Sum256(sql)
	var appliedChecksum []byte
	err = conn.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE name = $1", name).Scan(&appliedChecksum)
	if err == nil {
		if string(appliedChecksum) != string(checksum[:]) {
			return fmt.Errorf("migration %q checksum differs from ledger", name)
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read migration ledger for %q: %w", name, err)
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration %q: %w", name, err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("execute migration %q: %w", name, err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (name, checksum, applied_at) VALUES ($1, $2, $3)", name, checksum[:], time.Now().UTC()); err != nil {
		return fmt.Errorf("record migration %q: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %q: %w", name, err)
	}
	return nil
}
