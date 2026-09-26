import sys
import types
import unittest
from datetime import date

sys.modules.setdefault("pymysql", types.ModuleType("pymysql"))

from zk_sync import content_hash, logical_date, resolve_weekly, slots, source_owned, target_key, task_key


class ZKSyncPlanningTests(unittest.TestCase):
    def test_legacy_record_expands_only_completed_components(self):
        record = {"daily": "done", "book": None, "video": "done", "kind": "reflection"}
        self.assertEqual(slots(record), [
            ("daily", "daily_devotion"),
            ("video", "weekly_video"),
            ("reflection", "reflection"),
        ])

    def test_missing_date_is_not_inferred_from_submission_time(self):
        self.assertIsNone(logical_date({"checkin_time": "2026-09-23T01:00:00Z"}))
        self.assertEqual(logical_date({"logical_date": "2026-09-22"}), date(2026, 9, 22))

    def test_weekly_aggregate_and_specific_task_keep_original_semantics(self):
        weeks = [{"id": 8, "start_date": date(2026, 9, 21), "end_date": date(2026, 9, 27)}]
        tasks = {8: [
            {"id": 20, "task_type": "weekly_checkin", "title": "周任务"},
            {"id": 21, "task_type": "weekly_video", "title": "第一课"},
            {"id": 22, "task_type": "weekly_video", "title": "第二课"},
        ]}
        self.assertEqual(resolve_weekly("weekly_book", {"detail": "读物"}, date(2026, 9, 23), weeks, tasks),
                         ("weekly_checkin", 8, 20, False))
        self.assertEqual(resolve_weekly("weekly_video", {"detail": "第二课"}, date(2026, 9, 23), weeks, tasks),
                         ("weekly_video", 8, 22, False))
        self.assertEqual(resolve_weekly("weekly_video", {"detail": "未知"}, date(2026, 9, 23), weeks, tasks),
                         ("weekly_video", 8, None, True))

    def test_source_edit_changes_fingerprint_without_changing_destination_key(self):
        original = {"id": 12, "name": "测试成员", "logical_date": "2026-09-22", "daily": "done", "note": "旧"}
        changed = {**original, "note": "新"}
        self.assertNotEqual(content_hash(original, "daily"), content_hash(changed, "daily"))
        target = {"user_id": 3, "task_type": "daily_devotion", "logical_date": date(2026, 9, 22),
                  "part": "", "task_id": None}
        self.assertEqual(target_key(target), (3, "daily_devotion", date(2026, 9, 22), ""))
        self.assertIsNone(task_key(target))

    def test_only_zk_originated_rows_are_owned_for_delete_propagation(self):
        self.assertTrue(source_owned("zk_sync"))
        self.assertTrue(source_owned("json_migration"))
        self.assertFalse(source_owned("web"))


if __name__ == "__main__":
    unittest.main()
