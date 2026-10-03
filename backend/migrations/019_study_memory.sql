CREATE TABLE IF NOT EXISTS study_progress (
  user_id BIGINT UNSIGNED NOT NULL,
  media_key VARCHAR(512) COLLATE utf8mb4_bin NOT NULL,
  position_seconds DOUBLE NOT NULL,
  duration_seconds DOUBLE NOT NULL,
  completed BOOLEAN NOT NULL DEFAULT FALSE,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (user_id, media_key),
  CONSTRAINT fk_study_progress_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS study_favorites (
  user_id BIGINT UNSIGNED NOT NULL,
  media_key VARCHAR(512) COLLATE utf8mb4_bin NOT NULL,
  item_json JSON NOT NULL,
  saved_at DATETIME(3) NOT NULL,
  PRIMARY KEY (user_id, media_key),
  CONSTRAINT fk_study_favorites_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
