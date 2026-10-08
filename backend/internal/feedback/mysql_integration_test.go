//go:build integration

package feedback

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestSystemFeedbackStoresNullAuthorAndRemainsAdminOnly(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "018_feedback_workflow.sql")
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES(1,'feedback','反馈组',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES(11,'member','成员','member',NOW(),NOW())`)
	service := NewService(NewMySQLRepository(db), NewLocalStorage(t.TempDir()))
	result, err := service.CreateSystemAutomatic(t.Context(), CreateInput{
		GroupID: 1, Message: "通知最终失败",
		Diagnostics: map[string]string{"environment": "server", "error_code": "notification_delivery_failed"},
	}, time.Now())
	if err != nil || result.Feedback == nil {
		t.Fatalf("system create=%+v err=%v", result, err)
	}
	var author sql.NullInt64
	if err := db.QueryRow(`SELECT user_id FROM feedbacks WHERE id=?`, result.Feedback.ID).Scan(&author); err != nil || author.Valid {
		t.Fatalf("author=%+v err=%v", author, err)
	}
	manual, err := service.Create(t.Context(), CreateInput{
		UserID: 11, GroupID: 1, Message: "用户反馈",
		Diagnostics: map[string]string{"page_origin": "https://cedar.example.test", "environment": "production"},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.ListOwn(t.Context(), 11, "", 100)
	if err != nil || len(items) != 1 || items[0].ID != manual.ID {
		t.Fatalf("user items=%+v err=%v", items, err)
	}
	if _, err := service.OwnDetail(t.Context(), 11, result.Feedback.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("system feedback exposed to user: %v", err)
	}
	detail, err := service.AdminDetail(t.Context(), result.Feedback.ID)
	if err != nil || detail.UserID != 0 || detail.LegacyName != "系统" || detail.GroupName != "反馈组" ||
		detail.Diagnostics["environment"] != "server" {
		t.Fatalf("admin detail=%+v err=%v", detail, err)
	}
}

func TestMySQLRepositoryScopesFeedbackAndClearsClosedDiagnostics(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "018_feedback_workflow.sql")
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES(1,'feedback','反馈组',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES
			(11,'member-a','成员甲','member-a',NOW(),NOW()),
			(12,'member-b','成员乙','member-b',NOW(),NOW()),
			(99,'admin','管理员','admin',NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
		VALUES(1,11,'组内成员甲',NOW(),NOW(),NOW());
		INSERT INTO feedbacks(group_id,user_id,name,contact,message,page,user_agent,created_at)
		VALUES(1,NULL,'历史用户','legacy@example.com','历史反馈','home','legacy-agent',NOW())`)

	repo := NewMySQLRepository(db)
	service := NewService(repo, NewLocalStorage(t.TempDir()))
	settings, err := service.AutomaticSettings(t.Context())
	if err != nil || !settings.Enabled || len(settings.MutedErrorTypes) != 0 {
		t.Fatalf("default automatic settings = %#v, err = %v", settings, err)
	}
	settings, err = service.UpdateAutomaticSettings(t.Context(), AutomaticSettings{
		Enabled:         true,
		MutedErrorTypes: []string{"asset_not_found"},
	}, 99, time.Now())
	if err != nil || len(settings.MutedErrorTypes) != 1 {
		t.Fatalf("updated automatic settings = %#v, err = %v", settings, err)
	}
	automatic, err := service.CreateAutomatic(t.Context(), CreateInput{
		UserID:      11,
		GroupID:     1,
		Message:     "自动错误",
		Diagnostics: map[string]string{"error_code": "asset_not_found"},
	}, time.Now())
	if err != nil || automatic.Feedback != nil || automatic.Reason != "muted" {
		t.Fatalf("automatic result = %#v, err = %v", automatic, err)
	}

	created, err := service.Create(t.Context(), CreateInput{
		UserID:      11,
		GroupID:     1,
		Message:     "新反馈",
		LogID:       "0123456789abcdef0123456789abcdef",
		Diagnostics: map[string]string{"page": "home"},
	}, time.Now())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := service.Reply(t.Context(), created.ID, 99, "已收到", time.Now()); err != nil {
		t.Fatalf("Reply() error = %v", err)
	}

	automaticCreated, err := service.CreateAutomatic(t.Context(), CreateInput{
		UserID: 11, GroupID: 1, Message: "自动反馈",
		Diagnostics: map[string]string{"error_code": "network_request_failed"},
	}, time.Now())
	if err != nil || automaticCreated.Feedback == nil {
		t.Fatalf("automatic create=%+v err=%v", automaticCreated, err)
	}

	ownerItems, err := service.ListOwn(t.Context(), 11, "", 100)
	if err != nil || len(ownerItems) != 2 {
		t.Fatalf("owner items = %#v, err = %v", ownerItems, err)
	}
	manualItems, err := service.ListOwn(t.Context(), 11, SourceManual, 100)
	if err != nil || len(manualItems) != 1 || manualItems[0].ID != created.ID {
		t.Fatalf("manual items = %#v, err = %v", manualItems, err)
	}
	automaticItems, err := service.ListOwn(t.Context(), 11, SourceAutomatic, 100)
	if err != nil || len(automaticItems) != 1 || automaticItems[0].ID != automaticCreated.Feedback.ID {
		t.Fatalf("automatic items = %#v, err = %v", automaticItems, err)
	}
	otherItems, err := service.ListOwn(t.Context(), 12, "", 100)
	if err != nil || len(otherItems) != 0 {
		t.Fatalf("other user items = %#v, err = %v", otherItems, err)
	}
	detail, err := service.OwnDetail(t.Context(), 11, created.ID)
	if err != nil || len(detail.Replies) != 1 || detail.Replies[0].Message != "已收到" {
		t.Fatalf("owner detail = %#v, err = %v", detail, err)
	}

	adminItems, err := service.AdminList(t.Context(), "", "", 100)
	if err != nil || len(adminItems) != 3 {
		t.Fatalf("admin items = %#v, err = %v", adminItems, err)
	}
	adminManual, err := service.AdminList(t.Context(), "", SourceManual, 100)
	if err != nil || len(adminManual) != 2 {
		t.Fatalf("admin manual items = %#v, err = %v", adminManual, err)
	}
	if adminManual[0].LogID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("new feedback log ID = %q", adminManual[0].LogID)
	}
	if adminManual[0].GroupName != "反馈组" || adminManual[0].MemberName != "组内成员甲" {
		t.Fatalf("new feedback identity = %#v", adminManual[0])
	}
	if adminManual[1].UserID != 0 || adminManual[1].LegacyName != "历史用户" {
		t.Fatalf("legacy feedback = %#v", adminManual[1])
	}

	if err := service.UpdateStatus(t.Context(), created.ID, StatusClosed, time.Now()); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
	adminDetail, err := service.AdminDetail(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(adminDetail.Diagnostics) != 0 {
		t.Fatalf("closed diagnostics = %#v", adminDetail)
	}
}

func TestUnreadCandidatesExcludeHistoryAndAutomaticReportsWithoutListLimit(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "018_feedback_workflow.sql")
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
 VALUES(11,'reader','读者','reader',NOW(),NOW()),(12,'other','其他','other',NOW(),NOW());
 INSERT INTO feedbacks(id,user_id,name,contact,message,source,status,page,user_agent,created_at,updated_at)
 VALUES(41,11,'','','历史','manual','closed','','','2020-01-01','2020-01-01'),
 (42,11,'','','新反馈','manual','pending','','','2030-01-01','2030-01-01'),
 (43,12,'','','他人反馈','manual','pending','','','2030-01-01','2030-01-01'),
 (44,11,'','','自动','automatic','pending','','','2030-01-01','2030-01-01');
 INSERT INTO feedback_replies(id,feedback_id,admin_user_id,message,created_at)
 VALUES(10,41,99,'历史回复','2020-01-01'),(11,41,99,'旧反馈的新回复','2030-01-01'),
 (12,43,99,'他人回复','2030-01-01'),(13,44,99,'自动反馈的回复','2030-01-01')`)
	repo := NewMySQLRepository(db)
	since := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	own, err := repo.UnreadCandidates(t.Context(), 11, false, since)
	if err != nil || len(own) != 2 || own[0].ID != 44 || own[0].LastReplyID != 13 || own[1].ID != 41 || own[1].LastReplyID != 11 {
		t.Fatalf("own=%+v err=%v", own, err)
	}
	admin, err := repo.UnreadCandidates(t.Context(), 99, true, since)
	if err != nil || len(admin) != 2 || admin[0].ID != 43 || admin[1].ID != 42 {
		t.Fatalf("admin=%+v err=%v", admin, err)
	}
}
