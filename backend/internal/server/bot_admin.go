package server

import (
	"errors"
	"net/http"
	"time"

	notificationdomain "agp/backend/internal/notification"
)

func (a *app) handleBotManagement(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	if a.botManager == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured":   false,
			"robots":       []any{},
			"study_groups": user.Groups,
		})
		return
	}
	robots := a.botManager.Robots(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":   len(robots) > 0,
		"robots":       robots,
		"study_groups": user.Groups,
	})
}

func (a *app) handleBotRobot(w http.ResponseWriter, r *http.Request) {
	if a.botManager == nil {
		writeError(w, http.StatusServiceUnavailable, "bot_not_configured")
		return
	}
	var req struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	status, err := a.botManager.Register(r.Context(), notificationdomain.RobotRegistration{
		ID: req.ID, Name: req.Name, Token: req.Token,
	})
	if err != nil {
		switch {
		case errors.Is(err, notificationdomain.ErrInvalidRobotConfig):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, notificationdomain.ErrRobotAuthentication):
			writeError(w, http.StatusBadGateway, err.Error())
		case errors.Is(err, notificationdomain.ErrRobotAlreadyExists),
			errors.Is(err, notificationdomain.ErrRobotTokenExists),
			errors.Is(err, notificationdomain.ErrRobotLimitExceeded):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "bot_robot_save_failed")
		}
		return
	}
	user := mustUser(r)
	a.audit(0, user.ID, "create_bot_robot", "potato_robot", 0,
		nil,
		map[string]any{"robot_id": status.ID, "name": status.Name},
		r,
	)
	writeJSON(w, http.StatusCreated, map[string]any{"robot": status})
}

func (a *app) handleBotRobotDelete(w http.ResponseWriter, r *http.Request) {
	if a.botManager == nil { writeError(w, http.StatusServiceUnavailable, "bot_not_configured"); return }
	if err := a.botManager.Remove(r.PathValue("id")); err != nil {
		if errors.Is(err, notificationdomain.ErrRobotNotFound) { writeError(w, http.StatusNotFound, err.Error()); return }
		if errors.Is(err, notificationdomain.ErrRobotCannotRemove) { writeError(w, http.StatusBadRequest, err.Error()); return }
		writeError(w, http.StatusInternalServerError, "bot_robot_delete_failed"); return
	}
	user := mustUser(r)
	a.audit(0, user.ID, "delete_bot_robot", "potato_robot", 0, nil, map[string]any{"robot_id": r.PathValue("id")}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleBotBinding(w http.ResponseWriter, r *http.Request) {
	if a.botManager == nil {
		writeError(w, http.StatusServiceUnavailable, "bot_not_configured")
		return
	}
	var req struct {
		RobotID  string `json:"robot_id"`
		ChatID   int64  `json:"chat_id"`
		ChatType int    `json:"chat_type"`
		GroupID  uint64 `json:"group_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.RobotID == "" {
		req.RobotID = "default"
	}
	if req.ChatID <= 0 || (req.ChatType != 2 && req.ChatType != 3) {
		writeError(w, http.StatusBadRequest, "invalid_bot_chat")
		return
	}
	user := mustUser(r)
	if req.GroupID > 0 {
		found := false
		for _, group := range user.Groups {
			if group.ID == req.GroupID {
				found = true
				break
			}
		}
		if !found {
			writeError(w, http.StatusBadRequest, "study_group_not_found")
			return
		}
	}
	previousGroupID := a.botManager.BindingGroupID(req.RobotID, req.ChatID)
	target := notificationdomain.Target{ChatID: req.ChatID, ChatType: req.ChatType}
	if err := a.botManager.Assign(r.Context(), req.RobotID, target, req.GroupID, time.Now().UTC()); err != nil {
		if errors.Is(err, notificationdomain.ErrRobotNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, notificationdomain.ErrChatNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, notificationdomain.ErrRobotAuthentication) {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "bot_binding_save_failed")
		return
	}
	a.audit(0, user.ID, "save_bot_binding", "potato_chat", uint64(req.ChatID),
		map[string]any{"robot_id": req.RobotID, "group_id": previousGroupID},
		map[string]any{"robot_id": req.RobotID, "group_id": req.GroupID, "chat_type": req.ChatType},
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
