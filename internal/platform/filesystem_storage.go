package platform

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

var storageKeyPattern = regexp.MustCompile(`^objects/[0-9a-f]{32}\.(pdf|jpg|jpeg|png)$`)
var intakeTemporaryPattern = regexp.MustCompile(`^invoiceflow-upload-[A-Za-z0-9]+$`)

// FileStorage stores server-selected keys beneath a private root. It never
// joins client-provided path material and uses no-overwrite hard-link promotion
// on one filesystem.
type FileStorage struct{ root string }

func NewFileStorage(root string) (*FileStorage, error) {
	if root == "" {
		return nil, fmt.Errorf("storage root must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return nil, fmt.Errorf("create storage parent: %w", err)
	}
	if info, err := os.Lstat(root); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		return nil, fmt.Errorf("storage root is not a directory")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect storage root: %w", err)
	}
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("storage root is not a directory")
	}
	objects := filepath.Join(root, "objects")
	if err := ensurePrivateDirectory(objects); err != nil {
		return nil, fmt.Errorf("storage objects directory is unsafe: %w", err)
	}
	if err := ensurePrivateDirectory(filepath.Join(root, "tmp")); err != nil {
		return nil, fmt.Errorf("storage temporary directory is unsafe: %w", err)
	}
	return &FileStorage{root: root}, nil
}

func ensurePrivateDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		return fmt.Errorf("not a directory")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("not a directory")
	}
	return nil
}

// TemporaryDirectory is the private location for unpromoted request bytes.
func (s *FileStorage) TemporaryDirectory() string { return filepath.Join(s.root, "tmp") }

func (s *FileStorage) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !storageKeyPattern.MatchString(key) || size < 0 {
		return fmt.Errorf("invalid storage key")
	}
	target := filepath.Join(s.root, filepath.FromSlash(key))
	dir := filepath.Dir(target)
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe storage directory")
	}
	tmp, err := os.CreateTemp(dir, ".promote-*")
	if err != nil {
		return fmt.Errorf("create object temporary: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	written, err := io.Copy(tmp, io.LimitReader(r, size+1))
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write object: %w", err)
	}
	if written != size {
		_ = tmp.Close()
		return fmt.Errorf("object size changed during promotion")
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmpName, target); err != nil {
		return fmt.Errorf("promote object: %w", err)
	}
	if err := os.Remove(tmpName); err != nil {
		return fmt.Errorf("remove promoted temporary: %w", err)
	}
	return nil
}
func (s *FileStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !storageKeyPattern.MatchString(key) {
		return nil, fmt.Errorf("invalid storage key")
	}
	path := filepath.Join(s.root, filepath.FromSlash(key))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("unsafe storage object")
	}
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}
func (s *FileStorage) Delete(ctx context.Context, key string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !storageKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid storage key")
	}
	err := os.Remove(filepath.Join(s.root, filepath.FromSlash(key)))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ListObjectsOlderThan returns only server-owned, regular object files. It is
// used by the worker's conservative orphan reconciliation, never for request
// paths or client-controlled names.
func (s *FileStorage) ListObjectsOlderThan(ctx context.Context, cutoff time.Time) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.root, "objects")
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("unsafe storage directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := "objects/" + entry.Name()
		if !storageKeyPattern.MatchString(key) {
			continue
		}
		entryInfo, err := os.Lstat(filepath.Join(dir, entry.Name()))
		if err != nil || !entryInfo.Mode().IsRegular() || entryInfo.ModTime().After(cutoff) {
			continue
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// CleanupTemporaryFilesOlderThan removes only aged, regular intake files from
// the storage-owned temporary directory. A request removes its own temporary
// file normally; this covers crashes before that cleanup can run.
func (s *FileStorage) CleanupTemporaryFilesOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	dir := s.TemporaryDirectory()
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("unsafe storage temporary directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if !intakeTemporaryPattern.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		entryInfo, err := os.Lstat(path)
		if err != nil || !entryInfo.Mode().IsRegular() || entryInfo.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
