CREATE TABLE IF NOT EXISTS learning_reminders (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  sender_id BIGINT UNSIGNED NOT NULL,
  recipient_id BIGINT UNSIGNED NOT NULL,
  logical_date DATE NOT NULL,
  created_at DATETIME(3) NOT NULL,
  read_at DATETIME(3) NULL,
  UNIQUE KEY uk_daily_sender (group_id,sender_id,recipient_id,logical_date),
  KEY idx_recipient_unread (group_id,recipient_id,read_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS learning_reminder_preferences (
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  muted BOOLEAN NOT NULL DEFAULT FALSE,
  PRIMARY KEY (group_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
