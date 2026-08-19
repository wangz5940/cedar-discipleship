package server

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	ministrydomain "agp/backend/internal/ministry"
)

func (a *app) handleMinistryAttendance(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	groupID := pathUint64(r, "id")
	sheet, err := a.ministry.AttendanceSheet(
		r.Context(),
		studyGroupID,
		groupID,
		ministryActor(user),
		r.URL.Query().Get("month"),
		a.location,
	)
	if err != nil {
		a.writeMinistryAttendanceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sheet)
}

func (a *app) handleMinistryAttendanceSettings(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input ministrydomain.AttendanceSettingsInput
	if !readJSON(w, r, &input) {
		return
	}
	groupID := pathUint64(r, "id")
	err := a.ministry.SaveAttendanceSettings(
		r.Context(),
		studyGroupID,
		groupID,
		ministryActor(user),
		input,
		time.Now().UTC(),
	)
	if err != nil {
		a.writeMinistryAttendanceError(w, r, err)
		return
	}
	a.audit(
		studyGroupID,
		user.ID,
		"update_ministry_attendance_settings",
		"ministry_groups",
		groupID,
		nil,
		input,
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMinistryAttendanceMark(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input struct {
		Present bool `json:"present"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	groupID := pathUint64(r, "id")
	targetUserID := pathUint64(r, "user_id")
	err := a.ministry.SetAttendance(
		r.Context(),
		studyGroupID,
		groupID,
		targetUserID,
		ministryActor(user),
		r.PathValue("date"),
		input.Present,
		time.Now().UTC(),
	)
	if err != nil {
		a.writeMinistryAttendanceError(w, r, err)
		return
	}
	a.audit(
		studyGroupID,
		user.ID,
		"mark_ministry_attendance",
		"ministry_attendance_records",
		targetUserID,
		nil,
		map[string]any{
			"ministry_group_id": groupID,
			"attendance_date":   r.PathValue("date"),
			"present":           input.Present,
		},
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMinistryAttendanceExport(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	groupID := pathUint64(r, "id")
	sheet, err := a.ministry.AttendanceSheet(
		r.Context(),
		studyGroupID,
		groupID,
		ministryActor(user),
		r.URL.Query().Get("month"),
		a.location,
	)
	if err != nil {
		a.writeMinistryAttendanceError(w, r, err)
		return
	}
	sort.SliceStable(sheet.Members, func(i, j int) bool {
		if sheet.Members[i].PresentCount != sheet.Members[j].PresentCount {
			return sheet.Members[i].PresentCount > sheet.Members[j].PresentCount
		}
		return sheet.Members[i].DisplayName < sheet.Members[j].DisplayName
	})
	var buffer bytes.Buffer
	buffer.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(&buffer)
	header := []string{"成员", "账号"}
	header = append(header, sheet.Dates...)
	header = append(header, "出勤次数")
	if err := writer.Write(safeCSVRow(header...)); err != nil {
		a.logMinistryAttendanceError(r, err)
		writeError(w, http.StatusInternalServerError, "ministry_attendance_export_failed")
		return
	}
	for _, member := range sheet.Members {
		row := []string{member.DisplayName, member.Username}
		for _, date := range sheet.Dates {
			if member.Present[date] {
				row = append(row, "出勤")
			} else {
				row = append(row, "")
			}
		}
		row = append(row, strconv.Itoa(member.PresentCount))
		if err := writer.Write(safeCSVRow(row...)); err != nil {
			a.logMinistryAttendanceError(r, err)
			writeError(w, http.StatusInternalServerError, "ministry_attendance_export_failed")
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		a.logMinistryAttendanceError(r, err)
		writeError(w, http.StatusInternalServerError, "ministry_attendance_export_failed")
		return
	}
	writeAttachmentHeaders(
		w,
		fmt.Sprintf("ministry-attendance-%s.csv", sheet.Month),
		"text/csv; charset=utf-8",
	)
	if _, err := w.Write(buffer.Bytes()); err != nil {
		a.logMinistryAttendanceError(r, err)
	}
}

func (a *app) writeMinistryAttendanceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ministrydomain.ErrInvalidAttendanceDate),
		errors.Is(err, ministrydomain.ErrInvalidAttendanceRule):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ministrydomain.ErrForbidden),
		errors.Is(err, ministrydomain.ErrNotMember):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ministrydomain.ErrGroupNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ministrydomain.ErrAttendanceUnsupported):
		writeError(w, http.StatusConflict, err.Error())
	default:
		a.logMinistryAttendanceError(r, err)
		writeError(w, http.StatusInternalServerError, "ministry_attendance_failed")
	}
}

func (a *app) logMinistryAttendanceError(r *http.Request, err error) {
	user := mustUser(r)
	log.Printf(
		"ministry attendance request failed method=%s path=%s user_id=%d group_id=%d ministry_group_id=%d err=%v",
		r.Method,
		r.URL.Path,
		user.ID,
		user.CurrentGroupID,
		pathUint64(r, "id"),
		err,
	)
}
