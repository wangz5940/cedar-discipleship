//go:build integration

package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agp/backend/internal/audit"
	"agp/backend/internal/learning"
	"agp/backend/internal/logctx"
	"agp/backend/internal/testdb"
	"agp/backend/internal/user"
)

func TestLearningConfigAuditRecordsOnlyChangedFields(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES(1,'audit','审计组',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES(9,'admin','管理员','admin',NOW(),NOW());
		INSERT INTO group_settings(group_id,settings,created_at,updated_at)
		VALUES(1,'{"checkin_notifications":{"daily_enabled":true,"weekly_enabled":true},"unchanged":"value"}',NOW(),NOW())`)

	application := &app{
		db:       db,
		location: time.FixedZone("CST", 8*60*60),
		users:    user.NewService(user.NewMySQLRepository(db)),
		learning: learning.NewService(learning.NewMySQLRepository(db)),
		audits:   audit.NewService(audit.NewMySQLRepository(db)),
	}
	body := `{"checkin_notifications":{"daily_enabled":false,"weekly_enabled":true},"unchanged":"value"}`
	request := httptest.NewRequest(http.MethodPut, "/api/admin/learning-config", bytes.NewBufferString(body))
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
		ID:             9,
		Username:       "admin",
		DisplayName:    "管理员",
		CurrentGroupID: 1,
	}))
	request = request.WithContext(logctx.WithLogID(request.Context(), "0123456789abcdef0123456789abcdef"))

	response := httptest.NewRecorder()
	application.handleAdminSaveLearningConfig(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("first save status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}

	var beforeJSON, afterJSON, logID string
	if err := db.QueryRow(`SELECT before_json,after_json,log_id FROM audit_logs
		WHERE group_id=1 AND action='save_learning_config' ORDER BY id DESC LIMIT 1`).
		Scan(&beforeJSON, &afterJSON, &logID); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	assertJSONEqual(t, `{"checkin_notifications":{"daily_enabled":true}}`, beforeJSON)
	assertJSONEqual(t, `{"checkin_notifications":{"daily_enabled":false}}`, afterJSON)
	if logID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("stored log ID = %q", logID)
	}
	items, err := application.audits.ListByGroup(t.Context(), 1, 100)
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(items) != 1 ||
		items[0].ActorUsername != "admin" ||
		items[0].ActorDisplayName != "管理员" ||
		items[0].LogID != logID {
		t.Fatalf("listed audit = %#v", items)
	}

	request = httptest.NewRequest(http.MethodPut, "/api/admin/learning-config", bytes.NewBufferString(body))
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
		ID:             9,
		CurrentGroupID: 1,
	}))
	response = httptest.NewRecorder()
	application.handleAdminSaveLearningConfig(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("second save status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs
		WHERE group_id=1 AND action='save_learning_config'`).Scan(&count); err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit count = %d, want 1 after no-op save", count)
	}
}

func TestDeleteGroupPreservesAuditHistory(t *testing.T) {
	db := testdb.Open(t)
	for _, migration := range []string{
		"003_ministry_groups.sql",
		"004_ministry_attendance.sql",
		"005_ministry_catalog_and_pins.sql",
		"006_ministry_content_deletions.sql",
		"011_refresh_sessions.sql",
		"016_refresh_session_group_version.sql",
	} {
		testdb.Apply(t, db, migration)
	}
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES(7,'deleted','待删除组',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES(9,'admin','管理员','admin',NOW(),NOW());
		INSERT INTO audit_logs(group_id,actor_user_id,action,target_type,target_id,created_at)
		VALUES(7,9,'save_learning_config','group_settings',7,NOW());
		INSERT INTO feedbacks(group_id,user_id,name,contact,message,page,user_agent,created_at,updated_at)
		VALUES(7,9,'','','保留反馈','','',NOW(),NOW())`)

	service := user.NewService(user.NewMySQLRepository(db))
	if _, err := service.DeleteGroup(t.Context(), 7, time.Now().UTC()); err != nil {
		t.Fatalf("DeleteGroup() error = %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE group_id=7`).Scan(&count); err != nil {
		t.Fatalf("count audit history: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit history count = %d, want 1", count)
	}
	var feedbackGroupID sql.NullInt64
	if err := db.QueryRow(`SELECT group_id FROM feedbacks WHERE user_id=9`).Scan(&feedbackGroupID); err != nil {
		t.Fatalf("query preserved feedback: %v", err)
	}
	if feedbackGroupID.Valid {
		t.Fatalf("feedback group ID = %d, want NULL", feedbackGroupID.Int64)
	}
	var groupCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM study_groups WHERE id=7`).Scan(&groupCount); err != nil {
		t.Fatalf("count deleted group: %v", err)
	}
	if groupCount != 0 {
		t.Fatalf("group count = %d, want 0", groupCount)
	}
}

func assertJSONEqual(t *testing.T, want, got string) {
	t.Helper()
	var wantValue, gotValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode expected JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("decode actual JSON: %v", err)
	}
	wantPayload, _ := json.Marshal(wantValue)
	gotPayload, _ := json.Marshal(gotValue)
	if !bytes.Equal(wantPayload, gotPayload) {
		t.Fatalf("JSON = %s, want %s", gotPayload, wantPayload)
	}
}
