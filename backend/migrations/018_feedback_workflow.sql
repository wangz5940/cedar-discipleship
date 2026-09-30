SET @add_feedback_status = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'feedbacks' AND COLUMN_NAME = 'status') = 0,
  'ALTER TABLE feedbacks ADD COLUMN status VARCHAR(16) NOT NULL DEFAULT ''pending'' AFTER message',
  'SELECT 1'
);
PREPARE add_feedback_status FROM @add_feedback_status;
EXECUTE add_feedback_status;
DEALLOCATE PREPARE add_feedback_status;

SET @add_feedback_source = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'feedbacks' AND COLUMN_NAME = 'source') = 0,
  'ALTER TABLE feedbacks ADD COLUMN source VARCHAR(16) NOT NULL DEFAULT ''manual'' AFTER message',
  'SELECT 1'
);
PREPARE add_feedback_source FROM @add_feedback_source;
EXECUTE add_feedback_source;
DEALLOCATE PREPARE add_feedback_source;

SET @add_feedback_log_id = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'feedbacks' AND COLUMN_NAME = 'log_id') = 0,
  'ALTER TABLE feedbacks ADD COLUMN log_id VARCHAR(32) NOT NULL DEFAULT '''' AFTER user_agent',
  'SELECT 1'
);
PREPARE add_feedback_log_id FROM @add_feedback_log_id;
EXECUTE add_feedback_log_id;
DEALLOCATE PREPARE add_feedback_log_id;

SET @add_feedback_diagnostics_json = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'feedbacks' AND COLUMN_NAME = 'diagnostics_json') = 0,
  'ALTER TABLE feedbacks ADD COLUMN diagnostics_json JSON NULL AFTER log_id',
  'SELECT 1'
);
PREPARE add_feedback_diagnostics_json FROM @add_feedback_diagnostics_json;
EXECUTE add_feedback_diagnostics_json;
DEALLOCATE PREPARE add_feedback_diagnostics_json;

SET @add_feedback_updated_at = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'feedbacks' AND COLUMN_NAME = 'updated_at') = 0,
  'ALTER TABLE feedbacks ADD COLUMN updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) AFTER created_at',
  'SELECT 1'
);
PREPARE add_feedback_updated_at FROM @add_feedback_updated_at;
EXECUTE add_feedback_updated_at;
DEALLOCATE PREPARE add_feedback_updated_at;

SET @add_feedback_user_created_index = IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'feedbacks' AND INDEX_NAME = 'idx_feedback_user_created') = 0,
  'ALTER TABLE feedbacks ADD KEY idx_feedback_user_created (user_id, created_at)',
  'SELECT 1'
);
PREPARE add_feedback_user_created_index FROM @add_feedback_user_created_index;
EXECUTE add_feedback_user_created_index;
DEALLOCATE PREPARE add_feedback_user_created_index;

SET @add_feedback_status_updated_index = IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'feedbacks' AND INDEX_NAME = 'idx_feedback_status_updated') = 0,
  'ALTER TABLE feedbacks ADD KEY idx_feedback_status_updated (status, updated_at)',
  'SELECT 1'
);
PREPARE add_feedback_status_updated_index FROM @add_feedback_status_updated_index;
EXECUTE add_feedback_status_updated_index;
DEALLOCATE PREPARE add_feedback_status_updated_index;

SET @add_feedback_log_id_index = IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'feedbacks' AND INDEX_NAME = 'idx_feedback_log_id') = 0,
  'ALTER TABLE feedbacks ADD KEY idx_feedback_log_id (log_id)',
  'SELECT 1'
);
PREPARE add_feedback_log_id_index FROM @add_feedback_log_id_index;
EXECUTE add_feedback_log_id_index;
DEALLOCATE PREPARE add_feedback_log_id_index;

CREATE TABLE IF NOT EXISTS feedback_attachments (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  feedback_id BIGINT UNSIGNED NOT NULL,
  original_name VARCHAR(255) NOT NULL,
  storage_path VARCHAR(512) NOT NULL,
  mime_type VARCHAR(64) NOT NULL,
  file_size BIGINT UNSIGNED NOT NULL,
  width INT UNSIGNED NOT NULL,
  height INT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_feedback_attachment_path (storage_path),
  KEY idx_feedback_attachment (feedback_id, id),
  CONSTRAINT fk_feedback_attachment_feedback
    FOREIGN KEY (feedback_id) REFERENCES feedbacks(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS feedback_replies (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  feedback_id BIGINT UNSIGNED NOT NULL,
  admin_user_id BIGINT UNSIGNED NOT NULL,
  message TEXT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  KEY idx_feedback_reply (feedback_id, id),
  KEY idx_feedback_reply_admin (admin_user_id, created_at),
  CONSTRAINT fk_feedback_reply_feedback
    FOREIGN KEY (feedback_id) REFERENCES feedbacks(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS feedback_automatic_settings (
  id TINYINT UNSIGNED PRIMARY KEY,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  updated_by BIGINT UNSIGNED NULL,
  updated_at DATETIME(3) NOT NULL,
  CONSTRAINT chk_feedback_automatic_settings_singleton CHECK (id = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO feedback_automatic_settings(id,enabled,updated_at)
VALUES(1,1,CURRENT_TIMESTAMP(3));

CREATE TABLE IF NOT EXISTS feedback_muted_error_types (
  error_type VARCHAR(128) PRIMARY KEY,
  muted_by BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
