package asset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) *LocalStorage {
	return &LocalStorage{root: root}
}

func (s *LocalStorage) Save(ctx context.Context, relativeDir, fileName string, src io.Reader) (*StoredObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relativePath := filepath.Join(relativeDir, fileName)
	if !isNewResourceStoragePath(relativePath) {
		return nil, fmt.Errorf("%w: invalid resource path", ErrStorageWrite)
	}
	absolutePath, _, err := ResolveFileInRoot(s.root, relativePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageWrite, err)
	}
	relativePath = filepath.ToSlash(relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o750); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageDirectory, err)
	}
	if err := rejectSymlinks(s.root, filepath.Dir(absolutePath)); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageWrite, err)
	}
	dst, err := os.CreateTemp(filepath.Dir(absolutePath), ".upload-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageWrite, err)
	}
	tempPath := dst.Name()
	keepFile := false
	defer func() {
		if !keepFile {
			_ = os.Remove(tempPath)
		}
	}()
	hasher := sha256.New()
	size, copyErr := io.Copy(dst, io.TeeReader(src, hasher))
	closeErr := dst.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageWrite, copyErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageWrite, closeErr)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if size < 0 {
		return nil, fmt.Errorf("%w: invalid file size", ErrStorageWrite)
	}
	if err := os.Rename(tempPath, absolutePath); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageWrite, err)
	}
	keepFile = true
	return &StoredObject{
		StoragePath:    relativePath,
		FileSize:       uint64(size),
		ChecksumSHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func (s *LocalStorage) Resolve(ctx context.Context, storagePath string) (*ResolvedObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !isNewResourceStoragePath(storagePath) {
		return nil, errors.New("invalid_resource_path")
	}
	absolutePath, original, err := ResolveFileInRoot(s.root, storagePath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("path_is_directory")
	}
	if err := rejectSymlinks(s.root, absolutePath); err != nil {
		return nil, err
	}
	return &ResolvedObject{AbsolutePath: absolutePath, OriginalName: original}, nil
}

func (s *LocalStorage) Delete(ctx context.Context, objectKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isNewResourceStoragePath(objectKey) {
		return errors.New("invalid_resource_path")
	}
	absolutePath, _, err := ResolveFileInRoot(s.root, objectKey)
	if err != nil {
		return err
	}
	if err := rejectSymlinks(s.root, absolutePath); err != nil {
		return err
	}
	return os.Remove(absolutePath)
}

func ResolveFileInRoot(root, path string) (string, string, error) {
	if strings.TrimSpace(root) == "" {
		return "", "", errors.New("empty_root")
	}
	decoded, err := url.PathUnescape(strings.TrimSpace(path))
	if err == nil && decoded != "" {
		path = decoded
	}
	clean := filepath.Clean(strings.TrimPrefix(path, "/"))
	full := filepath.Join(root, clean)
	absRoot, _ := filepath.Abs(root)
	abs, _ := filepath.Abs(full)
	if abs != absRoot && !strings.HasPrefix(abs, absRoot+string(os.PathSeparator)) {
		return "", "", errors.New("invalid_path")
	}
	return abs, filepath.Base(clean), nil
}

func rejectSymlinks(root, path string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(absRoot, absPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return errors.New("invalid_path")
	}
	current := absRoot
	for _, part := range strings.Split(relative, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlink_not_allowed")
		}
	}
	return nil
}
