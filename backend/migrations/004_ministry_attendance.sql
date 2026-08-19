CREATE TABLE IF NOT EXISTS ministry_attendance_settings (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  study_group_id BIGINT UNSIGNED NOT NULL,
  ministry_group_id BIGINT UNSIGNED NOT NULL,
  weekday_mask INT UNSIGNED NOT NULL DEFAULT 65,
  updated_by BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_ministry_attendance_settings (ministry_group_id),
  KEY idx_ministry_attendance_settings_group (study_group_id, ministry_group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ministry_attendance_dates (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  study_group_id BIGINT UNSIGNED NOT NULL,
  ministry_group_id BIGINT UNSIGNED NOT NULL,
  attendance_date DATE NOT NULL,
  created_by BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_ministry_attendance_date (ministry_group_id, attendance_date),
  KEY idx_ministry_attendance_date_group (study_group_id, ministry_group_id, attendance_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ministry_attendance_records (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  study_group_id BIGINT UNSIGNED NOT NULL,
  ministry_group_id BIGINT UNSIGNED NOT NULL,
  attendance_date DATE NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  present TINYINT NOT NULL DEFAULT 1,
  marked_by BIGINT UNSIGNED NOT NULL,
  marked_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_ministry_attendance_record (ministry_group_id, attendance_date, user_id),
  KEY idx_ministry_attendance_record_group (study_group_id, ministry_group_id, attendance_date),
  KEY idx_ministry_attendance_record_user (study_group_id, user_id, attendance_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
