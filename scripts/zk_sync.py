"""One-way, opt-in sync of the legacy zk check-in file into Cedar's zk group."""

import hashlib
import json
import os
import signal
import sys
import time
from datetime import date, datetime, timedelta, timezone
from pathlib import Path

import pymysql


SOURCE = Path(os.environ.get("ZK_RECORDS_PATH", "/data/zk/records.json"))
REPORT = Path(os.environ.get("ZK_SYNC_REPORT", "/data/zk-sync/skipped.json"))
ZONE = timezone(timedelta(hours=8))
RUNNING = True
SLOTS = {
    "daily": "daily_devotion",
    "book": "weekly_book",
    "video": "weekly_video",
    "verse": "weekly_verse",
}


def stop(_signum, _frame):
    global RUNNING
    RUNNING = False


def done(value):
    return str(value or "").strip().lower() in ("done", "completed", "已完成")


def slots(record):
    found = [(slot, task_type) for slot, task_type in SLOTS.items() if done(record.get(slot))]
    if record.get("kind") in ("reflection", "recite_exam"):
        found.append((record["kind"], record["kind"]))
    return found


def logical_date(record):
    value = str(record.get("logical_date") or "")
    try:
        parsed = date.fromisoformat(value)
    except ValueError:
        return None
    return parsed if parsed <= datetime.now(ZONE).date() else None


def checkin_time(record):
    value = str(record.get("checkin_time") or "").strip()
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=ZONE)
    return parsed.astimezone(timezone.utc).replace(tzinfo=None)


def content_hash(record, slot):
    fields = {key: record.get(key) for key in (
        "id", "name", "logical_date", "checkin_time", "is_retro",
        "daily", "book", "video", "verse", "kind", "detail", "note", "part",
    )}
    fields["slot"] = slot
    return hashlib.sha256(json.dumps(fields, ensure_ascii=False, sort_keys=True, default=str).encode()).hexdigest()


def resolve_weekly(task_type, record, day, weeks, tasks):
    if not task_type.startswith("weekly_"):
        return task_type, None, None, False
    week = next((item for item in weeks if item["start_date"] <= day <= item["end_date"]), None)
    if not week:
        return task_type, None, None, True
    candidates = tasks.get(week["id"], [])
    if task_type == "weekly_book":
        aggregate = [item for item in candidates if item["task_type"] == "weekly_checkin"]
        if len(aggregate) == 1:
            return "weekly_checkin", week["id"], aggregate[0]["id"], False
    matching = [item for item in candidates if item["task_type"] == task_type]
    title = str(record.get("part") or record.get("detail") or "").strip()
    titled = [item for item in matching if item["title"].strip() == title]
    chosen = titled if len(titled) == 1 else matching if len(matching) == 1 else []
    return task_type, week["id"], chosen[0]["id"] if chosen else None, not bool(chosen)


def target_key(row):
    return (row["user_id"], row["task_type"], row["logical_date"], row["part"])


def task_key(row):
    if row["task_id"] and row["task_type"].startswith("weekly_"):
        return (row["user_id"], row["task_type"], row["task_id"])
    return None


def source_owned(source):
    return source in ("zk_sync", "json_migration")


def connect():
    return pymysql.connect(
        host=os.environ.get("MYSQL_HOST", "mysql"),
        user=os.environ["MYSQL_USER"],
        password=os.environ["MYSQL_PASSWORD"],
        database=os.environ["MYSQL_DATABASE"],
        charset="utf8mb4",
        cursorclass=pymysql.cursors.DictCursor,
        autocommit=False,
        connect_timeout=10,
    )


def ensure_table(cursor, dry_run=False):
    if dry_run:
        cursor.execute("SHOW TABLES LIKE 'zk_checkin_sync'")
        if cursor.fetchone():
            return
    cursor.execute("""
        CREATE {temporary} TABLE IF NOT EXISTS zk_checkin_sync (
            source_id BIGINT NOT NULL,
            source_slot VARCHAR(32) NOT NULL,
            target_record_id BIGINT UNSIGNED NOT NULL,
            content_hash CHAR(64) NOT NULL,
            owned TINYINT NOT NULL DEFAULT 0,
            PRIMARY KEY (source_id, source_slot),
            KEY idx_zk_sync_target (target_record_id)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
    """.format(temporary="TEMPORARY" if dry_run else ""))


def load_context(cursor):
    cursor.execute("SELECT id FROM study_groups WHERE code='zk' AND status=1")
    group = cursor.fetchone()
    if not group:
        raise RuntimeError("zk study group is missing")
    group_id = group["id"]
    cursor.execute("""
        SELECT gm.user_id, gm.member_name FROM group_members gm
        JOIN users u ON u.id=gm.user_id
        WHERE gm.group_id=%s AND gm.status=1 AND u.status=1
    """, (group_id,))
    members = {}
    for row in cursor.fetchall():
        name = row["member_name"].strip()
        members.setdefault(name, set()).add(row["user_id"])
    cursor.execute("SELECT id,start_date,end_date FROM study_weeks WHERE group_id=%s ORDER BY start_date DESC", (group_id,))
    weeks = cursor.fetchall()
    cursor.execute("SELECT id,week_id,task_type,title FROM study_tasks WHERE group_id=%s AND enabled=1", (group_id,))
    tasks = {}
    for row in cursor.fetchall():
        tasks.setdefault(row["week_id"], []).append(row)
    return group_id, members, weeks, tasks


def sync_records(connection, records, dry_run=False):
    report = {"source_records": len(records), "inserted": 0, "adopted": 0,
              "unchanged": 0, "deleted": 0, "unbound_weekly": 0,
              "inserted_by_type": {}, "adopted_by_type": {}, "unbound_by_type": {}, "skipped": []}
    def increment(field, task_type):
        report[field][task_type] = report[field].get(task_type, 0) + 1
    with connection.cursor() as cursor:
        ensure_table(cursor, dry_run)
        group_id, members, weeks, tasks = load_context(cursor)
        cursor.execute("SELECT source_id,source_slot,target_record_id,content_hash,owned FROM zk_checkin_sync")
        mappings = {(row["source_id"], row["source_slot"]): row for row in cursor.fetchall()}
        cursor.execute("""
            SELECT id,user_id,task_id,logical_date,task_type,part,source
            FROM checkin_records WHERE group_id=%s AND deleted_at IS NULL
        """, (group_id,))
        active = {row["id"]: row for row in cursor.fetchall()}
        by_unique = {target_key(row): row for row in active.values()}
        by_task = {task_key(row): row for row in active.values() if task_key(row)}
        present_keys = {(record.get("id"), slot) for record in records if isinstance(record, dict)
                        for slot, _ in slots(record)}
        seen = set()

        def deactivate(row):
            cursor.execute("""
                UPDATE checkin_records SET deleted_at=UTC_TIMESTAMP(3),active_key=id,updated_at=UTC_TIMESTAMP(3)
                WHERE id=%s AND group_id=%s AND source IN ('zk_sync','json_migration') AND deleted_at IS NULL
            """, (row["id"], group_id))
            active.pop(row["id"], None)
            if by_unique.get(target_key(row), {}).get("id") == row["id"]:
                by_unique.pop(target_key(row), None)
            key = task_key(row)
            if key and by_task.get(key, {}).get("id") == row["id"]:
                by_task.pop(key, None)

        for record in records:
            source_id = record.get("id")
            if not isinstance(source_id, int) or source_id <= 0:
                report["skipped"].append({"id": source_id, "reason": "invalid_id"})
                continue
            record_slots = slots(record)
            if not record_slots and not logical_date(record):
                report["skipped"].append({"id": source_id, "reason": "invalid_logical_date_no_completed_slot"})
            for slot, task_type in record_slots:
                key = (source_id, slot)
                if key in seen:
                    report["skipped"].append({"id": source_id, "slot": slot, "reason": "duplicate_source_id"})
                    continue
                seen.add(key)
                day = logical_date(record)
                if not day:
                    report["skipped"].append({"id": source_id, "slot": slot, "reason": "invalid_logical_date"})
                    continue
                name = str(record.get("name") or "").strip()
                user_ids = members.get(name, set())
                if len(user_ids) != 1:
                    report["skipped"].append({"id": source_id, "slot": slot, "reason": "member_not_unique"})
                    continue
                occurred = checkin_time(record)
                if not occurred:
                    report["skipped"].append({"id": source_id, "slot": slot, "reason": "invalid_checkin_time"})
                    continue
                fingerprint = content_hash(record, slot)
                mapping = mappings.get(key)
                if mapping and mapping["content_hash"] == fingerprint:
                    report["unchanged"] += 1
                    continue
                if mapping:
                    old = active.get(mapping["target_record_id"])
                    other_refs = sum(item["target_record_id"] == mapping["target_record_id"] and other != key
                                     for other, item in mappings.items() if other in present_keys)
                    if mapping["owned"] and old and not other_refs:
                        deactivate(old)
                    cursor.execute("DELETE FROM zk_checkin_sync WHERE source_id=%s AND source_slot=%s", key)
                    mappings.pop(key, None)
                resolved_type, week_id, task_id, unbound = resolve_weekly(task_type, record, day, weeks, tasks)
                if unbound:
                    report["unbound_weekly"] += 1
                    increment("unbound_by_type", resolved_type)
                part = "" if resolved_type == "weekly_checkin" else str(record.get("part") or "")[:64]
                detail = str(record.get("detail") or resolved_type)[:1024]
                probe = {"user_id": next(iter(user_ids)), "task_type": resolved_type,
                         "logical_date": day, "part": part, "task_id": task_id}
                existing = by_task.get(task_key(probe)) if task_key(probe) else None
                if not existing:
                    existing = by_unique.get(target_key(probe))
                if existing:
                    target_id, owned = existing["id"], source_owned(existing["source"])
                    report["adopted"] += 1
                    increment("adopted_by_type", resolved_type)
                else:
                    cursor.execute("""
                        INSERT INTO checkin_records
                        (group_id,user_id,task_id,week_id,logical_date,checkin_time,task_type,status,is_retro,
                         detail,note,part,source,created_by,created_at,updated_at)
                        VALUES (%s,%s,%s,%s,%s,%s,%s,'done',%s,%s,%s,%s,'zk_sync',%s,UTC_TIMESTAMP(3),UTC_TIMESTAMP(3))
                    """, (group_id, probe["user_id"], task_id, week_id, day, occurred, resolved_type,
                          str(record.get("is_retro") or "").lower() in ("yes", "true", "1", "是"),
                          detail, str(record.get("note") or ""), part, probe["user_id"]))
                    target_id, owned = cursor.lastrowid, True
                    inserted = {**probe, "id": target_id, "source": "zk_sync"}
                    active[target_id] = inserted
                    by_unique[target_key(inserted)] = inserted
                    if task_key(inserted):
                        by_task[task_key(inserted)] = inserted
                    report["inserted"] += 1
                    increment("inserted_by_type", resolved_type)
                cursor.execute("""
                    INSERT INTO zk_checkin_sync (source_id,source_slot,target_record_id,content_hash,owned)
                    VALUES (%s,%s,%s,%s,%s)
                """, (source_id, slot, target_id, fingerprint, owned))
                mappings[key] = {"target_record_id": target_id, "content_hash": fingerprint, "owned": owned}

        missing = [key for key in mappings if key not in seen]
        if len(missing) > 50 and len(missing) > len(mappings) // 10:
            report["skipped"].append({"reason": "mass_deletion_held", "count": len(missing)})
        else:
            live_targets = {item["target_record_id"] for key, item in mappings.items() if key in seen}
            for key in missing:
                mapping = mappings[key]
                old = active.get(mapping["target_record_id"])
                if mapping["owned"] and old and mapping["target_record_id"] not in live_targets:
                    deactivate(old)
                    report["deleted"] += 1
                cursor.execute("DELETE FROM zk_checkin_sync WHERE source_id=%s AND source_slot=%s", key)
    if dry_run:
        connection.rollback()
    else:
        connection.commit()
    return report


def write_report(report):
    REPORT.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    temporary = REPORT.with_suffix(".tmp")
    with open(temporary, "w", encoding="utf-8") as file:
        json.dump(report, file, ensure_ascii=False, indent=2)
        file.write("\n")
    os.chmod(temporary, 0o600)
    os.replace(temporary, REPORT)


def main():
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    previous = None
    dry_run = os.environ.get("ZK_SYNC_DRY_RUN") == "1"
    once = os.environ.get("ZK_SYNC_ONCE") == "1"
    while RUNNING:
        try:
            raw = SOURCE.read_bytes()
            digest = hashlib.sha256(raw).digest()
            if digest != previous:
                records = json.loads(raw)
                if not isinstance(records, list):
                    raise ValueError("records file is not a list")
                connection = connect()
                try:
                    report = sync_records(connection, records, dry_run)
                except Exception:
                    connection.rollback()
                    raise
                finally:
                    connection.close()
                write_report(report)
                previous = digest
                print("zk sync: inserted={inserted} adopted={adopted} deleted={deleted} skipped={skipped}".format(
                    inserted=report["inserted"], adopted=report["adopted"],
                    deleted=report["deleted"], skipped=len(report["skipped"])), flush=True)
                if once:
                    return
        except Exception as error:
            print("zk sync retry: " + type(error).__name__, flush=True)
            if once:
                sys.exit(1)
        time.sleep(2)


if __name__ == "__main__":
    main()
