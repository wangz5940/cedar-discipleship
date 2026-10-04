-- 按账号与任务定位历史完成记录，避免随着账号使用年限增长扫描所有同类打卡。
SET @add_checkin_task_index = IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'checkin_records'
     AND INDEX_NAME = 'idx_group_user_task') = 0,
  'ALTER TABLE checkin_records ADD INDEX idx_group_user_task (group_id, user_id, task_id, task_type, deleted_at, logical_date)',
  'SELECT 1'
);
PREPARE add_checkin_task_index FROM @add_checkin_task_index;
EXECUTE add_checkin_task_index;
DEALLOCATE PREPARE add_checkin_task_index;
