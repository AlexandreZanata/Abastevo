#!/usr/bin/env python3
"""EMP-PLAT-011: actual Room-generated SQL on file-backed host SQLite.

Run :data:kspDebugKotlin first. This is SQL/persistence evidence, not native
Room/Keystore/device certification. All data is synthetic.
"""
import concurrent.futures
import json
from pathlib import Path
import re
import sqlite3
import subprocess
import sys
import tempfile
import threading
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCHEMA = ROOT / "data/schemas/com.anpfuel.data.enterprise.EnterpriseDatabase/1.json"
GENERATED = ROOT / "data/build/generated/ksp/debug/java/com/anpfuel/data/enterprise/EnterpriseDao_Impl.java"
SOURCE = ROOT / "data/src/main/kotlin/com/anpfuel/data/enterprise/EnterpriseDatabase.kt"
QUERIES = dict((name, sql) for sql, name in re.findall(
    r'@Query\("([^"\n]+)"\)\s*abstract suspend fun (\w+)', SOURCE.read_text()))
A = "7b0e2a54-0000-4000-8000-000000000001"
B = "7b0e2a54-0000-4000-8000-000000000002"
ORIGIN = "https://api.example.invalid"


def create(db, schema_path):
    schema = json.loads(schema_path.read_text())["database"]
    for entity in schema["entities"]:
        db.execute(entity["createSql"].replace("${TABLE_NAME}", entity["tableName"]))
        for index in entity["indices"]:
            db.execute(index["createSql"].replace("${TABLE_NAME}", entity["tableName"]))
    db.execute(f'PRAGMA user_version={schema["version"]}')
    db.commit()


class EnterpriseSQL(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not GENERATED.exists():
            raise RuntimeError("Run :data:kspDebugKotlin; generated Room SQL is required")
        java = GENERATED.read_text()
        cls.insert_operation = re.findall(r'return "(INSERT OR IGNORE INTO `enterprise_pending`[^"]+)";', java)[0]
        cls.insert_record = re.findall(r'return "(INSERT INTO `enterprise_records`[^"]+)";', java)[0]
        cls.update_record = re.findall(r'return "(UPDATE `enterprise_records`[^"]+)";', java)[0]
        assert "RoomDatabaseKt.withTransaction(__db" in java
        assert len(QUERIES) == 3

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="enterprise-local-sql-")
        self.path = Path(self.temp.name) / "enterprise.db"
        self.db = sqlite3.connect(self.path)
        create(self.db, SCHEMA)

    def tearDown(self):
        self.db.close()
        self.temp.cleanup()

    def q(self, name, owner=A, origin=ORIGIN, org=A, id=A, limit=50, db=None):
        return (db or self.db).execute(QUERIES[name], dict(owner=owner, origin=origin, org=org, id=id, limit=limit))

    def test_empty_and_colliding_partition_ids_survive_a_new_process(self):
        self.assertEqual([], self.q("pending").fetchall())
        contexts = [(A, ORIGIN, A), (B, ORIGIN, A), (A, "https://other.example.invalid", A), (A, ORIGIN, B)]
        for i, context in enumerate(contexts):
            self.db.execute(self.insert_record, (*context, A, bytes([i])))
            self.db.execute(self.insert_operation, (*context, A, B, bytes([i])))
        self.db.commit()
        self.db.close()
        # Independent interpreter/connection: no in-memory store or DI state survives.
        subprocess.run([sys.executable, "-c",
            "import sqlite3,sys; d=sqlite3.connect(sys.argv[1]); "
            "assert d.execute('SELECT count(*) FROM enterprise_pending').fetchone()[0]==4; "
            "assert d.execute('PRAGMA user_version').fetchone()[0]==1", str(self.path)], check=True)
        self.db = sqlite3.connect(self.path)
        for i, (owner, origin, org) in enumerate(contexts):
            self.assertEqual(bytes([i]), self.q("record", owner, origin, org).fetchone()[-1])
            pending = self.q("pending", owner, origin, org).fetchall()
            self.assertEqual(1, len(pending))
            self.assertEqual((owner, origin, org, A, B, bytes([i])), pending[0])
        self.assertEqual([], self.q("pending", owner=B, org=B).fetchall())
        self.assertIsNone(self.q("operation", id=B).fetchone())

    def test_queue_duplicate_does_not_replace_or_retag_original(self):
        row = (A, ORIGIN, A, A, B, b"synthetic-envelope")
        self.assertEqual(1, self.db.execute(self.insert_operation, row).rowcount)
        self.assertEqual(0, self.db.execute(self.insert_operation, row).rowcount)
        for changed in [row[:-1] + (b"changed",), row[:4] + (A, row[-1])]:
            self.assertEqual(0, self.db.execute(self.insert_operation, changed).rowcount)
            self.assertEqual(row, self.q("operation").fetchone())
        self.db.rollback()
        self.assertEqual([], self.q("pending").fetchall())

    def test_real_sqlite_concurrent_admission_converges_to_one_row(self):
        start = threading.Barrier(12)
        def publish(_):
            with sqlite3.connect(self.path, timeout=10) as db:
                start.wait(timeout=10)
                return db.execute(self.insert_operation, (A, ORIGIN, A, A, B, b"synthetic-envelope")).rowcount
        with concurrent.futures.ThreadPoolExecutor(max_workers=12) as pool:
            counts = list(pool.map(publish, range(12)))
        self.assertEqual(1, sum(counts))
        self.assertEqual(1, len(self.q("pending").fetchall()))

    def test_generated_record_update_cannot_cross_any_scope_component(self):
        contexts = [(A, ORIGIN, A), (B, ORIGIN, A), (A, "https://other.example.invalid", A), (A, ORIGIN, B)]
        for context in contexts:
            self.db.execute(self.insert_record, (*context, A, b"before"))
        values = (*contexts[0], A, b"after", *contexts[0], A)
        self.assertEqual(1, self.db.execute(self.update_record, values).rowcount)
        for i, (owner, origin, org) in enumerate(contexts):
            self.assertEqual(b"after" if i == 0 else b"before", self.q("record", owner, origin, org).fetchone()[-1])

    def test_existing_personal_database_is_byte_identical_and_has_no_enterprise_table(self):
        personal = Path(self.temp.name) / "personal.db"
        with sqlite3.connect(personal) as db:
            create(db, ROOT / "data/schemas/com.anpfuel.data.local.AnpFuelDatabase/37.json")
            db.execute("INSERT INTO survey_week VALUES('synthetic-week','2026-11-01','2026-11-07',1,NULL)")
        before = personal.read_bytes()
        self.db.execute(self.insert_operation, (A, ORIGIN, A, A, B, b"synthetic-envelope"))
        self.db.commit()
        self.assertEqual(before, personal.read_bytes())
        with sqlite3.connect(personal) as db:
            self.assertEqual([], db.execute("SELECT name FROM sqlite_master WHERE name LIKE 'enterprise_%'").fetchall())
            self.assertEqual(37, db.execute("PRAGMA user_version").fetchone()[0])
        enterprise_tables = self.db.execute("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").fetchall()
        self.assertEqual({("enterprise_records",), ("enterprise_pending",)}, set(enterprise_tables))


if __name__ == "__main__":
    unittest.main()
