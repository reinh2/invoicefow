package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileStorageDoesNotOverwriteOrFollowSymlinks(t *testing.T) {
	s, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := "objects/0123456789abcdef0123456789abcdef.pdf"
	if err := s.Put(context.Background(), key, strings.NewReader("one"), 3); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), key, strings.NewReader("two"), 3); err == nil {
		t.Fatal("Put overwrote existing object")
	}
	target := filepath.Join(filepath.Dir(filepath.Join(s.root, key)), "abcdefabcdefabcdefabcdefabcdefab.pdf")
	if err := os.Symlink("/etc/passwd", target); err != nil {
		t.Fatal(err)
	}
	_, err = s.Open(context.Background(), "objects/abcdefabcdefabcdefabcdefabcdefab.pdf")
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("Open symlink: %v", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatal("expected symlink rejection")
	}
}

func TestFileStorageRejectsSymlinkedRoot(t *testing.T) {
	target := t.TempDir()
	root := filepath.Join(t.TempDir(), "storage")
	if err := os.Symlink(target, root); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStorage(root); err == nil {
		t.Fatal("NewFileStorage accepted a symlinked root")
	}
}

func TestFileStorageCleansOnlyAgedPrivateTemporaryFiles(t *testing.T) {
	s, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	aged, err := os.CreateTemp(s.TemporaryDirectory(), "invoiceflow-upload-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := aged.Close(); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(aged.Name(), old, old); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(s.TemporaryDirectory(), "not-an-intake-file")
	if err := os.WriteFile(keep, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	removed, err := s.CleanupTemporaryFilesOlderThan(context.Background(), time.Now().Add(-24*time.Hour))
	if err != nil || removed != 1 {
		t.Fatalf("CleanupTemporaryFilesOlderThan() = %d, %v", removed, err)
	}
	if _, err := os.Stat(aged.Name()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("aged intake file still exists: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unrelated temporary file removed: %v", err)
	}
}
