package notification

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type Binding struct {
	Target
	GroupID uint64 `json:"group_id"`
}

type BindingStore struct {
	path     string
	mu       sync.RWMutex
	bindings map[int64]Binding
}

func NewBindingStore(path string, seed map[uint64]Target) (*BindingStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create notification binding directory: %w", err)
	}
	store := &BindingStore{path: path, bindings: make(map[int64]Binding)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		for groupID, target := range seed {
			if err := validateBinding(target, groupID); err != nil {
				return nil, err
			}
			if _, exists := store.bindings[target.ChatID]; exists {
				return nil, errors.New("notification chat is assigned more than once")
			}
			store.bindings[target.ChatID] = Binding{Target: target, GroupID: groupID}
		}
		if err := store.writeLocked(); err != nil {
			return nil, err
		}
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read notification bindings: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("secure notification bindings: %w", err)
	}
	var document struct {
		Bindings []Binding `json:"bindings"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode notification bindings: %w", err)
	}
	for _, binding := range document.Bindings {
		if err := validateBinding(binding.Target, binding.GroupID); err != nil {
			return nil, err
		}
		if _, exists := store.bindings[binding.ChatID]; exists {
			return nil, errors.New("notification chat is assigned more than once")
		}
		store.bindings[binding.ChatID] = binding
	}
	return store, nil
}

func (s *BindingStore) Bindings() []Binding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedBindings(s.bindings)
}

func (s *BindingStore) Assign(target Target, groupID uint64) error {
	if groupID > 0 {
		if err := validateBinding(target, groupID); err != nil {
			return err
		}
	} else if target.ChatID <= 0 {
		return errors.New("notification chat ID must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[int64]Binding, len(s.bindings)+1)
	for chatID, binding := range s.bindings {
		next[chatID] = binding
	}
	if groupID == 0 {
		delete(next, target.ChatID)
	} else {
		next[target.ChatID] = Binding{Target: target, GroupID: groupID}
	}
	previous := s.bindings
	s.bindings = next
	if err := s.writeLocked(); err != nil {
		s.bindings = previous
		return err
	}
	return nil
}

func (s *BindingStore) Targets() map[uint64][]Target {
	s.mu.RLock()
	defer s.mu.RUnlock()
	targets := make(map[uint64][]Target)
	for _, binding := range s.bindings {
		targets[binding.GroupID] = append(targets[binding.GroupID], binding.Target)
	}
	for groupID := range targets {
		sort.Slice(targets[groupID], func(i, j int) bool {
			return targets[groupID][i].ChatID < targets[groupID][j].ChatID
		})
	}
	return targets
}

func (s *BindingStore) writeLocked() error {
	document := struct {
		Bindings []Binding `json:"bindings"`
	}{Bindings: sortedBindings(s.bindings)}
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode notification bindings: %w", err)
	}
	if err := writeAtomic(s.path, data); err != nil {
		return fmt.Errorf("write notification bindings: %w", err)
	}
	return nil
}

func sortedBindings(bindings map[int64]Binding) []Binding {
	items := make([]Binding, 0, len(bindings))
	for _, binding := range bindings {
		items = append(items, binding)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ChatID < items[j].ChatID
	})
	return items
}

func validateBinding(target Target, groupID uint64) error {
	if groupID == 0 || target.ChatID <= 0 || (target.ChatType != 2 && target.ChatType != 3) {
		return errors.New("invalid notification binding")
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".binding-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
