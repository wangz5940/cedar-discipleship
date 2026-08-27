UPDATE assets
SET category = 'mentor',
    updated_at = UTC_TIMESTAMP(3)
WHERE storage_path LIKE 'team-%-resources/objects/%'
  AND category IN ('book', 'handout', 'passage', 'share')
  AND (
    title LIKE '%导读%'
    OR original_name LIKE '%导读%'
    OR title LIKE '%内容概要%'
    OR original_name LIKE '%内容概要%'
    OR title LIKE '%圣经纵览的目的与价值%'
    OR original_name LIKE '%圣经纵览的目的与价值%'
  );
