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
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	roleMember      = "member"
	sourceMigration = "json_migration"
	assetKindOwned  = "owned"
	assetKindImport = "imported"
	sharePermImport = "import"
	shareStatusOn   = "active"
)

type options struct {
	dsn                         string
	groupCode                   string
	groupName                   string
	configPath                  string
	recordsPath                 string
	defaultPassword             string
	reportDir                   string
	dryRun                      bool
	allowDuplicateAsDeleted     bool
	allowUnmatchedWeeklyRecords bool
	reuseGroupMembersByName     bool
	namespaceGeneratedUsernames bool
	skipConfig                  bool
	skipRecords                 bool
	failOnGeneratedUsernames    bool
	forceOverwrite              bool
	preferSharedAssets          bool
	dailyCheckinMode            string
	weeklyCheckinMode           string
	devotionMode                string
}

type oldConfig struct {
	SiteInfo             siteInfo        `json:"site_info"`
	Members              []string        `json:"members"`
	WeeklySchedule       []oldWeek       `json:"weekly_schedule"`
	TaskSections         json.RawMessage `json:"task_sections"`
	MountedFiles         json.RawMessage `json:"mounted_files"`
	DailyReading         json.RawMessage `json:"daily_reading"`
	WeeklyReadingCatalog json.RawMessage `json:"weekly_reading_catalog"`
	ClassRepShares       json.RawMessage `json:"class_rep_shares"`
	AdminUI              json.RawMessage `json:"admin_ui"`
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
	WeeklyCheckin  bool            `json:"-"`
	ReadingPath    string          `json:"-"`
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
	Scripture   string `json:"daily_scripture"`
	Weekly      string `json:"weekly_checkin"`
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
	Type     string
	Title    string
	Content  string
	Enabled  bool
	Optional bool
	Assets   []plannedAssetLink
}

type recordTask struct {
	ID    uint64
	Type  string
	Title string
}

type plannedAssetLink struct {
	Ref       oldAssetRef
	Category  string
	UsageType string
}

type migratedReadingMetadata struct {
	BookName    string `json:"book_name"`
	PageStart   int    `json:"page_start,omitempty"`
	PageEnd     int    `json:"page_end,omitempty"`
	ReadingNote string `json:"reading_note,omitempty"`
	SourceTitle string `json:"source_title"`
	ReadingPath string `json:"reading_path,omitempty"`
}

type scriptureBook struct {
	Book     string
	BookID   string
	Chapters int
}

var bibleBooks = []scriptureBook{
	{Book: "创世记", BookID: "1", Chapters: 50},
	{Book: "出埃及记", BookID: "2", Chapters: 40},
	{Book: "利未记", BookID: "3", Chapters: 27},
	{Book: "民数记", BookID: "4", Chapters: 36},
	{Book: "申命记", BookID: "5", Chapters: 34},
	{Book: "约书亚记", BookID: "6", Chapters: 24},
	{Book: "士师记", BookID: "7", Chapters: 21},
	{Book: "路得记", BookID: "8", Chapters: 4},
	{Book: "撒母耳记上", BookID: "9", Chapters: 31},
	{Book: "撒母耳记下", BookID: "10", Chapters: 24},
	{Book: "列王纪上", BookID: "11", Chapters: 22},
	{Book: "列王纪下", BookID: "12", Chapters: 25},
	{Book: "历代志上", BookID: "13", Chapters: 29},
	{Book: "历代志下", BookID: "14", Chapters: 36},
	{Book: "以斯拉记", BookID: "15", Chapters: 10},
	{Book: "尼希米记", BookID: "16", Chapters: 13},
	{Book: "以斯帖记", BookID: "17", Chapters: 10},
	{Book: "约伯记", BookID: "18", Chapters: 42},
	{Book: "诗篇", BookID: "19", Chapters: 150},
	{Book: "箴言", BookID: "20", Chapters: 31},
	{Book: "传道书", BookID: "21", Chapters: 12},
	{Book: "雅歌", BookID: "22", Chapters: 8},
	{Book: "以赛亚书", BookID: "23", Chapters: 66},
	{Book: "耶利米书", BookID: "24", Chapters: 52},
	{Book: "耶利米哀歌", BookID: "25", Chapters: 5},
	{Book: "以西结书", BookID: "26", Chapters: 48},
	{Book: "但以理书", BookID: "27", Chapters: 12},
	{Book: "何西阿书", BookID: "28", Chapters: 14},
	{Book: "约珥书", BookID: "29", Chapters: 3},
	{Book: "阿摩司书", BookID: "30", Chapters: 9},
	{Book: "俄巴底亚书", BookID: "31", Chapters: 1},
	{Book: "约拿书", BookID: "32", Chapters: 4},
	{Book: "弥迦书", BookID: "33", Chapters: 7},
	{Book: "那鸿书", BookID: "34", Chapters: 3},
	{Book: "哈巴谷书", BookID: "35", Chapters: 3},
	{Book: "西番雅书", BookID: "36", Chapters: 3},
	{Book: "哈该书", BookID: "37", Chapters: 2},
	{Book: "撒迦利亚书", BookID: "38", Chapters: 14},
	{Book: "玛拉基书", BookID: "39", Chapters: 4},
	{Book: "马太福音", BookID: "40", Chapters: 28},
	{Book: "马可福音", BookID: "41", Chapters: 16},
	{Book: "路加福音", BookID: "42", Chapters: 24},
	{Book: "约翰福音", BookID: "43", Chapters: 21},
	{Book: "使徒行传", BookID: "44", Chapters: 28},
	{Book: "罗马书", BookID: "45", Chapters: 16},
	{Book: "哥林多前书", BookID: "46", Chapters: 16},
	{Book: "哥林多后书", BookID: "47", Chapters: 13},
	{Book: "加拉太书", BookID: "48", Chapters: 6},
	{Book: "以弗所书", BookID: "49", Chapters: 6},
	{Book: "腓立比书", BookID: "50", Chapters: 4},
	{Book: "歌罗西书", BookID: "51", Chapters: 4},
	{Book: "帖撒罗尼迦前书", BookID: "52", Chapters: 5},
	{Book: "帖撒罗尼迦后书", BookID: "53", Chapters: 3},
	{Book: "提摩太前书", BookID: "54", Chapters: 6},
	{Book: "提摩太后书", BookID: "55", Chapters: 4},
	{Book: "提多书", BookID: "56", Chapters: 3},
	{Book: "腓利门书", BookID: "57", Chapters: 1},
	{Book: "希伯来书", BookID: "58", Chapters: 13},
	{Book: "雅各书", BookID: "59", Chapters: 5},
	{Book: "彼得前书", BookID: "60", Chapters: 5},
	{Book: "彼得后书", BookID: "61", Chapters: 3},
	{Book: "约翰一书", BookID: "62", Chapters: 5},
	{Book: "约翰二书", BookID: "63", Chapters: 1},
	{Book: "约翰三书", BookID: "64", Chapters: 1},
	{Book: "犹大书", BookID: "65", Chapters: 1},
	{Book: "启示录", BookID: "66", Chapters: 22},
}

var assetDownloadURLPattern = regexp.MustCompile(`^/api/assets/[1-9][0-9]*/download$`)

func main() {
	var opt options
	flag.StringVar(&opt.dsn, "dsn", env("AGP_DSN", ""), "MySQL DSN")
	flag.StringVar(&opt.groupCode, "group-code", "", "target study group code")
	flag.StringVar(&opt.groupName, "group-name", "", "target study group name")
	flag.StringVar(&opt.configPath, "config", "../config.json", "old config.json path")
	flag.StringVar(&opt.recordsPath, "records", "../data/records.json", "old records.json path")
	flag.StringVar(&opt.defaultPassword, "default-password", "", "default password for imported members")
	flag.StringVar(&opt.reportDir, "report-dir", "../data/migration-reports", "migration report directory")
	flag.BoolVar(&opt.dryRun, "dry-run", true, "parse and report without writing database")
	flag.BoolVar(&opt.allowDuplicateAsDeleted, "allow-duplicate-as-deleted", false, "import duplicate checkins as soft-deleted rows with non-zero active_key")
	flag.BoolVar(&opt.allowUnmatchedWeeklyRecords, "allow-unmatched-weekly-records", false, "preserve weekly checkins without a matching configured task as unbound history")
	flag.BoolVar(&opt.reuseGroupMembersByName, "reuse-group-members-by-name", false, "reuse an existing group's unique active member with the same member name")
	flag.BoolVar(&opt.namespaceGeneratedUsernames, "namespace-generated-usernames", false, "prefix auto-generated usernames with the group code")
	flag.BoolVar(&opt.skipConfig, "skip-config", false, "skip config import")
	flag.BoolVar(&opt.skipRecords, "skip-records", false, "skip records import")
	flag.BoolVar(&opt.failOnGeneratedUsernames, "fail-on-generated-usernames", false, "fail members whose usernames must be auto-generated")
	flag.BoolVar(&opt.forceOverwrite, "force-overwrite", false, "overwrite existing group settings and study weeks")
	flag.BoolVar(&opt.preferSharedAssets, "prefer-shared-assets", false, "deprecated compatibility flag; local files are deduplicated by checksum during resource migration")
	flag.StringVar(&opt.dailyCheckinMode, "daily-checkin-mode", "", "legacy daily completion mode: combined or separate")
	flag.StringVar(&opt.weeklyCheckinMode, "weekly-checkin-mode", "", "legacy weekly completion mode: per_reading or aggregate")
	flag.StringVar(&opt.devotionMode, "devotion-mode", "", "legacy devotion lookup mode: auto, numbered, or date")
	flag.Parse()

	if err := run(opt); err != nil {
		log.Fatal(err)
	}
}

func run(opt options) error {
	if strings.TrimSpace(opt.groupCode) == "" || strings.TrimSpace(opt.groupName) == "" {
		return errors.New("--group-code and --group-name are required")
	}
	if opt.namespaceGeneratedUsernames && normalizeUsername(opt.groupCode) == "" {
		return errors.New("--namespace-generated-usernames requires an alphanumeric group code")
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
	if err := normalizeLegacyConfig(&cfg, opt); err != nil {
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
			"config":  opt.configPath,
			"records": opt.recordsPath,
		},
		Group: groupReport{Code: opt.groupCode, Name: opt.groupName, WouldSave: opt.dryRun},
		Details: map[string]any{
			"generated_usernames": map[string]string{},
			"shared_assets":       map[string]string{},
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
			username, generated := usernameForImport(name, i+1, usernameMap, opt)
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
					if !shouldImportAssetRef(link.Ref.URL) {
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

	settingsJSON, err := learningSettings(cfg)
	if err != nil {
		return err
	}
	settingsBytes, _ := json.Marshal(settingsJSON)
	buttonLabels := extractButtonLabels(cfg.TaskSections)
	settingsExist, err := groupSettingsExist(ctx, tx, groupID)
	if err != nil {
		return err
	}
	if created || !settingsExist || opt.forceOverwrite {
		if err := upsertGroupSettings(ctx, tx, groupID, cfg.SiteInfo, buttonLabels, settingsBytes, now); err != nil {
			return err
		}
	} else {
		report.Warnings = append(report.Warnings, "existing group settings preserved; use --force-overwrite to replace them")
	}

	report.Members.Parsed = len(cfg.Members)
	for i, name := range cfg.Members {
		userID, reusedByName, err := reuseGroupMemberByName(ctx, tx, groupID, name, !created && opt.reuseGroupMembersByName)
		if err != nil {
			report.Members.Failed++
			report.Failures = append(report.Failures, failure{Scope: "member", Key: name, Message: err.Error()})
			continue
		}
		if reusedByName {
			if err := ensureRole(ctx, tx, groupID, userID, roleMember, now); err != nil {
				return err
			}
			state.memberIDs[name] = userID
			report.Members.Reused++
			continue
		}
		username, generated := usernameForImport(name, i+1, usernameMap, opt)
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
		weekID, created, err := ensureWeek(ctx, tx, groupID, week, now, opt.forceOverwrite)
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
		if !created && !opt.forceOverwrite {
			continue
		}
		if !created {
			if err := deleteWeekTasks(ctx, tx, groupID, weekID); err != nil {
				return err
			}
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
				if !shouldImportAssetRef(link.Ref.URL) {
					continue
				}
				report.Assets.Parsed++
				assetID, created, reusedShared, err := ensureAsset(ctx, tx, groupID, link.Ref, link.Category, now, opt.preferSharedAssets)
				if err != nil {
					report.Assets.Failed++
					report.Failures = append(report.Failures, failure{Scope: "asset", Key: link.Ref.URL, Message: err.Error()})
					continue
				}
				if reusedShared {
					report.Details["shared_assets"].(map[string]string)[link.Ref.URL] = fmt.Sprintf("asset:%d", assetID)
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
		weekID, err := findWeekID(ctx, tx, state.groupID, rec.LogicalDate)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		candidates, err := recordTasksForWeek(ctx, tx, state.groupID, weekID)
		if err != nil {
			return err
		}
		for _, row := range checkinRowsForRecord(rec) {
			report.Checkins.RowsPlanned++
			if isWeeklyRecordType(row.TaskType) {
				resolved, err := resolveRecordTaskForImport(row, candidates, opt.allowUnmatchedWeeklyRecords)
				if err != nil {
					report.Checkins.Failed++
					report.Failures = append(report.Failures, failure{Scope: "checkin", Key: recordKey(rec) + ":" + row.TaskType, Message: err.Error()})
					continue
				}
				row.TaskID = resolved.TaskID
				row.TaskType = resolved.TaskType
			}
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
	TaskID   uint64
}

type taskResolutionError struct {
	taskType string
	title    string
	matches  int
}

func (e taskResolutionError) Error() string {
	return fmt.Sprintf("task identity is ambiguous or missing: type=%s title=%q matches=%d", e.taskType, e.title, e.matches)
}

func resolveRecordTaskForImport(row checkinRow, candidates []recordTask, allowUnmatched bool) (checkinRow, error) {
	resolved, err := resolveRecordTask(row, candidates)
	if !allowUnmatched || err == nil {
		return resolved, err
	}
	var resolutionErr taskResolutionError
	if errors.As(err, &resolutionErr) && resolutionErr.matches == 0 {
		return row, nil
	}
	return resolved, err
}

func recordTasksForWeek(ctx context.Context, tx *sql.Tx, groupID, weekID uint64) ([]recordTask, error) {
	if weekID == 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,task_type,title FROM study_tasks
		WHERE group_id=? AND week_id=? AND enabled=1
		ORDER BY sort_order,id`, groupID, weekID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []recordTask
	for rows.Next() {
		var task recordTask
		if err := rows.Scan(&task.ID, &task.Type, &task.Title); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func resolveRecordTask(row checkinRow, candidates []recordTask) (checkinRow, error) {
	if row.TaskType == "weekly_book" {
		for _, candidate := range candidates {
			if candidate.Type == "weekly_checkin" {
				row.TaskType = candidate.Type
				row.TaskID = candidate.ID
				row.Part = ""
				return row, nil
			}
		}
	}
	var matches []recordTask
	title := strings.TrimSpace(firstNonEmpty(row.Part, row.Detail))
	for _, candidate := range candidates {
		if candidate.Type != row.TaskType {
			continue
		}
		if row.TaskType != "weekly_book" || title == candidate.Title {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		return row, taskResolutionError{taskType: row.TaskType, title: title, matches: len(matches)}
	}
	row.TaskID = matches[0].ID
	return row, nil
}

func isWeeklyRecordType(taskType string) bool {
	return strings.HasPrefix(taskType, "weekly_")
}

func completedStatus(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "done", "completed", "已完成":
		return true
	default:
		return false
	}
}

func checkinRowsForRecord(rec oldRecord) []checkinRow {
	var rows []checkinRow
	seen := map[string]bool{}
	add := func(taskType string) {
		if seen[taskType] {
			return
		}
		seen[taskType] = true
		rows = append(rows, checkinRow{TaskType: taskType, Detail: firstNonEmpty(rec.Detail, taskType), Part: rec.Part})
	}
	if completedStatus(rec.Daily) {
		add("daily_devotion")
	}
	if completedStatus(rec.Scripture) {
		add("daily_scripture")
	}
	if completedStatus(rec.Weekly) {
		add("weekly_checkin")
	}
	if completedStatus(rec.Book) {
		add("weekly_book")
	}
	if completedStatus(rec.Video) {
		add("weekly_video")
	}
	if completedStatus(rec.Verse) {
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
		if _, err := tx.ExecContext(ctx, "UPDATE study_groups SET name=?, updated_at=? WHERE id=?", name, now, id); err != nil {
			return 0, false, err
		}
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO study_groups (code,name,description,default_password_hash,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?)", code, name, "imported from legacy JSON", hash, nil, now, now)
	if err != nil {
		return 0, false, err
	}
	newID, err := insertedID(res)
	return newID, true, err
}

func lookupGroupID(ctx context.Context, tx *sql.Tx, code string) (uint64, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM study_groups WHERE code=?", code).Scan(&id)
	return id, err
}

func groupSettingsExist(ctx context.Context, tx *sql.Tx, groupID uint64) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM group_settings WHERE group_id=?", groupID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
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
		if _, err := tx.ExecContext(ctx, "UPDATE users SET display_name=?, name_pinyin=?, updated_at=? WHERE id=?", displayName, username, now, id); err != nil {
			return 0, false, err
		}
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
	newID, err := insertedID(res)
	return newID, true, err
}

func reuseGroupMemberByName(ctx context.Context, tx *sql.Tx, groupID uint64, name string, enabled bool) (uint64, bool, error) {
	if !enabled {
		return 0, false, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT user_id FROM group_members
		WHERE group_id=? AND status=1 AND member_name=? LIMIT 2`, groupID, name)
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()
	var userID uint64
	count := 0
	for rows.Next() {
		count++
		if err := rows.Scan(&userID); err != nil {
			return 0, false, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, false, err
	}
	if count > 1 {
		return 0, false, fmt.Errorf("multiple active group members match name %q", name)
	}
	return userID, count == 1, nil
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

func ensureWeek(ctx context.Context, tx *sql.Tx, groupID uint64, week oldWeek, now string, forceOverwrite bool) (uint64, bool, error) {
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
		if !forceOverwrite {
			return id, false, nil
		}
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
	newID, err := insertedID(res)
	return newID, true, err
}

func ensureTask(ctx context.Context, tx *sql.Tx, groupID, weekID uint64, task plannedTask, now string) (uint64, bool, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM study_tasks WHERE group_id=? AND week_id=? AND task_type=? AND title=? LIMIT 1", groupID, weekID, task.Type, task.Title).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(ctx, "UPDATE study_tasks SET content=?, required=?, enabled=?, updated_at=? WHERE id=?",
			nullString(task.Content), boolInt(!task.Optional), boolInt(task.Enabled), now, id)
		return id, false, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO study_tasks
		(group_id,week_id,task_type,title,content,required,enabled,sort_order,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		groupID, weekID, task.Type, task.Title, nullString(task.Content), boolInt(!task.Optional), boolInt(task.Enabled), 0, now, now)
	if err != nil {
		return 0, false, err
	}
	newID, err := insertedID(res)
	return newID, true, err
}

func ensureAsset(ctx context.Context, tx *sql.Tx, groupID uint64, ref oldAssetRef, category, now string, _ bool) (uint64, bool, bool, error) {
	storagePath := strings.TrimSpace(ref.URL)
	if storagePath == "" {
		return 0, false, false, errors.New("empty asset url")
	}
	if assetID := assetIDFromDownloadURL(storagePath); assetID > 0 {
		if ok, err := assetBelongsToGroup(ctx, tx, groupID, assetID); err != nil || ok {
			return assetID, false, false, err
		}
		sourceAssetID, err := canonicalSourceAssetID(ctx, tx, assetID)
		if err != nil {
			return 0, false, false, err
		}
		importedID, ok, err := importSharedAssetByID(ctx, tx, groupID, sourceAssetID, now)
		if err != nil || ok {
			return importedID, false, ok, err
		}
		return 0, false, false, errors.New("asset_download_url_not_importable")
	}
	var id uint64
	err := tx.QueryRowContext(ctx, "SELECT id FROM assets WHERE group_id=? AND storage_path=? LIMIT 1", groupID, storagePath).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(ctx, "UPDATE assets SET title=?, category=?, updated_at=? WHERE id=?", firstNonEmpty(ref.Title, assetBaseName(storagePath)), category, now, id)
		return id, false, false, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, false, err
	}
	original := assetBaseName(storagePath)
	res, err := tx.ExecContext(ctx, `INSERT INTO assets
		(group_id,category,title,original_name,storage_path,mime_type,file_size,checksum_sha256,visibility,created_by,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		groupID, category, firstNonEmpty(ref.Title, original), original, storagePath, mimeFromPath(storagePath), 0, "", "group", 0, now, now)
	if err != nil {
		return 0, false, false, err
	}
	newID, err := insertedID(res)
	return newID, true, false, err
}

func assetBelongsToGroup(ctx context.Context, tx *sql.Tx, groupID, assetID uint64) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.id=? AND a.group_id=? LIMIT 1`, assetID, groupID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func canonicalSourceAssetID(ctx context.Context, tx *sql.Tx, assetID uint64) (uint64, error) {
	var sourceAssetID sql.NullInt64
	var assetKind string
	err := tx.QueryRowContext(ctx, `SELECT b.asset_kind,b.source_asset_id
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.id=? LIMIT 1`, assetID).Scan(&assetKind, &sourceAssetID)
	if err != nil {
		return 0, err
	}
	if assetKind == assetKindImport {
		if !sourceAssetID.Valid || sourceAssetID.Int64 <= 0 {
			return 0, errors.New("imported_asset_missing_source")
		}
		return uint64(sourceAssetID.Int64), nil
	}
	if assetKind != assetKindOwned {
		return 0, errors.New("invalid_asset_kind")
	}
	return assetID, nil
}

func importSharedAssetByID(ctx context.Context, tx *sql.Tx, targetGroupID, sourceAssetID uint64, now string) (uint64, bool, error) {
	var source struct {
		id             uint64
		groupID        uint64
		category       string
		title          string
		originalName   string
		storagePath    string
		mimeType       string
		fileSize       uint64
		checksumSHA256 string
	}
	err := tx.QueryRowContext(ctx, `
		SELECT a.id,a.group_id,a.category,a.title,a.original_name,a.storage_path,a.mime_type,a.file_size,a.checksum_sha256
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.asset_kind=? AND b.deleted_at IS NULL
		JOIN study_groups sg ON sg.id=a.group_id AND sg.status=1
		JOIN asset_share_grants g ON g.asset_id=a.id AND g.owner_group_id=a.group_id
		WHERE a.id=? AND a.group_id<>?
		  AND g.permission=? AND g.status=?
		  AND (g.consumer_group_id IS NULL OR g.consumer_group_id=?)
		LIMIT 1`,
		assetKindOwned, sourceAssetID, targetGroupID, sharePermImport, shareStatusOn, targetGroupID,
	).Scan(
		&source.id,
		&source.groupID,
		&source.category,
		&source.title,
		&source.originalName,
		&source.storagePath,
		&source.mimeType,
		&source.fileSize,
		&source.checksumSHA256,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	assetID, err := importSharedAsset(ctx, tx, targetGroupID, source.id, source.groupID, source.category, source.title, source.originalName, source.storagePath, source.mimeType, source.fileSize, source.checksumSHA256, now)
	if err != nil {
		return 0, false, err
	}
	return assetID, true, nil
}

func importSharedAsset(
	ctx context.Context,
	tx *sql.Tx,
	targetGroupID uint64,
	sourceAssetID uint64,
	sourceGroupID uint64,
	category string,
	title string,
	originalName string,
	storagePath string,
	mimeType string,
	fileSize uint64,
	checksumSHA256 string,
	now string,
) (uint64, error) {
	var importedAssetID uint64
	var existing sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT asset_id FROM asset_bindings WHERE group_id=? AND source_asset_id=? FOR UPDATE`, targetGroupID, sourceAssetID).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if existing.Valid && existing.Int64 > 0 {
		importedAssetID = uint64(existing.Int64)
		if _, err := tx.ExecContext(ctx, `UPDATE asset_bindings
			SET imported_at=COALESCE(imported_at,?),deleted_at=NULL,updated_at=?
			WHERE asset_id=? AND group_id=?`, now, now, importedAssetID, targetGroupID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE assets
			SET category=?,title=?,original_name=?,storage_path=?,mime_type=?,file_size=?,checksum_sha256=?,updated_at=?
			WHERE id=? AND group_id=?`,
			category, title, originalName, storagePath, mimeType, fileSize, checksumSHA256, now, importedAssetID, targetGroupID); err != nil {
			return 0, err
		}
	} else {
		res, err := tx.ExecContext(ctx, `INSERT INTO assets
			(group_id,category,title,original_name,storage_path,mime_type,file_size,checksum_sha256,visibility,created_by,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			targetGroupID, category, title, originalName, storagePath, mimeType, fileSize, checksumSHA256, "imported", 0, now, now)
		if err != nil {
			return 0, err
		}
		importedAssetID, err = insertedID(res)
		if err != nil {
			return 0, err
		}
		resourceKey, err := randomResourceKey()
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO asset_bindings
			(asset_id,group_id,resource_key,asset_kind,source_asset_id,imported_at,deleted_at,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			importedAssetID, targetGroupID, resourceKey, assetKindImport, sourceAssetID, now, nil, now, now); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_dependencies
		(consumer_group_id,consumer_asset_id,provider_group_id,provider_asset_id,dependency_type,status,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE status=VALUES(status),updated_at=VALUES(updated_at)`,
		targetGroupID, importedAssetID, sourceGroupID, sourceAssetID, sharePermImport, shareStatusOn, now, now); err != nil {
		return 0, err
	}
	detail := fmt.Sprintf(`{"migration":true,"source":"%s"}`, sourceMigration)
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_import_events
		(target_group_id,imported_asset_id,source_asset_id,event_type,actor_user_id,detail,created_at)
		VALUES (?,?,?,?,?,?,?)`,
		targetGroupID, importedAssetID, sourceAssetID, assetKindImport, 0, detail, now); err != nil {
		return 0, err
	}
	return importedAssetID, nil
}

func randomResourceKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func ensureTaskAsset(ctx context.Context, tx *sql.Tx, groupID, taskID, assetID uint64, usageType, now string) (bool, error) {
	res, err := tx.ExecContext(ctx, "INSERT IGNORE INTO task_assets (group_id,task_id,asset_id,usage_type,sort_order,created_at) VALUES (?,?,?,?,0,?)", groupID, taskID, assetID, usageType, now)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

func deleteWeekTasks(ctx context.Context, tx *sql.Tx, groupID, weekID uint64) error {
	if _, err := tx.ExecContext(ctx, `DELETE ta FROM task_assets ta
		JOIN study_tasks st ON st.id=ta.task_id
		WHERE st.group_id=? AND st.week_id=?`, groupID, weekID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM study_tasks WHERE group_id=? AND week_id=?", groupID, weekID)
	return err
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
	if exists, err := checkinExists(ctx, tx, groupID, userID, weekID, rec.LogicalDate, row); err != nil {
		return "", err
	} else if exists {
		if !allowDuplicateAsDeleted {
			return "duplicate", nil
		}
		// #nosec G115 -- maxInt64 guarantees a positive int64, which is representable as uint64.
		activeKey = uint64(maxInt64(rec.ID, 1))
		deletedAt = nowSQL()
		status = "deleted"
	}
	res, err := tx.ExecContext(ctx, `INSERT IGNORE INTO checkin_records
		(group_id,user_id,task_id,week_id,logical_date,checkin_time,task_type,status,is_retro,detail,note,part,source,active_key,created_by,created_at,updated_at,deleted_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		groupID, userID, nullableID(row.TaskID), nullableID(weekID), rec.LogicalDate, checkinTime.UTC().Format("2006-01-02 15:04:05.000"),
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

func checkinExists(ctx context.Context, tx *sql.Tx, groupID, userID, weekID uint64, logicalDate string, row checkinRow) (bool, error) {
	var count int
	if isWeeklyRecordType(row.TaskType) && row.TaskID > 0 {
		err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM checkin_records
			WHERE group_id=? AND user_id=? AND task_id=? AND week_id=? AND task_type=?
			  AND deleted_at IS NULL AND status='done'`,
			groupID, userID, row.TaskID, weekID, row.TaskType).Scan(&count)
		return count > 0, err
	}
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM checkin_records
		WHERE group_id=? AND user_id=? AND task_type=? AND logical_date=? AND part=?
		  AND deleted_at IS NULL AND status='done'`,
		groupID, userID, row.TaskType, logicalDate, truncate(row.Part, 64)).Scan(&count)
	return count > 0, err
}

func tasksForWeek(week oldWeek) []plannedTask {
	var tasks []plannedTask
	tasks = append(tasks, readingTasksForWeek(week)...)

	videoTitle := strings.TrimSpace(week.Video)
	var videoAssets []plannedAssetLink
	for _, ref := range week.Videos {
		if strings.TrimSpace(ref.URL) != "" {
			videoAssets = append(videoAssets, plannedAssetLink{Ref: ref, Category: "video", UsageType: "video"})
		}
	}
	if week.URL != "" && !assetRefExists(videoAssets, week.URL) {
		videoAssets = append(videoAssets, plannedAssetLink{Ref: oldAssetRef{Title: firstNonEmpty(videoTitle, "周视频"), URL: week.URL}, Category: "video", UsageType: "video"})
	}
	if videoTitle != "" || len(videoAssets) > 0 {
		content := ""
		for _, link := range videoAssets {
			if isExternalContentURL(link.Ref.URL) {
				content = strings.TrimSpace(link.Ref.URL)
				break
			}
		}
		tasks = append(tasks, plannedTask{Type: "weekly_video", Title: firstNonEmpty(videoTitle, "周视频"), Content: content, Enabled: defaultBool(week.VideoEnabled, true), Assets: videoAssets})
	}
	if strings.TrimSpace(week.Verse) != "" || strings.TrimSpace(week.ReciteText) != "" {
		tasks = append(tasks, plannedTask{Type: "weekly_verse", Title: firstNonEmpty(week.Verse, "背经"), Content: week.ReciteText, Enabled: defaultBool(week.VerseEnabled, true)})
	}
	if week.OutlineImage != "" {
		tasks = append(tasks, plannedTask{Type: "weekly_outline", Title: "提纲背诵", Enabled: defaultBool(week.OutlineEnabled, true), Assets: []plannedAssetLink{{Ref: oldAssetRef{Title: "提纲图片", URL: week.OutlineImage}, Category: "outline", UsageType: "outline"}}})
	}
	for _, ref := range week.Shares {
		tasks = append(tasks, plannedTask{Type: "share", Title: firstNonEmpty(ref.Title, "课代表分享"), Enabled: true, Assets: []plannedAssetLink{{Ref: ref, Category: "share", UsageType: "share"}}})
	}
	if week.WeeklyCheckin {
		tasks = append(tasks, plannedTask{
			Type: "weekly_checkin", Title: firstNonEmpty(strings.Join(titleList(week.Title), "；"), "周任务"), Enabled: true,
		})
	}
	return tasks
}

func assetRefExists(links []plannedAssetLink, value string) bool {
	value = strings.TrimSpace(value)
	for _, link := range links {
		if strings.TrimSpace(link.Ref.URL) == value {
			return true
		}
	}
	return false
}

func readingTasksForWeek(week oldWeek) []plannedTask {
	titles := titleList(week.Title)
	enabled := defaultBool(week.BookEnabled, true)
	if week.WeeklyCheckin && len(week.Readings) == 0 {
		return nil
	}
	total := len(week.Readings)
	if len(titles) > total {
		total = len(titles)
	}
	if total == 0 {
		return nil
	}

	tasks := make([]plannedTask, 0, total)
	for index := 0; index < total; index += 1 {
		var ref oldAssetRef
		hasRef := index < len(week.Readings)
		if hasRef {
			ref = week.Readings[index]
		}
		title := firstNonEmpty(
			strings.TrimSpace(ref.Title),
			titleAt(titles, index),
			assetBaseName(strings.TrimSpace(ref.URL)),
			"周读物",
		)
		task := plannedTask{
			Type:     "weekly_book",
			Title:    title,
			Content:  migratedReadingContent(title, week.ReadingPath),
			Enabled:  enabled,
			Optional: week.WeeklyCheckin,
		}
		if isExternalContentURL(ref.URL) {
			task.Content = strings.TrimSpace(ref.URL)
		}
		if hasRef && (strings.TrimSpace(ref.URL) != "" || strings.TrimSpace(ref.Title) != "") {
			task.Assets = []plannedAssetLink{{
				Ref:       ref,
				Category:  "book",
				UsageType: "reading",
			}}
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func migratedReadingContent(title string, readingPaths ...string) string {
	metadata := parseReadingMetadata(title)
	if len(readingPaths) > 0 {
		metadata.ReadingPath = strings.TrimSpace(readingPaths[0])
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return ""
	}
	return string(data)
}

func parseReadingMetadata(title string) migratedReadingMetadata {
	sourceTitle := strings.TrimSpace(title)
	metadata := migratedReadingMetadata{SourceTitle: sourceTitle}
	if sourceTitle == "" {
		return metadata
	}

	metadata.BookName = sourceTitle
	if strings.HasPrefix(sourceTitle, "《") {
		inner := strings.TrimPrefix(sourceTitle, "《")
		if end := strings.Index(inner, "》"); end > 0 {
			metadata.BookName = strings.TrimSpace(inner[:end])
		}
	}

	pagePattern := regexp.MustCompile(`([0-9]+)\s*[-—~至]\s*([0-9]+)\s*页`)
	pageMatch := pagePattern.FindStringSubmatch(sourceTitle)
	pageIndex := pagePattern.FindStringIndex(sourceTitle)
	if len(pageMatch) == 3 {
		metadata.PageStart = atoiOrZero(pageMatch[1])
		metadata.PageEnd = atoiOrZero(pageMatch[2])
	}

	if len(pageIndex) == 2 {
		note := strings.TrimSpace(sourceTitle[pageIndex[1]:])
		note = strings.TrimSpace(strings.TrimSuffix(note, "）"))
		note = strings.TrimSpace(strings.TrimPrefix(note, "页"))
		note = strings.TrimSpace(strings.TrimPrefix(note, "，"))
		note = strings.TrimSpace(strings.TrimPrefix(note, ","))
		note = strings.Trim(note, "（()")
		note = strings.TrimSpace(strings.TrimPrefix(note, "，"))
		note = strings.TrimSpace(strings.TrimPrefix(note, ","))
		if note != "" && note != "页" {
			metadata.ReadingNote = note
		}
	}
	return metadata
}

func atoiOrZero(value string) int {
	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return 0
	}
	return result
}

func titleAt(items []string, index int) string {
	if index < 0 || index >= len(items) {
		return ""
	}
	return strings.TrimSpace(items[index])
}

func loadConfig(path string, skip bool) (oldConfig, error) {
	if skip {
		return oldConfig{}, nil
	}
	var cfg oldConfig
	// #nosec G304 -- path is an explicit operator-provided CLI input.
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
	// #nosec G304 -- path is an explicit operator-provided CLI input.
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

func insertedID(result sql.Result) (uint64, error) {
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, errors.New("invalid_insert_id")
	}
	return uint64(id), nil
}

func writeAndPrintReport(opt options, report migrationReport) error {
	if err := os.MkdirAll(opt.reportDir, 0o750); err != nil {
		return err
	}
	file := filepath.Join(opt.reportDir, fmt.Sprintf("%s-%s.json", sanitizeFilename(opt.groupCode), time.Now().Format("20060102-150405")))
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
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

func normalizeTaskSections(raw json.RawMessage) (json.RawMessage, error) {
	var sections map[string]any
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, err
	}
	if sections == nil {
		sections = map[string]any{}
	}

	daily := mapValue(sections, "daily")
	if daily == nil {
		daily = map[string]any{}
		sections["daily"] = daily
	}
	dailyPath := firstNonEmpty(databaseAssetDownloadURL(daily["path"]), stringValue(daily["path"]))
	if dailyPath != "" {
		daily["path"] = dailyPath
	}

	devotion := mapValue(daily, "devotion")
	if devotion == nil {
		devotion = map[string]any{}
		daily["devotion"] = devotion
	}
	devotionPath := firstNonEmpty(databaseAssetDownloadURL(devotion["path"]), stringValue(devotion["path"]))
	switch {
	case devotionPath != "":
		devotion["path"] = devotionPath
	case dailyPath != "":
		devotion["path"] = dailyPath
	}
	if _, ok := devotion["numbered_start_date"]; !ok {
		if startDate := stringValue(devotion["start_date"]); startDate != "" {
			devotion["numbered_start_date"] = startDate
		}
	}
	if _, ok := devotion["numbered_start"]; !ok {
		if start := numberValue(devotion["start_section"]); start > 0 {
			devotion["numbered_start"] = start
		}
	}
	if _, ok := devotion["mode"]; !ok {
		devotion["mode"] = "auto"
	}
	if _, ok := devotion["type"]; !ok {
		devotion["type"] = "markdown"
	}

	scripture := mapValue(daily, "scripture")
	if scripture == nil {
		scripture = map[string]any{}
		daily["scripture"] = scripture
	}
	sequence, hasSequence := scripture["sequence"].([]any)
	if !hasSequence || len(sequence) == 0 {
		book := stringValue(scripture["book"])
		bookID := stringValue(scripture["book_id"])
		chapters := numberValue(scripture["max_chapters"])
		if book != "" || bookID != "" || chapters > 0 {
			scripture["sequence"] = bibleBookSequence(book, bookID)
		}
	} else {
		first := mapValue(sequence[0], "")
		if first != nil {
			if stringValue(scripture["book"]) == "" {
				scripture["book"] = stringValue(first["book"])
			}
			if stringValue(scripture["book_id"]) == "" {
				scripture["book_id"] = stringValue(first["book_id"])
			}
			if numberValue(scripture["max_chapters"]) == 0 {
				scripture["max_chapters"] = numberValue(first["chapters"])
			}
		}
	}
	if _, ok := scripture["start_chapter"]; !ok {
		scripture["start_chapter"] = 1
	}
	if _, ok := scripture["type"]; !ok {
		scripture["type"] = "iframe"
	}
	if _, ok := scripture["url_template"]; !ok {
		scripture["url_template"] = "https://www.wordproject.org/bibles/gb/{book_id}/{chapter}.htm"
	}

	return json.Marshal(sections)
}

func bibleBookSequence(startBook, startBookID string) []any {
	startIndex := 0
	for index, book := range bibleBooks {
		if book.BookID == startBookID || book.Book == startBook {
			startIndex = index
			break
		}
	}
	sequence := make([]any, 0, len(bibleBooks)-startIndex)
	for _, book := range bibleBooks[startIndex:] {
		sequence = append(sequence, map[string]any{
			"book":     book.Book,
			"book_id":  book.BookID,
			"chapters": book.Chapters,
		})
	}
	return sequence
}

func mapValue(value any, key string) map[string]any {
	if key != "" {
		if object, ok := value.(map[string]any); ok {
			if nested, ok := object[key].(map[string]any); ok {
				return nested
			}
		}
		return nil
	}
	object, _ := value.(map[string]any)
	return object
}

func stringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func databaseAssetDownloadURL(value any) string {
	text := stringValue(value)
	assetID := assetIDFromDownloadURL(text)
	if assetID == 0 {
		return ""
	}
	return fmt.Sprintf("/api/assets/%d/download", assetID)
}

func assetIDFromDownloadURL(value string) uint64 {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0
	}
	pathValue := text
	if parsed, err := url.Parse(text); err == nil && parsed.Path != "" {
		pathValue = parsed.Path
	}
	if !assetDownloadURLPattern.MatchString(pathValue) {
		return 0
	}
	idText := strings.TrimSuffix(strings.TrimPrefix(pathValue, "/api/assets/"), "/download")
	assetID, err := strconv.ParseUint(idText, 10, 64)
	if err != nil {
		return 0
	}
	return assetID
}

func shouldImportAssetRef(value string) bool {
	text := strings.TrimSpace(value)
	if text == "" {
		return false
	}
	if assetIDFromDownloadURL(text) > 0 {
		return true
	}
	return !isExternalContentURL(text)
}

func isExternalContentURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func numberValue(value any) int {
	switch value := value.(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		number, _ := value.Int64()
		return int(number)
	default:
		return 0
	}
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

func usernameForImport(name string, index int, usernameMap map[string]string, opt options) (string, bool) {
	username, generated := usernameForMember(name, index, usernameMap)
	if generated && opt.namespaceGeneratedUsernames {
		username = normalizeUsername(opt.groupCode) + "-" + username
	}
	return username, generated
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
	if t, err := time.Parse("2006-01-02 15:04:05-07:00", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
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
		return x == "yes" || x == "true" || x == "1" || x == "retro" || x == "是"
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
