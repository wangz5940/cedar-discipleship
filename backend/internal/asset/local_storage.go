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
	"time"
)

type LocalStorage struct {
	root          string
	fallbackRoots []string
}

func NewLocalStorage(root string, fallbackRoots ...string) *LocalStorage {
	return &LocalStorage{root: root, fallbackRoots: fallbackRoots}
}

func (s *LocalStorage) Save(ctx context.Context, relativeDir, fileName string, src io.Reader) (*StoredObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relativePath := filepath.Join(relativeDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), fileName))
	absolutePath, _, err := ResolveFileInRoot(s.root, relativePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageWrite, err)
	}
	relativePath = filepath.ToSlash(relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageDirectory, err)
	}
	dst, err := os.Create(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStorageWrite, err)
	}
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
	roots := append([]string{s.root}, s.fallbackRoots...)
	absolutePath, original, err := ResolveExistingFileInRoots(storagePath, roots...)
	if err != nil {
		return nil, err
	}
	return &ResolvedObject{AbsolutePath: absolutePath, OriginalName: original}, nil
}

func (s *LocalStorage) Delete(ctx context.Context, objectKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	absolutePath, _, err := ResolveFileInRoot(s.root, objectKey)
	if err != nil {
		return err
	}
	return os.Remove(absolutePath)
}

func ResolveExistingFileInRoots(path string, roots ...string) (string, string, error) {
	var lastErr error
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		abs, original, err := ResolveFileInRoot(root, path)
		if err != nil {
			lastErr = err
			continue
		}
		info, statErr := os.Stat(abs)
		if statErr == nil && !info.IsDir() {
			return abs, original, nil
		}
		if statErr != nil {
			lastErr = statErr
		} else {
			lastErr = errors.New("path_is_directory")
		}
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return "", "", lastErr
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
