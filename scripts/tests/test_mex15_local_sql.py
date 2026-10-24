"""MEX-15 actual SQLite checks; run after :data:compileDebugKotlin exports schema 25.

Uses Python standard-library SQLite only. Android/Room device execution remains
separate; SQL is loaded from the migration, DAO and generated Room schema.
"""
import json
from pathlib import Path
import re
import sqlite3
import tempfile
import threading
import unittest

ROOT = Path(__file__).resolve().parents[2]
LOCAL = ROOT / "data/src/main/kotlin/com/anpfuel/data/local"
SCHEMA = ROOT / "data/schemas/com.anpfuel.data.local.AnpFuelDatabase/25.json"


def dao_query(name):
    source = (LOCAL / "dao/PremiumOutboxDao.kt").read_text()
    for match in re.finditer(r"@Query\((.*?)\)\s*suspend fun (\w+)", source, re.S):
        if match[2] == name:
            return "".join(json.loads(piece) for piece in re.findall(r'"(?:[^"\\]|\\.)*"', match[1]))
    raise AssertionError(f"Missing production SQL: {name}")


def migration_sql():
    source = (LOCAL / "AnpFuelDatabaseMigrations.kt").read_text().split("val MIGRATION_24_25:", 1)[1]
    return re.search(r'"""(.*?)"""', source, re.S)[1]


def prior_database(path=":memory:"):
    connection = sqlite3.connect(path)
    entities = json.loads(SCHEMA.read_text())["database"]["entities"]
    for row in entities:
        if row["tableName"] in ("ledger_draft", "premium_outbox"):
            connection.execute(row["createSql"].replace("${TABLE_NAME}", row["tableName"]))
    connection.execute("INSERT INTO ledger_draft VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                       ("draft", "owner", "origin", "refuel", "record", 0, "{}", 1, None))
    connection.execute("INSERT INTO premium_outbox VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                       ("operation", "owner", "origin", "refuel", "record", 0, "{}", 0, 0, "QUEUED", 0, 0, ""))
    connection.commit()
    return connection


class LedgerLocalSqlTest(unittest.TestCase):
    def test_upgrade_and_replay_preserve_private_rows(self):
        with prior_database() as db:
            before = list(db.execute("SELECT * FROM ledger_draft"))
            commands = list(db.execute("SELECT * FROM premium_outbox"))
            db.execute(migration_sql())
            db.execute(migration_sql())
            self.assertEqual(before, list(db.execute("SELECT * FROM ledger_draft")))
            self.assertEqual(commands, list(db.execute("SELECT * FROM premium_outbox")))
            fields = list(db.execute("PRAGMA table_info(ledger_erased)"))
            self.assertEqual([1, 2, 3, 4, 0], [field[5] for field in fields])
            self.assertTrue(all(field[3] for field in fields))

    def test_erasure_first_epoch_wins_and_clear_is_partitioned(self):
        with prior_database() as db:
            db.execute(migration_sql())
            insert = "INSERT OR IGNORE INTO ledger_erased VALUES (?, ?, ?, ?, ?)"
            for owner, origin, at in (("a", "o", 1), ("a", "o", 99), ("b", "o", 2), ("a", "other", 3)):
                db.execute(insert, (owner, origin, "refuel", "r", at))
            self.assertEqual(1, db.execute("SELECT erased_at_millis FROM ledger_erased WHERE owner_account='a' AND origin='o'").fetchone()[0])
            db.execute("DELETE FROM ledger_erased WHERE owner_account=? AND origin=?", ("a", "o"))
            self.assertEqual(2, db.execute("SELECT COUNT(*) FROM ledger_erased").fetchone()[0])

    def test_failure_rolls_back_delete_and_erasure(self):
        db = prior_database()
        self.addCleanup(db.close)
        db.execute(migration_sql())
        db.commit()
        with self.assertRaises(sqlite3.IntegrityError):
            with db:
                db.execute("INSERT INTO ledger_erased VALUES (?, ?, ?, ?, ?)", ("owner", "origin", "refuel", "record", 1))
                db.execute("DELETE FROM ledger_draft")
                db.execute("INSERT INTO premium_outbox (command_id) VALUES (NULL)")
        self.assertEqual(1, db.execute("SELECT COUNT(*) FROM ledger_draft").fetchone()[0])
        self.assertEqual(0, db.execute("SELECT COUNT(*) FROM ledger_erased").fetchone()[0])
        self.assertEqual(1, db.execute("SELECT COUNT(*) FROM premium_outbox").fetchone()[0])

    def test_dataset_count_and_record_cleanup_use_exact_owner(self):
        with prior_database() as db:
            params = dict(ownerAccount="owner", origin="origin", dataset="refuel")
            self.assertEqual(1, db.execute(dao_query("countPending"), params).fetchone()[0])
            self.assertEqual(0, db.execute(dao_query("countPending"), dict(params, dataset="expense")).fetchone()[0])
            db.execute(dao_query("deleteRecord"), dict(owner="other", origin="origin", dataset="refuel", recordId="record"))
            self.assertEqual(1, db.execute("SELECT COUNT(*) FROM premium_outbox").fetchone()[0])
            db.execute(dao_query("deleteRecord"), dict(owner="owner", origin="origin", dataset="refuel", recordId="record"))
            self.assertEqual(0, db.execute("SELECT COUNT(*) FROM premium_outbox").fetchone()[0])

    def test_legacy_delete_is_held_until_ack_and_other_owners_are_isolated(self):
        with prior_database() as db:
            db.execute("UPDATE premium_outbox SET delete_flag=1, state='FAILED'")
            params = dict(owner="owner", origin="origin", dataset="refuel", recordId="record")
            self.assertEqual(1, db.execute(dao_query("hasHeldRecord"), params).fetchone()[0])
            self.assertEqual(0, db.execute(dao_query("hasHeldRecord"), dict(params, owner="other")).fetchone()[0])

    def test_resolution_transaction_fences_concurrent_worker_claim(self):
        with tempfile.TemporaryDirectory(prefix="mex15-sql-") as directory:
            path = str(Path(directory) / "ledger.db")
            db = prior_database(path)
            self.addCleanup(db.close)
            db.execute("BEGIN IMMEDIATE")
            params = dict(owner="owner", origin="origin", dataset="refuel", recordId="record")
            self.assertEqual(0, db.execute(dao_query("countActiveRecord"), params).fetchone()[0])
            entered = threading.Event()
            result = []
            errors = []

            def worker():
                try:
                    with sqlite3.connect(path, timeout=5) as other:
                        entered.set()
                        cursor = other.execute(dao_query("claim"), dict(commandId="operation", ownerAccount="owner", origin="origin", nonce="worker", leaseUntil=100))
                        result.append(cursor.rowcount)
                except Exception as error:
                    errors.append(error)

            thread = threading.Thread(target=worker)
            thread.start()
            self.assertTrue(entered.wait(5))
            db.execute(dao_query("deleteRecord"), params)
            db.commit()
            thread.join(5)
            self.assertFalse(thread.is_alive())
            self.assertEqual([], errors)
            self.assertEqual([0], result)


if __name__ == "__main__":
    unittest.main()
