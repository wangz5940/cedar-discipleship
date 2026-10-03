//go:build integration

package server

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agp/backend/internal/audit"
	"agp/backend/internal/backup"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"

	"github.com/xuri/excelize/v2"
)

func TestStudyWeeksExcelRoundTrip(t *testing.T) {
	for _, mode := range []string{"weekly", "daily"} {
		for _, bookEnabled := range []bool{true, false} {
			for _, legacyAggregateColumn := range []bool{false, true} {
				t.Run(fmt.Sprintf("books=%v/legacy-aggregate-column=%v", bookEnabled, legacyAggregateColumn), func(t *testing.T) {
					db := testdb.Open(t)
					a := &app{
						db: db, location: time.UTC,
						learning: learning.NewService(learning.NewMySQLRepository(db)),
						backups:  backup.NewService(backup.NewMySQLRepository(db)),
						audits:   audit.NewService(audit.NewMySQLRepository(db)),
					}
					input := learning.WeekInput{
						StartDate: "2026-09-21", EndDate: "2026-09-27", Title: "整周学习",
						WeeklyCheckin: true, BookEnabled: bookEnabled,
						VerseEnabled: true, VerseMode: mode, VerseRef: "罗马书 8:1",
						Readings: []learning.TaskBinding{{Title: "本周读物", URL: "https://example.org/book.pdf"}},
					}
					if err := a.backups.ReplaceStudyWeeks(t.Context(), 1, []learning.WeekInput{input}, time.Now()); err != nil {
						t.Fatal(err)
					}
					request := func(method, contentType string, body *bytes.Buffer) *http.Request {
						req := httptest.NewRequest(method, "/", body)
						req.Header.Set("Content-Type", contentType)
						return req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: 1}))
					}
					export := httptest.NewRecorder()
					a.handleAdminExportStudyWeeksExcel(export, request(http.MethodGet, "", &bytes.Buffer{}))
					if export.Code != http.StatusOK {
						t.Fatalf("export: %d %s", export.Code, export.Body)
					}
					data := export.Body.Bytes()
					if legacyAggregateColumn {
						file, err := excelize.OpenReader(bytes.NewReader(data))
						if err != nil {
							t.Fatal(err)
						}
						defer file.Close()
						if err := file.InsertCols("Weeks", "J", 1); err != nil {
							t.Fatal(err)
						}
						if err := file.SetCellValue("Weeks", "J1", "整周签到"); err != nil {
							t.Fatal(err)
						}
						if err := file.SetCellValue("Weeks", "J2", true); err != nil {
							t.Fatal(err)
						}
						buf, err := file.WriteToBuffer()
						if err != nil {
							t.Fatal(err)
						}
						data = buf.Bytes()
					}
					var body bytes.Buffer
					form := multipart.NewWriter(&body)
					part, err := form.CreateFormFile("file", "weeks.xlsx")
					if err != nil {
						t.Fatal(err)
					}
					if _, err := part.Write(data); err != nil {
						t.Fatal(err)
					}
					if err := form.Close(); err != nil {
						t.Fatal(err)
					}
					imported := httptest.NewRecorder()
					a.handleAdminImportStudyWeeksExcel(imported, request(http.MethodPost, form.FormDataContentType(), &body))
					if imported.Code != http.StatusOK {
						t.Fatalf("import: %d %s", imported.Code, imported.Body)
					}
					weeks, err := a.learning.ListWeekInputs(t.Context(), 1)
					if err != nil {
						t.Fatal(err)
					}
					if len(weeks) != 1 || weeks[0].WeeklyCheckin || weeks[0].BookEnabled != bookEnabled || weeks[0].VerseMode != "weekly" {
						t.Fatalf("round trip changed completion mode: %+v", weeks)
					}
					var aggregates, requiredBooks int
					if err := db.QueryRow(`SELECT COUNT(*) FROM study_tasks WHERE group_id=1 AND task_type='weekly_checkin'`).Scan(&aggregates); err != nil {
						t.Fatal(err)
					}
					if err := db.QueryRow("SELECT COUNT(*) FROM study_tasks WHERE group_id=1 AND task_type='weekly_book' AND required=1").Scan(&requiredBooks); err != nil {
						t.Fatal(err)
					}
					wantRequiredBooks := 0
					if bookEnabled {
						wantRequiredBooks = 1
					}
					if aggregates != 0 || requiredBooks != wantRequiredBooks {
						t.Fatalf("task contract changed: aggregates=%d required books=%d", aggregates, requiredBooks)
					}
				})
			}
		}
	}
}
