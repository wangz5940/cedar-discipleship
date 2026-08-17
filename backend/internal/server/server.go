package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	assetdomain "agp/backend/internal/asset"
	auditdomain "agp/backend/internal/audit"
	backupdomain "agp/backend/internal/backup"
	checkindomain "agp/backend/internal/checkin"
	learningdomain "agp/backend/internal/learning"
	ministrydomain "agp/backend/internal/ministry"
	statisticsdomain "agp/backend/internal/statistics"
	userdomain "agp/backend/internal/user"

	_ "github.com/go-sql-driver/mysql"
)

const (
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
	tokenTTL      time.Duration
	loginLimiter  *loginLimiter
	audits        *auditdomain.Service
	assets        *assetdomain.Service
	backups       *backupdomain.Service
	checkins      *checkindomain.Service
	learning      *learningdomain.Service
	ministry      *ministrydomain.Service
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
	TokenTTL             string
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
	if err := validateConfig(cfg); err != nil {
		return err
	}
	tokenTTL, err := parseTokenTTL(cfg.TokenTTL)
	if err != nil {
		return err
	}
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
		tokenTTL:      tokenTTL,
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
		ministry:   ministrydomain.NewService(ministrydomain.NewMySQLRepository(db)),
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
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           withCommonHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	return server.ListenAndServe()
}

func loadConfig() config {
	return config{
		Addr:                 env("AGP_ADDR", ":8080"),
		DSN:                  env("AGP_DSN", "agp:agp@tcp(127.0.0.1:3306)/agp?parseTime=true&multiStatements=false&charset=utf8mb4,utf8"),
		JWTSecret:            env("AGP_JWT_SECRET", ""),
		AssetsRoot:           env("AGP_ASSETS_ROOT", "/data/agp/assets"),
		ContentRoot:          env("AGP_CONTENT_ROOT", "/data/agp/content"),
		MigrationsDir:        env("AGP_MIGRATIONS_DIR", "./migrations"),
		BootstrapUsername:    env("BOOTSTRAP_SUPERADMIN_USERNAME", "admin"),
		BootstrapPassword:    env("BOOTSTRAP_SUPERADMIN_PASSWORD", ""),
		BootstrapDisplayName: env("BOOTSTRAP_SUPERADMIN_DISPLAY_NAME", "超级管理员"),
		TokenTTL:             env("AGP_TOKEN_TTL", ""),
	}
}

func validateConfig(cfg config) error {
	if len(cfg.JWTSecret) < 32 {
		return errors.New("AGP_JWT_SECRET must be at least 32 characters")
	}
	if len(cfg.BootstrapPassword) < 8 {
		return errors.New("BOOTSTRAP_SUPERADMIN_PASSWORD must be at least 8 characters")
	}
	if _, err := parseTokenTTL(cfg.TokenTTL); err != nil {
		return err
	}
	return nil
}

func parseTokenTTL(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "", "0", "permanent", "never":
		return 0, nil
	}
	ttl, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid AGP_TOKEN_TTL %q: %w", value, err)
	}
	if ttl < 0 {
		return 0, errors.New("AGP_TOKEN_TTL must not be negative")
	}
	return ttl, nil
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

	mux.HandleFunc("GET /api/ministry-groups", a.auth(a.handleMinistryGroups))
	mux.HandleFunc("GET /api/ministry-groups/{id}", a.auth(a.handleMinistryGroup))
	mux.HandleFunc("POST /api/ministry-groups/{id}/join-request", a.auth(a.handleMinistryJoinRequest))
	mux.HandleFunc("POST /api/ministry-groups/{id}/leave", a.auth(a.handleMinistryLeave))
	mux.HandleFunc("PUT /api/ministry-groups/{id}/identity", a.auth(a.handleMinistryIdentity))
	mux.HandleFunc("PUT /api/ministry-groups/{id}/settings", a.auth(a.handleMinistrySettings))
	mux.HandleFunc("PUT /api/ministry-groups/{id}/members/{user_id}/role", a.auth(a.handleMinistryMemberRole))
	mux.HandleFunc("GET /api/ministry-requests", a.auth(a.handleMinistryRequests))
	mux.HandleFunc("POST /api/ministry-requests/{id}/decision", a.auth(a.handleMinistryRequestDecision))
	mux.HandleFunc("GET /api/ministry-notifications", a.auth(a.handleMinistryNotifications))
	mux.HandleFunc("POST /api/ministry-notifications/{id}/read", a.auth(a.handleMinistryNotificationRead))
	mux.HandleFunc("POST /api/ministry-groups/{id}/shares", a.auth(a.handleMinistryCreateShare))
	mux.HandleFunc("PUT /api/ministry-groups/{id}/shares/{share_id}", a.auth(a.handleMinistryUpdateShare))
	mux.HandleFunc("POST /api/ministry-groups/{id}/shares/{share_id}/decision", a.auth(a.handleMinistryShareDecision))
	mux.HandleFunc("POST /api/ministry-groups/{id}/progress", a.auth(a.handleMinistryCreateProgress))
	mux.HandleFunc("POST /api/ministry-groups/{id}/attachments", a.auth(a.handleMinistryAttachment))

	mux.HandleFunc("GET /api/study-weeks", a.auth(a.handleStudyWeeks))
	mux.HandleFunc("GET /api/study-weeks/current", a.auth(a.handleCurrentStudyWeek))
	mux.HandleFunc("POST /api/admin/study-weeks", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminCreateStudyWeek)))
	mux.HandleFunc("PUT /api/admin/study-weeks/{id}", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminUpdateStudyWeek)))
	mux.HandleFunc("DELETE /api/admin/study-weeks/{id}", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminDeleteStudyWeek)))

	mux.HandleFunc("POST /api/checkins", a.auth(a.handleCreateCheckin))
	mux.HandleFunc("DELETE /api/checkins/{id}", a.auth(a.handleDeleteOwnCheckin))
	mux.HandleFunc("GET /api/checkins", a.auth(a.handleListCheckins))
	mux.HandleFunc("DELETE /api/admin/checkins/{id}", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminDeleteCheckin)))

	mux.HandleFunc("GET /api/assets", a.auth(a.handleListAssets))
	mux.HandleFunc("GET /api/library", a.auth(a.handleResourceLibrary))
	mux.HandleFunc("GET /api/assets/{id}/download", a.auth(a.handleDownloadAsset))
	mux.HandleFunc("GET /api/assets/{id}/range", a.auth(a.handleDownloadAssetRange))
	mux.HandleFunc("POST /api/admin/assets/upload", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminUploadAsset)))
	mux.HandleFunc("GET /api/admin/resource-library", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminResourceLibrary)))
	mux.HandleFunc("GET /api/admin/learning-config", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminLearningConfig)))
	mux.HandleFunc("PUT /api/admin/learning-config", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminSaveLearningConfig)))
	mux.HandleFunc("GET /api/content/pdf-range", a.auth(a.handleStaticPDFRange))
	mux.HandleFunc("GET /api/admin/exports/checkins-detail", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminExportCheckinsCSV)))
	mux.HandleFunc("GET /api/admin/exports/daily-summary", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminExportDailySummaryCSV)))
	mux.HandleFunc("GET /api/admin/exports/study-weeks", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminExportStudyWeeksExcel)))
	mux.HandleFunc("POST /api/admin/imports/study-weeks", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminImportStudyWeeksExcel)))
	mux.HandleFunc("GET /api/admin/exports/feedbacks", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminExportFeedbacksCSV)))
	mux.HandleFunc("GET /api/admin/exports/local-backup", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminExportLocalBackupJSON)))
	mux.HandleFunc("POST /api/admin/imports/local-backup", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminImportLocalBackupJSON)))
	mux.HandleFunc("POST /api/admin/members", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminCreateMember)))
	mux.HandleFunc("DELETE /api/admin/members/{id}", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminRemoveMember)))
	mux.HandleFunc("PUT /api/admin/group/default-password", a.auth(a.requireRole(roleGroupAdmin, a.handleAdminSetGroupDefaultPassword)))
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
		// #nosec G201 -- partition name and boundary are derived exclusively from time.Time above.
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

const defaultJSONBodyLimit = int64(1 << 20)
const backupJSONBodyLimit = int64(64 << 20)

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return readJSONWithLimit(w, r, v, defaultJSONBodyLimit)
}

func readJSONWithLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	if int64(len(body)) > limit {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large")
		return false
	}
	if err := json.Unmarshal(body, v); err != nil {
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

func nullableUint64Value(v uint64) any {
	if v == 0 {
		return nil
	}
	return v
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
		parts := strings.Split(v, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
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
