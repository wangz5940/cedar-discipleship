SET @add_request_submission_round = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'ministry_group_requests'
     AND COLUMN_NAME = 'submission_round') = 0,
  'ALTER TABLE ministry_group_requests ADD COLUMN submission_round BIGINT UNSIGNED NOT NULL DEFAULT 1',
  'SELECT 1'
);
PREPARE add_request_submission_round FROM @add_request_submission_round;
EXECUTE add_request_submission_round;
DEALLOCATE PREPARE add_request_submission_round;

SET @add_share_submission_round = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'ministry_shares'
     AND COLUMN_NAME = 'submission_round') = 0,
  'ALTER TABLE ministry_shares ADD COLUMN submission_round BIGINT UNSIGNED NOT NULL DEFAULT 1',
  'SELECT 1'
);
PREPARE add_share_submission_round FROM @add_share_submission_round;
EXECUTE add_share_submission_round;
DEALLOCATE PREPARE add_share_submission_round;
