SET @drop_asset_binding_active_version_id = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'asset_bindings'
     AND COLUMN_NAME = 'active_version_id') > 0,
  'ALTER TABLE asset_bindings DROP COLUMN active_version_id',
  'SELECT 1'
);
PREPARE drop_asset_binding_active_version_id FROM @drop_asset_binding_active_version_id;
EXECUTE drop_asset_binding_active_version_id;
DEALLOCATE PREPARE drop_asset_binding_active_version_id;

SET @drop_asset_binding_version_policy = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'asset_bindings'
     AND COLUMN_NAME = 'version_policy') > 0,
  'ALTER TABLE asset_bindings DROP COLUMN version_policy',
  'SELECT 1'
);
PREPARE drop_asset_binding_version_policy FROM @drop_asset_binding_version_policy;
EXECUTE drop_asset_binding_version_policy;
DEALLOCATE PREPARE drop_asset_binding_version_policy;
