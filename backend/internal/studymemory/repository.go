package studymemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type Progress struct {
	Time      float64 `json:"time"`
	Duration  float64 `json:"duration"`
	Completed bool    `json:"completed"`
	UpdatedAt int64   `json:"updatedAt"`
}
type Favorite struct {
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	CourseTitle string `json:"courseTitle"`
	Type        string `json:"type"`
	URL         string `json:"url,omitempty"`
	CourseID    string `json:"courseId,omitempty"`
	LessonID    string `json:"lessonId,omitempty"`
	SavedAt     int64  `json:"savedAt"`
}
type Snapshot struct {
	Progress  map[string]Progress `json:"progress"`
	Favorites map[string]Favorite `json:"favorites"`
}
type Repository interface {
	Load(context.Context, uint64) (Snapshot, error)
	SaveProgress(context.Context, uint64, string, Progress) error
	SetFavorite(context.Context, uint64, string, *Favorite) error
}

func ValidKey(key string) bool {
	return strings.TrimSpace(key) != "" && utf8.RuneCountInString(key) <= 512
}
func ValidProgress(p Progress) bool {
	return !math.IsNaN(p.Time) && !math.IsInf(p.Time, 0) && p.Time >= 0 && p.Time <= 1e9 &&
		!math.IsNaN(p.Duration) && !math.IsInf(p.Duration, 0) && p.Duration >= 0 && p.Duration <= 1e9
}
func ValidFavorite(f Favorite) bool {
	if !ValidKey(f.Key) || strings.TrimSpace(f.Title) == "" || len(f.Title) > 2048 || len(f.CourseTitle) > 2048 || (f.Type != "audio" && f.Type != "video") {
		return false
	}
	if f.Kind == "ovcm" {
		return f.CourseID != "" && f.LessonID != "" && len(f.CourseID) <= 256 && len(f.LessonID) <= 256
	}
	if f.Kind != "media" || len(f.URL) > 4096 {
		return false
	}
	u, err := url.Parse(f.URL)
	return err == nil && ((u.Scheme == "http" || u.Scheme == "https") && u.Host != "" || strings.HasPrefix(f.URL, "/api/assets/") && strings.HasSuffix(f.URL, "/download"))
}

type MySQLRepository struct{ db *sql.DB }

func NewMySQLRepository(db *sql.DB) *MySQLRepository { return &MySQLRepository{db: db} }
func (r *MySQLRepository) Load(ctx context.Context, userID uint64) (Snapshot, error) {
	result := Snapshot{Progress: map[string]Progress{}, Favorites: map[string]Favorite{}}
	rows, err := r.db.QueryContext(ctx, `SELECT media_key,position_seconds,duration_seconds,completed,updated_at FROM study_progress WHERE user_id=?`, userID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var key string
		var p Progress
		var at time.Time
		if err := rows.Scan(&key, &p.Time, &p.Duration, &p.Completed, &at); err != nil {
			rows.Close()
			return result, err
		}
		p.UpdatedAt = at.UnixMilli()
		result.Progress[key] = p
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = r.db.QueryContext(ctx, `SELECT media_key,item_json,saved_at FROM study_favorites WHERE user_id=?`, userID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var data []byte
		var at time.Time
		var f Favorite
		if err := rows.Scan(&key, &data, &at); err != nil {
			return result, err
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return result, err
		}
		f.Key = key
		f.SavedAt = at.UnixMilli()
		result.Favorites[key] = f
	}
	return result, rows.Err()
}
func (r *MySQLRepository) SaveProgress(ctx context.Context, userID uint64, key string, p Progress) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO study_progress(user_id,media_key,position_seconds,duration_seconds,completed,updated_at) VALUES (?,?,?,?,?,?) ON DUPLICATE KEY UPDATE position_seconds=VALUES(position_seconds),duration_seconds=VALUES(duration_seconds),completed=VALUES(completed),updated_at=VALUES(updated_at)`, userID, key, p.Time, p.Duration, p.Completed, time.UnixMilli(p.UpdatedAt).UTC())
	return err
}
func (r *MySQLRepository) SetFavorite(ctx context.Context, userID uint64, key string, item *Favorite) error {
	if item == nil {
		_, err := r.db.ExecContext(ctx, `DELETE FROM study_favorites WHERE user_id=? AND media_key=?`, userID, key)
		return err
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO study_favorites(user_id,media_key,item_json,saved_at) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE item_json=VALUES(item_json),saved_at=VALUES(saved_at)`, userID, key, data, time.UnixMilli(item.SavedAt).UTC())
	return err
}
