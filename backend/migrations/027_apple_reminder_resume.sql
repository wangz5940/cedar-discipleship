-- 苹果设备解除静音后仅推送新提醒，保留历史提醒与已读状态。
SET @add_apple_push_after_id = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'learning_reminder_preferences' AND COLUMN_NAME = 'apple_push_after_id') = 0,
  'ALTER TABLE learning_reminder_preferences ADD COLUMN apple_push_after_id BIGINT UNSIGNED NOT NULL DEFAULT 0',
  'SELECT 1'
);
PREPARE add_apple_push_after_id FROM @add_apple_push_after_id;
EXECUTE add_apple_push_after_id;
DEALLOCATE PREPARE add_apple_push_after_id;
