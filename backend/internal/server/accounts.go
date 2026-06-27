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
	"math"
	"net/http"
	"strings"
	"time"

	auditdomain "agp/backend/internal/audit"
)

func (a *app) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		claims, err := a.verifyToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		u, err := a.loadCurrentUser(claims.UserID, claims.CurrentGroupID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), currentUserKey, u)))
	}
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

func (a *app) loadCurrentUser(userID, currentGroupID uint64) (currentUser, error) {
	return a.users.CurrentUser(context.Background(), userID, currentGroupID)
}

func (a *app) visibleGroups(userID uint64, isSuperAdmin bool) ([]group, error) {
	return a.users.VisibleGroups(context.Background(), userID, isSuperAdmin)
}

func (a *app) allGroups() ([]group, error) {
	return a.users.VisibleGroups(context.Background(), 0, true)
}

func (a *app) userGroups(userID uint64) ([]group, error) {
	return a.users.VisibleGroups(context.Background(), userID, false)
}

func (a *app) userRoles(userID, groupID uint64) ([]string, error) {
	return a.users.Roles(context.Background(), userID, groupID)
}

func requireGroupID(w http.ResponseWriter, u currentUser) uint64 {
	if u.CurrentGroupID == 0 {
		writeError(w, http.StatusBadRequest, "group_required")
		return 0
	}
	return u.CurrentGroupID
}

func (a *app) listMembers(groupID uint64) ([]map[string]any, error) {
	members, err := a.users.Members(context.Background(), groupID)
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

func (a *app) groupLearningConfig(groupID uint64) (map[string]any, error) {
	return a.learning.LearningConfig(context.Background(), groupID)
}

func (a *app) upsertGroupLearningConfig(groupID uint64, settings map[string]any) error {
	return a.learning.SaveLearningConfig(context.Background(), groupID, settings)
}

func (a *app) setGroupDefaultPassword(groupID uint64, password string, includeLeaders bool, actorID uint64, r *http.Request) (int64, error) {
	if len(password) < 8 {
		return 0, errors.New("password_too_short")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return 0, err
	}
	affected, err := a.users.SetGroupDefaultPassword(context.Background(), groupID, hash, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	a.audit(groupID, actorID, "set_group_default_password", "study_groups", groupID, nil, map[string]any{"affected_users": affected}, r)
	return affected, nil
}

func (a *app) groupDefaultPasswordHash(groupID uint64) (string, error) {
	return a.users.GroupDefaultPasswordHash(context.Background(), groupID)
}

func (a *app) createUserWithHash(username, displayName, namePinyin, hash string, isSuper bool, actorID uint64) (uint64, error) {
	return a.users.CreateUserWithHash(context.Background(), username, displayName, namePinyin, hash, isSuper, actorID, time.Now().UTC())
}

func (a *app) addMember(groupID, userID uint64, memberName string, actorID uint64) error {
	return a.users.AddMember(context.Background(), groupID, userID, memberName, actorID, time.Now().UTC())
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
	failures map[string]loginFailure
}

type loginFailure struct {
	Count     int
	BlockedTo time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: map[string]loginFailure{}}
}

func (l *loginLimiter) key(ip, username string) string {
	return ip + "|" + username
}

func (l *loginLimiter) blocked(ip, username string) bool {
	item := l.failures[l.key(ip, username)]
	return item.BlockedTo.After(time.Now())
}

func (l *loginLimiter) fail(ip, username string) {
	key := l.key(ip, username)
	item := l.failures[key]
	item.Count++
	if item.Count >= 8 {
		item.BlockedTo = time.Now().Add(10 * time.Minute)
	}
	l.failures[key] = item
}

func (l *loginLimiter) success(ip, username string) {
	delete(l.failures, l.key(ip, username))
}
