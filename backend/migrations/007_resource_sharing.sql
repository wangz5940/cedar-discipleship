CREATE TABLE IF NOT EXISTS asset_bindings (
  asset_id BIGINT UNSIGNED PRIMARY KEY,
  group_id BIGINT UNSIGNED NOT NULL,
  resource_key CHAR(32) NOT NULL,
  asset_kind VARCHAR(16) NOT NULL DEFAULT 'owned',
  source_asset_id BIGINT UNSIGNED NULL,
  imported_at DATETIME(3) NULL,
  deleted_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_asset_resource_key (resource_key),
  UNIQUE KEY uk_asset_import (group_id, source_asset_id),
  KEY idx_asset_source (source_asset_id),
  KEY idx_asset_active (group_id, deleted_at, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS asset_share_grants (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  asset_id BIGINT UNSIGNED NOT NULL,
  owner_group_id BIGINT UNSIGNED NOT NULL,
  consumer_group_id BIGINT UNSIGNED NULL,
  consumer_group_key BIGINT UNSIGNED GENERATED ALWAYS AS (COALESCE(consumer_group_id, 0)) STORED,
  permission VARCHAR(16) NOT NULL DEFAULT 'import',
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  created_by BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL,
  revoked_by BIGINT UNSIGNED NULL,
  revoked_at DATETIME(3) NULL,
  UNIQUE KEY uk_asset_share_grant (asset_id, consumer_group_key, permission),
  KEY idx_asset_share_consumer (consumer_group_id, status, created_at),
  KEY idx_asset_share_owner (owner_group_id, status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS asset_import_events (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  target_group_id BIGINT UNSIGNED NOT NULL,
  imported_asset_id BIGINT UNSIGNED NOT NULL,
  source_asset_id BIGINT UNSIGNED NOT NULL,
  event_type VARCHAR(24) NOT NULL,
  actor_user_id BIGINT UNSIGNED NOT NULL,
  detail JSON NULL,
  created_at DATETIME(3) NOT NULL,
  KEY idx_asset_import_history (target_group_id, imported_asset_id, created_at),
  KEY idx_asset_import_source (source_asset_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS asset_dependencies (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  consumer_group_id BIGINT UNSIGNED NOT NULL,
  consumer_asset_id BIGINT UNSIGNED NOT NULL,
  provider_group_id BIGINT UNSIGNED NOT NULL,
  provider_asset_id BIGINT UNSIGNED NOT NULL,
  dependency_type VARCHAR(16) NOT NULL DEFAULT 'import',
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_asset_dependency (consumer_asset_id, provider_asset_id),
  KEY idx_asset_dependency_provider (provider_group_id, provider_asset_id, status),
  KEY idx_asset_dependency_consumer (consumer_group_id, consumer_asset_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO asset_bindings
  (asset_id, group_id, resource_key, asset_kind, source_asset_id, imported_at, deleted_at, created_at, updated_at)
SELECT
  id,
  group_id,
  LOWER(LPAD(HEX(id), 32, '0')),
  'owned',
  NULL,
  NULL,
  NULL,
  created_at,
  updated_at
FROM assets;

UPDATE assets a
JOIN asset_bindings b ON b.asset_id = a.id AND b.group_id = a.group_id
SET a.visibility = 'all_groups'
WHERE b.asset_kind = 'owned';

INSERT INTO asset_share_grants
  (asset_id, owner_group_id, consumer_group_id, permission, status, created_by, created_at, revoked_by, revoked_at)
SELECT
  a.id,
  a.group_id,
  NULL,
  'import',
  'active',
  COALESCE(NULLIF(a.created_by, 0), 1),
  a.created_at,
  NULL,
  NULL
FROM assets a
JOIN asset_bindings b ON b.asset_id = a.id AND b.group_id = a.group_id
WHERE b.asset_kind = 'owned'
ON DUPLICATE KEY UPDATE
  status = 'active',
  created_by = VALUES(created_by),
  created_at = VALUES(created_at),
  revoked_by = NULL,
  revoked_at = NULL;
