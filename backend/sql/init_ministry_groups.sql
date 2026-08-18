INSERT IGNORE INTO ministry_groups
  (study_group_id, code, name, created_at, updated_at)
SELECT g.id, catalog.code, catalog.name, UTC_TIMESTAMP(3), UTC_TIMESTAMP(3)
FROM study_groups g
JOIN (
  SELECT 'leading' AS code, '领会组' AS name
  UNION ALL SELECT 'hosting', '主持组'
  UNION ALL SELECT 'catering', '伙食组'
  UNION ALL SELECT 'logistics', '后勤组'
  UNION ALL SELECT 'cleaning', '整洁组'
  UNION ALL SELECT 'technology', '技术组'
  UNION ALL SELECT 'planning', '策划组'
  UNION ALL SELECT 'counting', '数点组'
  UNION ALL SELECT 'visitation', '探望组'
  UNION ALL SELECT 'reporting', '回报组'
  UNION ALL SELECT 'children', '娃娃组'
  UNION ALL SELECT 'intercession', '守望组'
  UNION ALL SELECT 'discipleship-counting', '门训数点组'
  UNION ALL SELECT 'discipleship-planning', '门训规划发布组'
  UNION ALL SELECT 'discipleship-review', '门训批改组'
) catalog ON 1=1;
