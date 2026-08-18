INSERT IGNORE INTO ministry_groups
  (study_group_id, code, name, created_at, updated_at)
SELECT g.id, catalog.code, catalog.name, UTC_TIMESTAMP(3), UTC_TIMESTAMP(3)
FROM study_groups g
JOIN (
  SELECT 'leading' AS code, '领会组' AS name
  UNION ALL SELECT 'hosting', '主持组'
  UNION ALL SELECT 'catering', '伙食组'
  UNION ALL SELECT 'logistics', '后勤组'
  UNION ALL SELECT 'cleaning', '整洁组'
  UNION ALL SELECT 'technology', '技术组'
  UNION ALL SELECT 'planning', '策划组'
  UNION ALL SELECT 'counting', '数点组'
  UNION ALL SELECT 'visitation', '探望组'
  UNION ALL SELECT 'reporting', '回报组'
  UNION ALL SELECT 'children', '娃娃组'
  UNION ALL SELECT 'intercession', '守望组'
  UNION ALL SELECT 'discipleship-counting', '门训数点组'
  UNION ALL SELECT 'discipleship-planning', '门训规划发布组'
  UNION ALL SELECT 'discipleship-review', '门训批改组'
) catalog ON 1=1;

UPDATE ministry_groups ministry
JOIN (
  SELECT group_id, MIN(user_id) AS user_id
  FROM user_group_roles
  WHERE role='group_leader'
  GROUP BY group_id
) leaders ON leaders.group_id=ministry.study_group_id
SET ministry.leader_user_id=leaders.user_id,
    ministry.updated_at=UTC_TIMESTAMP(3)
WHERE ministry.leader_user_id IS NULL;

INSERT INTO ministry_group_members
  (study_group_id,ministry_group_id,user_id,role,identity_public,status,joined_at,created_at,updated_at)
SELECT ministry.study_group_id,ministry.id,ministry.leader_user_id,'member',1,1,
       UTC_TIMESTAMP(3),UTC_TIMESTAMP(3),UTC_TIMESTAMP(3)
FROM ministry_groups ministry
WHERE ministry.leader_user_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  status=1,
  identity_public=1,
  updated_at=VALUES(updated_at);
