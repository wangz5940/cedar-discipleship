package ministry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

var defaultCatalog = []struct {
	code string
	name string
}{
	{code: "leading", name: "领会组"},
	{code: "hosting", name: "主持组"},
	{code: "catering", name: "伙食组"},
	{code: "logistics", name: "后勤组"},
	{code: "cleaning", name: "整洁组"},
	{code: "technology", name: "技术组"},
	{code: "planning", name: "策划组"},
	{code: "counting", name: "数点组"},
	{code: "visitation", name: "探望组"},
	{code: "reporting", name: "回报组"},
	{code: "children", name: "娃娃组"},
	{code: "intercession", name: "守望组"},
	{code: "discipleship-counting", name: "门训数点组"},
	{code: "discipleship-planning", name: "门训规划发布组"},
	{code: "discipleship-review", name: "门训批改组"},
}

func (r *MySQLRepository) EnsureCatalog(ctx context.Context, studyGroupID uint64, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning ministry catalog initialization: %w", err)
	}
	defer tx.Rollback()

	for _, item := range defaultCatalog {
		_, err := tx.ExecContext(
			ctx,
			`INSERT IGNORE INTO ministry_groups
				(study_group_id,code,name,created_at,updated_at)
			 VALUES (?,?,?,?,?)`,
			studyGroupID,
			item.code,
			item.name,
			at,
			at,
		)
		if err != nil {
			return fmt.Errorf("ensuring ministry group %q: %w", item.code, err)
		}
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE ministry_groups ministry
		   JOIN (
		     SELECT group_id,MIN(user_id) AS user_id
		       FROM user_group_roles
		      WHERE group_id=? AND role='group_leader'
		      GROUP BY group_id
		   ) leader ON leader.group_id=ministry.study_group_id
		    SET ministry.leader_user_id=leader.user_id,ministry.updated_at=?
		  WHERE ministry.study_group_id=? AND ministry.leader_user_id IS NULL`,
		studyGroupID,
		at,
		studyGroupID,
	); err != nil {
		return fmt.Errorf("assigning default ministry leaders: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE ministry_groups ministry
		   JOIN (
		     SELECT MIN(id) AS user_id
		       FROM users
		      WHERE is_super_admin=1 AND status=1
		   ) super_admin ON super_admin.user_id IS NOT NULL
		    SET ministry.leader_user_id=super_admin.user_id,ministry.updated_at=?
		  WHERE ministry.study_group_id=? AND ministry.leader_user_id IS NULL`,
		at,
		studyGroupID,
	); err != nil {
		return fmt.Errorf("assigning fallback ministry leaders: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO ministry_group_members
			(study_group_id,ministry_group_id,user_id,role,identity_public,status,joined_at,created_at,updated_at)
		 SELECT study_group_id,id,leader_user_id,'member',1,1,?,?,?
		   FROM ministry_groups
		  WHERE study_group_id=? AND leader_user_id IS NOT NULL
		 ON DUPLICATE KEY UPDATE status=1,identity_public=1,updated_at=VALUES(updated_at)`,
		at,
		at,
		at,
		studyGroupID,
	); err != nil {
		return fmt.Errorf("adding default ministry leaders: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing ministry catalog initialization: %w", err)
	}
	return nil
}

func (r *MySQLRepository) ListGroups(
	ctx context.Context,
	studyGroupID, userID uint64,
) ([]GroupSummary, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT g.id,g.code,g.name,g.description,g.member_visibility,g.share_auto_approve,
		        COUNT(DISTINCT CASE WHEN members.status=1 THEN members.id END) AS member_count,
		        COALESCE(self.role,''),COALESCE(self.identity_public,0),
		        CASE WHEN self.id IS NULL OR self.status<>1 THEN 0 ELSE 1 END AS joined,
		        CASE WHEN g.leader_user_id=? THEN 1 ELSE 0 END AS is_leader,
		        COALESCE(req.status,'')
		   FROM ministry_groups g
		   LEFT JOIN ministry_group_members members ON members.ministry_group_id=g.id
		   LEFT JOIN ministry_group_members self
		     ON self.ministry_group_id=g.id AND self.user_id=?
		   LEFT JOIN ministry_group_requests req
		     ON req.ministry_group_id=g.id AND req.user_id=? AND req.request_type='join'
		  WHERE g.study_group_id=? AND g.status=1
		  GROUP BY g.id,g.code,g.name,g.description,g.member_visibility,g.share_auto_approve,
		           self.id,self.status,self.role,self.identity_public,g.leader_user_id,req.status
		  ORDER BY joined DESC,g.name,g.id`,
		userID,
		userID,
		userID,
		studyGroupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing ministry groups: %w", err)
	}
	defer rows.Close()

	groups := []GroupSummary{}
	for rows.Next() {
		var item GroupSummary
		var role string
		var requestStatus string
		if err := rows.Scan(
			&item.ID,
			&item.Code,
			&item.Name,
			&item.Description,
			&item.MemberVisibility,
			&item.ShareAutoApprove,
			&item.MemberCount,
			&role,
			&item.IdentityPublic,
			&item.Joined,
			&item.IsLeader,
			&requestStatus,
		); err != nil {
			return nil, fmt.Errorf("scanning ministry group: %w", err)
		}
		item.Role = MemberRole(role)
		item.RequestStatus = Status(requestStatus)
		groups = append(groups, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ministry groups: %w", err)
	}
	return groups, nil
}

func (r *MySQLRepository) Group(
	ctx context.Context,
	studyGroupID, groupID, userID uint64,
) (*GroupSummary, error) {
	groups, err := r.ListGroups(ctx, studyGroupID, userID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].ID == groupID {
			return &groups[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *MySQLRepository) Access(
	ctx context.Context,
	studyGroupID, groupID, userID uint64,
) (Access, error) {
	var access Access
	var role sql.NullString
	var memberStatus sql.NullInt64
	var identityPublic sql.NullBool
	err := r.db.QueryRowContext(
		ctx,
		`SELECT g.id,COALESCE(g.leader_user_id,0),g.member_visibility,g.share_auto_approve,
		        m.status,m.role,m.identity_public
		   FROM ministry_groups g
		   LEFT JOIN ministry_group_members m
		     ON m.ministry_group_id=g.id AND m.user_id=?
		  WHERE g.id=? AND g.study_group_id=? AND g.status=1`,
		userID,
		groupID,
		studyGroupID,
	).Scan(
		&access.GroupID,
		&access.LeaderUserID,
		&access.MemberVisibility,
		&access.ShareAutoApprove,
		&memberStatus,
		&role,
		&identityPublic,
	)
	if err != nil {
		return Access{}, err
	}
	access.IsMember = memberStatus.Valid && memberStatus.Int64 == 1
	access.IsLeader = access.LeaderUserID == userID
	access.IsAdmin = access.IsMember && role.Valid && MemberRole(role.String) == MemberRoleAdmin
	access.IdentityPublic = identityPublic.Valid && identityPublic.Bool
	return access, nil
}

func (r *MySQLRepository) ListMembers(
	ctx context.Context,
	studyGroupID, groupID uint64,
) ([]Member, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT m.user_id,u.username,u.display_name,m.role,m.identity_public,
		        CASE WHEN g.leader_user_id=m.user_id THEN 1 ELSE 0 END
		   FROM ministry_group_members m
		   JOIN ministry_groups g ON g.id=m.ministry_group_id AND g.study_group_id=m.study_group_id
		   JOIN users u ON u.id=m.user_id
		  WHERE m.study_group_id=? AND m.ministry_group_id=? AND m.status=1
		  ORDER BY is_leader DESC,m.role DESC,u.display_name,m.id`,
		studyGroupID,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing ministry members: %w", err)
	}
	defer rows.Close()

	members := []Member{}
	for rows.Next() {
		var item Member
		if err := rows.Scan(
			&item.UserID,
			&item.Username,
			&item.DisplayName,
			&item.Role,
			&item.IdentityPublic,
			&item.IsLeader,
		); err != nil {
			return nil, fmt.Errorf("scanning ministry member: %w", err)
		}
		members = append(members, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ministry members: %w", err)
	}
	return members, nil
}

func (r *MySQLRepository) RequestJoin(
	ctx context.Context,
	studyGroupID, groupID, userID uint64,
	message string,
	at time.Time,
) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning join request: %w", err)
	}
	defer tx.Rollback()

	var isMember int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM ministry_group_members
		  WHERE study_group_id=? AND ministry_group_id=? AND user_id=? AND status=1`,
		studyGroupID,
		groupID,
		userID,
	).Scan(&isMember); err != nil {
		return 0, fmt.Errorf("checking ministry membership: %w", err)
	}
	if isMember > 0 {
		return 0, ErrAlreadyMember
	}

	var requestID uint64
	var status Status
	err = tx.QueryRowContext(
		ctx,
		`SELECT id,status FROM ministry_group_requests
		  WHERE study_group_id=? AND ministry_group_id=? AND user_id=? AND request_type='join'
		  FOR UPDATE`,
		studyGroupID,
		groupID,
		userID,
	).Scan(&requestID, &status)
	switch {
	case err == nil && status == StatusPending:
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("committing existing join request: %w", err)
		}
		return requestID, nil
	case err == nil:
		_, err = tx.ExecContext(
			ctx,
			`UPDATE ministry_group_requests
			    SET message=?,status='pending',reviewed_by=NULL,reviewed_at=NULL,created_at=?,updated_at=?
			  WHERE id=?`,
			message,
			at,
			at,
			requestID,
		)
	case errors.Is(err, sql.ErrNoRows):
		var result sql.Result
		result, err = tx.ExecContext(
			ctx,
			`INSERT INTO ministry_group_requests
				(study_group_id,ministry_group_id,user_id,request_type,message,status,created_at,updated_at)
			 VALUES (?,?,?,'join',?,'pending',?,?)`,
			studyGroupID,
			groupID,
			userID,
			message,
			at,
			at,
		)
		if err == nil {
			requestID, err = lastInsertID(result)
		}
	default:
		return 0, fmt.Errorf("locking join request: %w", err)
	}
	if err != nil {
		return 0, fmt.Errorf("saving join request: %w", err)
	}

	displayName, err := userDisplayNameTx(ctx, tx, userID)
	if err != nil {
		return 0, err
	}
	groupName, err := groupNameTx(ctx, tx, studyGroupID, groupID)
	if err != nil {
		return 0, err
	}
	eventKey := "join-request:" + strconv.FormatUint(requestID, 10)
	if err := notifyModeratorsTx(
		ctx,
		tx,
		studyGroupID,
		groupID,
		eventKey,
		"join_request",
		groupName+"有新的加入申请",
		displayName+"申请加入"+groupName,
		at,
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing join request: %w", err)
	}
	return requestID, nil
}

func (r *MySQLRepository) Leave(
	ctx context.Context,
	studyGroupID, groupID, userID uint64,
	at time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning ministry leave: %w", err)
	}
	defer tx.Rollback()

	var leaderUserID uint64
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COALESCE(leader_user_id,0) FROM ministry_groups
		  WHERE id=? AND study_group_id=? FOR UPDATE`,
		groupID,
		studyGroupID,
	).Scan(&leaderUserID); err != nil {
		return fmt.Errorf("locking ministry group: %w", err)
	}
	if leaderUserID == userID {
		return ErrLeaderCannotLeave
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE ministry_group_members
		    SET status=0,role='member',updated_at=?
		  WHERE study_group_id=? AND ministry_group_id=? AND user_id=? AND status=1`,
		at,
		studyGroupID,
		groupID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("leaving ministry group: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking ministry leave: %w", err)
	}
	if affected != 1 {
		return ErrNotMember
	}
	displayName, err := userDisplayNameTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	groupName, err := groupNameTx(ctx, tx, studyGroupID, groupID)
	if err != nil {
		return err
	}
	eventKey := fmt.Sprintf("member-left:%d:%d:%d", groupID, userID, at.UnixNano())
	if err := notifyModeratorsTx(
		ctx,
		tx,
		studyGroupID,
		groupID,
		eventKey,
		"member_left",
		groupName+"有成员退出",
		displayName+"已退出"+groupName,
		at,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing ministry leave: %w", err)
	}
	return nil
}

func (r *MySQLRepository) SetIdentityPublic(
	ctx context.Context,
	studyGroupID, groupID, userID uint64,
	public bool,
	at time.Time,
) error {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE ministry_group_members
		    SET identity_public=?,updated_at=?
		  WHERE study_group_id=? AND ministry_group_id=? AND user_id=? AND status=1`,
		public,
		at,
		studyGroupID,
		groupID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("updating ministry identity visibility: %w", err)
	}
	return requireOneRow(result, ErrNotMember)
}

func (r *MySQLRepository) SetMemberRole(
	ctx context.Context,
	studyGroupID, groupID, userID uint64,
	role MemberRole,
	at time.Time,
) error {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE ministry_group_members
		    SET role=?,updated_at=?
		  WHERE study_group_id=? AND ministry_group_id=? AND user_id=? AND status=1`,
		role,
		at,
		studyGroupID,
		groupID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("updating ministry member role: %w", err)
	}
	return requireOneRow(result, ErrNotMember)
}

func (r *MySQLRepository) UpdateSettings(
	ctx context.Context,
	studyGroupID, groupID uint64,
	input GroupSettingsInput,
	at time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning ministry settings update: %w", err)
	}
	defer tx.Rollback()

	if input.MemberVisibility != nil {
		result, err := tx.ExecContext(
			ctx,
			`UPDATE ministry_groups SET member_visibility=?,updated_at=?
			  WHERE id=? AND study_group_id=?`,
			*input.MemberVisibility,
			at,
			groupID,
			studyGroupID,
		)
		if err != nil {
			return fmt.Errorf("updating ministry visibility: %w", err)
		}
		if err := requireOneRow(result, ErrGroupNotFound); err != nil {
			return err
		}
	}
	if input.ShareAutoApprove != nil {
		result, err := tx.ExecContext(
			ctx,
			`UPDATE ministry_groups SET share_auto_approve=?,updated_at=?
			  WHERE id=? AND study_group_id=?`,
			*input.ShareAutoApprove,
			at,
			groupID,
			studyGroupID,
		)
		if err != nil {
			return fmt.Errorf("updating ministry share approval: %w", err)
		}
		if err := requireOneRow(result, ErrGroupNotFound); err != nil {
			return err
		}
	}
	if input.LeaderUserID != nil {
		if *input.LeaderUserID == 0 {
			return ErrNotMember
		}
		var exists int
		if err := tx.QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM group_members gm
			   JOIN users u ON u.id=gm.user_id
			  WHERE gm.group_id=? AND gm.user_id=? AND gm.status=1 AND u.status=1`,
			studyGroupID,
			*input.LeaderUserID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("checking ministry leader: %w", err)
		}
		if exists == 0 {
			return ErrNotMember
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO ministry_group_members
				(study_group_id,ministry_group_id,user_id,role,identity_public,status,joined_at,created_at,updated_at)
			 VALUES (?,?,?,'member',0,1,?,?,?)
			 ON DUPLICATE KEY UPDATE status=1,updated_at=VALUES(updated_at)`,
			studyGroupID,
			groupID,
			*input.LeaderUserID,
			at,
			at,
			at,
		); err != nil {
			return fmt.Errorf("adding ministry leader membership: %w", err)
		}
		result, err := tx.ExecContext(
			ctx,
			`UPDATE ministry_groups SET leader_user_id=?,updated_at=?
			  WHERE id=? AND study_group_id=?`,
			*input.LeaderUserID,
			at,
			groupID,
			studyGroupID,
		)
		if err != nil {
			return fmt.Errorf("updating ministry leader: %w", err)
		}
		if err := requireOneRow(result, ErrGroupNotFound); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing ministry settings: %w", err)
	}
	return nil
}

func (r *MySQLRepository) ListPendingRequests(
	ctx context.Context,
	studyGroupID uint64,
) ([]Request, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT req.id,req.ministry_group_id,req.user_id,u.display_name,
		        req.message,req.status,req.created_at
		   FROM ministry_group_requests req
		   JOIN users u ON u.id=req.user_id
		  WHERE req.study_group_id=? AND req.status='pending'
		  ORDER BY req.created_at,req.id`,
		studyGroupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing ministry requests: %w", err)
	}
	defer rows.Close()
	return scanRequests(rows)
}

func (r *MySQLRepository) Request(
	ctx context.Context,
	studyGroupID, requestID uint64,
) (*Request, error) {
	var item Request
	err := r.db.QueryRowContext(
		ctx,
		`SELECT req.id,req.ministry_group_id,req.user_id,u.display_name,
		        req.message,req.status,req.created_at
		   FROM ministry_group_requests req
		   JOIN users u ON u.id=req.user_id
		  WHERE req.study_group_id=? AND req.id=?`,
		studyGroupID,
		requestID,
	).Scan(
		&item.ID,
		&item.GroupID,
		&item.UserID,
		&item.UserDisplayName,
		&item.Message,
		&item.Status,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MySQLRepository) DecideRequest(
	ctx context.Context,
	studyGroupID, requestID, reviewerID uint64,
	decision Status,
	at time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning ministry request decision: %w", err)
	}
	defer tx.Rollback()

	var groupID, userID uint64
	var status Status
	if err := tx.QueryRowContext(
		ctx,
		`SELECT ministry_group_id,user_id,status
		   FROM ministry_group_requests
		  WHERE id=? AND study_group_id=? FOR UPDATE`,
		requestID,
		studyGroupID,
	).Scan(&groupID, &userID, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRequestNotFound
		}
		return fmt.Errorf("locking ministry request: %w", err)
	}
	if status != StatusPending {
		return ErrRequestAlreadyReviewed
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE ministry_group_requests
		    SET status=?,reviewed_by=?,reviewed_at=?,updated_at=?
		  WHERE id=? AND study_group_id=? AND status='pending'`,
		decision,
		reviewerID,
		at,
		at,
		requestID,
		studyGroupID,
	)
	if err != nil {
		return fmt.Errorf("deciding ministry request: %w", err)
	}
	if err := requireOneRow(result, ErrRequestAlreadyReviewed); err != nil {
		return err
	}
	if decision == StatusApproved {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO ministry_group_members
				(study_group_id,ministry_group_id,user_id,role,identity_public,status,joined_at,created_at,updated_at)
			 VALUES (?,?,?,'member',0,1,?,?,?)
			 ON DUPLICATE KEY UPDATE status=1,role='member',joined_at=VALUES(joined_at),updated_at=VALUES(updated_at)`,
			studyGroupID,
			groupID,
			userID,
			at,
			at,
			at,
		); err != nil {
			return fmt.Errorf("approving ministry membership: %w", err)
		}
	}
	groupName, err := groupNameTx(ctx, tx, studyGroupID, groupID)
	if err != nil {
		return err
	}
	title := "加入申请未通过"
	body := "你加入" + groupName + "的申请未通过"
	if decision == StatusApproved {
		title = "加入申请已通过"
		body = "你已加入" + groupName
	}
	if err := insertNotificationTx(
		ctx,
		tx,
		studyGroupID,
		groupID,
		userID,
		fmt.Sprintf("join-decision:%d:%s", requestID, decision),
		"join_decision",
		title,
		body,
		at,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing ministry request decision: %w", err)
	}
	return nil
}

func (r *MySQLRepository) ListNotifications(
	ctx context.Context,
	studyGroupID, userID uint64,
	limit int,
) ([]Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT n.id,n.ministry_group_id,g.name,n.notification_type,n.title,n.body,
		        CASE WHEN n.read_at IS NULL THEN 0 ELSE 1 END,n.created_at
		   FROM ministry_notifications n
		   JOIN ministry_groups g ON g.id=n.ministry_group_id AND g.study_group_id=n.study_group_id
		  WHERE n.study_group_id=? AND n.user_id=?
		  ORDER BY n.created_at DESC,n.id DESC LIMIT ?`,
		studyGroupID,
		userID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing ministry notifications: %w", err)
	}
	defer rows.Close()

	items := []Notification{}
	for rows.Next() {
		var item Notification
		if err := rows.Scan(
			&item.ID,
			&item.GroupID,
			&item.GroupName,
			&item.Type,
			&item.Title,
			&item.Body,
			&item.IsRead,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning ministry notification: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ministry notifications: %w", err)
	}
	return items, nil
}

func (r *MySQLRepository) ReadNotification(
	ctx context.Context,
	studyGroupID, notificationID, userID uint64,
	at time.Time,
) error {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE ministry_notifications SET read_at=?
		  WHERE id=? AND study_group_id=? AND user_id=?`,
		at,
		notificationID,
		studyGroupID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("reading ministry notification: %w", err)
	}
	return requireOneRow(result, sql.ErrNoRows)
}

func (r *MySQLRepository) ListShares(
	ctx context.Context,
	studyGroupID, groupID, userID uint64,
	canModerate bool,
) ([]Share, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT s.id,s.ministry_group_id,s.author_user_id,u.display_name,
		        s.title,s.body_markdown,s.status,COALESCE(s.reviewed_by,0),
		        s.published_at,s.created_at,s.updated_at
		   FROM ministry_shares s
		   JOIN users u ON u.id=s.author_user_id
		  WHERE s.study_group_id=? AND s.ministry_group_id=?
		    AND (s.status='published' OR s.author_user_id=? OR ?)
		  ORDER BY CASE WHEN s.status='published' THEN 0 ELSE 1 END,
		           COALESCE(s.published_at,s.updated_at) DESC,s.id DESC`,
		studyGroupID,
		groupID,
		userID,
		canModerate,
	)
	if err != nil {
		return nil, fmt.Errorf("listing ministry shares: %w", err)
	}
	defer rows.Close()

	items := []Share{}
	for rows.Next() {
		var item Share
		var publishedAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.GroupID,
			&item.AuthorID,
			&item.AuthorName,
			&item.Title,
			&item.Body,
			&item.Status,
			&item.ReviewedBy,
			&publishedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning ministry share: %w", err)
		}
		if publishedAt.Valid {
			item.PublishedAt = &publishedAt.Time
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ministry shares: %w", err)
	}
	return items, nil
}

func (r *MySQLRepository) CreateShare(
	ctx context.Context,
	studyGroupID, groupID, authorID uint64,
	input ShareInput,
	status Status,
	at time.Time,
) (uint64, error) {
	var publishedAt any
	if status == StatusPublished {
		publishedAt = at
	}
	result, err := r.db.ExecContext(
		ctx,
		`INSERT INTO ministry_shares
			(study_group_id,ministry_group_id,author_user_id,title,body_markdown,status,published_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		studyGroupID,
		groupID,
		authorID,
		input.Title,
		input.Body,
		status,
		publishedAt,
		at,
		at,
	)
	if err != nil {
		return 0, fmt.Errorf("creating ministry share: %w", err)
	}
	shareID, err := lastInsertID(result)
	if err != nil {
		return 0, err
	}
	if status == StatusPending {
		if err := r.notifyModerators(
			ctx,
			studyGroupID,
			groupID,
			"share-review:"+strconv.FormatUint(shareID, 10),
			"share_review",
			"有新的分享待审批",
			input.Title,
			at,
		); err != nil {
			return 0, err
		}
	}
	return shareID, nil
}

func (r *MySQLRepository) UpdateShare(
	ctx context.Context,
	studyGroupID, groupID, shareID, actorID uint64,
	input ShareInput,
	status Status,
	canManage bool,
	at time.Time,
) error {
	var publishedAt any
	if status == StatusPublished {
		publishedAt = at
	}
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE ministry_shares
		    SET title=?,body_markdown=?,status=?,reviewed_by=NULL,reviewed_at=NULL,
		        published_at=?,updated_at=?
		  WHERE id=? AND study_group_id=? AND ministry_group_id=?
		    AND (author_user_id=? OR ?)`,
		input.Title,
		input.Body,
		status,
		publishedAt,
		at,
		shareID,
		studyGroupID,
		groupID,
		actorID,
		canManage,
	)
	if err != nil {
		return fmt.Errorf("updating ministry share: %w", err)
	}
	if err := requireOneRow(result, ErrShareNotFound); err != nil {
		return err
	}
	if status == StatusPending {
		return r.notifyModerators(
			ctx,
			studyGroupID,
			groupID,
			fmt.Sprintf("share-review:%d:%d", shareID, at.UnixNano()),
			"share_review",
			"分享修改待审批",
			input.Title,
			at,
		)
	}
	return nil
}

func (r *MySQLRepository) DecideShare(
	ctx context.Context,
	studyGroupID, groupID, shareID, reviewerID uint64,
	decision Status,
	at time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning share decision: %w", err)
	}
	defer tx.Rollback()

	var authorID uint64
	var status Status
	var title string
	if err := tx.QueryRowContext(
		ctx,
		`SELECT ministry_group_id,author_user_id,title,status
		   FROM ministry_shares
		  WHERE id=? AND study_group_id=? AND ministry_group_id=? FOR UPDATE`,
		shareID,
		studyGroupID,
		groupID,
	).Scan(&groupID, &authorID, &title, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrShareNotFound
		}
		return fmt.Errorf("locking ministry share: %w", err)
	}
	if status != StatusPending {
		return ErrShareAlreadyReviewed
	}
	var publishedAt any
	if decision == StatusPublished {
		publishedAt = at
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE ministry_shares
		    SET status=?,reviewed_by=?,reviewed_at=?,published_at=?,updated_at=?
		  WHERE id=? AND study_group_id=? AND ministry_group_id=? AND status='pending'`,
		decision,
		reviewerID,
		at,
		publishedAt,
		at,
		shareID,
		studyGroupID,
		groupID,
	)
	if err != nil {
		return fmt.Errorf("deciding ministry share: %w", err)
	}
	if err := requireOneRow(result, ErrShareAlreadyReviewed); err != nil {
		return err
	}
	notificationTitle := "分享申请未通过"
	if decision == StatusPublished {
		notificationTitle = "分享已发布"
	}
	if err := insertNotificationTx(
		ctx,
		tx,
		studyGroupID,
		groupID,
		authorID,
		fmt.Sprintf("share-decision:%d:%s", shareID, decision),
		"share_decision",
		notificationTitle,
		title,
		at,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing share decision: %w", err)
	}
	return nil
}

func (r *MySQLRepository) ListProgress(
	ctx context.Context,
	studyGroupID, groupID uint64,
	limit int,
) ([]Progress, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT p.id,p.ministry_group_id,p.author_user_id,u.display_name,
		        p.occurred_at,p.content_markdown,p.created_at
		   FROM ministry_progress p
		   JOIN users u ON u.id=p.author_user_id
		  WHERE p.study_group_id=? AND p.ministry_group_id=?
		  ORDER BY p.occurred_at DESC,p.id DESC LIMIT ?`,
		studyGroupID,
		groupID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing ministry progress: %w", err)
	}
	defer rows.Close()

	items := []Progress{}
	byID := map[uint64]int{}
	for rows.Next() {
		var item Progress
		item.Attachments = []Attachment{}
		if err := rows.Scan(
			&item.ID,
			&item.GroupID,
			&item.AuthorID,
			&item.AuthorName,
			&item.OccurredAt,
			&item.Content,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning ministry progress: %w", err)
		}
		byID[item.ID] = len(items)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ministry progress: %w", err)
	}
	if len(items) == 0 {
		return items, nil
	}

	assetRows, err := r.db.QueryContext(
		ctx,
		`SELECT pa.progress_id,a.id,a.title,a.original_name,a.mime_type
		   FROM ministry_progress_assets pa
		   JOIN ministry_progress p
		     ON p.id=pa.progress_id AND p.study_group_id=pa.study_group_id
		   JOIN assets a ON a.id=pa.asset_id AND a.group_id=pa.study_group_id
		  WHERE pa.study_group_id=? AND p.ministry_group_id=?
		  ORDER BY pa.progress_id,pa.sort_order,pa.id`,
		studyGroupID,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing ministry progress assets: %w", err)
	}
	defer assetRows.Close()
	for assetRows.Next() {
		var progressID uint64
		var attachment Attachment
		if err := assetRows.Scan(
			&progressID,
			&attachment.ID,
			&attachment.Title,
			&attachment.OriginalName,
			&attachment.MimeType,
		); err != nil {
			return nil, fmt.Errorf("scanning ministry progress asset: %w", err)
		}
		index, ok := byID[progressID]
		if !ok {
			continue
		}
		attachment.URL = fmt.Sprintf("/api/assets/%d/download", attachment.ID)
		items[index].Attachments = append(items[index].Attachments, attachment)
	}
	if err := assetRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ministry progress assets: %w", err)
	}
	return items, nil
}

func (r *MySQLRepository) CreateProgress(
	ctx context.Context,
	studyGroupID, groupID, authorID uint64,
	input ProgressInput,
	at time.Time,
) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning ministry progress: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO ministry_progress
			(study_group_id,ministry_group_id,author_user_id,occurred_at,content_markdown,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?)`,
		studyGroupID,
		groupID,
		authorID,
		input.OccurredAt,
		input.Content,
		at,
		at,
	)
	if err != nil {
		return 0, fmt.Errorf("creating ministry progress: %w", err)
	}
	progressID, err := lastInsertID(result)
	if err != nil {
		return 0, err
	}
	for index, assetID := range input.AssetIDs {
		result, err := tx.ExecContext(
			ctx,
			`INSERT INTO ministry_progress_assets
				(study_group_id,progress_id,asset_id,sort_order,created_at)
			 SELECT ?,?,?,?,?
			   FROM assets
			  WHERE id=? AND group_id=? AND category=?`,
			studyGroupID,
			progressID,
			assetID,
			index,
			at,
			assetID,
			studyGroupID,
			fmt.Sprintf("ministry-%d", groupID),
		)
		if err != nil {
			return 0, fmt.Errorf("linking ministry progress asset: %w", err)
		}
		if err := requireOneRow(result, ErrInvalidAttachment); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing ministry progress: %w", err)
	}
	return progressID, nil
}

func (r *MySQLRepository) notifyModerators(
	ctx context.Context,
	studyGroupID, groupID uint64,
	eventKey, notificationType, title, body string,
	at time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning moderator notification: %w", err)
	}
	defer tx.Rollback()
	if err := notifyModeratorsTx(
		ctx,
		tx,
		studyGroupID,
		groupID,
		eventKey,
		notificationType,
		title,
		body,
		at,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing moderator notification: %w", err)
	}
	return nil
}

func notifyModeratorsTx(
	ctx context.Context,
	tx *sql.Tx,
	studyGroupID, groupID uint64,
	eventKey, notificationType, title, body string,
	at time.Time,
) error {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT user_id FROM (
		    SELECT leader_user_id AS user_id
		      FROM ministry_groups
		     WHERE id=? AND study_group_id=? AND leader_user_id IS NOT NULL
		    UNION
		    SELECT user_id
		      FROM ministry_group_members
		     WHERE study_group_id=? AND ministry_group_id=? AND status=1 AND role='admin'
		    UNION
		    SELECT user_id
		      FROM user_group_roles
		     WHERE group_id=? AND role IN ('group_admin','group_leader')
		    UNION
		    SELECT id AS user_id FROM users WHERE is_super_admin=1 AND status=1
		  ) recipients
		  WHERE user_id IS NOT NULL`,
		groupID,
		studyGroupID,
		studyGroupID,
		groupID,
		studyGroupID,
	)
	if err != nil {
		return fmt.Errorf("listing ministry moderators: %w", err)
	}
	defer rows.Close()
	recipients := []uint64{}
	for rows.Next() {
		var userID uint64
		if err := rows.Scan(&userID); err != nil {
			return fmt.Errorf("scanning ministry moderator: %w", err)
		}
		recipients = append(recipients, userID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating ministry moderators: %w", err)
	}
	for _, userID := range recipients {
		if err := insertNotificationTx(
			ctx,
			tx,
			studyGroupID,
			groupID,
			userID,
			eventKey,
			notificationType,
			title,
			body,
			at,
		); err != nil {
			return err
		}
	}
	return nil
}

func insertNotificationTx(
	ctx context.Context,
	tx *sql.Tx,
	studyGroupID, groupID, userID uint64,
	eventKey, notificationType, title, body string,
	at time.Time,
) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT IGNORE INTO ministry_notifications
			(study_group_id,ministry_group_id,user_id,event_key,notification_type,title,body,created_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		studyGroupID,
		groupID,
		userID,
		eventKey,
		notificationType,
		title,
		body,
		at,
	)
	if err != nil {
		return fmt.Errorf("creating ministry notification: %w", err)
	}
	return nil
}

func scanRequests(rows *sql.Rows) ([]Request, error) {
	items := []Request{}
	for rows.Next() {
		var item Request
		if err := rows.Scan(
			&item.ID,
			&item.GroupID,
			&item.UserID,
			&item.UserDisplayName,
			&item.Message,
			&item.Status,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning ministry request: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ministry requests: %w", err)
	}
	return items, nil
}

func groupNameTx(
	ctx context.Context,
	tx *sql.Tx,
	studyGroupID, groupID uint64,
) (string, error) {
	var name string
	if err := tx.QueryRowContext(
		ctx,
		`SELECT name FROM ministry_groups WHERE id=? AND study_group_id=?`,
		groupID,
		studyGroupID,
	).Scan(&name); err != nil {
		return "", fmt.Errorf("loading ministry group name: %w", err)
	}
	return name, nil
}

func userDisplayNameTx(ctx context.Context, tx *sql.Tx, userID uint64) (string, error) {
	var name string
	if err := tx.QueryRowContext(
		ctx,
		`SELECT display_name FROM users WHERE id=?`,
		userID,
	).Scan(&name); err != nil {
		return "", fmt.Errorf("loading ministry user name: %w", err)
	}
	return name, nil
}

func lastInsertID(result sql.Result) (uint64, error) {
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading insert id: %w", err)
	}
	if id <= 0 {
		return 0, errors.New("ministry: invalid insert id")
	}
	return uint64(id), nil
}

func requireOneRow(result sql.Result, notFound error) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading affected rows: %w", err)
	}
	if affected != 1 {
		return notFound
	}
	return nil
}
