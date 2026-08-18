UPDATE ministry_groups g
JOIN users u ON u.id = g.leader_user_id
SET g.leader_user_id = NULL,
    g.updated_at = UTC_TIMESTAMP(3)
WHERE u.is_super_admin = 1;

UPDATE ministry_group_members m
JOIN users u ON u.id = m.user_id
SET m.status = 0,
    m.role = 'member',
    m.updated_at = UTC_TIMESTAMP(3)
WHERE u.is_super_admin = 1
  AND m.status = 1;
