package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"agp/backend/internal/learning"
)

type serviceTestRepository struct {
	group         GroupInfo
	imported      bool
	snapshotCalls int
}

func (r *serviceTestRepository) CheckinDetails(context.Context, uint64, *time.Location) ([]CheckinDetail, error) {
	return nil, nil
}

func (r *serviceTestRepository) GroupInfo(context.Context, uint64) (*GroupInfo, error) {
	group := r.group
	return &group, nil
}

func (r *serviceTestRepository) LocalBackupSnapshot(context.Context, uint64) (Snapshot, error) {
	r.snapshotCalls++
	const generation = "snapshot-1"
	return Snapshot{
		Group:    GroupInfo{ID: 1, Code: generation},
		Settings: map[string]any{"generation": generation},
		Weeks:    []learning.WeekInput{{Title: generation}},
		Members:  []Member{{Username: generation}},
		Checkins: []Checkin{{Detail: generation}},
		Assets:   []Asset{{Title: generation}},
	}, nil
}

func (r *serviceTestRepository) ReplaceStudyWeeks(context.Context, uint64, []learning.WeekInput, time.Time) error {
	return nil
}

func (r *serviceTestRepository) ImportLocalBackup(context.Context, uint64, uint64, Payload, time.Time) error {
	r.imported = true
	return nil
}

func TestServiceImportLocalBackup(t *testing.T) {
	valid := Payload{
		Version:    CurrentVersion,
		ExportedAt: "2026-09-29T00:00:00Z",
		Group: map[string]any{
			"id":   float64(1),
			"code": "group-a",
		},
		Members: []Member{{Username: "admin"}},
	}
	tests := []struct {
		name    string
		payload Payload
		wantErr bool
	}{
		{name: "empty payload", payload: Payload{}, wantErr: true},
		{name: "unknown version", payload: func() Payload {
			payload := valid
			payload.Version = 2
			return payload
		}(), wantErr: true},
		{name: "wrong group id", payload: func() Payload {
			payload := valid
			payload.Group = map[string]any{"id": float64(2), "code": "group-a"}
			return payload
		}(), wantErr: true},
		{name: "wrong group code", payload: func() Payload {
			payload := valid
			payload.Group = map[string]any{"id": float64(1), "code": "group-b"}
			return payload
		}(), wantErr: true},
		{name: "missing snapshot content", payload: Payload{
			Version:    CurrentVersion,
			ExportedAt: "2026-09-29T00:00:00Z",
			Group:      map[string]any{"id": float64(1), "code": "group-a"},
		}, wantErr: true},
		{name: "empty settings only", payload: Payload{
			Version:    CurrentVersion,
			ExportedAt: "2026-09-29T00:00:00Z",
			Group:      map[string]any{"id": float64(1), "code": "group-a"},
			Settings:   map[string]any{},
		}, wantErr: true},
		{name: "legacy version one backup", payload: valid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &serviceTestRepository{group: GroupInfo{ID: 1, Code: "group-a"}}
			confirmation, err := NewService(repo).PrepareLocalBackupImport(
				t.Context(),
				1,
				7,
				test.payload,
				time.Now(),
			)
			if test.wantErr {
				if err == nil {
					t.Fatal("unsafe backup was accepted")
				}
				if repo.imported {
					t.Fatal("repository import ran for an unsafe backup")
				}
				return
			}
			if err != nil {
				t.Fatalf("valid version one backup rejected: %v", err)
			}
			if confirmation == "" {
				t.Fatal("valid backup did not receive a confirmation value")
			}
			if repo.imported {
				t.Fatal("backup was imported during confirmation preparation")
			}
		})
	}
}

func TestServiceImportLocalBackupConsumesConfirmation(t *testing.T) {
	repo := &serviceTestRepository{group: GroupInfo{ID: 1, Code: "group-a"}}
	service := NewService(repo)
	payload := Payload{
		Version:    CurrentVersion,
		ExportedAt: "2026-09-29T00:00:00Z",
		Group:      map[string]any{"id": float64(1), "code": "group-a"},
		Members:    []Member{{Username: "admin"}},
	}
	now := time.Now()
	confirmation, err := service.PrepareLocalBackupImport(t.Context(), 1, 7, payload, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ImportLocalBackup(t.Context(), 1, 7, payload, confirmation, now); err != nil {
		t.Fatalf("confirmed import failed: %v", err)
	}
	repo.imported = false
	if err := service.ImportLocalBackup(t.Context(), 1, 7, payload, confirmation, now); !errors.Is(err, ErrBackupConfirmationRequired) {
		t.Fatalf("reused confirmation error = %v, want %v", err, ErrBackupConfirmationRequired)
	}
	if repo.imported {
		t.Fatal("reused confirmation reached repository")
	}
}

func TestServiceImportLocalBackupBindsConfirmationToPayloadAndExpiry(t *testing.T) {
	repo := &serviceTestRepository{group: GroupInfo{ID: 1, Code: "group-a"}}
	service := NewService(repo)
	payload := Payload{
		Version:    CurrentVersion,
		ExportedAt: "2026-09-29T00:00:00Z",
		Group:      map[string]any{"id": float64(1), "code": "group-a"},
		Members:    []Member{{Username: "admin"}},
	}
	now := time.Now()
	confirmation, err := service.PrepareLocalBackupImport(t.Context(), 1, 7, payload, now)
	if err != nil {
		t.Fatal(err)
	}

	changed := payload
	changed.Members = []Member{{Username: "other"}}
	if err := service.ImportLocalBackup(t.Context(), 1, 7, changed, confirmation, now); !errors.Is(err, ErrBackupConfirmationRequired) {
		t.Fatalf("changed payload error = %v, want %v", err, ErrBackupConfirmationRequired)
	}
	if repo.imported {
		t.Fatal("changed payload reached repository")
	}
	if err := service.ImportLocalBackup(t.Context(), 1, 7, payload, confirmation, now.Add(backupConfirmationTTL)); !errors.Is(err, ErrBackupConfirmationRequired) {
		t.Fatalf("expired confirmation error = %v, want %v", err, ErrBackupConfirmationRequired)
	}
	if repo.imported {
		t.Fatal("expired confirmation reached repository")
	}
}

func TestServiceLocalBackupUsesOneRepositorySnapshot(t *testing.T) {
	repo := &serviceTestRepository{}
	service := NewService(repo)

	payload, err := service.LocalBackup(t.Context(), 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}

	want := "snapshot-1"
	got := []string{
		payload.Group["code"].(string),
		payload.Settings["generation"].(string),
		payload.Weeks[0].Title,
		payload.Members[0].Username,
		payload.Checkins[0].Detail,
		payload.Assets[0].Title,
	}
	for index, value := range got {
		if value != want {
			t.Fatalf("backup field %d came from %q, want %q", index, value, want)
		}
	}
	if repo.snapshotCalls != 1 {
		t.Fatalf("snapshot reads = %d, want 1", repo.snapshotCalls)
	}
}

func TestServiceAcceptsLegacyFeedbackPayload(t *testing.T) {
	repo := &serviceTestRepository{group: GroupInfo{ID: 1, Code: "group-a"}}
	service := NewService(repo)
	payload := Payload{
		Version:    CurrentVersion,
		ExportedAt: "2026-09-30T00:00:00Z",
		Group:      map[string]any{"id": float64(1), "code": "group-a"},
		Members:    []Member{{Username: "admin"}},
		Feedbacks:  []Feedback{{Message: "legacy private feedback"}},
	}
	now := time.Now()
	confirmation, err := service.PrepareLocalBackupImport(t.Context(), 1, 7, payload, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ImportLocalBackup(t.Context(), 1, 7, payload, confirmation, now); err != nil {
		t.Fatal(err)
	}
	if !repo.imported {
		t.Fatal("compatible backup was not imported")
	}
}
