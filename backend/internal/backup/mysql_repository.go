package backup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"agp/backend/internal/learning"
)

const (
	roleGroupAdmin  = "group_admin"
	roleGroupLeader = "group_leader"

	resourceStoragePattern = "team-%-resources/objects/%"
	assetKindOwned         = "owned"
	sharePermissionImport  = "import"
	shareStatusActive      = "active"
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) CheckinDetails(ctx context.Context, groupID uint64, loc *time.Location) ([]CheckinDetail, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT c.id,c.logical_date,c.checkin_time,c.task_type,c.part,c.detail,COALESCE(c.note,''),c.is_retro,u.username,COALESCE(m.member_name,u.display_name)
		FROM checkin_records c
		JOIN users u ON u.id=c.user_id
		LEFT JOIN group_members m ON m.group_id=c.group_id AND m.user_id=c.user_id AND m.status=1
		WHERE c.group_id=? AND c.deleted_at IS NULL
		ORDER BY c.logical_date DESC,c.id DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CheckinDetail
	for rows.Next() {
		var item CheckinDetail
		var checkinTime time.Time
		if err := rows.Scan(&item.ID, &item.LogicalDate, &checkinTime, &item.TaskType, &item.Part, &item.Detail, &item.Note, &item.IsRetro, &item.Username, &item.MemberName); err != nil {
			return nil, err
		}
		item.CheckinTime = checkinTime.In(loc).Format("2006-01-02 15:04:05")
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) DailySummaries(ctx context.Context, groupID uint64) (int, []DailySummary, error) {
	var activeMembers int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM group_members WHERE group_id=? AND status=1`, groupID).Scan(&activeMembers); err != nil {
		return 0, nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT logical_date,
		COUNT(*) AS total_checkins,
		COUNT(DISTINCT user_id) AS checked_members,
		SUM(CASE WHEN task_type='daily_devotion' THEN 1 ELSE 0 END) AS devotion_count,
		SUM(CASE WHEN task_type='weekly_book' THEN 1 ELSE 0 END) AS book_count,
		SUM(CASE WHEN task_type='weekly_video' THEN 1 ELSE 0 END) AS video_count,
		SUM(CASE WHEN task_type='weekly_verse' THEN 1 ELSE 0 END) AS verse_count
		FROM checkin_records
		WHERE group_id=? AND deleted_at IS NULL
		GROUP BY logical_date
		ORDER BY logical_date DESC`, groupID)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	var items []DailySummary
	for rows.Next() {
		var item DailySummary
		if err := rows.Scan(&item.LogicalDate, &item.TotalCheckins, &item.CheckedMembers, &item.DevotionCount, &item.BookCount, &item.VideoCount, &item.VerseCount); err != nil {
			return 0, nil, err
		}
		items = append(items, item)
	}
	return activeMembers, items, rows.Err()
}

func (r *MySQLRepository) FeedbackExports(ctx context.Context, groupID uint64, loc *time.Location) ([]FeedbackExport, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT f.created_at,COALESCE(u.username,''),f.name,f.contact,f.message,f.page,f.user_agent
		FROM feedbacks f
		LEFT JOIN users u ON u.id=f.user_id
		LEFT JOIN group_members gm ON gm.user_id=f.user_id AND gm.group_id=? AND gm.status=1
		WHERE f.group_id=? OR (f.group_id IS NULL AND gm.id IS NOT NULL)
		ORDER BY f.id DESC`, groupID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []FeedbackExport
	for rows.Next() {
		var item FeedbackExport
		var created time.Time
		if err := rows.Scan(&created, &item.Username, &item.Name, &item.Contact, &item.Message, &item.Page, &item.UserAgent); err != nil {
			return nil, err
		}
		item.CreatedAt = created.In(loc).Format("2006-01-02 15:04:05")
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) GroupInfo(ctx context.Context, groupID uint64) (*GroupInfo, error) {
	var item GroupInfo
	item.ID = groupID
	err := r.db.QueryRowContext(ctx, `SELECT code,name,description FROM study_groups WHERE id=?`, groupID).Scan(&item.Code, &item.Name, &item.Description)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MySQLRepository) BackupMembers(ctx context.Context, groupID uint64) ([]Member, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT u.username,u.display_name,u.name_pinyin
		FROM group_members m JOIN users u ON u.id=m.user_id
		WHERE m.group_id=? AND m.status=1
		ORDER BY m.member_name,u.username`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roleMap, err := r.memberRoleMap(ctx, groupID)
	if err != nil {
		return nil, err
	}
	var items []Member
	for rows.Next() {
		var item Member
		if err := rows.Scan(&item.Username, &item.DisplayName, &item.NamePinyin); err != nil {
			return nil, err
		}
		item.Roles = roleMap[item.Username]
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) BackupCheckins(ctx context.Context, groupID uint64) ([]Checkin, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT u.username,c.task_id,c.week_id,c.logical_date,c.checkin_time,c.task_type,c.part,c.detail,COALESCE(c.note,''),c.is_retro
		FROM checkin_records c JOIN users u ON u.id=c.user_id
		WHERE c.group_id=? AND c.deleted_at IS NULL
		ORDER BY c.logical_date,c.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Checkin
	for rows.Next() {
		var item Checkin
		var taskID, weekID sql.NullInt64
		var logicalDate, checkinTime time.Time
		if err := rows.Scan(&item.Username, &taskID, &weekID, &logicalDate, &checkinTime, &item.TaskType, &item.Part, &item.Detail, &item.Note, &item.IsRetro); err != nil {
			return nil, err
		}
		if taskID.Valid && taskID.Int64 > 0 {
			item.TaskID = uint64(taskID.Int64)
		}
		if weekID.Valid && weekID.Int64 > 0 {
			item.WeekID = uint64(weekID.Int64)
		}
		item.LogicalDate = logicalDate.Format("2006-01-02")
		item.CheckinTime = checkinTime.Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) BackupFeedbacks(ctx context.Context, groupID uint64) ([]Feedback, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT COALESCE(u.username,''),f.name,f.contact,f.message,f.page,f.user_agent,f.created_at
		FROM feedbacks f
		LEFT JOIN users u ON u.id=f.user_id
		LEFT JOIN group_members gm ON gm.user_id=f.user_id AND gm.group_id=? AND gm.status=1
		WHERE f.group_id=? OR (f.group_id IS NULL AND gm.id IS NOT NULL)
		ORDER BY f.id`, groupID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Feedback
	for rows.Next() {
		var item Feedback
		var created time.Time
		if err := rows.Scan(&item.Username, &item.Name, &item.Contact, &item.Message, &item.Page, &item.UserAgent, &created); err != nil {
			return nil, err
		}
		item.CreatedAt = created.Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) BackupAssets(ctx context.Context, groupID uint64) ([]Asset, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,category,title,original_name,storage_path,mime_type,file_size FROM assets WHERE group_id=? ORDER BY category,title,id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Asset
	for rows.Next() {
		var item Asset
		if err := rows.Scan(&item.ID, &item.Category, &item.Title, &item.OriginalName, &item.StoragePath, &item.MimeType, &item.FileSize); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) ReplaceStudyWeeks(ctx context.Context, groupID uint64, weeks []learning.WeekInput, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := learning.DeleteAllWeeksTx(ctx, tx, groupID); err != nil {
		return err
	}
	for _, week := range weeks {
		weekID, err := learning.InsertWeekTx(ctx, tx, groupID, week, now)
		if err != nil {
			return err
		}
		if err := learning.ReplaceWeekTasksTx(ctx, tx, groupID, weekID, learning.BuildTaskDrafts(week, ""), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MySQLRepository) ImportLocalBackup(ctx context.Context, groupID, actorID uint64, payload Payload, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	roleAssignments, err := r.importBackupMembersTx(ctx, tx, groupID, actorID, payload.Members)
	if err != nil {
		return err
	}
	if err := r.replaceRolesTx(ctx, tx, groupID, roleAssignments, now); err != nil {
		return err
	}
	assetIDs, err := r.importBackupAssetsTx(ctx, tx, groupID, actorID, payload.Assets, now)
	if err != nil {
		return err
	}
	settings, err := normalizeBackupSettings(payload.Settings, backupAssetResolver(ctx, tx, groupID, assetIDs))
	if err != nil {
		return err
	}
	if err := learning.UpsertLearningConfigTx(ctx, tx, groupID, settings); err != nil {
		return err
	}
	if err := learning.DeleteAllWeeksTx(ctx, tx, groupID); err != nil {
		return err
	}
	weekIDs := make(map[uint64]uint64, len(payload.Weeks))
	taskIDs := make(map[uint64]uint64)
	for _, originalWeek := range payload.Weeks {
		week, err := r.remapWeekAssetsTx(ctx, tx, groupID, originalWeek, assetIDs)
		if err != nil {
			return err
		}
		weekID, err := learning.InsertWeekTx(ctx, tx, groupID, week, now)
		if err != nil {
			return err
		}
		if originalWeek.ID > 0 {
			weekIDs[originalWeek.ID] = weekID
		}
		drafts := learning.BuildTaskDrafts(week, "")
		newTaskIDs, err := learning.ReplaceWeekTasksWithIDsTx(ctx, tx, groupID, weekID, drafts, now)
		if err != nil {
			return err
		}
		mapBackupTaskIDs(originalWeek, drafts, newTaskIDs, taskIDs)
	}
	userIDs, err := r.usernameMapFromRolesTx(ctx, tx, roleAssignments)
	if err != nil {
		return err
	}
	if err := r.replaceCheckinsTx(ctx, tx, groupID, actorID, userIDs, weekIDs, taskIDs, payload.Checkins, now); err != nil {
		return err
	}
	if err := r.replaceFeedbacksTx(ctx, tx, groupID, userIDs, payload.Feedbacks, now); err != nil {
		return err
	}
	return tx.Commit()
}

type backupAssetResolveFunc func(value, preferredCategory string) (uint64, error)

func normalizeBackupSettings(settings map[string]any, resolve backupAssetResolveFunc) (map[string]any, error) {
	normalized, err := cloneBackupSettings(settings)
	if err != nil {
		return nil, err
	}
	daily, ok := nestedBackupSettingsMap(normalized, "task_sections", "daily")
	if !ok {
		return normalized, nil
	}

	dailyPath, hasDailyPath, err := normalizeBackupSettingAssetPath(daily["path"], "markdown", resolve)
	if err != nil {
		return nil, err
	}
	if hasDailyPath {
		daily["path"] = dailyPath
	} else if shouldDropBackupSettingPath(daily["path"]) {
		delete(daily, "path")
	}

	devotion, ok := nestedBackupSettingsMap(daily, "devotion")
	if !ok {
		return normalized, nil
	}
	devotionPath, hasDevotionPath, err := normalizeBackupSettingAssetPath(devotion["path"], "markdown", resolve)
	if err != nil {
		return nil, err
	}
	switch {
	case hasDevotionPath:
		devotion["path"] = devotionPath
	case hasDailyPath:
		devotion["path"] = dailyPath
	case shouldDropBackupSettingPath(devotion["path"]):
		delete(devotion, "path")
	}
	return normalized, nil
}

func cloneBackupSettings(settings map[string]any) (map[string]any, error) {
	if settings == nil {
		return map[string]any{}, nil
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("marshal backup settings: %w", err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return nil, fmt.Errorf("unmarshal backup settings: %w", err)
	}
	if cloned == nil {
		return map[string]any{}, nil
	}
	return cloned, nil
}

func nestedBackupSettingsMap(root map[string]any, path ...string) (map[string]any, bool) {
	current := root
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func normalizeBackupSettingAssetPath(value any, preferredCategory string, resolve backupAssetResolveFunc) (string, bool, error) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", false, nil
	}
	assetID, err := resolve(text, preferredCategory)
	if err != nil {
		return "", false, err
	}
	if assetID == 0 {
		return "", false, nil
	}
	return backupAssetDownloadURL(assetID), true, nil
}

func backupAssetResolver(ctx context.Context, tx *sql.Tx, groupID uint64, assetIDs map[uint64]uint64) backupAssetResolveFunc {
	return func(value, preferredCategory string) (uint64, error) {
		if oldID := backupAssetIDFromDownloadURL(value); oldID > 0 {
			if newID := assetIDs[oldID]; newID > 0 {
				return newID, nil
			}
			exists, err := activeBackupAssetExistsTx(ctx, tx, groupID, oldID)
			if err != nil {
				return 0, fmt.Errorf("check backup asset %d: %w", oldID, err)
			}
			if exists {
				return oldID, nil
			}
			return 0, nil
		}

		fileName := backupResourceFileName(value)
		if fileName == "" {
			return 0, nil
		}
		assetID, err := findBackupAssetByFileNameTx(ctx, tx, groupID, fileName, preferredCategory)
		if err != nil {
			return 0, fmt.Errorf("find backup asset %q: %w", fileName, err)
		}
		return assetID, nil
	}
}

func activeBackupAssetExistsTx(ctx context.Context, tx *sql.Tx, groupID, assetID uint64) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `SELECT 1
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.id=? AND a.group_id=? AND a.storage_path LIKE ?
		LIMIT 1`, assetID, groupID, resourceStoragePattern).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func findBackupAssetByFileNameTx(ctx context.Context, tx *sql.Tx, groupID uint64, fileName, preferredCategory string) (uint64, error) {
	var assetID uint64
	err := tx.QueryRowContext(ctx, `SELECT a.id
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.group_id=? AND a.storage_path LIKE ?
		  AND (a.original_name=? OR SUBSTRING_INDEX(a.storage_path,'/',-1)=?)
		ORDER BY CASE WHEN a.category=? THEN 0 ELSE 1 END,a.id
		LIMIT 1`, groupID, resourceStoragePattern, fileName, fileName, preferredCategory).Scan(&assetID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return assetID, err
}

func backupAssetIDFromDownloadURL(value string) uint64 {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0
	}
	pathValue := text
	if parsed, err := url.Parse(text); err == nil && parsed.Path != "" {
		pathValue = parsed.Path
	}
	rest, ok := strings.CutPrefix(pathValue, "/api/assets/")
	if !ok {
		return 0
	}
	idText, ok := strings.CutSuffix(rest, "/download")
	if !ok || idText == "" {
		return 0
	}
	id, err := strconv.ParseUint(idText, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func backupAssetDownloadURL(assetID uint64) string {
	if assetID == 0 {
		return ""
	}
	return "/api/assets/" + strconv.FormatUint(assetID, 10) + "/download"
}

func backupResourceFileName(value string) string {
	text := strings.TrimSpace(value)
	if text == "" || backupAssetIDFromDownloadURL(text) > 0 {
		return ""
	}
	if parsed, err := url.Parse(text); err == nil && parsed.Scheme != "" {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return ""
		}
		text = parsed.Path
	}
	if decoded, err := url.PathUnescape(text); err == nil {
		text = decoded
	}
	name := path.Base(strings.ReplaceAll(text, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		return ""
	}
	return name
}

func shouldDropBackupSettingPath(value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if backupAssetIDFromDownloadURL(text) > 0 {
		return true
	}
	if parsed, err := url.Parse(text); err == nil && parsed.Scheme != "" {
		return false
	}
	return true
}

func mapBackupTaskIDs(
	week learning.WeekInput,
	drafts []learning.TaskDraft,
	newTaskIDs []uint64,
	taskIDs map[uint64]uint64,
) {
	used := make([]bool, len(drafts))
	mapBinding := func(binding learning.TaskBinding, taskType string) {
		if binding.TaskID == 0 {
			return
		}
		for index, draft := range drafts {
			if used[index] || draft.TaskType != taskType || draft.Title != binding.Title {
				continue
			}
			if index < len(newTaskIDs) {
				taskIDs[binding.TaskID] = newTaskIDs[index]
			}
			used[index] = true
			return
		}
	}
	for _, reading := range week.Readings {
		mapBinding(reading, "weekly_book")
	}
	for _, video := range week.Videos {
		mapBinding(video, "weekly_video")
	}
}

func (r *MySQLRepository) importBackupAssetsTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID, actorID uint64,
	assets []Asset,
	now time.Time,
) (map[uint64]uint64, error) {
	assetIDs := make(map[uint64]uint64, len(assets))
	for _, item := range assets {
		storagePath := strings.TrimSpace(item.StoragePath)
		if storagePath == "" {
			continue
		}
		title := canonicalBackupAssetTitle(item.Category, item.Title, item.OriginalName)
		var id uint64
		err := tx.QueryRowContext(ctx, `
			SELECT id
			FROM assets
			WHERE group_id=? AND storage_path=?
			ORDER BY id
			LIMIT 1`,
			groupID,
			storagePath,
		).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			res, insertErr := tx.ExecContext(ctx, `
				INSERT INTO assets
				(group_id,category,title,original_name,storage_path,mime_type,
				 file_size,visibility,created_by,created_at,updated_at)
				VALUES (?,?,?,?,?,?,?,'group',?,?,?)`,
				groupID,
				item.Category,
				title,
				item.OriginalName,
				storagePath,
				item.MimeType,
				item.FileSize,
				actorID,
				now,
				now,
			)
			if insertErr != nil {
				return nil, insertErr
			}
			newID, insertErr := res.LastInsertId()
			if insertErr != nil {
				return nil, insertErr
			}
			if newID <= 0 {
				return nil, errors.New("invalid_insert_id")
			}
			id = uint64(newID)
		} else if err != nil {
			return nil, err
		} else {
			if _, err := tx.ExecContext(ctx, `
				UPDATE assets
				SET category=?,title=?,original_name=?,mime_type=?,
				    file_size=?,updated_at=?
				WHERE id=? AND group_id=?`,
				item.Category,
				title,
				item.OriginalName,
				item.MimeType,
				item.FileSize,
				now,
				id,
				groupID,
			); err != nil {
				return nil, err
			}
		}
		if err := ensureBackupAssetBindingTx(ctx, tx, groupID, actorID, id, storagePath, now); err != nil {
			return nil, err
		}
		if item.ID > 0 {
			assetIDs[item.ID] = id
		}
	}
	return assetIDs, nil
}

func ensureBackupAssetBindingTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID, actorID, assetID uint64,
	storagePath string,
	now time.Time,
) error {
	resourceKey := backupResourceKeyFromStoragePath(storagePath)
	if resourceKey != "" {
		ownerID, exists, err := backupResourceKeyOwnerTx(ctx, tx, resourceKey)
		if err != nil {
			return err
		}
		if exists && ownerID != assetID {
			resourceKey = ""
		}
	}
	if resourceKey == "" {
		var err error
		resourceKey, err = randomUnusedBackupResourceKeyTx(ctx, tx)
		if err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_bindings
		(asset_id,group_id,resource_key,asset_kind,source_asset_id,imported_at,deleted_at,created_at,updated_at)
		VALUES (?,?,?,?,NULL,NULL,NULL,?,?)
		ON DUPLICATE KEY UPDATE group_id=VALUES(group_id),resource_key=VALUES(resource_key),asset_kind=VALUES(asset_kind),
			source_asset_id=NULL,imported_at=NULL,deleted_at=NULL,updated_at=VALUES(updated_at)`,
		assetID, groupID, resourceKey, assetKindOwned, now, now); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO asset_share_grants
		(asset_id,owner_group_id,consumer_group_id,permission,status,created_by,created_at,revoked_by,revoked_at)
		VALUES (?,?,NULL,?,?,?,?,NULL,NULL)
		ON DUPLICATE KEY UPDATE status=VALUES(status),created_by=VALUES(created_by),created_at=VALUES(created_at),
			revoked_by=NULL,revoked_at=NULL`,
		assetID, groupID, sharePermissionImport, shareStatusActive, firstNonZero(actorID, 1), now)
	return err
}

func backupResourceKeyOwnerTx(ctx context.Context, tx *sql.Tx, resourceKey string) (uint64, bool, error) {
	var assetID uint64
	err := tx.QueryRowContext(ctx, `SELECT asset_id FROM asset_bindings WHERE resource_key=? LIMIT 1`, resourceKey).Scan(&assetID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return assetID, true, nil
}

func randomUnusedBackupResourceKeyTx(ctx context.Context, tx *sql.Tx) (string, error) {
	for attempt := 0; attempt < 16; attempt++ {
		resourceKey, err := randomBackupResourceKey()
		if err != nil {
			return "", err
		}
		if _, exists, err := backupResourceKeyOwnerTx(ctx, tx, resourceKey); err != nil {
			return "", err
		} else if !exists {
			return resourceKey, nil
		}
	}
	return "", errors.New("generate_unique_resource_key_failed")
}

func randomBackupResourceKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate resource key: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func backupResourceKeyFromStoragePath(storagePath string) string {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(storagePath, "\\", "/"), "/"), "/")
	if len(parts) < 4 || strings.ToLower(parts[1]) != "objects" || !isBackupResourceKey(parts[2]) {
		return ""
	}
	return strings.ToLower(parts[2])
}

func isBackupResourceKey(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, r := range value {
		if !('0' <= r && r <= '9') && !('a' <= r && r <= 'f') && !('A' <= r && r <= 'F') {
			return false
		}
	}
	return true
}

func (r *MySQLRepository) remapWeekAssetsTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID uint64,
	week learning.WeekInput,
	assetIDs map[uint64]uint64,
) (learning.WeekInput, error) {
	for index := range week.Readings {
		id, err := remapTaskBindingAssetIDTx(
			ctx,
			tx,
			groupID,
			week.Readings[index].AssetID,
			week.Readings[index].Title,
			week.Readings[index].URL,
			"book",
			assetIDs,
		)
		if err != nil {
			return learning.WeekInput{}, err
		}
		week.Readings[index].AssetID = id
	}
	for index := range week.Videos {
		id, err := remapTaskBindingAssetIDTx(
			ctx,
			tx,
			groupID,
			week.Videos[index].AssetID,
			week.Videos[index].Title,
			week.Videos[index].URL,
			"video",
			assetIDs,
		)
		if err != nil {
			return learning.WeekInput{}, err
		}
		week.Videos[index].AssetID = id
	}
	id, err := remapTaskBindingAssetIDTx(
		ctx,
		tx,
		groupID,
		week.Outline.AssetID,
		week.Outline.Title,
		week.Outline.URL,
		"outline",
		assetIDs,
	)
	if err != nil {
		return learning.WeekInput{}, err
	}
	week.Outline.AssetID = id
	return week, nil
}

func remapTaskBindingAssetIDTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID, oldID uint64,
	title, urlValue, preferredCategory string,
	assetIDs map[uint64]uint64,
) (uint64, error) {
	for _, assetID := range []uint64{oldID, backupAssetIDFromDownloadURL(urlValue)} {
		if assetID == 0 {
			continue
		}
		if id := assetIDs[assetID]; id > 0 {
			return id, nil
		}
		exists, err := activeBackupAssetExistsTx(ctx, tx, groupID, assetID)
		if err != nil {
			return 0, err
		}
		if exists {
			return assetID, nil
		}
	}

	if fileName := backupResourceFileName(urlValue); fileName != "" {
		id, err := findBackupAssetByFileNameTx(ctx, tx, groupID, fileName, preferredCategory)
		if err != nil || id > 0 {
			return id, err
		}
	}

	return findBackupTaskAssetByReferenceTx(ctx, tx, groupID, preferredCategory, backupTaskAssetRefs(title, urlValue))
}

func findBackupTaskAssetByReferenceTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID uint64,
	preferredCategory string,
	refs []string,
) (uint64, error) {
	if len(refs) == 0 {
		return 0, nil
	}

	matchedID := uint64(0)
	matchedScore := 0
	ambiguous := false
	for _, ref := range refs {
		refKey := normalizeBackupTaskAssetText(ref)
		if refKey == "" {
			continue
		}
		rows, err := tx.QueryContext(ctx, `SELECT a.id,a.title,a.original_name
			FROM assets a
			JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
			WHERE a.group_id=? AND a.storage_path LIKE ? AND a.category=?
			ORDER BY a.id`, groupID, resourceStoragePattern, preferredCategory)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var id uint64
			var title, originalName string
			if err := rows.Scan(&id, &title, &originalName); err != nil {
				_ = rows.Close()
				return 0, err
			}
			score := maxInt(
				backupTaskAssetMatchScore(refKey, normalizeBackupTaskAssetText(title)),
				backupTaskAssetMatchScore(refKey, normalizeBackupTaskAssetText(originalName)),
			)
			if score > matchedScore {
				matchedID = id
				matchedScore = score
				ambiguous = false
			} else if score > 0 && score == matchedScore && id != matchedID {
				ambiguous = true
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if err := rows.Close(); err != nil {
			return 0, err
		}
	}
	if ambiguous || matchedScore == 0 {
		return 0, nil
	}
	return matchedID, nil
}

func backupTaskAssetRefs(title, urlValue string) []string {
	var refs []string
	add := func(value string) {
		if normalizeBackupTaskAssetText(value) == "" {
			return
		}
		for _, existing := range refs {
			if normalizeBackupTaskAssetText(existing) == normalizeBackupTaskAssetText(value) {
				return
			}
		}
		refs = append(refs, strings.TrimSpace(value))
	}

	var metadata struct {
		BookName    string `json:"book_name"`
		SourceTitle string `json:"source_title"`
	}
	if strings.HasPrefix(strings.TrimSpace(urlValue), "{") && json.Unmarshal([]byte(urlValue), &metadata) == nil {
		add(metadata.SourceTitle)
		add(metadata.BookName)
	}
	add(title)
	return refs
}

func backupTaskAssetMatchScore(refKey, candidateKey string) int {
	if refKey == "" || candidateKey == "" {
		return 0
	}
	switch {
	case refKey == candidateKey:
		return 100
	case strings.Contains(refKey, candidateKey) && len([]rune(candidateKey)) >= 4:
		return 90
	case strings.Contains(candidateKey, refKey) && len([]rune(refKey)) >= 4:
		return 80
	default:
		return 0
	}
}

func normalizeBackupTaskAssetText(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	if decoded, err := url.PathUnescape(text); err == nil {
		text = decoded
	}
	text = path.Base(strings.ReplaceAll(text, "\\", "/"))
	text = strings.TrimSuffix(text, path.Ext(text))
	text = regexp.MustCompile(`[0-9]{1,4}\s*(?:[-~—–至到]\s*[0-9]{1,4})?\s*页`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`[12][0-9]{5,7}`).ReplaceAllString(text, "")

	var builder strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
		}
	}
	return builder.String()
}

func canonicalBackupAssetTitle(category, title, originalName string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "book", "passage":
	default:
		return title
	}
	if !regexp.MustCompile(`[0-9]{1,4}\s*(?:[-~—–至到]\s*[0-9]{1,4})?\s*页`).MatchString(title) {
		return title
	}
	baseTitle := strings.TrimSpace(strings.TrimSuffix(path.Base(originalName), path.Ext(originalName)))
	if baseTitle == "" || normalizeBackupTaskAssetText(baseTitle) == normalizeBackupTaskAssetText(title) {
		return title
	}
	return baseTitle
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func firstNonZero(values ...uint64) uint64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (r *MySQLRepository) memberRoleMap(ctx context.Context, groupID uint64) (map[string][]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT u.username,r.role
		FROM user_group_roles r JOIN users u ON u.id=r.user_id
		WHERE r.group_id=? AND r.role IN (?,?)
		ORDER BY u.username,r.role`, groupID, roleGroupAdmin, roleGroupLeader)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roleMap := map[string][]string{}
	for rows.Next() {
		var username, role string
		if err := rows.Scan(&username, &role); err != nil {
			return nil, err
		}
		roleMap[username] = append(roleMap[username], role)
	}
	return roleMap, rows.Err()
}

func (r *MySQLRepository) importBackupMembersTx(ctx context.Context, tx *sql.Tx, groupID, actorID uint64, members []Member) (map[uint64][]string, error) {
	roleAssignments := map[uint64][]string{}
	for _, member := range members {
		userID, err := ensureGroupMemberUserTx(ctx, tx, groupID, member, actorID)
		if err != nil {
			return nil, err
		}
		roleAssignments[userID] = append([]string{}, member.Roles...)
	}
	return roleAssignments, nil
}

func ensureGroupMemberUserTx(ctx context.Context, tx *sql.Tx, groupID uint64, member Member, actorID uint64) (uint64, error) {
	username := normalizeUsername(member.Username)
	if username == "" {
		return 0, errors.New("username_required")
	}
	displayName := firstNonEmpty(strings.TrimSpace(member.DisplayName), username)
	namePinyin := firstNonEmpty(strings.TrimSpace(member.NamePinyin), username)
	var userID uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, username).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		hash, err := groupDefaultPasswordHashTx(ctx, tx, groupID)
		if err != nil {
			return 0, err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO users (username,display_name,name_pinyin,password_hash,created_by,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?)`, username, displayName, namePinyin, hash, actorID, nowSQL(), nowSQL())
		if err != nil {
			return 0, err
		}
		id64, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		if id64 <= 0 {
			return 0, errors.New("invalid_insert_id")
		}
		userID = uint64(id64)
	} else if err != nil {
		return 0, err
	}
	if err := addMemberTx(ctx, tx, groupID, userID, displayName, actorID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET display_name=?, name_pinyin=?, updated_at=? WHERE id=?`, displayName, namePinyin, nowSQL(), userID); err != nil {
		return 0, err
	}
	return userID, nil
}

func (r *MySQLRepository) replaceRolesTx(ctx context.Context, tx *sql.Tx, groupID uint64, roleAssignments map[uint64][]string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_group_roles WHERE group_id=? AND role IN (?,?)`, groupID, roleGroupAdmin, roleGroupLeader); err != nil {
		return err
	}
	for userID, roles := range roleAssignments {
		for _, role := range roles {
			role = strings.TrimSpace(role)
			if role != roleGroupAdmin && role != roleGroupLeader {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO user_group_roles (group_id,user_id,role,created_at) VALUES (?,?,?,?)`, groupID, userID, role, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *MySQLRepository) usernameMapFromRolesTx(ctx context.Context, tx *sql.Tx, roleAssignments map[uint64][]string) (map[string]uint64, error) {
	userIDs := map[string]uint64{}
	for userID := range roleAssignments {
		var username string
		if err := tx.QueryRowContext(ctx, `SELECT username FROM users WHERE id=?`, userID).Scan(&username); err == nil {
			userIDs[username] = userID
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return userIDs, nil
}

func (r *MySQLRepository) replaceCheckinsTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID, actorID uint64,
	userIDs map[string]uint64,
	weekIDs map[uint64]uint64,
	taskIDs map[uint64]uint64,
	checkins []Checkin,
	now time.Time,
) error {
	if _, err := tx.ExecContext(ctx, `UPDATE checkin_records SET deleted_at=?, active_key=id, updated_at=? WHERE group_id=? AND deleted_at IS NULL`, nowSQL(), nowSQL(), groupID); err != nil {
		return err
	}
	for _, checkin := range checkins {
		userID := userIDs[normalizeUsername(checkin.Username)]
		if userID == 0 {
			continue
		}
		logicalDate, err := normalizeBackupLogicalDate(checkin.LogicalDate)
		if err != nil {
			return err
		}
		checkin.LogicalDate = logicalDate
		checkinTime := parseTimeOrNow(checkin.CheckinTime, now)
		taskID, weekID, err := resolveCheckinTargetTx(ctx, tx, groupID, weekIDs, taskIDs, checkin)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO checkin_records (group_id,user_id,task_id,week_id,logical_date,checkin_time,task_type,status,is_retro,detail,note,part,source,created_by,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			groupID, userID, taskID, weekID, checkin.LogicalDate, checkinTime, checkin.TaskType, "done", checkin.IsRetro, checkin.Detail, checkin.Note, truncate(checkin.Part, 64), "import", actorID, checkinTime, checkinTime); err != nil {
			return err
		}
	}
	return nil
}

func resolveCheckinTargetTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID uint64,
	weekIDs map[uint64]uint64,
	taskIDs map[uint64]uint64,
	checkin Checkin,
) (any, any, error) {
	if checkin.TaskType == "daily_devotion" {
		return nil, nil, nil
	}
	weekID := weekIDs[checkin.WeekID]
	if taskID := taskIDs[checkin.TaskID]; taskID > 0 && weekID > 0 {
		var validatedTaskID uint64
		err := tx.QueryRowContext(ctx, `
			SELECT id
			FROM study_tasks
			WHERE id=? AND group_id=? AND week_id=? AND task_type=?
			LIMIT 1`,
			taskID,
			groupID,
			weekID,
			checkin.TaskType,
		).Scan(&validatedTaskID)
		if err == nil {
			return validatedTaskID, weekID, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, nil, err
		}
	}
	if weekID == 0 {
		err := tx.QueryRowContext(ctx, `
			SELECT id
			FROM study_weeks
			WHERE group_id=? AND start_date<=? AND end_date>=?
			ORDER BY start_date DESC
			LIMIT 1`,
			groupID,
			checkin.LogicalDate,
			checkin.LogicalDate,
		).Scan(&weekID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
	}

	var taskID uint64
	title := strings.TrimSpace(firstNonEmpty(checkin.Part, checkin.Detail))
	var err error
	if checkin.TaskType == "weekly_book" && title != "" {
		err = tx.QueryRowContext(ctx, `
			SELECT id
			FROM study_tasks
			WHERE group_id=? AND week_id=? AND task_type=? AND title=?
			ORDER BY sort_order,id
			LIMIT 1`,
			groupID,
			weekID,
			checkin.TaskType,
			title,
		).Scan(&taskID)
	} else {
		err = tx.QueryRowContext(ctx, `
			SELECT id
			FROM study_tasks
			WHERE group_id=? AND week_id=? AND task_type=?
			ORDER BY sort_order,id
			LIMIT 1`,
			groupID,
			weekID,
			checkin.TaskType,
		).Scan(&taskID)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, weekID, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return taskID, weekID, nil
}

func (r *MySQLRepository) replaceFeedbacksTx(ctx context.Context, tx *sql.Tx, groupID uint64, userIDs map[string]uint64, feedbacks []Feedback, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM feedbacks WHERE group_id=?`, groupID); err != nil {
		return err
	}
	for _, feedback := range feedbacks {
		var userID any
		if id := userIDs[normalizeUsername(feedback.Username)]; id > 0 {
			userID = id
		}
		createdAt := parseTimeOrNow(feedback.CreatedAt, now)
		if _, err := tx.ExecContext(ctx, `INSERT INTO feedbacks (group_id,user_id,name,contact,message,page,user_agent,created_at)
			VALUES (?,?,?,?,?,?,?,?)`, groupID, userID, feedback.Name, feedback.Contact, feedback.Message, feedback.Page, feedback.UserAgent, createdAt); err != nil {
			return err
		}
	}
	return nil
}

func groupDefaultPasswordHashTx(ctx context.Context, tx *sql.Tx, groupID uint64) (string, error) {
	var hash string
	err := tx.QueryRowContext(ctx, `SELECT default_password_hash FROM study_groups WHERE id=?`, groupID).Scan(&hash)
	return hash, err
}

func addMemberTx(ctx context.Context, tx *sql.Tx, groupID, userID uint64, memberName string, actorID uint64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO group_members (group_id,user_id,member_name,joined_at,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE status=1, updated_at=VALUES(updated_at)`, groupID, userID, memberName, nowSQL(), actorID, nowSQL(), nowSQL())
	return err
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseTimeOrNow(value string, fallback time.Time) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04:05.000"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return fallback
}

func normalizeBackupLogicalDate(value string) (string, error) {
	text := strings.TrimSpace(value)
	if len(text) >= len("2006-01-02") {
		text = text[:len("2006-01-02")]
	}
	if _, err := time.Parse("2006-01-02", text); err != nil {
		return "", fmt.Errorf("invalid logical_date %q: %w", value, err)
	}
	return text, nil
}

func truncate(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= n {
		return string(rs)
	}
	return string(rs[:n])
}

func nowSQL() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05.000")
}
