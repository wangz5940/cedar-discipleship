CREATE TABLE IF NOT EXISTS study_groups (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description VARCHAR(512) NOT NULL DEFAULT '',
  default_password_hash VARCHAR(255) NOT NULL DEFAULT '',
  status TINYINT NOT NULL DEFAULT 1,
  created_by BIGINT UNSIGNED NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  username VARCHAR(64) NOT NULL UNIQUE,
  display_name VARCHAR(128) NOT NULL,
  name_pinyin VARCHAR(128) NOT NULL,
  password_hash VARCHAR(255) NOT NULL DEFAULT '',
  is_super_admin TINYINT NOT NULL DEFAULT 0,
  default_group_id BIGINT UNSIGNED NULL,
  must_change_password TINYINT NOT NULL DEFAULT 0,
  email VARCHAR(255) NOT NULL DEFAULT '',
  phone VARCHAR(32) NOT NULL DEFAULT '',
  status TINYINT NOT NULL DEFAULT 1,
  created_by BIGINT UNSIGNED NULL,
  last_login_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_default_group (default_group_id),
  KEY idx_pinyin (name_pinyin)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS group_members (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  member_name VARCHAR(128) NOT NULL,
  status TINYINT NOT NULL DEFAULT 1,
  joined_at DATETIME(3) NOT NULL,
  created_by BIGINT UNSIGNED NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_group_user (group_id, user_id),
  KEY idx_user_status (user_id, status),
  KEY idx_group_status_name (group_id, status, member_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_group_roles (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  role VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_group_user_role (group_id, user_id, role),
  KEY idx_user_role (user_id, role)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS group_settings (
  group_id BIGINT UNSIGNED PRIMARY KEY,
  site_title VARCHAR(128) NOT NULL DEFAULT '',
  brand_name VARCHAR(128) NOT NULL DEFAULT '',
  hero_kicker VARCHAR(128) NOT NULL DEFAULT '',
  hero_desc VARCHAR(512) NOT NULL DEFAULT '',
  dashboard_title VARCHAR(128) NOT NULL DEFAULT '',
  button_labels JSON NULL,
  settings JSON NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS study_weeks (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  start_date DATE NOT NULL,
  end_date DATE NOT NULL,
  title VARCHAR(512) NOT NULL DEFAULT '',
  verse_ref VARCHAR(255) NOT NULL DEFAULT '',
  recite_text TEXT NULL,
  book_enabled TINYINT NOT NULL DEFAULT 1,
  video_enabled TINYINT NOT NULL DEFAULT 1,
  verse_enabled TINYINT NOT NULL DEFAULT 1,
  outline_enabled TINYINT NOT NULL DEFAULT 1,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_group_week (group_id, start_date, end_date),
  KEY idx_group_start (group_id, start_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS study_tasks (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  week_id BIGINT UNSIGNED NULL,
  task_type VARCHAR(32) NOT NULL,
  title VARCHAR(512) NOT NULL,
  content TEXT NULL,
  required TINYINT NOT NULL DEFAULT 1,
  enabled TINYINT NOT NULL DEFAULT 1,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_group_week_type (group_id, week_id, task_type),
  KEY idx_group_type_enabled (group_id, task_type, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS assets (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  category VARCHAR(32) NOT NULL,
  title VARCHAR(255) NOT NULL,
  original_name VARCHAR(255) NOT NULL,
  storage_path VARCHAR(1024) NOT NULL,
  mime_type VARCHAR(128) NOT NULL DEFAULT '',
  file_size BIGINT UNSIGNED NOT NULL DEFAULT 0,
  checksum_sha256 CHAR(64) NOT NULL DEFAULT '',
  visibility VARCHAR(32) NOT NULL DEFAULT 'group',
  created_by BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_group_category (group_id, category),
  KEY idx_group_title (group_id, title)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS task_assets (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  task_id BIGINT UNSIGNED NOT NULL,
  asset_id BIGINT UNSIGNED NOT NULL,
  usage_type VARCHAR(32) NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_task_asset_usage (task_id, asset_id, usage_type),
  KEY idx_group_task (group_id, task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS recite_attempts (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  week_id BIGINT UNSIGNED NULL,
  checkin_record_id BIGINT UNSIGNED NULL,
  verse_ref VARCHAR(255) NOT NULL,
  logical_date DATE NOT NULL,
  blank_percent TINYINT NOT NULL,
  blank_count INT NOT NULL,
  correct_count INT NOT NULL,
  accuracy TINYINT NOT NULL,
  score INT NOT NULL,
  attempt_no INT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_attempt (group_id, user_id, week_id, verse_ref, attempt_no),
  KEY idx_group_ref_score (group_id, verse_ref, score),
  KEY idx_group_user_date (group_id, user_id, logical_date),
  KEY idx_group_week (group_id, week_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS feedbacks (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NULL,
  user_id BIGINT UNSIGNED NULL,
  name VARCHAR(128) NOT NULL DEFAULT '',
  contact VARCHAR(255) NOT NULL DEFAULT '',
  message TEXT NOT NULL,
  page VARCHAR(255) NOT NULL DEFAULT '',
  user_agent VARCHAR(512) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_group_created (group_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NULL,
  actor_user_id BIGINT UNSIGNED NOT NULL,
  action VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id BIGINT UNSIGNED NULL,
  before_json JSON NULL,
  after_json JSON NULL,
  ip VARCHAR(64) NOT NULL DEFAULT '',
  user_agent VARCHAR(512) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_group_created (group_id, created_at),
  KEY idx_actor_created (actor_user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
