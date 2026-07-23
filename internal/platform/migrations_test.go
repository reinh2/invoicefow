package platform

import (
	"path/filepath"
	"testing"
)

func TestValidateMigrationOrderRejectsNewEarlierMigration(t *testing.T) {
	files := []string{
		filepath.Join("db", "migrations", "0001_foundation.sql"),
		filepath.Join("db", "migrations", "0002_audit_truncate.sql"),
	}
	applied := map[string]struct{}{"0002_audit_truncate.sql": {}}
	if err := validateMigrationOrder(files, applied); err == nil {
		t.Fatal("expected new earlier migration to be rejected")
	}
}

func TestValidateMigrationOrderAllowsAppendOnlyMigration(t *testing.T) {
	files := []string{filepath.Join("db", "migrations", "0003_next.sql")}
	applied := map[string]struct{}{"0002_audit_truncate.sql": {}}
	if err := validateMigrationOrder(files, applied); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
