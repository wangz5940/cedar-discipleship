-- Final resource storage model:
-- only data/resources/team-{group_code}-resources/objects/{resource_key}/{filename}
-- is a valid resource path. Any asset row outside that layout is detached from
-- active resource access.

SET @drop_asset_import_source_version_id = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'asset_import_events'
     AND COLUMN_NAME = 'source_version_id') > 0,
  'ALTER TABLE asset_import_events DROP COLUMN source_version_id',
  'SELECT 1'
);
PREPARE drop_asset_import_source_version_id FROM @drop_asset_import_source_version_id;
EXECUTE drop_asset_import_source_version_id;
DEALLOCATE PREPARE drop_asset_import_source_version_id;

SET @drop_asset_dependency_selected_version_id = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'asset_dependencies'
     AND COLUMN_NAME = 'selected_version_id') > 0,
  'ALTER TABLE asset_dependencies DROP COLUMN selected_version_id',
  'SELECT 1'
);
PREPARE drop_asset_dependency_selected_version_id FROM @drop_asset_dependency_selected_version_id;
EXECUTE drop_asset_dependency_selected_version_id;
DEALLOCATE PREPARE drop_asset_dependency_selected_version_id;

UPDATE asset_bindings b
JOIN assets a ON a.id = b.asset_id
SET b.deleted_at = COALESCE(b.deleted_at, UTC_TIMESTAMP(3)),
    b.updated_at = UTC_TIMESTAMP(3),
    a.updated_at = UTC_TIMESTAMP(3)
WHERE a.storage_path NOT LIKE 'team-%-resources/objects/%'
  AND b.deleted_at IS NULL;

UPDATE asset_share_grants g
JOIN assets a ON a.id = g.asset_id
SET g.status = 'revoked',
    g.revoked_at = COALESCE(g.revoked_at, UTC_TIMESTAMP(3))
WHERE a.storage_path NOT LIKE 'team-%-resources/objects/%'
  AND g.status = 'active';

UPDATE asset_dependencies d
JOIN assets ca ON ca.id = d.consumer_asset_id
JOIN assets pa ON pa.id = d.provider_asset_id
SET d.status = 'removed',
    d.updated_at = UTC_TIMESTAMP(3)
WHERE d.status = 'active'
  AND (
    ca.storage_path NOT LIKE 'team-%-resources/objects/%'
    OR pa.storage_path NOT LIKE 'team-%-resources/objects/%'
  );
