//go:build integration

package feedback

import (
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

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

	ownerItems, err := service.ListOwn(t.Context(), 11, 100)
	if err != nil || len(ownerItems) != 1 {
		t.Fatalf("owner items = %#v, err = %v", ownerItems, err)
	}
	otherItems, err := service.ListOwn(t.Context(), 12, 100)
	if err != nil || len(otherItems) != 0 {
		t.Fatalf("other user items = %#v, err = %v", otherItems, err)
	}
	detail, err := service.OwnDetail(t.Context(), 11, created.ID)
	if err != nil || len(detail.Replies) != 1 || detail.Replies[0].Message != "已收到" {
		t.Fatalf("owner detail = %#v, err = %v", detail, err)
	}

	adminItems, err := service.AdminList(t.Context(), "", 100)
	if err != nil || len(adminItems) != 2 {
		t.Fatalf("admin items = %#v, err = %v", adminItems, err)
	}
	if adminItems[0].LogID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("new feedback log ID = %q", adminItems[0].LogID)
	}
	if adminItems[0].GroupName != "反馈组" || adminItems[0].MemberName != "组内成员甲" {
		t.Fatalf("new feedback identity = %#v", adminItems[0])
	}
	if adminItems[1].UserID != 0 || adminItems[1].LegacyName != "历史用户" {
		t.Fatalf("legacy feedback = %#v", adminItems[1])
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
