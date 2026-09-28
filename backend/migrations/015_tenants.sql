CREATE TABLE IF NOT EXISTS tenants (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(128) NOT NULL,
  status TINYINT NOT NULL DEFAULT 1,
  created_by BIGINT UNSIGNED NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO tenants (id,name,status,created_at,updated_at)
VALUES (1,'原有小家',1,UTC_TIMESTAMP(3),UTC_TIMESTAMP(3));

UPDATE tenants SET name='原有小家',updated_at=UTC_TIMESTAMP(3)
WHERE id=1 AND name='原有主体';

SET @add_tenant_to_groups = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='study_groups' AND COLUMN_NAME='tenant_id')=0,
  'ALTER TABLE study_groups ADD COLUMN tenant_id BIGINT UNSIGNED NOT NULL DEFAULT 1, ADD KEY idx_study_groups_tenant (tenant_id,status,id)',
  'SELECT 1'
);
PREPARE add_tenant_to_groups FROM @add_tenant_to_groups;
EXECUTE add_tenant_to_groups;
DEALLOCATE PREPARE add_tenant_to_groups;

CREATE TABLE IF NOT EXISTS tenant_members (
  tenant_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  role VARCHAR(16) NOT NULL DEFAULT 'member',
  status TINYINT NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (tenant_id,user_id),
  KEY idx_tenant_members_user (user_id,status,tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO tenant_members (tenant_id,user_id,role,status,created_at,updated_at)
SELECT DISTINCT g.tenant_id,m.user_id,'member',1,UTC_TIMESTAMP(3),UTC_TIMESTAMP(3)
FROM group_members m JOIN study_groups g ON g.id=m.group_id
WHERE m.status=1;
