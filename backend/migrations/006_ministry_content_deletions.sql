CREATE TABLE IF NOT EXISTS ministry_content_deletions (
  content_type VARCHAR(16) NOT NULL,
  content_id BIGINT UNSIGNED NOT NULL,
  study_group_id BIGINT UNSIGNED NOT NULL,
  ministry_group_id BIGINT UNSIGNED NOT NULL,
  deleted_by BIGINT UNSIGNED NOT NULL,
  deleted_at DATETIME(3) NOT NULL,
  restored_by BIGINT UNSIGNED NULL,
  restored_at DATETIME(3) NULL,
  PRIMARY KEY (content_type, content_id),
  KEY idx_ministry_content_trash (study_group_id, ministry_group_id, restored_at, deleted_at),
  KEY idx_ministry_content_deleted_by (study_group_id, deleted_by, restored_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
