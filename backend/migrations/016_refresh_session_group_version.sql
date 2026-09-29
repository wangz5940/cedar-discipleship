SET @add_refresh_session_group_version = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'refresh_sessions'
     AND COLUMN_NAME = 'group_version') = 0,
  'ALTER TABLE refresh_sessions ADD COLUMN group_version BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER current_group_id',
  'SELECT 1'
);
PREPARE add_refresh_session_group_version FROM @add_refresh_session_group_version;
EXECUTE add_refresh_session_group_version;
DEALLOCATE PREPARE add_refresh_session_group_version;
