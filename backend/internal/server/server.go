package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	assetdomain "agp/backend/internal/asset"
	auditdomain "agp/backend/internal/audit"
	backupdomain "agp/backend/internal/backup"
	checkindomain "agp/backend/internal/checkin"
	learningdomain "agp/backend/internal/learning"
	statisticsdomain "agp/backend/internal/statistics"
	userdomain "agp/backend/internal/user"

	_ "github.com/go-sql-driver/mysql"
)

const (
	roleMember      = "member"
	roleGroupAdmin  = "group_admin"
	roleGroupLeader = "group_leader"
	appTZName       = "Asia/Shanghai"
)

type app struct {
	db            *sql.DB
	secret        []byte
	contentRoot   string
	migrationsDir string
	location      *time.Location
	loginLimiter  *loginLimiter
	audits        *auditdomain.Service
	assets        *assetdomain.Service
	backups       *backupdomain.Service
	checkins      *checkindomain.Service
	learning      *learningdomain.Service
	statistics    *statisticsdomain.Service
	users         *userdomain.Service
}

type config struct {
	Addr                 string
	DSN                  string
	JWTSecret            string
	AssetsRoot           string
	ContentRoot          string
	MigrationsDir        string
	BootstrapUsername    string
	BootstrapPassword    string
	BootstrapDisplayName string
}

type ctxKey string

const currentUserKey ctxKey = "currentUser"

type currentUser = userdomain.UserVO
type group = userdomain.Group

type tokenClaims struct {
	UserID         uint64 `json:"uid"`
	CurrentGroupID uint64 `json:"gid,omitempty"`
	ExpiresAt      int64  `json:"exp"`
}

func Run() error {
	cfg := loadConfig()
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.Ping(); err != nil {
		return err
	}

	loc, err := time.LoadLocation(appTZName)
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	checkinSvc := checkindomain.NewService(checkindomain.NewMySQLRepository(db))
	a := &app{
		db:            db,
		secret:        []byte(cfg.JWTSecret),
		contentRoot:   cfg.ContentRoot,
		migrationsDir: cfg.MigrationsDir,
		location:      loc,
		loginLimiter:  newLoginLimiter(),
		audits:        auditdomain.NewService(auditdomain.NewMySQLRepository(db)),
		backups:       backupdomain.NewService(backupdomain.NewMySQLRepository(db)),
		assets: assetdomain.NewService(
			assetdomain.NewMySQLRepository(db),
			assetdomain.NewLocalStorage(cfg.AssetsRoot, cfg.ContentRoot),
			cfg.ContentRoot,
		),
		checkins: checkinSvc,
		learning: learningdomain.NewService(
			learningdomain.NewMySQLRepository(db),
			nil,
			checkinSvc,
		),
		statistics: statisticsdomain.NewService(statisticsdomain.NewMySQLRepository(db)),
		users:      userdomain.NewService(userdomain.NewMySQLRepository(db)),
	}
	if err := a.runMigrations(); err != nil {
		return err
	}
	if err := a.ensureFuturePartitions(time.Now().In(loc), 2); err != nil {
		log.Printf("partition maintenance failed: %v", err)
	}
	if err := a.bootstrapSuperAdmin(cfg); err != nil {
		return err
	}

	mux := http.NewServeMux()
	a.routes(mux)
	log.Printf("AGP backend listening on %s", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, withCommonHeaders(mux))
}

func loadConfig() config {
	return config{
		Addr:                 env("AGP_ADDR", ":8080"),
		DSN:                  env("AGP_DSN", "agp:agp@tcp(127.0.0.1:3306)/agp?parseTime=true&multiStatements=false&charset=utf8mb4,utf8"),
		JWTSecret:            env("AGP_JWT_SECRET", "dev-secret-change-me"),
		AssetsRoot:           env("AGP_ASSETS_ROOT", "/data/agp/assets"),
		ContentRoot:          env("AGP_CONTENT_ROOT", "/data/agp/content"),
		MigrationsDir:        env("AGP_MIGRATIONS_DIR", "./migrations"),
		BootstrapUsername:    env("BOOTSTRAP_SUPERADMIN_USERNAME", "admin"),
		BootstrapPassword:    env("BOOTSTRAP_SUPERADMIN_PASSWORD", "ChangeMe123"),
		BootstrapDisplayName: env("BOOTSTRAP_SUPERADMIN_DISPLAY_NAME", "超级管理员"),
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func (a *app) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("GET /api/auth/me", a.auth(a.handleMe))
	mux.HandleFunc("POST /api/auth/switch-group", a.auth(a.handleSwitchGroup))
	mux.HandleFunc("POST /api/auth/default-group", a.auth(a.handleSetDefaultGroup))
	mux.HandleFunc("POST /api/auth/change-password", a.auth(a.handleChangePassword))

	mux.HandleFunc("GET /api/app/bootstrap", a.auth(a.handleBootstrap))
	mux.HandleFunc("GET /api/today", a.auth(a.handleToday))
	mux.HandleFunc("GET /api/dashboard/summary", a.auth(a.handleDashboardSummary))
	mux.HandleFunc("GET /api/dashboard/monthly-ranking", a.auth(a.handleDashboardMonthlyRanking))
	mux.HandleFunc("GET /api/members", a.auth(a.handleMembers))
	mux.HandleFunc("GET /api/members/{id}/calendar", a.auth(a.handleMemberCalendar))

	mux.HandleFunc("GET /api/study-weeks", a.auth(a.handleStudyWeeks))
	mux.HandleFunc("GET /api/study-weeks/current", a.auth(a.handleCurrentStudyWeek))
	mux.HandleFunc("POST /api/admin/study-weeks", a.auth(a.requireRole(roleGroupLeader, a.handleAdminCreateStudyWeek)))
	mux.HandleFunc("PUT /api/admin/study-weeks/{id}", a.auth(a.requireRole(roleGroupLeader, a.handleAdminUpdateStudyWeek)))
	mux.HandleFunc("DELETE /api/admin/study-weeks/{id}", a.auth(a.requireRole(roleGroupLeader, a.handleAdminDeleteStudyWeek)))

	mux.HandleFunc("POST /api/checkins", a.auth(a.handleCreateCheckin))
	mux.HandleFunc("DELETE /api/checkins/{id}", a.auth(a.handleDeleteOwnCheckin))
	mux.HandleFunc("GET /api/checkins", a.auth(a.handleListCheckins))
	mux.HandleFunc("DELETE /api/admin/checkins/{id}", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminDeleteCheckin)))

	mux.HandleFunc("GET /api/assets", a.auth(a.handleListAssets))
	mux.HandleFunc("GET /api/library", a.auth(a.handleResourceLibrary))
	mux.HandleFunc("GET /api/assets/{id}/download", a.auth(a.handleDownloadAsset))
	mux.HandleFunc("GET /api/assets/{id}/range", a.auth(a.handleDownloadAssetRange))
	mux.HandleFunc("POST /api/admin/assets/upload", a.auth(a.requireRole(roleGroupLeader, a.handleAdminUploadAsset)))
	mux.HandleFunc("GET /api/admin/resource-library", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminResourceLibrary)))
	mux.HandleFunc("GET /api/admin/learning-config", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminLearningConfig)))
	mux.HandleFunc("PUT /api/admin/learning-config", a.auth(a.requireRole(roleGroupLeader, a.handleAdminSaveLearningConfig)))
	mux.HandleFunc("GET /api/content/pdf-range", a.auth(a.handleStaticPDFRange))
	mux.HandleFunc("GET /api/admin/exports/checkins-detail", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminExportCheckinsCSV)))
	mux.HandleFunc("GET /api/admin/exports/daily-summary", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminExportDailySummaryCSV)))
	mux.HandleFunc("GET /api/admin/exports/study-weeks", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminExportStudyWeeksExcel)))
	mux.HandleFunc("POST /api/admin/imports/study-weeks", a.auth(a.requireRole(roleGroupLeader, a.handleAdminImportStudyWeeksExcel)))
	mux.HandleFunc("GET /api/admin/exports/feedbacks", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminExportFeedbacksCSV)))
	mux.HandleFunc("GET /api/admin/exports/local-backup", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminExportLocalBackupJSON)))
	mux.HandleFunc("POST /api/admin/imports/local-backup", a.auth(a.requireRole(roleGroupLeader, a.handleAdminImportLocalBackupJSON)))
	mux.HandleFunc("POST /api/admin/members", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminCreateMember)))
	mux.HandleFunc("DELETE /api/admin/members/{id}", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminRemoveMember)))
	mux.HandleFunc("PUT /api/admin/group/default-password", a.auth(a.requireRole(roleGroupLeader, a.handleAdminSetGroupDefaultPassword)))
	mux.HandleFunc("POST /api/admin/members/{id}/admins", a.auth(a.requireRole(roleGroupAdmin, a.handleGrantGroupAdmin)))
	mux.HandleFunc("DELETE /api/admin/members/{id}/admins", a.auth(a.requireRole(roleGroupAdmin, a.handleRevokeGroupAdmin)))
	mux.HandleFunc("GET /api/admin/audit-logs", a.auth(a.requireRole(roleGroupAdmin, a.handleAuditLogs)))

	mux.HandleFunc("GET /api/super-admin/groups", a.auth(a.requireSuper(a.handleSuperListGroups)))
	mux.HandleFunc("POST /api/super-admin/groups", a.auth(a.requireSuper(a.handleSuperCreateGroup)))
	mux.HandleFunc("POST /api/super-admin/groups/{id}/default-password", a.auth(a.requireSuper(a.handleSuperSetGroupDefaultPassword)))
	mux.HandleFunc("GET /api/super-admin/users", a.auth(a.requireSuper(a.handleSuperListUsers)))
	mux.HandleFunc("POST /api/super-admin/users", a.auth(a.requireSuper(a.handleSuperCreateUser)))
	mux.HandleFunc("POST /api/super-admin/users/reset-all-passwords", a.auth(a.requireSuper(a.handleSuperResetAllPasswords)))
	mux.HandleFunc("POST /api/super-admin/groups/{id}/members", a.auth(a.requireSuper(a.handleSuperAddGroupMember)))
	mux.HandleFunc("POST /api/super-admin/groups/{id}/leaders", a.auth(a.requireSuper(a.handleSuperSetLeader)))
	mux.HandleFunc("DELETE /api/super-admin/groups/{id}/leaders/{user_id}", a.auth(a.requireSuper(a.handleSuperUnsetLeader)))
}

func withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) runMigrations() error {
	entries, err := os.ReadDir(a.migrationsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(a.migrationsDir, entry.Name()))
		if err != nil {
			return err
		}
		for _, stmt := range splitSQL(string(data)) {
			if _, err := a.db.Exec(stmt); err != nil {
				return fmt.Errorf("%s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

func splitSQL(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ";") {
		stmt := strings.TrimSpace(part)
		if stmt != "" {
			out = append(out, stmt)
		}
	}
	return out
}

func (a *app) ensureFuturePartitions(now time.Time, quartersAhead int) error {
	for i := 0; i <= quartersAhead; i++ {
		start := quarterStart(now).AddDate(0, i*3, 0)
		name := fmt.Sprintf("p%dq%d", start.Year(), int(start.Month()-1)/3+1)
		lessThan := start.AddDate(0, 3, 0).Format("2006-01-02")
		var exists int
		err := a.db.QueryRow(`
			SELECT COUNT(*) FROM information_schema.PARTITIONS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'checkin_records' AND PARTITION_NAME = ?`, name).Scan(&exists)
		if err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE checkin_records REORGANIZE PARTITION pmax INTO (PARTITION %s VALUES LESS THAN ('%s'), PARTITION pmax VALUES LESS THAN (MAXVALUE))", name, lessThan)
		if _, err := a.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func quarterStart(t time.Time) time.Time {
	month := time.Month((int(t.Month())-1)/3*3 + 1)
	return time.Date(t.Year(), month, 1, 0, 0, 0, 0, t.Location())
}

func (a *app) bootstrapSuperAdmin(cfg config) error {
	hash, err := hashPassword(cfg.BootstrapPassword)
	if err != nil {
		return err
	}
	return a.users.EnsureBootstrapSuperAdmin(context.Background(), cfg.BootstrapUsername, cfg.BootstrapDisplayName, hash, time.Now().UTC())
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || json.Unmarshal(body, v) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": code})
}

func nowSQL() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05.000")
}

func nullableID(id uint64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nullableUint64(v sql.NullInt64) any {
	if !v.Valid || v.Int64 <= 0 {
		return nil
	}
	return uint64(v.Int64)
}

func nullableUint64Value(v uint64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableUint64Ptr(v sql.NullInt64) *uint64 {
	if !v.Valid || v.Int64 <= 0 {
		return nil
	}
	value := uint64(v.Int64)
	return &value
}

func nullJSON(b []byte) any {
	if string(b) == "null" || len(b) == 0 {
		return nil
	}
	return string(b)
}

func queryDate(r *http.Request, key string, fallback time.Time) string {
	v := r.URL.Query().Get(key)
	if _, err := time.Parse("2006-01-02", v); err == nil {
		return v
	}
	return fallback.Format("2006-01-02")
}

func queryInt(r *http.Request, key string, fallback int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return v
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func containsGroup(groups []group, id uint64) bool {
	for _, g := range groups {
		if g.ID == id {
			return true
		}
	}
	return false
}

func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

func clientIP(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func mapUint64(m map[string]any, key string) uint64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case uint64:
		return v
	case uint:
		return uint64(v)
	case int:
		if v > 0 {
			return uint64(v)
		}
	case int64:
		if v > 0 {
			return uint64(v)
		}
	case float64:
		if v > 0 {
			return uint64(v)
		}
	}
	return 0
}

func mapBool(m map[string]any, key string, fallback bool) bool {
	if m == nil {
		return fallback
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return fallback
}

func nestedString(root map[string]any, path []string, fallback string) string {
	var current any = root
	for _, key := range path {
		values, ok := current.(map[string]any)
		if !ok {
			return fallback
		}
		current = values[key]
	}
	if value := strings.TrimSpace(asString(current)); value != "" {
		return value
	}
	return fallback
}

func normalizeUsername(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func truncate(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= n {
		return string(rs)
	}
	return string(rs[:n])
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func randomPassword(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "Agp" + strconv.FormatInt(time.Now().Unix()%100000, 10)
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}
