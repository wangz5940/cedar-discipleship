package notification

import (
	"context"
	"errors"
	"path/filepath"
	"time"
)

var ErrChatNotFound = errors.New("bot_chat_not_found")

type Manager struct {
	client *PotatoClient
	store  *BindingStore
	queue  *Queue
}

func NewManager(
	dir string,
	seed map[uint64]Target,
	source SnapshotSource,
	client *PotatoClient,
) (*Manager, error) {
	store, err := NewBindingStore(filepath.Join(dir, "bindings.json"), seed)
	if err != nil {
		return nil, err
	}
	queue, err := NewQueue(dir, store.Targets(), source, client)
	if err != nil {
		return nil, err
	}
	return &Manager{client: client, store: store, queue: queue}, nil
}

func (m *Manager) Run(ctx context.Context) {
	m.queue.Run(ctx)
}

func (m *Manager) Enqueue(event Event) error {
	return m.queue.Enqueue(event)
}

func (m *Manager) EnqueueInitial(now time.Time) error {
	return m.queue.EnqueueInitial(now)
}

func (m *Manager) WakeInitial(groupID uint64, now time.Time) error {
	return m.queue.WakeInitial(groupID, now)
}

func (m *Manager) Chats(ctx context.Context) ([]Chat, error) {
	return m.client.ListChats(ctx)
}

func (m *Manager) Bindings() []Binding {
	return m.store.Bindings()
}

func (m *Manager) Assign(ctx context.Context, target Target, groupID uint64, now time.Time) error {
	if groupID > 0 {
		chats, err := m.client.ListChats(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, chat := range chats {
			if chat.ChatID == target.ChatID && chat.ChatType == target.ChatType {
				found = true
				break
			}
		}
		if !found {
			return ErrChatNotFound
		}
	}
	if err := m.store.Assign(target, groupID); err != nil {
		return err
	}
	m.queue.SetTargets(m.store.Targets())
	if groupID > 0 {
		return m.queue.EnqueueInitialBinding(groupID, target, now)
	}
	return nil
}
