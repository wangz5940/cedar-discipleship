SET @add_recite_paper = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'recite_attempts' AND COLUMN_NAME = 'paper') = 0,
  'ALTER TABLE recite_attempts ADD COLUMN paper JSON NULL',
  'SELECT 1'
);
PREPARE add_recite_paper FROM @add_recite_paper;
EXECUTE add_recite_paper;
DEALLOCATE PREPARE add_recite_paper;
