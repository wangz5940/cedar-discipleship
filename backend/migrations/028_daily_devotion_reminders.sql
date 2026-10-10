-- 每个小组每天只生成一次早间提醒，重启和多实例不会重复生成。
CREATE TABLE IF NOT EXISTS daily_devotion_reminder_runs (
  group_id BIGINT UNSIGNED NOT NULL,
  logical_date DATE NOT NULL,
  PRIMARY KEY (group_id,logical_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
