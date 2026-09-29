package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agp/backend/internal/backup"
	"agp/backend/internal/learning"
)

type backupImportTestRepository struct {
	imported bool
}

func (r *backupImportTestRepository) CheckinDetails(context.Context, uint64, *time.Location) ([]backup.CheckinDetail, error) {
	return nil, nil
}

func (r *backupImportTestRepository) FeedbackExports(context.Context, uint64, *time.Location) ([]backup.FeedbackExport, error) {
	return nil, nil
}

func (r *backupImportTestRepository) GroupInfo(context.Context, uint64) (*backup.GroupInfo, error) {
	return &backup.GroupInfo{ID: 1, Code: "group-a"}, nil
}

func (r *backupImportTestRepository) LocalBackupSnapshot(context.Context, uint64) (backup.Snapshot, error) {
	return backup.Snapshot{}, nil
}

func (r *backupImportTestRepository) ReplaceStudyWeeks(context.Context, uint64, []learning.WeekInput, time.Time) error {
	return nil
}

func (r *backupImportTestRepository) ImportLocalBackup(context.Context, uint64, uint64, backup.Payload, time.Time) error {
	r.imported = true
	return errors.New("import must not run without confirmation")
}

func TestHandleAdminImportLocalBackupRequiresConfirmation(t *testing.T) {
	repo := &backupImportTestRepository{}
	app := &app{
		backups:  backup.NewService(repo),
		location: time.UTC,
	}
	body := bytes.NewBufferString(`{
		"version": 1,
		"exported_at": "2026-09-29T00:00:00Z",
		"group": {"id": 1, "code": "group-a"},
		"members": [{"username": "admin"}]
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local-backup", body)
	request = request.WithContext(context.WithValue(
		request.Context(),
		currentUserKey,
		currentUser{ID: 7, CurrentGroupID: 1},
	))
	recorder := httptest.NewRecorder()

	app.handleAdminImportLocalBackupJSON(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if repo.imported {
		t.Fatal("backup import ran without explicit confirmation")
	}
	if !strings.Contains(recorder.Body.String(), `"confirmation"`) {
		t.Fatalf("response does not include an explicit confirmation value: %s", recorder.Body.String())
	}
}
