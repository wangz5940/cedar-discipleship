-- 普通发送者保持数据库每日唯一约束；超级管理员使用 NULL 槽位允许重复提醒。
SET @add_daily_limit_slot = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'learning_reminders' AND COLUMN_NAME = 'daily_limit_slot') = 0,
  'ALTER TABLE learning_reminders ADD COLUMN daily_limit_slot TINYINT UNSIGNED NULL DEFAULT 1',
  'SELECT 1'
);
PREPARE add_daily_limit_slot FROM @add_daily_limit_slot;
EXECUTE add_daily_limit_slot;
DEALLOCATE PREPARE add_daily_limit_slot;

SET @replace_daily_sender_index = IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'learning_reminders' AND INDEX_NAME = 'uk_daily_sender' AND COLUMN_NAME = 'daily_limit_slot') = 0,
  'ALTER TABLE learning_reminders DROP INDEX uk_daily_sender, ADD UNIQUE KEY uk_daily_sender (group_id,sender_id,recipient_id,logical_date,daily_limit_slot)',
  'SELECT 1'
);
PREPARE replace_daily_sender_index FROM @replace_daily_sender_index;
EXECUTE replace_daily_sender_index;
DEALLOCATE PREPARE replace_daily_sender_index;
