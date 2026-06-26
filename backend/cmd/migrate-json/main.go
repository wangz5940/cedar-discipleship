package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io/fs"
	"log"
	"math"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	roleMember      = "member"
	sourceMigration = "json_migration"
)

type options struct {
	dsn                      string
	groupCode                string
	groupName                string
	configPath               string
	recordsPath              string
	defaultPassword          string
	assetsRoot               string
	reportDir                string
	dryRun                   bool
	allowDuplicateAsDeleted  bool
	skipConfig               bool
	skipRecords              bool
	failOnGeneratedUsernames bool
}

type oldConfig struct {
	SiteInfo       siteInfo        `json:"site_info"`
	Members        []string        `json:"members"`
	WeeklySchedule []oldWeek       `json:"weekly_schedule"`
	TaskSections   json.RawMessage `json:"task_sections"`
	MountedFiles   json.RawMessage `json:"mounted_files"`
}

type siteInfo struct {
	Title          string `json:"title"`
	BrandName      string `json:"brand_name"`
	HeroKicker     string `json:"hero_kicker"`
	HeroDesc       string `json:"hero_desc"`
	DashboardTitle string `json:"dashboard_title"`
}

type oldWeek struct {
	ID             int             `json:"id"`
	Start          string          `json:"start"`
	End            string          `json:"end"`
	Title          json.RawMessage `json:"title"`
	Readings       []oldAssetRef   `json:"readings"`
	Video          string          `json:"video"`
	Videos         []oldAssetRef   `json:"videos"`
	BookEnabled    *bool           `json:"book_enabled"`
	VideoEnabled   *bool           `json:"video_enabled"`
	VerseEnabled   *bool           `json:"verse_enabled"`
	OutlineEnabled *bool           `json:"outline_enabled"`
	Verse          string          `json:"verse"`
	ReciteText     string          `json:"reciteText"`
	URL            string          `json:"url"`
	OutlineImage   string          `json:"outlineImage"`
	Shares         []oldAssetRef   `json:"shares"`
	SortOrder      int             `json:"sort_order"`
}

type oldAssetRef struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

type oldRecord struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	CheckinTime string `json:"checkin_time"`
	LogicalDate string `json:"logical_date"`
	IsRetro     any    `json:"is_retro"`
	Daily       string `json:"daily"`
	Book        string `json:"book"`
	Video       string `json:"video"`
	Verse       string `json:"verse"`
	Detail      string `json:"detail"`
	Note        string `json:"note"`
	Kind        string `json:"kind"`
	Part        string `json:"part"`
}

type migrationReport struct {
	GeneratedAt string            `json:"generated_at"`
	DryRun      bool              `json:"dry_run"`
	Inputs      map[string]string `json:"inputs"`
	Group       groupReport       `json:"group"`
	Members     counterReport     `json:"members"`
	Weeks       counterReport     `json:"study_weeks"`
	Tasks       counterReport     `json:"study_tasks"`
	Assets      counterReport     `json:"assets"`
	TaskAssets  counterReport     `json:"task_assets"`
	Checkins    checkinReport     `json:"checkins"`
	Warnings    []string          `json:"warnings,omitempty"`
	Failures    []failure         `json:"failures,omitempty"`
	Details     map[string]any    `json:"details,omitempty"`
}

type groupReport struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	ID        uint64 `json:"id,omitempty"`
	Created   bool   `json:"created"`
	Reused    bool   `json:"reused"`
	WouldSave bool   `json:"would_save"`
}

type counterReport struct {
	Parsed    int `json:"parsed"`
	Created   int `json:"created"`
	Reused    int `json:"reused"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
	WouldSave int `json:"would_save"`
}

type checkinReport struct {
	RecordsParsed     int `json:"records_parsed"`
	RowsPlanned       int `json:"rows_planned"`
	Inserted          int `json:"inserted"`
	SkippedDuplicate  int `json:"skipped_duplicate"`
	ImportedAsDeleted int `json:"imported_as_deleted"`
	Failed            int `json:"failed"`
	WouldSave         int `json:"would_save"`
}

type failure struct {
	Scope   string `json:"scope"`
	Key     string `json:"key,omitempty"`
	Message string `json:"message"`
}

type migrationState struct {
	groupID      uint64
	memberIDs    map[string]uint64
	weekIDs      map[string]uint64
	taskIDs      map[string]uint64
	assetIDs     map[string]uint64
	taskAssetSet map[string]bool
	plannedKeys  map[string]int
	warnings     []string
	failures     []failure
}

type plannedTask struct {
	Type    string
	Title   string
	Content string
	Enabled bool
	Assets  []plannedAssetLink
}

type plannedAssetLink struct {
	Ref       oldAssetRef
	Category  string
	UsageType string
}

func main() {
	var opt options
	flag.StringVar(&opt.dsn, "dsn", env("AGP_DSN", ""), "MySQL DSN")
	flag.StringVar(&opt.groupCode, "group-code", "", "target study group code")
	flag.StringVar(&opt.groupName, "group-name", "", "target study group name")
	flag.StringVar(&opt.configPath, "config", "../config.json", "old config.json path")
	flag.StringVar(&opt.recordsPath, "records", "../data/records.json", "old records.json path")
	flag.StringVar(&opt.defaultPassword, "default-password", "", "default password for imported members")
	flag.StringVar(&opt.assetsRoot, "assets-root", "", "logical assets root note")
	flag.StringVar(&opt.reportDir, "report-dir", "../data/migration-reports", "migration report directory")
	flag.BoolVar(&opt.dryRun, "dry-run", true, "parse and report without writing database")
	flag.BoolVar(&opt.allowDuplicateAsDeleted, "allow-duplicate-as-deleted", false, "import duplicate checkins as soft-deleted rows with non-zero active_key")
	flag.BoolVar(&opt.skipConfig, "skip-config", false, "skip config import")
	flag.BoolVar(&opt.skipRecords, "skip-records", false, "skip records import")
        flag.BoolVar(&opt.failOnGeneratedUsernames, "fail-on-generated-usernames", false, "fail members whose usernames must be auto-generated")
	flag.Parse()

	if err := run(opt); err != nil {
		log.Fatal(err)
	}
}

func run(opt options) error {
	if strings.TrimSpace(opt.groupCode) == "" || strings.TrimSpace(opt.groupName) == "" {
		return errors.New("--group-code and --group-name are required")
	}
	if opt.defaultPassword == "" {
		opt.defaultPassword = randomPassword(10)
	}
	if len(opt.defaultPassword) < 8 {
		return errors.New("--default-password must be at least 8 characters")
	}

	cfg, err := loadConfig(opt.configPath, opt.skipConfig)
	if err != nil {
		return err
	}
	records, err := loadRecords(opt.recordsPath, opt.skipRecords)
	if err != nil {
		return err
	}
        usernameMap := defaultUsernameMap()

	report := migrationReport{
		GeneratedAt: time.Now().Format(time.RFC3339),
		DryRun:      opt.dryRun,
		Inputs: map[string]string{
			"config":      opt.configPath,
			"records":     opt.recordsPath,
			"assets_root": opt.assetsRoot,
		},
		Group: groupReport{Code: opt.groupCode, Name: opt.groupName, WouldSave: opt.dryRun},
		Details: map[string]any{
			"generated_usernames": map[string]string{},
		},
	}
	state := migrationState{
		memberIDs:    map[string]uint64{},
		weekIDs:      map[string]uint64{},
		taskIDs:      map[string]uint64{},
		assetIDs:     map[string]uint64{},
		taskAssetSet: map[string]bool{},
		plannedKeys:  map[string]int{},
	}

	if opt.skipConfig {
		report.Warnings = append(report.Warnings, "config import skipped")
	}
	if opt.skipRecords {
		report.Warnings = append(report.Warnings, "records import skipped")
	}

	if opt.dryRun {
                planDryRun(cfg, records, usernameMap, opt, &state, &report)
		return writeAndPrintReport(opt, report)
	}
	if opt.dsn == "" {
		return errors.New("--dsn is required when --dry-run=false")
	}

	db, err := sql.Open("mysql", opt.dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return err
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

        if err := importConfig(ctx, tx, cfg, usernameMap, opt, &state, &report); err != nil {
		return err
	}
	if err := importRecords(ctx, tx, records, opt, &state, &report); err != nil {
		return err
	}
	report.Warnings = append(report.Warnings, state.warnings...)
	report.Failures = append(report.Failures, state.failures...)
	if err := tx.Commit(); err != nil {
		return err
	}
	return writeAndPrintReport(opt, report)
}

func planDryRun(cfg oldConfig, records []oldRecord, usernameMap map[string]string, opt options, state *migrationState, report *migrationReport) {
	report.Members.Parsed = len(cfg.Members)
	report.Weeks.Parsed = len(cfg.WeeklySchedule)
	report.Checkins.RecordsParsed = len(records)
	if !opt.skipConfig {
		report.Group.WouldSave = true
		report.Members.WouldSave = len(cfg.Members)
		report.Weeks.WouldSave = len(cfg.WeeklySchedule)
		for i, name := range cfg.Members {
                        username, generated := usernameForMember(name, i+1, usernameMap)
			report.Details["generated_usernames"].(map[string]string)[name] = username
			if generated {
                                report.Warnings = append(report.Warnings, fmt.Sprintf("member %q username auto-generated as %q", name, username))
			}
			if generated && opt.failOnGeneratedUsernames {
				report.Members.Failed++
                                report.Failures = append(report.Failures, failure{Scope: "member", Key: name, Message: "username would be auto-generated"})
			}
			state.memberIDs[name] = uint64(i + 1)
		}
		for _, week := range cfg.WeeklySchedule {
			tasks := tasksForWeek(week)
			report.Tasks.Parsed += len(tasks)
			report.Tasks.WouldSave += len(tasks)
			for _, task := range tasks {
				for _, link := range task.Assets {
					if strings.TrimSpace(link.Ref.URL) == "" {
						continue
					}
					report.Assets.Parsed++
					report.Assets.WouldSave++
					report.TaskAssets.Parsed++
					report.TaskAssets.WouldSave++
				}
			}
		}
	}
	if !opt.skipRecords {
		for _, rec := range records {
			if _, ok := state.memberIDs[rec.Name]; !ok && len(cfg.Members) > 0 {
				report.Checkins.Failed++
				report.Failures = append(report.Failures, failure{Scope: "record", Key: recordKey(rec), Message: "member not found in config"})
				continue
			}
			rows := checkinRowsForRecord(rec)
			report.Checkins.RowsPlanned += len(rows)
			report.Checkins.WouldSave += len(rows)
		}
	}
	report.Warnings = append(report.Warnings, state.warnings...)
	report.Failures = append(report.Failures, state.failures...)
}

func importConfig(ctx context.Context, tx *sql.Tx, cfg oldConfig, usernameMap map[string]string, opt options, state *migrationState, report *migrationReport) error {
	if opt.skipConfig {
		return nil
	}
	now := nowSQL()
	hash, err := hashPassword(opt.defaultPassword)
	if err != nil {
		return err
	}
	groupID, created, err := ensureGroup(ctx, tx, opt.groupCode, opt.groupName, hash, now)
	if err != nil {
		return err
	}
	state.groupID = groupID
	report.Group.ID = groupID
	report.Group.Created = created
	report.Group.Reused = !created

	settingsJSON := map[string]json.RawMessage{}
	if len(cfg.TaskSections) > 0 {
		settingsJSON["task_sections"] = cfg.TaskSections
	}
	if len(cfg.MountedFiles) > 0 {
		settingsJSON["mounted_files"] = cfg.MountedFiles
	}
	settingsBytes, _ := json.Marshal(settingsJSON)
	buttonLabels := extractButtonLabels(cfg.TaskSections)
	if err := upsertGroupSettings(ctx, tx, groupID, cfg.SiteInfo, buttonLabels, settingsBytes, now); err != nil {
		return err
	}

	report.Members.Parsed = len(cfg.Members)
	for i, name := range cfg.Members {
                username, generated := usernameForMember(name, i+1, usernameMap)
		report.Details["generated_usernames"].(map[string]string)[name] = username
		if generated {
                        report.Warnings = append(report.Warnings, fmt.Sprintf("member %q username auto-generated as %q", name, username))
			if opt.failOnGeneratedUsernames {
				report.Members.Failed++
                                report.Failures = append(report.Failures, failure{Scope: "member", Key: name, Message: "username would be auto-generated"})
				continue
			}
		}
		userID, userCreated, err := ensureUser(ctx, tx, username, name, hash, now)
		if err != nil {
			report.Members.Failed++
			report.Failures = append(report.Failures, failure{Scope: "member", Key: name, Message: err.Error()})
			continue
		}
		if err := ensureMember(ctx, tx, groupID, userID, name, now); err != nil {
			report.Members.Failed++
			report.Failures = append(report.Failures, failure{Scope: "member", Key: name, Message: err.Error()})
			continue
		}
		if err := ensureRole(ctx, tx, groupID, userID, roleMember, now); err != nil {
			return err
		}
		state.memberIDs[name] = userID
		if userCreated {
			report.Members.Created++
		} else {
			report.Members.Reused++
		}
	}

	report.Weeks.Parsed = len(cfg.WeeklySchedule)
	for _, week := range cfg.WeeklySchedule {
		weekID, created, err := ensureWeek(ctx, tx, groupID, week, now)
		if err != nil {
			report.Weeks.Failed++
			report.Failures = append(report.Failures, failure{Scope: "study_week", Key: week.Start + ":" + week.End, Message: err.Error()})
			continue
		}
		state.weekIDs[week.Start] = weekID
		if created {
			report.Weeks.Created++
		} else {
			report.Weeks.Reused++
		}
		for _, task := range tasksForWeek(week) {
			report.Tasks.Parsed++
			taskID, created, err := ensureTask(ctx, tx, groupID, weekID, task, now)
			if err != nil {
				report.Tasks.Failed++
				report.Failures = append(report.Failures, failure{Scope: "study_task", Key: task.Type + ":" + task.Title, Message: err.Error()})
				continue
			}
			state.taskIDs[fmt.Sprintf("%d:%s:%s", weekID, task.Type, task.Title)] = taskID
			if created {
				report.Tasks.Created++
			} else {
				report.Tasks.Reused++
			}
			for _, link := range task.Assets {
				if strings.TrimSpace(link.Ref.URL) == "" {
					continue
				}
				report.Assets.Parsed++
				assetID, created, err := ensureAsset(ctx, tx, groupID, link.Ref, link.Category, now)
				if err != nil {
					report.Assets.Failed++
					report.Failures = append(report.Failures, failure{Scope: "asset", Key: link.Ref.URL, Message: err.Error()})
					continue
				}
				if created {
					report.Assets.Created++
				} else {
					report.Assets.Reused++
				}
				report.TaskAssets.Parsed++
				linked, err := ensureTaskAsset(ctx, tx, groupID, taskID, assetID, link.UsageType, now)
				if err != nil {
					report.TaskAssets.Failed++
					report.Failures = append(report.Failures, failure{Scope: "task_asset", Key: link.Ref.URL, Message: err.Error()})
					continue
				}
				if linked {
					report.TaskAssets.Created++
				} else {
					report.TaskAssets.Reused++
				}
			}
		}
	}
	return nil
}

func importRecords(ctx context.Context, tx *sql.Tx, records []oldRecord, opt options, state *migrationState, report *migrationReport) error {
	if opt.skipRecords {
		return nil
	}
	if state.groupID == 0 {
		groupID, err := lookupGroupID(ctx, tx, opt.groupCode)
		if err != nil {
			return err
		}
		state.groupID = groupID
		if err := loadMembers(ctx, tx, groupID, state.memberIDs); err != nil {
			return err
		}
	}
	report.Checkins.RecordsParsed = len(records)
	for _, rec := range records {
		userID, ok := state.memberIDs[rec.Name]
		if !ok {
			report.Checkins.Failed++
			report.Failures = append(report.Failures, failure{Scope: "record", Key: recordKey(rec), Message: "member not found"})
			continue
		}
		checkinTime, err := parseTime(rec.CheckinTime)
		if err != nil {
			report.Checkins.Failed++
			report.Failures = append(report.Failures, failure{Scope: "record", Key: recordKey(rec), Message: "invalid checkin_time"})
			continue
		}
		if _, err := time.Parse("2006-01-02", rec.LogicalDate); err != nil {
			report.Checkins.Failed++
			report.Failures = append(report.Failures, failure{Scope: "record", Key: recordKey(rec), Message: "invalid logical_date"})
			continue
		}
		weekID, _ := findWeekID(ctx, tx, state.groupID, rec.LogicalDate)
		for _, row := range checkinRowsForRecord(rec) {
			report.Checkins.RowsPlanned++
			status, err := insertCheckin(ctx, tx, state.groupID, userID, weekID, rec, row, checkinTime, opt.allowDuplicateAsDeleted)
			if err != nil {
				report.Checkins.Failed++
				report.Failures = append(report.Failures, failure{Scope: "checkin", Key: recordKey(rec) + ":" + row.TaskType, Message: err.Error()})
				continue
			}
			switch status {
			case "inserted":
				report.Checkins.Inserted++
			case "duplicate":
				report.Checkins.SkippedDuplicate++
			case "deleted":
				report.Checkins.ImportedAsDeleted++
			}
		}
	}
	return nil
}

type checkinRow struct {
	TaskType string
	Detail   string
	Part     string
}

func checkinRowsForRecord(rec oldRecord) []checkinRow {
	var rows []checkinRow
	add := func(taskType string) {
		rows = append(rows, checkinRow{TaskType: taskType, Detail: firstNonEmpty(rec.Detail, taskType), Part: rec.Part})
	}
	if strings.EqualFold(rec.Daily, "done") {
		add("daily_devotion")
	}
	if strings.EqualFold(rec.Book, "done") {
		add("weekly_book")
	}
	if strings.EqualFold(rec.Video, "done") {
		add("weekly_video")
	}
	if strings.EqualFold(rec.Verse, "done") {
		add("weekly_verse")
	}
	switch strings.TrimSpace(rec.Kind) {
	case "reflection":
		add("reflection")
	case "recite_exam":
		add("recite_exam")
	}
	return rows
}

func ensureGroup(ctx context.Context, tx *sql.Tx, code, name, hash, now string) (uint64, bool, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM study_groups WHERE code=?", code).Scan(&id)
	if err == nil {
		_, _ = tx.ExecContext(ctx, "UPDATE study_groups SET name=?, updated_at=? WHERE id=?", name, now, id)
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO study_groups (code,name,description,default_password_hash,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?)", code, name, "imported from legacy JSON", hash, nil, now, now)
	if err != nil {
		return 0, false, err
	}
	newID, _ := res.LastInsertId()
	return uint64(newID), true, nil
}

func lookupGroupID(ctx context.Context, tx *sql.Tx, code string) (uint64, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM study_groups WHERE code=?", code).Scan(&id)
	return id, err
}

func upsertGroupSettings(ctx context.Context, tx *sql.Tx, groupID uint64, info siteInfo, buttonLabels any, settings []byte, now string) error {
	buttonBytes, _ := json.Marshal(buttonLabels)
	_, err := tx.ExecContext(ctx, `INSERT INTO group_settings
		(group_id,site_title,brand_name,hero_kicker,hero_desc,dashboard_title,button_labels,settings,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		site_title=VALUES(site_title), brand_name=VALUES(brand_name), hero_kicker=VALUES(hero_kicker),
		hero_desc=VALUES(hero_desc), dashboard_title=VALUES(dashboard_title),
		button_labels=VALUES(button_labels), settings=VALUES(settings), updated_at=VALUES(updated_at)`,
		groupID, info.Title, info.BrandName, info.HeroKicker, info.HeroDesc, info.DashboardTitle, nullJSON(buttonBytes), nullJSON(settings), now, now)
	return err
}

func ensureUser(ctx context.Context, tx *sql.Tx, username, displayName, hash, now string) (uint64, bool, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM users WHERE username=?", username).Scan(&id)
	if err == nil {
		_, _ = tx.ExecContext(ctx, "UPDATE users SET display_name=?, name_pinyin=?, updated_at=? WHERE id=?", displayName, username, now, id)
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO users
		(username,display_name,name_pinyin,password_hash,is_super_admin,must_change_password,created_by,created_at,updated_at)
		VALUES (?,?,?,?,0,1,?,?,?)`, username, displayName, username, hash, nil, now, now)
	if err != nil {
		return 0, false, err
	}
	newID, _ := res.LastInsertId()
	return uint64(newID), true, nil
}

func ensureMember(ctx context.Context, tx *sql.Tx, groupID, userID uint64, name, now string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO group_members
		(group_id,user_id,member_name,status,joined_at,created_by,created_at,updated_at)
		VALUES (?,?,?,1,?,?,?,?)
		ON DUPLICATE KEY UPDATE member_name=VALUES(member_name), status=1, updated_at=VALUES(updated_at)`,
		groupID, userID, name, now, nil, now, now)
	return err
}

func ensureRole(ctx context.Context, tx *sql.Tx, groupID, userID uint64, role, now string) error {
	_, err := tx.ExecContext(ctx, "INSERT IGNORE INTO user_group_roles (group_id,user_id,role,created_at) VALUES (?,?,?,?)", groupID, userID, role, now)
	return err
}

func ensureWeek(ctx context.Context, tx *sql.Tx, groupID uint64, week oldWeek, now string) (uint64, bool, error) {
	title := strings.Join(titleList(week.Title), "\n")
	_, err := time.Parse("2006-01-02", week.Start)
	if err != nil {
		return 0, false, err
	}
	_, err = time.Parse("2006-01-02", week.End)
	if err != nil {
		return 0, false, err
	}
	var id uint64
	err = tx.QueryRowContext(ctx, "SELECT id FROM study_weeks WHERE group_id=? AND start_date=? AND end_date=?", groupID, week.Start, week.End).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE study_weeks SET title=?, verse_ref=?, recite_text=?, book_enabled=?, video_enabled=?, verse_enabled=?, outline_enabled=?, sort_order=?, updated_at=? WHERE id=?`,
			title, week.Verse, nullString(week.ReciteText), boolInt(defaultBool(week.BookEnabled, true)), boolInt(defaultBool(week.VideoEnabled, true)), boolInt(defaultBool(week.VerseEnabled, true)), boolInt(defaultBool(week.OutlineEnabled, true)), week.SortOrder, now, id)
		return id, false, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO study_weeks
		(group_id,start_date,end_date,title,verse_ref,recite_text,book_enabled,video_enabled,verse_enabled,outline_enabled,sort_order,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		groupID, week.Start, week.End, title, week.Verse, nullString(week.ReciteText),
		boolInt(defaultBool(week.BookEnabled, true)), boolInt(defaultBool(week.VideoEnabled, true)), boolInt(defaultBool(week.VerseEnabled, true)), boolInt(defaultBool(week.OutlineEnabled, true)), week.SortOrder, now, now)
	if err != nil {
		return 0, false, err
	}
	newID, _ := res.LastInsertId()
	return uint64(newID), true, nil
}

func ensureTask(ctx context.Context, tx *sql.Tx, groupID, weekID uint64, task plannedTask, now string) (uint64, bool, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM study_tasks WHERE group_id=? AND week_id=? AND task_type=? AND title=? LIMIT 1", groupID, weekID, task.Type, task.Title).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(ctx, "UPDATE study_tasks SET content=?, enabled=?, updated_at=? WHERE id=?", nullString(task.Content), boolInt(task.Enabled), now, id)
		return id, false, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO study_tasks
		(group_id,week_id,task_type,title,content,required,enabled,sort_order,created_at,updated_at)
		VALUES (?,?,?,?,?,1,?,?,?,?)`,
		groupID, weekID, task.Type, task.Title, nullString(task.Content), boolInt(task.Enabled), 0, now, now)
	if err != nil {
		return 0, false, err
	}
	newID, _ := res.LastInsertId()
	return uint64(newID), true, nil
}

func ensureAsset(ctx context.Context, tx *sql.Tx, groupID uint64, ref oldAssetRef, category, now string) (uint64, bool, error) {
	storagePath := strings.TrimSpace(ref.URL)
	if storagePath == "" {
		return 0, false, errors.New("empty asset url")
	}
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM assets WHERE group_id=? AND storage_path=? LIMIT 1", groupID, storagePath).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(ctx, "UPDATE assets SET title=?, category=?, updated_at=? WHERE id=?", firstNonEmpty(ref.Title, assetBaseName(storagePath)), category, now, id)
		return id, false, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	original := assetBaseName(storagePath)
	res, err := tx.ExecContext(ctx, `INSERT INTO assets
		(group_id,category,title,original_name,storage_path,mime_type,file_size,checksum_sha256,visibility,created_by,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		groupID, category, firstNonEmpty(ref.Title, original), original, storagePath, mimeFromPath(storagePath), 0, "", "group", 0, now, now)
	if err != nil {
		return 0, false, err
	}
	newID, _ := res.LastInsertId()
	return uint64(newID), true, nil
}

func ensureTaskAsset(ctx context.Context, tx *sql.Tx, groupID, taskID, assetID uint64, usageType, now string) (bool, error) {
	res, err := tx.ExecContext(ctx, "INSERT IGNORE INTO task_assets (group_id,task_id,asset_id,usage_type,sort_order,created_at) VALUES (?,?,?,?,0,?)", groupID, taskID, assetID, usageType, now)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

func loadMembers(ctx context.Context, tx *sql.Tx, groupID uint64, out map[string]uint64) error {
	rows, err := tx.QueryContext(ctx, "SELECT user_id, member_name FROM group_members WHERE group_id=? AND status=1", groupID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id uint64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		out[name] = id
	}
	return rows.Err()
}

func findWeekID(ctx context.Context, tx *sql.Tx, groupID uint64, logicalDate string) (uint64, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM study_weeks WHERE group_id=? AND start_date <= ? AND end_date >= ? ORDER BY start_date DESC LIMIT 1", groupID, logicalDate, logicalDate).Scan(&id)
	return id, err
}

func insertCheckin(ctx context.Context, tx *sql.Tx, groupID, userID, weekID uint64, rec oldRecord, row checkinRow, checkinTime time.Time, allowDuplicateAsDeleted bool) (string, error) {
	activeKey := uint64(0)
	deletedAt := any(nil)
	status := "inserted"
	if exists, err := checkinExists(ctx, tx, groupID, userID, row.TaskType, rec.LogicalDate, row.Part); err != nil {
		return "", err
	} else if exists {
		if !allowDuplicateAsDeleted {
			return "duplicate", nil
		}
		activeKey = uint64(maxInt64(rec.ID, 1))
		deletedAt = nowSQL()
		status = "deleted"
	}
	res, err := tx.ExecContext(ctx, `INSERT IGNORE INTO checkin_records
		(group_id,user_id,task_id,week_id,logical_date,checkin_time,task_type,status,is_retro,detail,note,part,source,active_key,created_by,created_at,updated_at,deleted_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		groupID, userID, nil, nullableID(weekID), rec.LogicalDate, checkinTime.UTC().Format("2006-01-02 15:04:05.000"),
		row.TaskType, "done", boolInt(isRetro(rec.IsRetro)), truncate(row.Detail, 1024), nullString(rec.Note), truncate(row.Part, 64), sourceMigration,
		activeKey, userID, nowSQL(), nowSQL(), deletedAt)
	if err != nil {
		return "", err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 && status == "inserted" {
		return "duplicate", nil
	}
	return status, nil
}

func checkinExists(ctx context.Context, tx *sql.Tx, groupID, userID uint64, taskType, logicalDate, part string) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM checkin_records
		WHERE group_id=? AND user_id=? AND task_type=? AND logical_date=? AND part=? AND active_key=0`,
		groupID, userID, taskType, logicalDate, truncate(part, 64)).Scan(&count)
	return count > 0, err
}

func tasksForWeek(week oldWeek) []plannedTask {
	var tasks []plannedTask
	titles := titleList(week.Title)
	readingTitle := strings.Join(titles, "\n")
	if readingTitle == "" {
		readingTitle = "周读物"
	}
	readAssets := make([]plannedAssetLink, 0, len(week.Readings))
	for _, ref := range week.Readings {
		readAssets = append(readAssets, plannedAssetLink{Ref: ref, Category: "book", UsageType: "reading"})
	}
	tasks = append(tasks, plannedTask{Type: "weekly_book", Title: readingTitle, Enabled: defaultBool(week.BookEnabled, true), Assets: readAssets})

	videoTitle := firstNonEmpty(week.Video, "周视频")
	var videoAssets []plannedAssetLink
	for _, ref := range week.Videos {
		videoAssets = append(videoAssets, plannedAssetLink{Ref: ref, Category: "video", UsageType: "video"})
	}
	if week.URL != "" {
		videoAssets = append(videoAssets, plannedAssetLink{Ref: oldAssetRef{Title: videoTitle, URL: week.URL}, Category: "video", UsageType: "video"})
	}
	tasks = append(tasks, plannedTask{Type: "weekly_video", Title: videoTitle, Enabled: defaultBool(week.VideoEnabled, true), Assets: videoAssets})
	tasks = append(tasks, plannedTask{Type: "weekly_verse", Title: firstNonEmpty(week.Verse, "背经"), Content: week.ReciteText, Enabled: defaultBool(week.VerseEnabled, true)})
	if week.OutlineImage != "" {
		tasks = append(tasks, plannedTask{Type: "outline", Title: "提纲背诵", Enabled: defaultBool(week.OutlineEnabled, true), Assets: []plannedAssetLink{{Ref: oldAssetRef{Title: "提纲图片", URL: week.OutlineImage}, Category: "outline", UsageType: "outline"}}})
	}
	for _, ref := range week.Shares {
		tasks = append(tasks, plannedTask{Type: "share", Title: firstNonEmpty(ref.Title, "课代表分享"), Enabled: true, Assets: []plannedAssetLink{{Ref: ref, Category: "share", UsageType: "share"}}})
	}
	return tasks
}

func loadConfig(path string, skip bool) (oldConfig, error) {
	if skip {
		return oldConfig{}, nil
	}
	var cfg oldConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func loadRecords(path string, skip bool) ([]oldRecord, error) {
	if skip {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var records []oldRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func defaultUsernameMap() map[string]string {
	return map[string]string{
		"张迦勒":  "zhangjiale",
		"陈思佳":  "chensijia",
		"廖美倩":  "liaomeiqian",
		"苏相宜":  "suxiangyi",
		"李群":   "liqun",
		"邹桂芬":  "zouguifen",
		"戴许诺":  "daixunuo",
		"许水英":  "xushuiying",
		"贺丽华":  "helihua",
		"朱灵":   "zhuling",
		"李思思":  "lisisi",
		"何金群":  "hejinqun",
		"胡方舟":  "hufangzhou",
		"戴维多尔": "daiweiduoer",
		"仇健棒":  "qiujianbang",
		"彭朋":   "pengpeng",
		"李英红":  "liyinghong",
		"杨留影":  "yangliuying",
	}
}

func writeAndPrintReport(opt options, report migrationReport) error {
	if err := os.MkdirAll(opt.reportDir, 0o755); err != nil {
		return err
	}
	file := filepath.Join(opt.reportDir, fmt.Sprintf("%s-%s.json", sanitizeFilename(opt.groupCode), time.Now().Format("20060102-150405")))
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("migration report: %s\n", file)
	fmt.Printf("dry_run=%v members=%+v weeks=%+v tasks=%+v assets=%+v checkins=%+v failures=%d warnings=%d\n",
		report.DryRun, report.Members, report.Weeks, report.Tasks, report.Assets, report.Checkins, len(report.Failures), len(report.Warnings))
	return nil
}

func extractButtonLabels(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v["buttons"]
}

func titleList(raw json.RawMessage) []string {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && strings.TrimSpace(s) != "" {
		return []string{s}
	}
	return nil
}

func usernameForMember(name string, index int, usernameMap map[string]string) (string, bool) {
        if v := strings.TrimSpace(usernameMap[name]); v != "" {
		return normalizeUsername(v), false
	}
	base := normalizeUsername(name)
	if base != "" {
		return base, false
	}
	return fmt.Sprintf("member%03d", index), true
}

func normalizeUsername(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "._-")
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", s)
}

func isRetro(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		x = strings.ToLower(strings.TrimSpace(x))
		return x == "yes" || x == "true" || x == "1" || x == "retro"
	case float64:
		return x != 0
	default:
		return false
	}
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := pbkdf2Key([]byte(password), salt, 120000, 32, sha256.New)
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(dk), nil
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

func randomPassword(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, n)
	random := make([]byte, n)
	if _, err := rand.Read(random); err != nil {
		for i := range b {
			b[i] = alphabet[i%len(alphabet)]
		}
		return string(b)
	}
	for i := range b {
		b[i] = alphabet[int(random[i])%len(alphabet)]
	}
	return string(b)
}

func assetBaseName(p string) string {
	u, err := url.PathUnescape(p)
	if err == nil {
		p = u
	}
	name := path.Base(strings.TrimSpace(p))
	if name == "." || name == "/" || name == "" {
		return "asset"
	}
	return name
}

func mimeFromPath(p string) string {
	ext := strings.ToLower(path.Ext(p))
	if ext == "" {
		return ""
	}
	return firstNonEmpty(mime.TypeByExtension(ext), "")
}

func sanitizeFilename(s string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	out := re.ReplaceAllString(s, "-")
	out = strings.Trim(out, "-")
	if out == "" {
		return "migration"
	}
	return out
}

func defaultBool(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullJSON(b []byte) any {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
}

func nullableID(id uint64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nowSQL() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05.000")
}

func truncate(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func recordKey(rec oldRecord) string {
	if rec.ID > 0 {
		return fmt.Sprintf("%d", rec.ID)
	}
	return rec.Name + ":" + rec.LogicalDate + ":" + rec.CheckinTime
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
