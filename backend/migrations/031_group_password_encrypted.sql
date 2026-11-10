SET @add_group_password_encrypted = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'study_groups'
     AND COLUMN_NAME = 'default_password_encrypted') = 0,
  'ALTER TABLE study_groups ADD COLUMN default_password_encrypted TEXT NULL',
  'SELECT 1'
);
PREPARE add_group_password_encrypted FROM @add_group_password_encrypted;
EXECUTE add_group_password_encrypted;
DEALLOCATE PREPARE add_group_password_encrypted;
