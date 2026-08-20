package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	auditdomain "agp/backend/internal/audit"
	userdomain "agp/backend/internal/user"
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
		u, err := a.loadCurrentUser(r.Context(), claims.UserID, claims.CurrentGroupID)
		if err != nil {
			logAuthFailure(r, "load_current_user", err)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), currentUserKey, u)))
	}
}

func logAuthFailure(r *http.Request, reason string, err error) {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	log.Printf(
		"auth failed method=%s path=%s reason=%s authorization_header_present=%t referer=%q user_agent=%q client_ip=%s err=%s",
		r.Method,
		r.URL.Path,
		reason,
		r.Header.Get("Authorization") != "",
		r.Referer(),
		r.UserAgent(),
		clientIP(r),
		errText,
	)
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
	return a.learning.SaveLearningConfig(ctx, groupID, settings)
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
	_ = a.audits.Create(r.Context(), auditdomain.CreateLogInput{
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
	_ = a.users.RecordLoginLog(r.Context(), input, time.Now().UTC())
}
