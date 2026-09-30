package backup

import (
	"context"
	"database/sql"
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

	"agp/backend/internal/asset"
	checkindomain "agp/backend/internal/checkin"
	"agp/backend/internal/learning"
)

const (
	roleGroupAdmin  = "group_admin"
	roleGroupLeader = "group_leader"

	resourceStoragePattern = "team-%-resources/objects/%"
)

type MySQLRepository struct {
	db *sql.DB
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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

func (r *MySQLRepository) GroupInfo(ctx context.Context, groupID uint64) (*GroupInfo, error) {
	return backupGroupInfo(ctx, r.db, groupID)
}

func backupGroupInfo(ctx context.Context, q queryer, groupID uint64) (*GroupInfo, error) {
	var item GroupInfo
	item.ID = groupID
	err := q.QueryRowContext(ctx, `SELECT code,name,description FROM study_groups WHERE id=?`, groupID).Scan(&item.Code, &item.Name, &item.Description)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MySQLRepository) BackupMembers(ctx context.Context, groupID uint64) ([]Member, error) {
	return backupMembers(ctx, r.db, groupID)
}

func backupMembers(ctx context.Context, q queryer, groupID uint64) ([]Member, error) {
	roleMap, err := memberRoleMap(ctx, q, groupID)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT u.username,u.display_name,u.name_pinyin,m.member_name,m.status
		FROM group_members m JOIN users u ON u.id=m.user_id
		WHERE m.group_id=? AND (
			m.status=1 OR EXISTS (
				SELECT 1 FROM checkin_records c
				WHERE c.group_id=m.group_id AND c.user_id=m.user_id AND c.deleted_at IS NULL
			)
		)
		ORDER BY m.status DESC,m.member_name,u.username`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Member
	for rows.Next() {
		var item Member
		var status int
		if err := rows.Scan(&item.Username, &item.DisplayName, &item.NamePinyin, &item.MemberName, &status); err != nil {
			return nil, err
		}
		active := status == 1
		item.Active = &active
		if active {
			item.Roles = roleMap[item.Username]
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) BackupCheckins(ctx context.Context, groupID uint64) ([]Checkin, error) {
	return backupCheckins(ctx, r.db, groupID)
}

func backupCheckins(ctx context.Context, q queryer, groupID uint64) ([]Checkin, error) {
	rows, err := q.QueryContext(ctx, `SELECT u.username,c.task_id,c.week_id,c.logical_date,c.checkin_time,c.task_type,c.part,c.detail,COALESCE(c.note,''),c.is_retro
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

func (r *MySQLRepository) BackupAssets(ctx context.Context, groupID uint64) ([]Asset, error) {
	return backupAssets(ctx, r.db, groupID)
}

func backupAssets(ctx context.Context, q queryer, groupID uint64) ([]Asset, error) {
	rows, err := q.QueryContext(ctx, `SELECT a.id,a.category,a.title,a.original_name,a.storage_path,a.mime_type,a.file_size
		FROM assets a JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id
		WHERE a.group_id=? AND b.deleted_at IS NULL ORDER BY a.category,a.title,a.id`, groupID)
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

func (r *MySQLRepository) LocalBackupSnapshot(ctx context.Context, groupID uint64) (Snapshot, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()

	group, err := backupGroupInfo(ctx, tx, groupID)
	if err != nil {
		return Snapshot{}, err
	}
	settings, weeks, err := learning.BackupLearningDataTx(ctx, tx, groupID)
	if err != nil {
		return Snapshot{}, err
	}
	members, err := backupMembers(ctx, tx, groupID)
	if err != nil {
		return Snapshot{}, err
	}
	checkins, err := backupCheckins(ctx, tx, groupID)
	if err != nil {
		return Snapshot{}, err
	}
	assets, err := backupAssets(ctx, tx, groupID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Group:    *group,
		Settings: settings,
		Members:  members,
		Weeks:    weeks,
		Checkins: checkins,
		Assets:   assets,
	}, nil
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
	if err := ensureBackupLeaderRoleChangeAllowedTx(ctx, tx, groupID, actorID, roleAssignments); err != nil {
		return err
	}
	if err := r.replaceRolesTx(ctx, tx, groupID, roleAssignments, now); err != nil {
		return err
	}
	userIDs, err := backupUsernamesTx(ctx, tx, groupID)
	if err != nil {
		return err
	}
	if err := ensureHistoricalCheckinUsersTx(ctx, tx, groupID, actorID, userIDs, payload.Checkins); err != nil {
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
	candidates := make(backupTaskAssetCandidates)
	for _, originalWeek := range payload.Weeks {
		week, err := r.remapWeekAssetsTx(ctx, tx, groupID, originalWeek, assetIDs, candidates)
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
	if err := r.replaceCheckinsTx(ctx, tx, groupID, actorID, userIDs, weekIDs, taskIDs, payload.Checkins, now); err != nil {
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
	if err := normalizeBackupScheduleHistoryPaths(devotion, resolve); err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeBackupScheduleHistoryPaths(config map[string]any, resolve backupAssetResolveFunc) error {
	history, ok := config["schedule_history"].([]any)
	if !ok {
		return nil
	}
	for _, item := range history {
		version, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path, found, err := normalizeBackupSettingAssetPath(version["path"], "markdown", resolve)
		if err != nil {
			return err
		}
		switch {
		case found:
			version["path"] = path
		case shouldDropBackupSettingPath(version["path"]):
			delete(version, "path")
		}
	}
	return nil
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
	resources := asset.NewMySQLRepository(r.db)
	for _, item := range assets {
		storagePath := strings.TrimSpace(item.StoragePath)
		if storagePath == "" {
			continue
		}
		title := canonicalBackupAssetTitle(item.Category, item.Title, item.OriginalName)
		id, err := resources.RestoreReferenceTx(ctx, tx, groupID, actorID, storagePath, now)
		if err != nil {
			return nil, fmt.Errorf("restore backup asset %d: %w", item.ID, err)
		}
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
		if item.ID > 0 {
			assetIDs[item.ID] = id
		}
	}
	return assetIDs, nil
}

func (r *MySQLRepository) remapWeekAssetsTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID uint64,
	week learning.WeekInput,
	assetIDs map[uint64]uint64,
	candidates backupTaskAssetCandidates,
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
			candidates,
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
			candidates,
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
		candidates,
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
	candidates backupTaskAssetCandidates,
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

	return findBackupTaskAssetByReferenceTx(ctx, tx, groupID, preferredCategory, backupTaskAssetRefs(title, urlValue), candidates)
}

type backupTaskAssetCandidate struct {
	id                  uint64
	title, originalName string
}

// The cache belongs to one restore transaction, after all assets are restored.
type backupTaskAssetCandidates map[string][]backupTaskAssetCandidate

func findBackupTaskAssetByReferenceTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID uint64,
	preferredCategory string,
	refs []string,
	cache backupTaskAssetCandidates,
) (uint64, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	candidates, loaded := cache[preferredCategory]
	if !loaded {
		rows, err := tx.QueryContext(ctx, `SELECT a.id,a.title,a.original_name
			FROM assets a
			JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
			WHERE a.group_id=? AND a.storage_path LIKE ? AND a.category=?
			ORDER BY a.id`, groupID, resourceStoragePattern, preferredCategory)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var candidate backupTaskAssetCandidate
			if err := rows.Scan(&candidate.id, &candidate.title, &candidate.originalName); err != nil {
				_ = rows.Close()
				return 0, err
			}
			candidate.title = normalizeBackupTaskAssetText(candidate.title)
			candidate.originalName = normalizeBackupTaskAssetText(candidate.originalName)
			candidates = append(candidates, candidate)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if err := rows.Close(); err != nil {
			return 0, err
		}
		cache[preferredCategory] = candidates
	}
	matchedID := uint64(0)
	matchedScore := 0
	ambiguous := false
	for _, ref := range refs {
		refKey := normalizeBackupTaskAssetText(ref)
		if refKey == "" {
			continue
		}
		for _, candidate := range candidates {
			score := maxInt(
				backupTaskAssetMatchScore(refKey, candidate.title),
				backupTaskAssetMatchScore(refKey, candidate.originalName),
			)
			if score > matchedScore {
				matchedID = candidate.id
				matchedScore = score
				ambiguous = false
			} else if score > 0 && score == matchedScore && candidate.id != matchedID {
				ambiguous = true
			}
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

var (
	backupPageRangePattern = regexp.MustCompile(`[0-9]{1,4}\s*(?:[-~—–至到]\s*[0-9]{1,4})?\s*页`)
	backupDatePattern      = regexp.MustCompile(`[12][0-9]{5,7}`)
)

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
	text = backupPageRangePattern.ReplaceAllString(text, "")
	text = backupDatePattern.ReplaceAllString(text, "")

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
	if !backupPageRangePattern.MatchString(title) {
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

func memberRoleMap(ctx context.Context, q queryer, groupID uint64) (map[string][]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT u.username,r.role
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
		var (
			userID uint64
			err    error
		)
		if member.Active == nil || *member.Active {
			userID, err = ensureGroupMemberUserTx(ctx, tx, groupID, member, actorID)
		} else {
			userID, err = ensureHistoricalGroupMemberUserTx(ctx, tx, groupID, member, actorID)
		}
		if err != nil {
			return nil, err
		}
		if member.Active == nil || *member.Active {
			roleAssignments[userID] = append([]string{}, member.Roles...)
		}
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
	createdUser := false
	err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, username).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		createdUser = true
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
	if createdUser {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
			SELECT tenant_id,?,'member',1,UTC_TIMESTAMP(3),UTC_TIMESTAMP(3)
			FROM study_groups WHERE id=?`, userID, groupID); err != nil {
			return 0, err
		}
	} else {
		var allowed bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tenant_members tm
			JOIN study_groups g ON g.tenant_id=tm.tenant_id
			WHERE g.id=? AND tm.user_id=? AND tm.status=1)`, groupID, userID).Scan(&allowed); err != nil || !allowed {
			return 0, errors.New("backup_member_outside_tenant")
		}
	}
	memberName := firstNonEmpty(member.MemberName, displayName)
	if err := addMemberTx(ctx, tx, groupID, userID, memberName, actorID); err != nil {
		return 0, err
	}
	return userID, nil
}

func ensureBackupLeaderRoleChangeAllowedTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID, actorID uint64,
	roleAssignments map[uint64][]string,
) error {
	currentLeaders, err := lockedGroupLeaderIDsTx(ctx, tx, groupID)
	if err != nil {
		return err
	}
	nextLeaders := make(map[uint64]struct{})
	for userID, roles := range roleAssignments {
		for _, role := range roles {
			if strings.TrimSpace(role) == roleGroupLeader {
				nextLeaders[userID] = struct{}{}
				break
			}
		}
	}
	if sameUserIDSet(currentLeaders, nextLeaders) {
		return nil
	}
	if _, ok := currentLeaders[actorID]; ok {
		return nil
	}

	var isSuperAdmin, isTenantAdmin bool
	err = tx.QueryRowContext(ctx, `SELECT u.is_super_admin,COALESCE(tm.role='admin' AND tm.status=1,FALSE)
		FROM users u
		JOIN study_groups g ON g.id=?
		LEFT JOIN tenant_members tm ON tm.tenant_id=g.tenant_id AND tm.user_id=u.id
		WHERE u.id=? AND u.status=1
		FOR UPDATE`, groupID, actorID).Scan(&isSuperAdmin, &isTenantAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBackupRoleChangeForbidden
	}
	if err != nil {
		return err
	}
	if !isSuperAdmin && !isTenantAdmin {
		return ErrBackupRoleChangeForbidden
	}
	return nil
}

func lockedGroupLeaderIDsTx(ctx context.Context, tx *sql.Tx, groupID uint64) (map[uint64]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `SELECT user_id FROM user_group_roles
		WHERE group_id=? AND role=? ORDER BY user_id FOR UPDATE`, groupID, roleGroupLeader)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leaders := make(map[uint64]struct{})
	for rows.Next() {
		var userID uint64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		leaders[userID] = struct{}{}
	}
	return leaders, rows.Err()
}

func sameUserIDSet(left, right map[uint64]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for userID := range left {
		if _, ok := right[userID]; !ok {
			return false
		}
	}
	return true
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

// Historical membership resolves checkin authors without reactivating them.
func backupUsernamesTx(ctx context.Context, tx *sql.Tx, groupID uint64) (map[string]uint64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT u.username,u.id
		FROM group_members m JOIN users u ON u.id=m.user_id WHERE m.group_id=?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	userIDs := map[string]uint64{}
	for rows.Next() {
		var username string
		var userID uint64
		if err := rows.Scan(&username, &userID); err != nil {
			return nil, err
		}
		userIDs[normalizeUsername(username)] = userID
	}
	return userIDs, rows.Err()
}

func ensureHistoricalCheckinUsersTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID, actorID uint64,
	userIDs map[string]uint64,
	checkins []Checkin,
) error {
	for _, checkin := range checkins {
		username := normalizeUsername(checkin.Username)
		if username == "" {
			return errors.New("username_required")
		}
		if userIDs[username] > 0 {
			continue
		}
		userID, err := ensureHistoricalGroupMemberUserTx(ctx, tx, groupID, Member{
			Username:    username,
			DisplayName: username,
			NamePinyin:  username,
			MemberName:  username,
		}, actorID)
		if err != nil {
			return fmt.Errorf("restore historical member %q: %w", checkin.Username, err)
		}
		userIDs[username] = userID
	}
	return nil
}

func ensureHistoricalGroupMemberUserTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID uint64,
	member Member,
	actorID uint64,
) (uint64, error) {
	username := normalizeUsername(member.Username)
	if username == "" {
		return 0, errors.New("username_required")
	}
	displayName := firstNonEmpty(strings.TrimSpace(member.DisplayName), username)
	namePinyin := firstNonEmpty(strings.TrimSpace(member.NamePinyin), username)
	memberName := firstNonEmpty(strings.TrimSpace(member.MemberName), displayName)
	var userID uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, username).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		hash, hashErr := groupDefaultPasswordHashTx(ctx, tx, groupID)
		if hashErr != nil {
			return 0, hashErr
		}
		result, insertErr := tx.ExecContext(ctx, `INSERT INTO users
			(username,display_name,name_pinyin,password_hash,created_by,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?)`,
			username, displayName, namePinyin, hash, actorID, nowSQL(), nowSQL())
		if insertErr != nil {
			return 0, insertErr
		}
		id, idErr := result.LastInsertId()
		if idErr != nil {
			return 0, idErr
		}
		if id <= 0 {
			return 0, errors.New("invalid_insert_id")
		}
		userID = uint64(id)
	} else if err != nil {
		return 0, err
	} else {
		var allowed bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM group_members WHERE group_id=? AND user_id=?
			UNION ALL
			SELECT 1 FROM tenant_members tm
			JOIN study_groups g ON g.tenant_id=tm.tenant_id
			WHERE g.id=? AND tm.user_id=?
		)`, groupID, userID, groupID, userID).Scan(&allowed); err != nil {
			return 0, err
		}
		if !allowed {
			return 0, errors.New("backup_member_outside_tenant")
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO group_members
		(group_id,user_id,member_name,status,joined_at,created_by,created_at,updated_at)
		VALUES (?,?,?,0,?,?,?,?)`,
		groupID, userID, memberName, nowSQL(), actorID, nowSQL(), nowSQL()); err != nil {
		return 0, err
	}
	return userID, nil
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
			return fmt.Errorf("backup checkin user %q has no group identity", checkin.Username)
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
		activeKey := uint64(0)
		if resolvedTaskID, ok := taskID.(uint64); ok {
			activeKey = checkindomain.ActiveRecordKey(checkin.TaskType, resolvedTaskID)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO checkin_records (group_id,user_id,task_id,week_id,logical_date,checkin_time,task_type,status,is_retro,detail,note,part,source,active_key,created_by,created_at,updated_at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			groupID, userID, taskID, weekID, checkin.LogicalDate, checkinTime, checkin.TaskType, "done", checkin.IsRetro, checkin.Detail, checkin.Note, truncate(checkin.Part, 64), "import", activeKey, actorID, checkinTime, checkinTime); err != nil {
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
	if checkin.TaskType == "daily_devotion" || checkin.TaskType == "daily_scripture" {
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

func groupDefaultPasswordHashTx(ctx context.Context, tx *sql.Tx, groupID uint64) (string, error) {
	var hash string
	err := tx.QueryRowContext(ctx, `SELECT default_password_hash FROM study_groups WHERE id=?`, groupID).Scan(&hash)
	return hash, err
}

func addMemberTx(ctx context.Context, tx *sql.Tx, groupID, userID uint64, memberName string, actorID uint64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO group_members (group_id,user_id,member_name,joined_at,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE status=1, member_name=VALUES(member_name), updated_at=VALUES(updated_at)`, groupID, userID, memberName, nowSQL(), actorID, nowSQL(), nowSQL())
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
