package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	assetdomain "agp/backend/internal/asset"
	ministrydomain "agp/backend/internal/ministry"
)

const ministryUploadLimit = int64(32 << 20)

func (a *app) handleMinistryGroups(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	groups, err := a.ministry.Groups(r.Context(), studyGroupID, ministryActor(user))
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (a *app) handleMinistryGroup(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	groupID := pathUint64(r, "id")
	detail, err := a.ministry.Detail(
		r.Context(),
		studyGroupID,
		groupID,
		ministryActor(user),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *app) handleMinistryJoinRequest(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input struct {
		Message string `json:"message"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	groupID := pathUint64(r, "id")
	requestID, err := a.ministry.RequestJoin(
		r.Context(),
		studyGroupID,
		groupID,
		ministryActor(user),
		input.Message,
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	a.audit(
		studyGroupID,
		user.ID,
		"request_ministry_join",
		"ministry_group_requests",
		requestID,
		nil,
		map[string]any{"ministry_group_id": groupID},
		r,
	)
	writeJSON(w, http.StatusCreated, map[string]any{"id": requestID, "status": "pending"})
}

func (a *app) handleMinistryLeave(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	groupID := pathUint64(r, "id")
	err := a.ministry.Leave(
		r.Context(),
		studyGroupID,
		groupID,
		ministryActor(user),
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	a.audit(
		studyGroupID,
		user.ID,
		"leave_ministry_group",
		"ministry_groups",
		groupID,
		nil,
		map[string]any{"user_id": user.ID},
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMinistryIdentity(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input struct {
		Public bool `json:"public"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	err := a.ministry.SetIdentityPublic(
		r.Context(),
		studyGroupID,
		pathUint64(r, "id"),
		ministryActor(user),
		input.Public,
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMinistrySettings(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input ministrydomain.GroupSettingsInput
	if !readJSON(w, r, &input) {
		return
	}
	groupID := pathUint64(r, "id")
	err := a.ministry.UpdateSettings(
		r.Context(),
		studyGroupID,
		groupID,
		ministryActor(user),
		input,
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	a.audit(
		studyGroupID,
		user.ID,
		"update_ministry_settings",
		"ministry_groups",
		groupID,
		nil,
		input,
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMinistryMemberRole(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input struct {
		Role ministrydomain.MemberRole `json:"role"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	groupID := pathUint64(r, "id")
	targetUserID := pathUint64(r, "user_id")
	err := a.ministry.SetMemberRole(
		r.Context(),
		studyGroupID,
		groupID,
		targetUserID,
		ministryActor(user),
		input.Role,
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	a.audit(
		studyGroupID,
		user.ID,
		"set_ministry_member_role",
		"ministry_group_members",
		targetUserID,
		nil,
		map[string]any{"ministry_group_id": groupID, "role": input.Role},
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMinistryRequests(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	requests, err := a.ministry.PendingRequests(
		r.Context(),
		studyGroupID,
		ministryActor(user),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": requests})
}

func (a *app) handleMinistryRequestDecision(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input struct {
		Decision ministrydomain.Status `json:"decision"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	requestID := pathUint64(r, "id")
	err := a.ministry.DecideRequest(
		r.Context(),
		studyGroupID,
		requestID,
		ministryActor(user),
		input.Decision,
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	a.audit(
		studyGroupID,
		user.ID,
		"decide_ministry_request",
		"ministry_group_requests",
		requestID,
		nil,
		map[string]any{"decision": input.Decision},
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMinistryNotifications(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	notifications, err := a.ministry.Notifications(
		r.Context(),
		studyGroupID,
		ministryActor(user),
		100,
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": notifications})
}

func (a *app) handleMinistryNotificationRead(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	err := a.ministry.ReadNotification(
		r.Context(),
		studyGroupID,
		pathUint64(r, "id"),
		ministryActor(user),
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMinistryCreateShare(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input ministrydomain.ShareInput
	if !readJSON(w, r, &input) {
		return
	}
	shareID, err := a.ministry.CreateShare(
		r.Context(),
		studyGroupID,
		pathUint64(r, "id"),
		ministryActor(user),
		input,
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": shareID})
}

func (a *app) handleMinistryUpdateShare(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input ministrydomain.ShareInput
	if !readJSON(w, r, &input) {
		return
	}
	err := a.ministry.UpdateShare(
		r.Context(),
		studyGroupID,
		pathUint64(r, "id"),
		pathUint64(r, "share_id"),
		ministryActor(user),
		input,
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMinistryShareDecision(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input struct {
		Decision ministrydomain.Status `json:"decision"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	shareID := pathUint64(r, "share_id")
	err := a.ministry.DecideShare(
		r.Context(),
		studyGroupID,
		pathUint64(r, "id"),
		shareID,
		ministryActor(user),
		input.Decision,
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	a.audit(
		studyGroupID,
		user.ID,
		"decide_ministry_share",
		"ministry_shares",
		shareID,
		nil,
		map[string]any{"decision": input.Decision},
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMinistryCreateProgress(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	var input ministrydomain.ProgressInput
	if !readJSON(w, r, &input) {
		return
	}
	progressID, err := a.ministry.CreateProgress(
		r.Context(),
		studyGroupID,
		pathUint64(r, "id"),
		ministryActor(user),
		input,
		time.Now().UTC(),
	)
	if err != nil {
		writeMinistryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": progressID})
}

func (a *app) handleMinistryAttachment(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	studyGroupID := requireGroupID(w, user)
	if studyGroupID == 0 {
		return
	}
	groupID := pathUint64(r, "id")
	if err := a.ministry.CanContribute(
		r.Context(),
		studyGroupID,
		groupID,
		ministryActor(user),
	); err != nil {
		writeMinistryError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, ministryUploadLimit)
	if err := r.ParseMultipartForm(ministryUploadLimit); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "ministry_attachment_too_large")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required")
		return
	}
	defer file.Close()
	item, err := a.assets.Upload(r.Context(), assetdomain.UploadRequest{
		GroupID:  studyGroupID,
		ActorID:  user.ID,
		Category: fmt.Sprintf("ministry-%d", groupID),
		FileName: header.Filename,
		Reader:   file,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ministry_attachment_failed")
		return
	}
	a.audit(
		studyGroupID,
		user.ID,
		"upload_ministry_attachment",
		"assets",
		item.ID,
		nil,
		map[string]any{"ministry_group_id": groupID, "title": item.Title},
		r,
	)
	writeJSON(w, http.StatusCreated, map[string]any{"asset": item})
}

func ministryActor(user currentUser) ministrydomain.Actor {
	return ministrydomain.Actor{
		UserID:       user.ID,
		IsSuperAdmin: user.IsSuperAdmin,
		IsStudyAdmin: user.IsSuperAdmin ||
			hasRole(user.Roles, roleGroupAdmin) ||
			hasRole(user.Roles, roleGroupLeader),
	}
}

func pathUint64(r *http.Request, name string) uint64 {
	value, _ := strconv.ParseUint(strings.TrimSpace(r.PathValue(name)), 10, 64)
	return value
}

func writeMinistryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ministrydomain.ErrContentRequired),
		errors.Is(err, ministrydomain.ErrInvalidDecision),
		errors.Is(err, ministrydomain.ErrInvalidVisibility),
		errors.Is(err, ministrydomain.ErrInvalidRole),
		errors.Is(err, ministrydomain.ErrInvalidAttachment):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ministrydomain.ErrForbidden),
		errors.Is(err, ministrydomain.ErrNotMember),
		errors.Is(err, ministrydomain.ErrLeaderCannotLeave):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ministrydomain.ErrGroupNotFound),
		errors.Is(err, ministrydomain.ErrRequestNotFound),
		errors.Is(err, ministrydomain.ErrShareNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ministrydomain.ErrAlreadyMember),
		errors.Is(err, ministrydomain.ErrRequestAlreadyReviewed),
		errors.Is(err, ministrydomain.ErrShareAlreadyReviewed):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "ministry_failed")
	}
}
