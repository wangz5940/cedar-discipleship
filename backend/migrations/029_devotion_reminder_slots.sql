-- 原有调度记录归入 7 点时段，新增 18 点时段独立去重；重启可重复执行。
SET @add_devotion_reminder_hour = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'daily_devotion_reminder_runs' AND COLUMN_NAME = 'reminder_hour') = 0,
  'ALTER TABLE daily_devotion_reminder_runs ADD COLUMN reminder_hour TINYINT UNSIGNED NOT NULL DEFAULT 7, DROP PRIMARY KEY, ADD PRIMARY KEY (group_id,logical_date,reminder_hour)',
  'SELECT 1'
);
PREPARE add_devotion_reminder_hour FROM @add_devotion_reminder_hour;
EXECUTE add_devotion_reminder_hour;
DEALLOCATE PREPARE add_devotion_reminder_hour;
