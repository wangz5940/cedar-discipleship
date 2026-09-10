package notification

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBindingStorePersistsUniqueChatAssignments(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bindings.json")
	seed := map[uint64]Target{
		1: {ChatID: 10, ChatType: 2},
		2: {ChatID: 20, ChatType: 3},
	}
	store, err := NewBindingStore(path, seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Assign(Target{ChatID: 10, ChatType: 2}, 3); err != nil {
		t.Fatal(err)
	}
	if err := store.Assign(Target{ChatID: 30, ChatType: 3}, 3); err != nil {
		t.Fatal(err)
	}
	if err := store.Assign(Target{ChatID: 20, ChatType: 3}, 0); err != nil {
		t.Fatal(err)
	}
	want := []Binding{
		{Target: Target{ChatID: 10, ChatType: 2}, GroupID: 3},
		{Target: Target{ChatID: 30, ChatType: 3}, GroupID: 3},
	}
	if got := store.Bindings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("bindings = %#v, want %#v", got, want)
	}
	restarted, err := NewBindingStore(path, map[uint64]Target{
		9: {ChatID: 99, ChatType: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.Bindings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("restarted bindings = %#v, want %#v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("binding file mode = %o, want 600", info.Mode().Perm())
	}
}

func TestBindingStoreRejectsInvalidData(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bindings.json")
	if err := os.WriteFile(path, []byte(
		`{"bindings":[{"chat_id":10,"chat_type":2,"group_id":1},{"chat_id":10,"chat_type":3,"group_id":2}]}`,
	), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewBindingStore(path, nil); err == nil {
		t.Fatal("duplicate chat binding was accepted")
	}

	store, err := NewBindingStore(filepath.Join(t.TempDir(), "bindings.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range []Binding{
		{Target: Target{ChatID: 0, ChatType: 2}, GroupID: 1},
		{Target: Target{ChatID: 1, ChatType: 1}, GroupID: 1},
	} {
		if err := store.Assign(binding.Target, binding.GroupID); err == nil {
			t.Fatalf("invalid binding accepted: %#v", binding)
		}
	}
}
