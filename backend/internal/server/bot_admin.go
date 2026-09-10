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
			"chats":        []any{},
			"study_groups": user.Groups,
		})
		return
	}
	chats, err := a.botManager.Chats(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "bot_groups_failed")
		return
	}
	boundGroups := make(map[int64]uint64)
	for _, binding := range a.botManager.Bindings() {
		boundGroups[binding.ChatID] = binding.GroupID
	}
	type chatResponse struct {
		notificationdomain.Chat
		GroupID uint64 `json:"group_id"`
	}
	items := make([]chatResponse, 0, len(chats))
	for _, chat := range chats {
		items = append(items, chatResponse{Chat: chat, GroupID: boundGroups[chat.ChatID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":   true,
		"chats":        items,
		"study_groups": user.Groups,
	})
}

func (a *app) handleBotBinding(w http.ResponseWriter, r *http.Request) {
	if a.botManager == nil {
		writeError(w, http.StatusServiceUnavailable, "bot_not_configured")
		return
	}
	var req struct {
		ChatID   int64  `json:"chat_id"`
		ChatType int    `json:"chat_type"`
		GroupID  uint64 `json:"group_id"`
	}
	if !readJSON(w, r, &req) {
		return
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
	var previousGroupID uint64
	for _, binding := range a.botManager.Bindings() {
		if binding.ChatID == req.ChatID {
			previousGroupID = binding.GroupID
			break
		}
	}
	target := notificationdomain.Target{ChatID: req.ChatID, ChatType: req.ChatType}
	if err := a.botManager.Assign(r.Context(), target, req.GroupID, time.Now().UTC()); err != nil {
		if errors.Is(err, notificationdomain.ErrChatNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "bot_binding_save_failed")
		return
	}
	a.audit(0, user.ID, "save_bot_binding", "potato_chat", uint64(req.ChatID),
		map[string]any{"group_id": previousGroupID},
		map[string]any{"group_id": req.GroupID, "chat_type": req.ChatType},
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
