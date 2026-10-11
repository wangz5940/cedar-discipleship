CREATE TABLE IF NOT EXISTS personal_daily_books (
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  logical_date DATE NOT NULL,
  title VARCHAR(255) NOT NULL,
  path VARCHAR(2048) NOT NULL,
  content_type VARCHAR(16) NOT NULL,
  section_number INT UNSIGNED NOT NULL DEFAULT 0,
  page_start INT UNSIGNED NOT NULL DEFAULT 0,
  page_end INT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (group_id, user_id, logical_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS personal_book_completions (
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  logical_date DATE NOT NULL,
  completed_at DATETIME NOT NULL,
  PRIMARY KEY (group_id, user_id, logical_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
