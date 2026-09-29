package audit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type memoryRepository struct {
	created Log
	items   []Log
}

func (r *memoryRepository) Create(_ context.Context, log Log) error {
	r.created = log
	return nil
}

func (r *memoryRepository) ListByGroup(context.Context, uint64, int) ([]Log, error) {
	return r.items, nil
}

func (r *memoryRepository) ListAll(context.Context, int) ([]Log, error) {
	return r.items, nil
}

func TestServiceCreateRedactsSensitiveValues(t *testing.T) {
	t.Parallel()

	repo := &memoryRepository{}
	service := NewService(repo)
	err := service.Create(t.Context(), CreateLogInput{
		GroupID:    7,
		ActorID:    9,
		Action:     "update_user",
		TargetType: "users",
		TargetID:   11,
		Before: map[string]any{
			"display_name":  "旧名称",
			"password_hash": "old-hash",
		},
		After: map[string]any{
			"display_name": "新名称",
			"entity_id":    uint64(18446744073709551615),
			"credentials": map[string]any{
				"access_token": "secret-token",
			},
		},
		IP:        strings.Repeat("界", 70),
		UserAgent: strings.Repeat("a", 600),
	}, time.Date(2026, 9, 29, 1, 2, 3, 4_000_000, time.UTC))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var before, after map[string]any
	if err := json.Unmarshal([]byte(repo.created.BeforeJSON), &before); err != nil {
		t.Fatalf("decode before JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(repo.created.AfterJSON), &after); err != nil {
		t.Fatalf("decode after JSON: %v", err)
	}
	if before["display_name"] != "旧名称" || before["password_hash"] != redactedValue {
		t.Fatalf("before JSON = %#v", before)
	}
	credentials, ok := after["credentials"].(map[string]any)
	if !ok || credentials["access_token"] != redactedValue {
		t.Fatalf("after JSON = %#v", after)
	}
	if !strings.Contains(repo.created.AfterJSON, `"entity_id":18446744073709551615`) {
		t.Fatalf("after JSON lost uint64 precision: %s", repo.created.AfterJSON)
	}
	if len([]rune(repo.created.IP)) != 64 || len([]rune(repo.created.UserAgent)) != 512 {
		t.Fatalf("metadata lengths = ip %d, user agent %d", len([]rune(repo.created.IP)), len([]rune(repo.created.UserAgent)))
	}
}

func TestChangedFieldsKeepsOnlyNestedChanges(t *testing.T) {
	t.Parallel()

	before, after, changed, err := ChangedFields(
		map[string]any{
			"checkin_notifications": map[string]any{
				"daily_enabled":  true,
				"weekly_enabled": true,
			},
			"unchanged": "value",
		},
		map[string]any{
			"checkin_notifications": map[string]any{
				"daily_enabled":  false,
				"weekly_enabled": true,
			},
			"new_field": "new",
			"unchanged": "value",
		},
	)
	if err != nil {
		t.Fatalf("ChangedFields() error = %v", err)
	}
	if !changed {
		t.Fatal("ChangedFields() changed = false, want true")
	}

	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(beforeJSON), `{"checkin_notifications":{"daily_enabled":true},"new_field":{"_missing":true}}`; got != want {
		t.Fatalf("before = %s, want %s", got, want)
	}
	if got, want := string(afterJSON), `{"checkin_notifications":{"daily_enabled":false},"new_field":"new"}`; got != want {
		t.Fatalf("after = %s, want %s", got, want)
	}
}

func TestChangedFieldsIgnoresEquivalentJSONValues(t *testing.T) {
	t.Parallel()

	before, after, changed, err := ChangedFields(
		struct {
			Enabled bool `json:"enabled"`
		}{Enabled: true},
		map[string]any{"enabled": true},
	)
	if err != nil {
		t.Fatalf("ChangedFields() error = %v", err)
	}
	if changed || before != nil || after != nil {
		t.Fatalf("ChangedFields() = %#v, %#v, %t; want no changes", before, after, changed)
	}
}

func TestChangedFieldsDistinguishesMissingFromNull(t *testing.T) {
	t.Parallel()

	before, after, changed, err := ChangedFields(
		map[string]any{"nullable": nil},
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("ChangedFields() error = %v", err)
	}
	if !changed {
		t.Fatal("ChangedFields() changed = false, want true")
	}
	beforeMap := before.(map[string]any)
	afterMap := after.(map[string]any)
	if _, ok := beforeMap["nullable"]; !ok {
		t.Fatal("before changes omitted nullable field")
	}
	if value, ok := afterMap["nullable"].(map[string]any); !ok || value["_missing"] != true {
		t.Fatal("after changes omitted removed field")
	}
}

func TestServiceListByGroupIncludesActorAndChanges(t *testing.T) {
	t.Parallel()

	repo := &memoryRepository{items: []Log{{
		ID:               3,
		GroupID:          7,
		ActorID:          9,
		ActorUsername:    "admin",
		ActorDisplayName: "管理员",
		Action:           "save_learning_config",
		TargetType:       "group_settings",
		TargetID:         7,
		BeforeJSON:       `{"daily_enabled":true}`,
		AfterJSON:        `{"daily_enabled":false}`,
		CreatedAt:        "2026-09-29T09:30:00+08:00",
	}}}

	items, err := NewService(repo).ListByGroup(t.Context(), 7, 100)
	if err != nil {
		t.Fatalf("ListByGroup() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("item count = %d, want 1", len(items))
	}
	item := items[0]
	if item.ActorUsername != "admin" || item.ActorDisplayName != "管理员" {
		t.Fatalf("actor = %q/%q", item.ActorUsername, item.ActorDisplayName)
	}
	if string(item.Before) != `{"daily_enabled":true}` || string(item.After) != `{"daily_enabled":false}` {
		t.Fatalf("changes = %s -> %s", item.Before, item.After)
	}
}

func TestServiceListAllUsesGlobalRepositoryQuery(t *testing.T) {
	t.Parallel()

	repo := &memoryRepository{items: []Log{{ID: 1, Action: "create_tenant"}}}
	items, err := NewService(repo).ListAll(t.Context(), 200)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(items) != 1 || items[0].Action != "create_tenant" {
		t.Fatalf("items = %#v", items)
	}
}
