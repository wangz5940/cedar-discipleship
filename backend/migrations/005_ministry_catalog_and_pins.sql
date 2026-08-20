CREATE TABLE IF NOT EXISTS ministry_share_pins (
  study_group_id BIGINT UNSIGNED NOT NULL,
  share_id BIGINT UNSIGNED NOT NULL,
  pinned_by BIGINT UNSIGNED NOT NULL,
  pinned_at DATETIME(3) NOT NULL,
  PRIMARY KEY (share_id),
  KEY idx_ministry_share_pin_group (study_group_id, pinned_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ministry_migration_markers (
  migration_key VARCHAR(96) PRIMARY KEY,
  applied_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

UPDATE ministry_groups
SET name = CASE code
  WHEN 'leading' THEN '领会'
  WHEN 'hosting' THEN '主持'
  WHEN 'catering' THEN '伙食'
  WHEN 'logistics' THEN '后勤'
  WHEN 'cleaning' THEN '整洁'
  WHEN 'technology' THEN '技术'
  WHEN 'planning' THEN '策划'
  WHEN 'counting' THEN '数点'
  WHEN 'reporting' THEN '汇报'
  WHEN 'children' THEN '主日学'
  WHEN 'discipleship-counting' THEN '门训数点'
  WHEN 'discipleship-planning' THEN '门训规划'
  ELSE name
END,
updated_at=UTC_TIMESTAMP(3)
WHERE (
  (code='leading' AND name='领会组')
  OR (code='hosting' AND name='主持组')
  OR (code='catering' AND name='伙食组')
  OR (code='logistics' AND name='后勤组')
  OR (code='cleaning' AND name='整洁组')
  OR (code='technology' AND name='技术组')
  OR (code='planning' AND name='策划组')
  OR (code='counting' AND name='数点组')
  OR (code='reporting' AND name='回报组')
  OR (code='children' AND name='娃娃组')
  OR (code='discipleship-counting' AND name='门训数点组')
  OR (code='discipleship-planning' AND name='门训规划发布组')
)
AND NOT EXISTS (
  SELECT 1 FROM ministry_migration_markers
  WHERE migration_key='20260820_ministry_catalog'
);

UPDATE ministry_groups
SET status=1, updated_at=UTC_TIMESTAMP(3)
WHERE code IN (
  'leading',
  'hosting',
  'catering',
  'logistics',
  'cleaning',
  'technology',
  'planning',
  'counting',
  'reporting',
  'children',
  'discipleship-counting',
  'discipleship-planning'
)
AND NOT EXISTS (
  SELECT 1 FROM ministry_migration_markers
  WHERE migration_key='20260820_ministry_catalog'
);

UPDATE ministry_groups
SET status=0, leader_user_id=NULL, updated_at=UTC_TIMESTAMP(3)
WHERE code NOT IN (
  'leading',
  'hosting',
  'catering',
  'logistics',
  'cleaning',
  'technology',
  'planning',
  'counting',
  'reporting',
  'children',
  'discipleship-counting',
  'discipleship-planning'
)
AND NOT EXISTS (
  SELECT 1 FROM ministry_migration_markers
  WHERE migration_key='20260820_ministry_catalog'
);

INSERT IGNORE INTO ministry_migration_markers (migration_key, applied_at)
VALUES ('20260820_ministry_catalog', UTC_TIMESTAMP(3));
