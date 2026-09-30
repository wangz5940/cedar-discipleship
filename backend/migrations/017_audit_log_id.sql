SET @add_audit_log_id = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'audit_logs'
     AND COLUMN_NAME = 'log_id') = 0,
  'ALTER TABLE audit_logs ADD COLUMN log_id VARCHAR(32) NOT NULL DEFAULT '''' AFTER user_agent',
  'SELECT 1'
);
PREPARE add_audit_log_id FROM @add_audit_log_id;
EXECUTE add_audit_log_id;
DEALLOCATE PREPARE add_audit_log_id;

SET @add_audit_log_id_index = IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'audit_logs'
     AND INDEX_NAME = 'idx_log_id') = 0,
  'ALTER TABLE audit_logs ADD KEY idx_log_id (log_id)',
  'SELECT 1'
);
PREPARE add_audit_log_id_index FROM @add_audit_log_id_index;
EXECUTE add_audit_log_id_index;
DEALLOCATE PREPARE add_audit_log_id_index;
