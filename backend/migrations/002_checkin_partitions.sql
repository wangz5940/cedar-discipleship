CREATE TABLE IF NOT EXISTS checkin_records (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  task_id BIGINT UNSIGNED NULL,
  week_id BIGINT UNSIGNED NULL,
  logical_date DATE NOT NULL,
  checkin_time DATETIME(3) NOT NULL,
  task_type VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'done',
  is_retro TINYINT NOT NULL DEFAULT 0,
  detail VARCHAR(1024) NOT NULL DEFAULT '',
  note TEXT NULL,
  part VARCHAR(64) NOT NULL DEFAULT '',
  source VARCHAR(32) NOT NULL DEFAULT 'web',
  active_key BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_by BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  deleted_at DATETIME(3) NULL,
  PRIMARY KEY (id, logical_date),
  UNIQUE KEY uk_one_active_checkin (group_id, user_id, task_type, logical_date, part, active_key),
  KEY idx_group_date_type (group_id, logical_date, task_type),
  KEY idx_group_user_date (group_id, user_id, logical_date),
  KEY idx_group_week_type (group_id, week_id, task_type),
  KEY idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
PARTITION BY RANGE COLUMNS(logical_date) (
  PARTITION p2026q1 VALUES LESS THAN ('2026-04-01'),
  PARTITION p2026q2 VALUES LESS THAN ('2026-07-01'),
  PARTITION p2026q3 VALUES LESS THAN ('2026-10-01'),
  PARTITION p2026q4 VALUES LESS THAN ('2027-01-01'),
  PARTITION pmax VALUES LESS THAN (MAXVALUE)
);
