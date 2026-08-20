INSERT IGNORE INTO ministry_groups
  (study_group_id, code, name, created_at, updated_at)
SELECT g.id, catalog.code, catalog.name, UTC_TIMESTAMP(3), UTC_TIMESTAMP(3)
FROM study_groups g
JOIN (
  SELECT 'leading' AS code, '领会' AS name
  UNION ALL SELECT 'hosting', '主持'
  UNION ALL SELECT 'catering', '伙食'
  UNION ALL SELECT 'logistics', '后勤'
  UNION ALL SELECT 'cleaning', '整洁'
  UNION ALL SELECT 'technology', '技术'
  UNION ALL SELECT 'planning', '策划'
  UNION ALL SELECT 'counting', '数点'
  UNION ALL SELECT 'reporting', '汇报'
  UNION ALL SELECT 'children', '主日学'
  UNION ALL SELECT 'discipleship-counting', '门训数点'
  UNION ALL SELECT 'discipleship-planning', '门训规划'
) catalog ON 1=1;
