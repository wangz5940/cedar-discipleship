package studymemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
)

type PageCue struct {
	Page int     `json:"page"`
	Time float64 `json:"time"`
}
type Handout struct {
	AssetID uint64    `json:"assetId"`
	Pages   []PageCue `json:"pages"`
}
type LearningRepository interface {
	Access(context.Context, uint64, string) (bool, error)
	Unlock(context.Context, uint64, string) error
	Revoke(context.Context, uint64) error
	Handout(context.Context, uint64, uint64) (*Handout, error)
	SaveHandout(context.Context, uint64, uint64, *Handout) error
}

func ValidHandout(h Handout) bool {
	if h.AssetID == 0 || len(h.Pages) > 2000 {
		return false
	}
	seen := map[int]bool{}
	for _, p := range h.Pages {
		if p.Page < 1 || p.Page > 10000 || seen[p.Page] || p.Time < 0 || p.Time > 1e9 || math.IsNaN(p.Time) || math.IsInf(p.Time, 0) {
			return false
		}
		seen[p.Page] = true
	}
	return true
}
func (r *MySQLRepository) Access(ctx context.Context, id uint64, hash string) (bool, error) {
	if hash == "" {
		return false, nil
	}
	var saved string
	err := r.db.QueryRowContext(ctx, `SELECT key_hash FROM study_access WHERE user_id=?`, id).Scan(&saved)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return saved == hash, err
}
func (r *MySQLRepository) Unlock(ctx context.Context, id uint64, hash string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO study_access(user_id,key_hash) VALUES (?,?) ON DUPLICATE KEY UPDATE key_hash=VALUES(key_hash)`, id, hash)
	return err
}
func (r *MySQLRepository) Revoke(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM study_access WHERE user_id=?`, id)
	return err
}
func (r *MySQLRepository) Handout(ctx context.Context, group, media uint64) (*Handout, error) {
	var data []byte
	err := r.db.QueryRowContext(ctx, `SELECT handout_json FROM media_handouts WHERE group_id=? AND media_asset_id=?`, group, media).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var h Handout
	err = json.Unmarshal(data, &h)
	return &h, err
}
func (r *MySQLRepository) SaveHandout(ctx context.Context, group, media uint64, h *Handout) error {
	if h == nil {
		_, err := r.db.ExecContext(ctx, `DELETE FROM media_handouts WHERE group_id=? AND media_asset_id=?`, group, media)
		return err
	}
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO media_handouts(group_id,media_asset_id,handout_json) VALUES (?,?,?) ON DUPLICATE KEY UPDATE handout_json=VALUES(handout_json)`, group, media, data)
	return err
}
