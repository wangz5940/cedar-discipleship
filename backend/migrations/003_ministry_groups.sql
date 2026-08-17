CREATE TABLE IF NOT EXISTS ministry_groups (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  study_group_id BIGINT UNSIGNED NOT NULL,
  code VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  description VARCHAR(512) NOT NULL DEFAULT '',
  leader_user_id BIGINT UNSIGNED NULL,
  member_visibility VARCHAR(16) NOT NULL DEFAULT 'all',
  share_auto_approve TINYINT NOT NULL DEFAULT 0,
  status TINYINT NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_ministry_group_code (study_group_id, code),
  KEY idx_ministry_group_status (study_group_id, status, name),
  KEY idx_ministry_group_leader (study_group_id, leader_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ministry_group_members (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  study_group_id BIGINT UNSIGNED NOT NULL,
  ministry_group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  role VARCHAR(16) NOT NULL DEFAULT 'member',
  identity_public TINYINT NOT NULL DEFAULT 0,
  status TINYINT NOT NULL DEFAULT 1,
  joined_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_ministry_member (ministry_group_id, user_id),
  KEY idx_ministry_member_user (study_group_id, user_id, status),
  KEY idx_ministry_member_group (study_group_id, ministry_group_id, status, role)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ministry_group_requests (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  study_group_id BIGINT UNSIGNED NOT NULL,
  ministry_group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  request_type VARCHAR(16) NOT NULL DEFAULT 'join',
  message VARCHAR(512) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  reviewed_by BIGINT UNSIGNED NULL,
  reviewed_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_ministry_request_current (ministry_group_id, user_id, request_type),
  KEY idx_ministry_request_pending (study_group_id, ministry_group_id, status, created_at),
  KEY idx_ministry_request_user (study_group_id, user_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ministry_notifications (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  study_group_id BIGINT UNSIGNED NOT NULL,
  ministry_group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  event_key VARCHAR(160) NOT NULL,
  notification_type VARCHAR(32) NOT NULL,
  title VARCHAR(255) NOT NULL,
  body VARCHAR(1000) NOT NULL DEFAULT '',
  read_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_ministry_notification_event (user_id, event_key),
  KEY idx_ministry_notification_user (study_group_id, user_id, read_at, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ministry_shares (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  study_group_id BIGINT UNSIGNED NOT NULL,
  ministry_group_id BIGINT UNSIGNED NOT NULL,
  author_user_id BIGINT UNSIGNED NOT NULL,
  title VARCHAR(255) NOT NULL,
  body_markdown MEDIUMTEXT NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  reviewed_by BIGINT UNSIGNED NULL,
  reviewed_at DATETIME(3) NULL,
  published_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_ministry_share_feed (study_group_id, ministry_group_id, status, updated_at),
  KEY idx_ministry_share_author (study_group_id, author_user_id, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ministry_progress (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  study_group_id BIGINT UNSIGNED NOT NULL,
  ministry_group_id BIGINT UNSIGNED NOT NULL,
  author_user_id BIGINT UNSIGNED NOT NULL,
  occurred_at DATETIME(3) NOT NULL,
  content_markdown MEDIUMTEXT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_ministry_progress_feed (study_group_id, ministry_group_id, occurred_at, id),
  KEY idx_ministry_progress_author (study_group_id, author_user_id, occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ministry_progress_assets (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  study_group_id BIGINT UNSIGNED NOT NULL,
  progress_id BIGINT UNSIGNED NOT NULL,
  asset_id BIGINT UNSIGNED NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_ministry_progress_asset (progress_id, asset_id),
  KEY idx_ministry_progress_asset_group (study_group_id, progress_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
