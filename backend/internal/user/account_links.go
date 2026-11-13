package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"
)

var ErrAccountLink = errors.New("account_link_conflict")
var ErrAccountLinkChanged = errors.New("account_link_changed")

type LinkAccount struct {
	ID          uint64  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	Groups      []Group `json:"study_groups"`
}

type AccountLinkReview struct {
	ID           uint64        `json:"id"`
	FirstUserID  uint64        `json:"first_user_id"`
	SecondUserID uint64        `json:"second_user_id"`
	Accounts     []LinkAccount `json:"accounts"`
	Status       string        `json:"status"`
	RequestedBy  uint64        `json:"requested_by"`
	ReviewedBy   uint64        `json:"reviewed_by"`
	CreatedAt    time.Time     `json:"created_at"`
	ReviewedAt   *time.Time    `json:"reviewed_at"`
}

type linkQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func linkedAccountIDs(ctx context.Context, q linkQuerier, userID uint64) ([]uint64, error) {
	rows, err := q.QueryContext(ctx, `SELECT u.id FROM users u
		LEFT JOIN account_links l ON l.user_id=u.id
		WHERE u.status=1 AND COALESCE(l.identity_id,u.id)=
		COALESCE((SELECT identity_id FROM account_links WHERE user_id=?),?)
		ORDER BY (u.id=?) DESC,u.id`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *MySQLRepository) linkedGroups(ctx context.Context, userID uint64) ([]Group, error) {
	ids, err := linkedAccountIDs(ctx, r.db, userID)
	if err != nil {
		return nil, err
	}
	if len(ids) < 2 {
		return r.ListMembershipGroups(ctx, userID)
	}
	groups := make([]Group, 0)
	seen := make(map[uint64]bool)
	for _, id := range ids {
		items, err := r.ListMembershipGroups(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, group := range items {
			if seen[group.ID] {
				continue
			}
			group.MemberUserID = id
			groups = append(groups, group)
			seen[group.ID] = true
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups, nil
}

// The snapshot includes every credential and visible group in both identities.
// Approval rechecks it inside the transaction before granting cross-account access.
func linkSnapshot(ctx context.Context, tx *sql.Tx, first, second uint64) ([]LinkAccount, error) {
	left, err := linkedAccountIDs(ctx, tx, first)
	if err != nil {
		return nil, err
	}
	right, err := linkedAccountIDs(ctx, tx, second)
	if err != nil {
		return nil, err
	}
	if len(left) == 0 || len(right) == 0 {
		return nil, ErrAccountLink
	}
	ids := append(left, right...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	accounts := make([]LinkAccount, 0, len(ids))
	seenGroups := make(map[uint64]bool)
	for index, id := range ids {
		if index > 0 && id == ids[index-1] {
			return nil, ErrAccountLink
		}
		var account LinkAccount
		var super bool
		var status int
		err := tx.QueryRowContext(ctx, `SELECT id,username,display_name,is_super_admin,status
			FROM users WHERE id=? FOR UPDATE`, id).Scan(&account.ID, &account.Username, &account.DisplayName, &super, &status)
		if err != nil {
			return nil, err
		}
		if super || status != 1 {
			return nil, ErrAccountLink
		}
		rows, err := tx.QueryContext(ctx, membershipGroupQuery+" FOR UPDATE", id)
		if err != nil {
			return nil, err
		}
		account.Groups, err = scanGroups(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		for _, group := range account.Groups {
			if seenGroups[group.ID] {
				return nil, ErrAccountLink
			}
			seenGroups[group.ID] = true
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func (r *MySQLRepository) beginAccountLink(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	var id int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM account_link_lock WHERE id=1 FOR UPDATE`).Scan(&id); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (r *MySQLRepository) CreateAccountLinkReview(ctx context.Context, firstName, secondName string, actor uint64) (AccountLinkReview, error) {
	tx, err := r.beginAccountLink(ctx)
	if err != nil {
		return AccountLinkReview{}, err
	}
	defer tx.Rollback()
	var first, second uint64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, firstName).Scan(&first); err != nil {
		return AccountLinkReview{}, ErrAccountLink
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, secondName).Scan(&second); err != nil {
		return AccountLinkReview{}, ErrAccountLink
	}
	accounts, err := linkSnapshot(ctx, tx, first, second)
	if err != nil {
		return AccountLinkReview{}, err
	}
	data, err := json.Marshal(accounts)
	if err != nil {
		return AccountLinkReview{}, err
	}
	at := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `INSERT INTO account_link_reviews(first_user_id,second_user_id,snapshot,requested_by,created_at)
		VALUES(?,?,?,?,?)`, first, second, data, actor, at)
	if err != nil {
		return AccountLinkReview{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return AccountLinkReview{}, err
	}
	if err = tx.Commit(); err != nil {
		return AccountLinkReview{}, err
	}
	return AccountLinkReview{ID: uint64(id), FirstUserID: first, SecondUserID: second, Accounts: accounts, Status: "pending", RequestedBy: actor, CreatedAt: at}, nil
}

func (r *MySQLRepository) ReviewAccountLink(ctx context.Context, id, actor uint64, approve bool) error {
	tx, err := r.beginAccountLink(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var first, second uint64
	var snapshot, status string
	if err := tx.QueryRowContext(ctx, `SELECT first_user_id,second_user_id,snapshot,status FROM account_link_reviews WHERE id=? FOR UPDATE`, id).
		Scan(&first, &second, &snapshot, &status); err != nil {
		return err
	}
	decision := "rejected"
	if approve {
		decision = "approved"
	}
	if status == decision {
		return nil
	}
	if status != "pending" {
		return ErrAccountLink
	}
	if approve {
		accounts, err := linkSnapshot(ctx, tx, first, second)
		if err != nil {
			return err
		}
		var previous []LinkAccount
		if err := json.Unmarshal([]byte(snapshot), &previous); err != nil {
			return err
		}
		before, _ := json.Marshal(previous)
		after, _ := json.Marshal(accounts)
		if string(before) != string(after) {
			return ErrAccountLinkChanged
		}
		identityID := accounts[0].ID
		for _, account := range accounts {
			if _, err := tx.ExecContext(ctx, `INSERT INTO account_links(user_id,identity_id) VALUES(?,?)
				ON DUPLICATE KEY UPDATE identity_id=VALUES(identity_id)`, account.ID, identityID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE account_link_reviews SET status=?,reviewed_by=?,reviewed_at=? WHERE id=?`, decision, actor, time.Now().UTC(), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLRepository) AccountLinkReviews(ctx context.Context) ([]AccountLinkReview, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,first_user_id,second_user_id,snapshot,status,requested_by,
		COALESCE(reviewed_by,0),created_at,reviewed_at FROM account_link_reviews ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AccountLinkReview, 0)
	for rows.Next() {
		var item AccountLinkReview
		var data []byte
		if err := rows.Scan(&item.ID, &item.FirstUserID, &item.SecondUserID, &data, &item.Status, &item.RequestedBy, &item.ReviewedBy, &item.CreatedAt, &item.ReviewedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &item.Accounts); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) CreateAccountLinkReview(ctx context.Context, first, second string, actor uint64) (AccountLinkReview, error) {
	return s.repo.CreateAccountLinkReview(ctx, first, second, actor)
}
func (s *Service) ReviewAccountLink(ctx context.Context, id, actor uint64, approve bool) error {
	return s.repo.ReviewAccountLink(ctx, id, actor, approve)
}
func (s *Service) AccountLinkReviews(ctx context.Context) ([]AccountLinkReview, error) {
	return s.repo.AccountLinkReviews(ctx)
}
