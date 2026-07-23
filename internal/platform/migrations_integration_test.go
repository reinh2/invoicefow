//go:build integration

package platform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrateCreatesFoundationSchema(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required for PostgreSQL integration tests; start Compose PostgreSQL and set DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	dir := filepath.Join("..", "..", "db", "migrations")
	if err := Migrate(ctx, pool, dir); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no migration ledger entries")
	}
	_, err = pool.Exec(ctx, "TRUNCATE audit_events")
	if err == nil || !strings.Contains(err.Error(), "audit events are append-only") {
		t.Fatalf("TRUNCATE audit_events error = %v, want append-only rejection", err)
	}

	const objectID = "11111111-1111-1111-1111-111111111111"
	const documentID = "22222222-2222-2222-2222-222222222222"
	const versionID = "33333333-3333-3333-3333-333333333333"
	if _, err := pool.Exec(ctx, `INSERT INTO stored_objects (id, storage_key, sha256, size_bytes, media_type)
		VALUES ($1, 'test/object-identity', decode(repeat('01', 32), 'hex'), 1, 'application/pdf')
		ON CONFLICT (id) DO NOTHING`, objectID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO documents (id, object_id, sha256, status)
		VALUES ($1, $2, decode(repeat('02', 32), 'hex'), 'uploaded')`, documentID, objectID)
	if err == nil || !strings.Contains(err.Error(), "document hash must match") {
		t.Fatalf("mismatched document hash error = %v, want hash match rejection", err)
	}
	_, err = pool.Exec(ctx, "UPDATE stored_objects SET storage_key = 'test/retargeted' WHERE id = $1", objectID)
	if err == nil || !strings.Contains(err.Error(), "stored object identity is immutable") {
		t.Fatalf("stored object update error = %v, want immutability rejection", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO documents (id, object_id, sha256, status)
		VALUES ($1, $2, decode(repeat('01', 32), 'hex'), 'uploaded')
		ON CONFLICT (id) DO NOTHING`, documentID, objectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO invoice_versions (id, document_id, version_number, currency, total_minor, source)
		VALUES ($1, $2, 1, 'USD', 100, 'extraction')
		ON CONFLICT (id) DO NOTHING`, versionID, documentID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, "UPDATE invoice_versions SET total_minor = 200 WHERE id = $1", versionID)
	if err == nil || !strings.Contains(err.Error(), "invoice versions are append-only") {
		t.Fatalf("invoice version update error = %v, want append-only rejection", err)
	}
	_, err = pool.Exec(ctx, "DELETE FROM invoice_versions WHERE id = $1", versionID)
	if err == nil || !strings.Contains(err.Error(), "invoice versions are append-only") {
		t.Fatalf("invoice version delete error = %v, want append-only rejection", err)
	}
}
