-- Restore the former weekly repetition mode without removing completion history.
UPDATE study_tasks SET task_type='weekly_verse'
WHERE task_type='daily_verse';

UPDATE checkin_records SET task_type='weekly_verse'
WHERE task_type='daily_verse' AND week_id IS NOT NULL;
