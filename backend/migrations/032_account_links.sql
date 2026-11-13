CREATE TABLE IF NOT EXISTS account_link_lock (
  id TINYINT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=InnoDB;
INSERT IGNORE INTO account_link_lock(id) VALUES (1);

CREATE TABLE IF NOT EXISTS account_links (
  user_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  identity_id BIGINT UNSIGNED NOT NULL,
  KEY idx_account_links_identity (identity_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS account_link_reviews (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  first_user_id BIGINT UNSIGNED NOT NULL,
  second_user_id BIGINT UNSIGNED NOT NULL,
  snapshot JSON NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  requested_by BIGINT UNSIGNED NOT NULL,
  reviewed_by BIGINT UNSIGNED NULL,
  created_at DATETIME(3) NOT NULL,
  reviewed_at DATETIME(3) NULL
) ENGINE=InnoDB;
