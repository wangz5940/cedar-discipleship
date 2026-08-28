package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	auditdomain "agp/backend/internal/audit"
	userdomain "agp/backend/internal/user"
)

const (
	refreshCookieName = "agp_refresh"
	csrfCookieName    = "agp_csrf"
	csrfHeaderName    = "X-CSRF-Token"
)

func (a *app) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			logAuthFailure(r, "missing_bearer_token", nil)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		claims, err := a.verifyToken(token)
		if err != nil {
			logAuthFailure(r, "verify_token", err)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if claims.SessionID > 0 {
			active, err := a.activeRefreshSession(r.Context(), claims.SessionID, claims.UserID)
			if err != nil || !active {
				logAuthFailure(r, "refresh_session_inactive", err)
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		u, err := a.loadCurrentUser(r.Context(), claims.UserID, claims.CurrentGroupID)
		if err != nil {
			logAuthFailure(r, "load_current_user", err)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		auditState := &requestAuditState{}
		ctx := context.WithValue(r.Context(), currentUserKey, u)
		ctx = context.WithValue(ctx, requestAuditStateKey, auditState)
		authenticatedRequest := r.WithContext(ctx)
		next(w, authenticatedRequest)
		if !auditState.recorded {
			logUserOperation(authenticatedRequest, w, u)
		}
	}
}

func logAuthFailure(r *http.Request, reason string, err error) {
	attrs := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"reason", reason,
		"authorization_header_present", r.Header.Get("Authorization") != "",
		"client_ip", clientIP(r),
	}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	slog.WarnContext(r.Context(), "auth failed", attrs...)
}

func (a *app) requireSuper(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !mustUser(r).IsSuperAdmin {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r)
	}
}

func (a *app) requireRole(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := mustUser(r)
		if u.IsSuperAdmin || hasRole(u.Roles, role) || (role == roleGroupAdmin && hasRole(u.Roles, roleGroupLeader)) {
			next(w, r)
			return
		}
		writeError(w, http.StatusForbidden, "forbidden")
	}
}

func mustUser(r *http.Request) currentUser {
	return r.Context().Value(currentUserKey).(currentUser)
}

func (a *app) loadCurrentUser(ctx context.Context, userID, currentGroupID uint64) (currentUser, error) {
	return a.users.CurrentUser(ctx, userID, currentGroupID)
}

func requireGroupID(w http.ResponseWriter, u currentUser) uint64 {
	if u.CurrentGroupID == 0 {
		writeError(w, http.StatusBadRequest, "group_required")
		return 0
	}
	return u.CurrentGroupID
}

func (a *app) listMembers(ctx context.Context, groupID uint64) ([]map[string]any, error) {
	members, err := a.users.Members(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(members))
	for _, member := range members {
		out = append(out, map[string]any{
			"member_id":      member.MemberID,
			"user_id":        member.UserID,
			"username":       member.Username,
			"display_name":   member.DisplayName,
			"member_name":    member.MemberName,
			"is_super_admin": member.IsSuperAdmin,
			"roles":          member.Roles,
		})
	}
	return out, nil
}

func (a *app) groupLearningConfig(ctx context.Context, groupID uint64) (map[string]any, error) {
	return a.learning.LearningConfig(ctx, groupID)
}

func (a *app) upsertGroupLearningConfig(ctx context.Context, groupID uint64, settings map[string]any) error {
	if err := a.learning.SaveLearningConfig(ctx, groupID, settings); err != nil {
		return err
	}
	a.refreshTodayContent(groupID)
	return nil
}

func (a *app) setGroupDefaultPassword(groupID uint64, password string, includeLeaders bool, actorID uint64, r *http.Request) (int64, error) {
	if len(password) < 8 {
		return 0, errors.New("password_too_short")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return 0, err
	}
	affected, err := a.users.SetGroupDefaultPassword(r.Context(), groupID, hash, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	a.audit(groupID, actorID, "set_group_default_password", "study_groups", groupID, nil, map[string]any{"affected_users": affected}, r)
	return affected, nil
}

func (a *app) groupDefaultPasswordHash(ctx context.Context, groupID uint64) (string, error) {
	return a.users.GroupDefaultPasswordHash(ctx, groupID)
}

func (a *app) addMember(ctx context.Context, groupID, userID uint64, memberName string, actorID uint64) error {
	return a.users.AddMember(ctx, groupID, userID, memberName, actorID, time.Now().UTC())
}

func (a *app) audit(groupID, actorID uint64, action, targetType string, targetID uint64, before, after any, r *http.Request) {
	err := a.audits.Create(r.Context(), auditdomain.CreateLogInput{
		GroupID:    groupID,
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Before:     before,
		After:      after,
		IP:         clientIP(r),
		UserAgent:  r.UserAgent(),
	}, time.Now())
	if err != nil {
		slog.ErrorContext(
			r.Context(),
			"audit log write failed",
			"action", action,
			"target_type", targetType,
			"target_id", targetID,
			"actor_id", actorID,
			"group_id", groupID,
			"error", err,
		)
		return
	}
	if state, ok := r.Context().Value(requestAuditStateKey).(*requestAuditState); ok {
		state.recorded = true
	}
	actor, _ := r.Context().Value(currentUserKey).(currentUser)
	attrs := []any{
		"actor_user_id", actorID,
		"actor_username", actor.Username,
		"actor_display_name", actor.DisplayName,
		"group_id", groupID,
		"action", action,
		"target_type", targetType,
		"target_id", targetID,
		"client_ip", clientIP(r),
	}
	slog.InfoContext(r.Context(), "audit event recorded", attrs...)
}

func (a *app) signToken(c tokenClaims) (string, error) {
	if c.ExpiresAt == 0 && a.tokenTTL > 0 {
		c.ExpiresAt = time.Now().Add(a.tokenTTL).Unix()
	}
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	body64 := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(body64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body64 + "." + sig, nil
}

func (a *app) verifyToken(token string) (tokenClaims, error) {
	var c tokenClaims
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return c, errors.New("invalid_token")
	}
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, got) {
		return c, errors.New("invalid_token")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(body, &c); err != nil {
		return c, err
	}
	if c.ExpiresAt > 0 && c.ExpiresAt < time.Now().Unix() {
		return c, errors.New("expired")
	}
	return c, nil
}

type refreshSession struct {
	ID             uint64
	UserID         uint64
	CurrentGroupID uint64
	ExpiresAt      time.Time
}

func (a *app) issueRefreshSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID, currentGroupID uint64) (uint64, error) {
	refreshToken, err := randomURLToken(32)
	if err != nil {
		return 0, err
	}
	csrfToken, err := randomURLToken(32)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(a.effectiveRefreshTTL())
	res, err := a.db.ExecContext(ctx, `INSERT INTO refresh_sessions
		(user_id,token_hash,csrf_hash,current_group_id,expires_at,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?)`,
		userID, tokenHash(refreshToken), tokenHash(csrfToken), nullableUint64SQL(currentGroupID), expiresAt, now, now)
	if err != nil {
		return 0, err
	}
	sessionID, err := insertedID(res)
	if err != nil {
		return 0, err
	}
	setAuthCookies(w, r, refreshToken, csrfToken, expiresAt)
	return sessionID, nil
}

func (a *app) refreshSession(ctx context.Context, token, csrf string) (refreshSession, error) {
	var session refreshSession
	now := time.Now().UTC()
	err := a.db.QueryRowContext(ctx, `SELECT id,user_id,COALESCE(current_group_id,0),expires_at
		FROM refresh_sessions
		WHERE token_hash=? AND csrf_hash=? AND revoked_at IS NULL AND expires_at>?`,
		tokenHash(token), tokenHash(csrf), now).Scan(&session.ID, &session.UserID, &session.CurrentGroupID, &session.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session, errors.New("invalid_refresh_session")
		}
		return session, err
	}
	session.ExpiresAt = now.Add(a.effectiveRefreshTTL())
	if _, err := a.db.ExecContext(ctx, `UPDATE refresh_sessions
		SET expires_at=?,last_used_at=?,updated_at=?
		WHERE token_hash=? AND revoked_at IS NULL`, session.ExpiresAt, now, now, tokenHash(token)); err != nil {
		return session, err
	}
	return session, nil
}

func (a *app) activeRefreshSession(ctx context.Context, sessionID, userID uint64) (bool, error) {
	var exists int
	err := a.db.QueryRowContext(ctx, `SELECT 1
		FROM refresh_sessions
		WHERE id=? AND user_id=? AND revoked_at IS NULL AND expires_at>?`,
		sessionID, userID, time.Now().UTC()).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *app) revokeRefreshSession(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	now := time.Now().UTC()
	_, err := a.db.ExecContext(ctx, `UPDATE refresh_sessions SET revoked_at=?,updated_at=? WHERE token_hash=? AND revoked_at IS NULL`, now, now, tokenHash(token))
	return err
}

func (a *app) updateRefreshSessionGroup(ctx context.Context, r *http.Request, groupID uint64) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return
	}
	now := time.Now().UTC()
	if _, err := a.db.ExecContext(ctx, `UPDATE refresh_sessions
		SET current_group_id=?,updated_at=?
		WHERE token_hash=? AND revoked_at IS NULL AND expires_at>?`,
		nullableUint64SQL(groupID), now, tokenHash(cookie.Value), now); err != nil {
		slog.WarnContext(ctx, "refresh session group update failed", "error", err)
	}
}

func refreshCredentials(r *http.Request) (string, string, error) {
	refreshCookie, err := r.Cookie(refreshCookieName)
	if err != nil || strings.TrimSpace(refreshCookie.Value) == "" {
		return "", "", errors.New("refresh_cookie_missing")
	}
	csrfCookie, err := r.Cookie(csrfCookieName)
	if err != nil || strings.TrimSpace(csrfCookie.Value) == "" {
		return "", "", errors.New("csrf_cookie_missing")
	}
	csrfHeader := strings.TrimSpace(r.Header.Get(csrfHeaderName))
	if csrfHeader == "" || !hmac.Equal([]byte(csrfHeader), []byte(csrfCookie.Value)) {
		return "", "", errors.New("csrf_mismatch")
	}
	return refreshCookie.Value, csrfHeader, nil
}

func setAuthCookies(w http.ResponseWriter, r *http.Request, refreshToken, csrfToken string, expiresAt time.Time) {
	secure := secureCookie(r)
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    refreshToken,
		Path:     "/api/auth",
		Expires:  expiresAt,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    csrfToken,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   maxAge,
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	secure := secureCookie(r)
	expired := time.Unix(0, 0)
	for _, cookie := range []http.Cookie{
		{Name: refreshCookieName, Path: "/api/auth", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode},
		{Name: csrfCookieName, Path: "/", HttpOnly: false, Secure: secure, SameSite: http.SameSiteStrictMode},
	} {
		cookie.Value = ""
		cookie.Expires = expired
		cookie.MaxAge = -1
		http.SetCookie(w, &cookie)
	}
}

func secureCookie(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *app) effectiveRefreshTTL() time.Duration {
	if a.refreshTTL > 0 {
		return a.refreshTTL
	}
	return 8760 * time.Hour
}

func randomURLToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func insertedID(result sql.Result) (uint64, error) {
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return uint64(id), nil
}

func nullableUint64SQL(id uint64) any {
	if id == 0 {
		return nil
	}
	return id
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := pbkdf2Key([]byte(password), salt, 120000, 32, sha256.New)
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(dk), nil
}

func verifyPassword(password, stored string) bool {
	parts := strings.Split(stored, ":")
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	got := pbkdf2Key([]byte(password), salt, 120000, len(want), sha256.New)
	return hmac.Equal(want, got)
}

func pbkdf2Key(password, salt []byte, iter, keyLen int, h func() hash.Hash) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := int(math.Ceil(float64(keyLen) / float64(hashLen)))
	var dk []byte
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		prf.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := prf.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for x := range t {
				t[x] ^= u[x]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:keyLen]
}

type loginLimiter struct {
	mu       sync.Mutex
	failures map[string]loginFailure
}

type loginFailure struct {
	Count     int
	BlockedTo time.Time
	LastSeen  time.Time
}

const maxLoginFailureEntries = 10_000
const loginFailureTTL = 10 * time.Minute

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: map[string]loginFailure{}}
}

func (l *loginLimiter) key(ip, username string) string {
	return ip + "|" + username
}

func (l *loginLimiter) blocked(ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := l.key(ip, username)
	item, ok := l.failures[key]
	if !ok {
		return false
	}
	now := time.Now()
	if !item.BlockedTo.After(now) && now.Sub(item.LastSeen) >= loginFailureTTL {
		delete(l.failures, key)
		return false
	}
	return item.BlockedTo.After(now)
}

func (l *loginLimiter) fail(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	key := l.key(ip, username)
	item, exists := l.failures[key]
	if exists && !item.BlockedTo.After(now) && now.Sub(item.LastSeen) >= loginFailureTTL {
		item = loginFailure{}
	}
	if !exists && len(l.failures) >= maxLoginFailureEntries {
		l.evictOneLocked(now)
	}
	item.Count++
	item.LastSeen = now
	if item.Count >= 8 {
		item.BlockedTo = now.Add(loginFailureTTL)
	}
	l.failures[key] = item
}

func (l *loginLimiter) evictOneLocked(now time.Time) {
	for key, item := range l.failures {
		if !item.BlockedTo.After(now) && now.Sub(item.LastSeen) >= loginFailureTTL {
			delete(l.failures, key)
			return
		}
	}
	for key := range l.failures {
		delete(l.failures, key)
		return
	}
}

func (l *loginLimiter) success(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, l.key(ip, username))
}

func (a *app) recordLoginLog(r *http.Request, input userdomain.LoginLog) {
	input.IP = clientIP(r)
	input.UserAgent = r.UserAgent()
	if err := a.users.RecordLoginLog(r.Context(), input, time.Now().UTC()); err != nil {
		slog.ErrorContext(r.Context(), "login audit write failed", "username", input.Username, "error", err)
		return
	}
	attrs := []any{
		"actor_user_id", input.UserID,
		"actor_username", input.Username,
		"group_id", input.GroupID,
		"success", input.Success,
		"client_ip", input.IP,
	}
	if input.FailureReason != "" {
		attrs = append(attrs, "failure_reason", input.FailureReason)
	}
	if input.Success {
		slog.InfoContext(r.Context(), "user login", attrs...)
		return
	}
	slog.WarnContext(r.Context(), "user login failed", attrs...)
}
