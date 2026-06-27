package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	backupdomain "agp/backend/internal/backup"

	"github.com/xuri/excelize/v2"
)

type localBackupMember = backupdomain.Member
type localBackupCheckin = backupdomain.Checkin
type localBackupFeedback = backupdomain.Feedback
type localBackupAsset = backupdomain.Asset
type localBackupPayload = backupdomain.Payload

func writeAttachmentHeaders(w http.ResponseWriter, filename, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
}

func groupExportPrefix(groupID uint64) string {
	if groupID == 0 {
		return "group"
	}
	return fmt.Sprintf("group-%d", groupID)
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func parseFlexibleBool(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return fallback
	case "1", "true", "yes", "y", "是":
		return true
	case "0", "false", "no", "n", "否":
		return false
	default:
		return fallback
	}
}

func excelCell(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func weekKey(startDate, endDate string) string {
	return strings.TrimSpace(startDate) + "|" + strings.TrimSpace(endDate)
}

func (a *app) listStudyWeekInputs(groupID uint64) ([]studyWeekInput, error) {
	return a.learning.ListWeekInputs(context.Background(), groupID)
}

func (a *app) handleAdminExportCheckinsCSV(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	items, err := a.backups.CheckinDetails(r.Context(), groupID, a.location)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export_checkins_failed")
		return
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"记录ID", "打卡日期", "打卡时间", "账号", "成员姓名", "任务类型", "子项", "详情", "备注", "是否补卡"})
	for _, item := range items {
		_ = writer.Write([]string{
			strconv.FormatUint(item.ID, 10),
			item.LogicalDate,
			item.CheckinTime,
			item.Username,
			item.MemberName,
			item.TaskType,
			item.Part,
			item.Detail,
			item.Note,
			boolString(item.IsRetro),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		writeError(w, http.StatusInternalServerError, "export_checkins_failed")
		return
	}
	writeAttachmentHeaders(w, fmt.Sprintf("%s-checkins-detail.csv", groupExportPrefix(groupID)), "text/csv; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (a *app) handleAdminExportDailySummaryCSV(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	activeMembers, items, err := a.backups.DailySummaries(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export_daily_summary_failed")
		return
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"日期", "组内活跃人数", "当日打卡人数", "总打卡数", "灵修数", "读物数", "视频数", "背经数", "完成率"})
	for _, item := range items {
		rate := 0.0
		if activeMembers > 0 {
			rate = float64(item.TotalCheckins) / float64(activeMembers*4) * 100
		}
		_ = writer.Write([]string{
			item.LogicalDate,
			strconv.Itoa(activeMembers),
			strconv.Itoa(item.CheckedMembers),
			strconv.Itoa(item.TotalCheckins),
			strconv.Itoa(item.DevotionCount),
			strconv.Itoa(item.BookCount),
			strconv.Itoa(item.VideoCount),
			strconv.Itoa(item.VerseCount),
			fmt.Sprintf("%.2f%%", rate),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		writeError(w, http.StatusInternalServerError, "export_daily_summary_failed")
		return
	}
	writeAttachmentHeaders(w, fmt.Sprintf("%s-daily-summary.csv", groupExportPrefix(groupID)), "text/csv; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (a *app) handleAdminExportStudyWeeksExcel(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	weeks, err := a.listStudyWeekInputs(groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export_study_weeks_failed")
		return
	}
	file := excelize.NewFile()
	file.SetSheetName("Sheet1", "Weeks")
	_ = file.SetSheetRow("Weeks", "A1", &[]string{"开始日期", "结束日期", "标题", "背经经文", "默写原文", "显示读物", "显示视频", "显示背经", "显示提纲"})
	_, _ = file.NewSheet("Readings")
	_ = file.SetSheetRow("Readings", "A1", &[]string{"开始日期", "结束日期", "排序", "标题", "URL", "资产ID"})
	_, _ = file.NewSheet("Videos")
	_ = file.SetSheetRow("Videos", "A1", &[]string{"开始日期", "结束日期", "排序", "标题", "URL", "资产ID"})
	_, _ = file.NewSheet("Outlines")
	_ = file.SetSheetRow("Outlines", "A1", &[]string{"开始日期", "结束日期", "标题", "URL", "类型", "资产ID"})
	weekRow, readingRow, videoRow, outlineRow := 2, 2, 2, 2
	for _, week := range weeks {
		_ = file.SetSheetRow("Weeks", fmt.Sprintf("A%d", weekRow), &[]any{
			week.StartDate,
			week.EndDate,
			week.Title,
			week.VerseRef,
			week.ReciteText,
			week.BookEnabled,
			week.VideoEnabled,
			week.VerseEnabled,
			week.OutlineEnabled,
		})
		weekRow++
		for index, reading := range week.Readings {
			_ = file.SetSheetRow("Readings", fmt.Sprintf("A%d", readingRow), &[]any{
				week.StartDate,
				week.EndDate,
				index + 1,
				reading.Title,
				reading.URL,
				reading.AssetID,
			})
			readingRow++
		}
		for index, video := range week.Videos {
			_ = file.SetSheetRow("Videos", fmt.Sprintf("A%d", videoRow), &[]any{
				week.StartDate,
				week.EndDate,
				index + 1,
				video.Title,
				video.URL,
				video.AssetID,
			})
			videoRow++
		}
		if strings.TrimSpace(week.Outline.Title) != "" || strings.TrimSpace(week.Outline.URL) != "" || week.Outline.AssetID > 0 {
			_ = file.SetSheetRow("Outlines", fmt.Sprintf("A%d", outlineRow), &[]any{
				week.StartDate,
				week.EndDate,
				week.Outline.Title,
				week.Outline.URL,
				week.Outline.Type,
				week.Outline.AssetID,
			})
			outlineRow++
		}
	}
	var buf bytes.Buffer
	if err := file.Write(&buf); err != nil {
		writeError(w, http.StatusInternalServerError, "export_study_weeks_failed")
		return
	}
	writeAttachmentHeaders(w, fmt.Sprintf("%s-study-weeks.xlsx", groupExportPrefix(groupID)), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	_, _ = w.Write(buf.Bytes())
}

func (a *app) handleAdminImportStudyWeeksExcel(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_import_form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required")
		return
	}
	defer file.Close()
	xlsx, err := excelize.OpenReader(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_excel_file")
		return
	}
	defer func() { _ = xlsx.Close() }()
	weeksMap := map[string]*studyWeekInput{}
	var order []string
	if rows, err := xlsx.GetRows("Weeks"); err == nil {
		for _, row := range rows[1:] {
			startDate := excelCell(row, 0)
			endDate := excelCell(row, 1)
			if startDate == "" || endDate == "" {
				continue
			}
			key := weekKey(startDate, endDate)
			weeksMap[key] = &studyWeekInput{
				StartDate:      startDate,
				EndDate:        endDate,
				Title:          excelCell(row, 2),
				VerseRef:       excelCell(row, 3),
				ReciteText:     excelCell(row, 4),
				BookEnabled:    parseFlexibleBool(excelCell(row, 5), true),
				VideoEnabled:   parseFlexibleBool(excelCell(row, 6), true),
				VerseEnabled:   parseFlexibleBool(excelCell(row, 7), true),
				OutlineEnabled: parseFlexibleBool(excelCell(row, 8), true),
				Readings:       []weekTaskBinding{},
				Videos:         []weekTaskBinding{},
				Outline:        weekTaskBinding{Type: "image"},
			}
			order = append(order, key)
		}
	}
	if len(order) == 0 {
		writeError(w, http.StatusBadRequest, "weeks_sheet_required")
		return
	}
	if rows, err := xlsx.GetRows("Readings"); err == nil {
		for _, row := range rows[1:] {
			key := weekKey(excelCell(row, 0), excelCell(row, 1))
			week := weeksMap[key]
			if week == nil {
				continue
			}
			if excelCell(row, 3) == "" && excelCell(row, 4) == "" && excelCell(row, 5) == "" {
				continue
			}
			assetID, _ := strconv.ParseUint(excelCell(row, 5), 10, 64)
			week.Readings = append(week.Readings, weekTaskBinding{
				Title:   excelCell(row, 3),
				URL:     excelCell(row, 4),
				Type:    "pdf",
				AssetID: assetID,
			})
		}
	}
	if rows, err := xlsx.GetRows("Videos"); err == nil {
		for _, row := range rows[1:] {
			key := weekKey(excelCell(row, 0), excelCell(row, 1))
			week := weeksMap[key]
			if week == nil {
				continue
			}
			if excelCell(row, 3) == "" && excelCell(row, 4) == "" && excelCell(row, 5) == "" {
				continue
			}
			assetID, _ := strconv.ParseUint(excelCell(row, 5), 10, 64)
			week.Videos = append(week.Videos, weekTaskBinding{
				Title:   excelCell(row, 3),
				URL:     excelCell(row, 4),
				Type:    "video",
				AssetID: assetID,
			})
		}
	}
	if rows, err := xlsx.GetRows("Outlines"); err == nil {
		for _, row := range rows[1:] {
			key := weekKey(excelCell(row, 0), excelCell(row, 1))
			week := weeksMap[key]
			if week == nil {
				continue
			}
			assetID, _ := strconv.ParseUint(excelCell(row, 5), 10, 64)
			week.Outline = weekTaskBinding{
				Title:   excelCell(row, 2),
				URL:     excelCell(row, 3),
				Type:    firstNonEmpty(excelCell(row, 4), "image"),
				AssetID: assetID,
			}
		}
	}
	weeks := make([]studyWeekInput, 0, len(order))
	nowTime := time.Now().In(a.location)
	for _, key := range order {
		week := weeksMap[key]
		if week == nil {
			continue
		}
		weeks = append(weeks, *week)
	}
	if err := a.backups.ReplaceStudyWeeks(r.Context(), groupID, weeks, nowTime); err != nil {
		writeError(w, http.StatusInternalServerError, "study_weeks_import_failed")
		return
	}
	a.audit(groupID, u.ID, "import_study_weeks_excel", "study_weeks", 0, nil, map[string]any{"weeks": len(order)}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "weeks": len(order)})
}

func (a *app) handleAdminExportFeedbacksCSV(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	items, err := a.backups.FeedbackExports(r.Context(), groupID, a.location)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export_feedbacks_failed")
		return
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"提交时间", "账号", "姓名", "联系方式", "页面", "反馈内容", "User-Agent"})
	for _, item := range items {
		_ = writer.Write([]string{
			item.CreatedAt,
			item.Username,
			item.Name,
			item.Contact,
			item.Page,
			item.Message,
			item.UserAgent,
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		writeError(w, http.StatusInternalServerError, "export_feedbacks_failed")
		return
	}
	writeAttachmentHeaders(w, fmt.Sprintf("%s-feedbacks.csv", groupExportPrefix(groupID)), "text/csv; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (a *app) handleAdminExportLocalBackupJSON(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	settings, err := a.groupLearningConfig(groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export_backup_failed")
		return
	}
	weeks, err := a.listStudyWeekInputs(groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export_backup_failed")
		return
	}
	payload, err := a.backups.LocalBackup(r.Context(), groupID, settings, weeks, time.Now().In(a.location).Format(time.RFC3339))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export_backup_failed")
		return
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export_backup_failed")
		return
	}
	code := groupExportPrefix(groupID)
	if groupCode, ok := payload.Group["code"].(string); ok && strings.TrimSpace(groupCode) != "" {
		code = strings.TrimSpace(groupCode)
	}
	writeAttachmentHeaders(w, fmt.Sprintf("%s-local-backup.json", code), "application/json; charset=utf-8")
	_, _ = w.Write(body)
}

func (a *app) handleAdminImportLocalBackupJSON(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var payload localBackupPayload
	if !readJSON(w, r, &payload) {
		return
	}
	nowTime := time.Now().In(a.location)
	if err := a.backups.ImportLocalBackup(r.Context(), groupID, u.ID, payload, nowTime); err != nil {
		writeError(w, http.StatusInternalServerError, "backup_import_failed")
		return
	}
	a.audit(groupID, u.ID, "import_local_backup", "study_groups", groupID, nil, map[string]any{
		"members":   len(payload.Members),
		"weeks":     len(payload.Weeks),
		"checkins":  len(payload.Checkins),
		"feedbacks": len(payload.Feedbacks),
	}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
