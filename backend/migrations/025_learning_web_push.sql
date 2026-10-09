CREATE TABLE IF NOT EXISTS learning_push_keys (
  id TINYINT UNSIGNED PRIMARY KEY,
  public_key VARCHAR(128) NOT NULL,
  private_key VARCHAR(128) NOT NULL
) ENGINE=InnoDB;

SET @add_reminder_created_index = IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'learning_reminders' AND INDEX_NAME = 'idx_reminder_created') = 0,
  'ALTER TABLE learning_reminders ADD INDEX idx_reminder_created (created_at)',
  'SELECT 1'
);
PREPARE add_reminder_created_index FROM @add_reminder_created_index;
EXECUTE add_reminder_created_index;
DEALLOCATE PREPARE add_reminder_created_index;

CREATE TABLE IF NOT EXISTS learning_push_subscriptions (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  endpoint_hash BINARY(32) NOT NULL UNIQUE,
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  subscription JSON NOT NULL,
  created_at DATETIME(3) NOT NULL,
  KEY idx_push_member (group_id,user_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS learning_push_deliveries (
  reminder_id BIGINT UNSIGNED NOT NULL,
  subscription_id BIGINT UNSIGNED NOT NULL,
  attempts SMALLINT UNSIGNED NOT NULL DEFAULT 0,
  next_attempt_at DATETIME(3) NOT NULL,
  finished BOOLEAN NOT NULL DEFAULT FALSE,
  PRIMARY KEY (reminder_id,subscription_id),
  KEY idx_push_pending (finished,next_attempt_at)
) ENGINE=InnoDB;
