package feedback

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const feedbackObjectDir = "feedback/objects"

type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) *LocalStorage {
	return &LocalStorage{root: root}
}

func (s *LocalStorage) Save(ctx context.Context, extension string, content []byte) (*StoredObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if extension != ".jpg" && extension != ".png" {
		return nil, ErrInvalidImage
	}
	name, err := randomObjectName()
	if err != nil {
		return nil, err
	}
	storagePath := filepath.ToSlash(filepath.Join(feedbackObjectDir, name+extension))
	absolutePath, err := s.resolvePath(storagePath)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(absolutePath)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return nil, fmt.Errorf("create feedback storage: %w", err)
	}
	if err := rejectStorageSymlinks(s.root, parent); err != nil {
		return nil, err
	}
	temp, err := os.CreateTemp(parent, ".upload-*")
	if err != nil {
		return nil, fmt.Errorf("create feedback upload: %w", err)
	}
	tempPath := temp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return nil, err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return nil, err
	}
	if err := temp.Close(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.Rename(tempPath, absolutePath); err != nil {
		return nil, err
	}
	keep = true
	return &StoredObject{StoragePath: storagePath, FileSize: uint64(len(content))}, nil
}

func (s *LocalStorage) Resolve(ctx context.Context, storagePath string) (*ResolvedObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absolutePath, err := s.resolvePath(storagePath)
	if err != nil {
		return nil, err
	}
	if err := rejectStorageSymlinks(s.root, absolutePath); err != nil {
		return nil, err
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrNotFound
	}
	return &ResolvedObject{AbsolutePath: absolutePath}, nil
}

func (s *LocalStorage) Delete(ctx context.Context, storagePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	absolutePath, err := s.resolvePath(storagePath)
	if err != nil {
		return err
	}
	if err := rejectStorageSymlinks(s.root, absolutePath); err != nil {
		return err
	}
	return os.Remove(absolutePath)
}

func (s *LocalStorage) resolvePath(storagePath string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(storagePath)))
	if filepath.ToSlash(filepath.Dir(clean)) != feedbackObjectDir {
		return "", errors.New("invalid_feedback_storage_path")
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	absolutePath, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(clean)))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absolutePath, root+string(os.PathSeparator)) {
		return "", errors.New("invalid_feedback_storage_path")
	}
	return absolutePath, nil
}

func randomObjectName() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func rejectStorageSymlinks(root, target string) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(absoluteRoot, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return errors.New("invalid_feedback_storage_path")
	}
	current := absoluteRoot
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
			return errors.New("feedback_storage_symlink_not_allowed")
		}
	}
	return nil
}
