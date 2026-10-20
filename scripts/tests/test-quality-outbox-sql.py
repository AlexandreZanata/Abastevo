#!/usr/bin/env python3
"""Real SQLite acceptance of production Room SQL; native Room execution is separate."""
import concurrent.futures
import json
from pathlib import Path
import re
import sqlite3
import tempfile
import threading
import unittest

ROOT = Path(__file__).resolve().parents[2]
DAO = ROOT / 'data/src/main/kotlin/com/anpfuel/data/local/dao/QualityReportOutboxDao.kt'
MIGRATION = ROOT / 'data/src/main/kotlin/com/anpfuel/data/local/AnpFuelDatabaseMigrations.kt'
SCHEMAS = ROOT / 'data/schemas/com.anpfuel.data.local.AnpFuelDatabase'
QUERIES = dict((name, sql) for sql, name in re.findall(r'@Query\("([^"\n]+)"\)\s*abstract suspend fun (\w+)', DAO.read_text()))
UPGRADE = re.findall(r'db.execSQL\("([^"\n]+)"\)', MIGRATION.read_text().split('val MIGRATION_13_14:', 1)[1].split('val MIGRATION_15_16:', 1)[0])


def schema(db, version):
    for entity in json.loads((SCHEMAS / f'{version}.json').read_text())['database']['entities']:
        db.execute(entity['createSql'].replace('${TABLE_NAME}', entity['tableName']))
        for index in entity['indices']:
            db.execute(index['createSql'].replace('${TABLE_NAME}', entity['tableName']))
    db.execute(f'PRAGMA user_version={version}')


def upgrade(db):
    # Room owns version selection; this harness uses SQLite's real version ledger.
    for source, target, statements in [(13, 14, UPGRADE[:-1]), (14, 15, UPGRADE[-1:])]:
        if db.execute('PRAGMA user_version').fetchone()[0] == source:
            db.execute('BEGIN IMMEDIATE')
            try:
                for sql in statements:
                    db.execute(sql)
                db.execute(f'PRAGMA user_version={target}')
                db.commit()
            except BaseException:
                db.rollback()
                raise


class QualityOutboxSQL(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='quality-outbox-sql-')
        self.path = Path(self.temp.name) / 'actual.db'
        self.db = self.connect()
        schema(self.db, 13)
        self.db.execute("INSERT INTO survey_week VALUES ('synthetic-week','2026-10-01','2026-10-07',123,NULL)")
        upgrade(self.db)

    def tearDown(self):
        self.db.close()
        self.temp.cleanup()

    def connect(self):
        return sqlite3.connect(self.path, isolation_level=None, timeout=5)

    def gate(self, key='a', local='owner-a', blocked=0, last=100, expiry=10000):
        self.db.execute('INSERT INTO quality_owner_gate VALUES (?,?,?,?,?,?,0)', (key, local, 'https://synthetic.invalid', blocked, last, expiry))

    def pending(self, key='a', command='command-a', expiry=9000):
        self.db.execute('INSERT INTO quality_report_outbox VALUES (?,?,?,?,?,?,?,?,?,?,?)', (key, command, 'a'*64, 'synthetic-ciphertext', 100, expiry, 'QUEUED', 0, 100, '', 0))

    def q(self, name, **params):
        return self.db.execute(QUERIES[name], params)

    def test_upgrade_matches_exported_fresh_schema_preserves_prior_data_and_replays(self):
        self.assertEqual(5, len(UPGRADE))
        self.assertEqual(('synthetic-week',), self.db.execute('SELECT id FROM survey_week').fetchone())
        fresh = sqlite3.connect(':memory:')
        try:
            schema(fresh, 15)
            for entity in json.loads((SCHEMAS / '15.json').read_text())['database']['entities']:
                name = entity['tableName']
                self.assertEqual(fresh.execute(f'PRAGMA table_info(`{name}`)').fetchall(), self.db.execute(f'PRAGMA table_info(`{name}`)').fetchall(), name)
                self.assertEqual(fresh.execute(f'PRAGMA index_list(`{name}`)').fetchall(), self.db.execute(f'PRAGMA index_list(`{name}`)').fetchall(), name)
            upgrade(self.db)
            self.assertEqual(15, self.db.execute('PRAGMA user_version').fetchone()[0])
            self.assertEqual(0, self.q('pendingCount').fetchone()[0])
        finally:
            fresh.close()

    def test_schema14_upgrade_preserves_live_ciphertext_until_confirmed_key_erasure(self):
        prior = sqlite3.connect(':memory:')
        try:
            schema(prior, 14)
            prior.execute("INSERT INTO quality_owner_gate VALUES ('a','owner-a','https://synthetic.invalid',0,100,10000)")
            prior.execute("INSERT INTO quality_report_outbox VALUES ('a','command-a',?,'synthetic-ciphertext',100,9000,'QUEUED',0,100,'',0)", ('a'*64,))
            prior.execute(UPGRADE[-1])
            self.assertEqual(0, prior.execute('SELECT key_erased FROM quality_owner_gate').fetchone()[0])
            self.assertEqual('synthetic-ciphertext', prior.execute('SELECT sealed_command FROM quality_report_outbox').fetchone()[0])
            self.assertEqual(0, prior.execute(QUERIES['keyErased'], dict(key='a', now=200)).rowcount)
            prior.execute(QUERIES['blockOwner'], dict(local='owner-a', now=200, until=20000))
            self.assertEqual([('a',)], prior.execute(QUERIES['keysToErase'], dict(now=201)).fetchall())
            prior.execute(QUERIES['keyErased'], dict(key='a', now=201))
            self.assertEqual([], prior.execute(QUERIES['keysToErase'], dict(now=201)).fetchall())
            self.assertEqual(0, prior.execute(QUERIES['touch'], dict(key='a', now=202)).rowcount)
        finally:
            prior.close()

    def test_twelve_way_claim_restart_and_stale_ack_are_fenced(self):
        self.gate(); self.pending()
        barrier = threading.Barrier(12)
        def claim(i):
            db = self.connect()
            try:
                barrier.wait(timeout=5)
                return db.execute(QUERIES['claimRow'], dict(key='a', command='command-a', claim=f'claim-{i}', now=200, until=300)).rowcount
            finally:
                db.close()
        with concurrent.futures.ThreadPoolExecutor(max_workers=12) as pool:
            self.assertEqual(1, sum(pool.map(claim, range(12))))
        old = self.q('pending', key='a').fetchone()
        self.assertEqual(1, old[7])
        self.assertEqual(0, self.q('claimRow', key='a', command='command-a', claim='new', now=299, until=400).rowcount)
        self.assertEqual(1, self.q('claimRow', key='a', command='command-a', claim='new', now=300, until=400).rowcount)
        self.assertEqual(0, self.q('acknowledge', key='a', command='command-a', claim=old[9], now=301).rowcount)
        self.assertEqual(0, self.q('failClaim', key='a', command='command-a', claim=old[9], now=301, retry=302).rowcount)
        self.assertEqual(1, self.q('acknowledge', key='a', command='command-a', claim='new', now=301).rowcount)

    def test_attempt_budget_clock_expiry_and_origin_owner_guards(self):
        self.gate(); self.pending()
        for now in range(101, 126):
            self.assertEqual(1, self.q('claimRow', key='a', command='command-a', claim='attempt', now=now, until=now+1).rowcount)
        self.assertEqual(25, self.q('pending', key='a').fetchone()[7])
        self.assertEqual(0, self.q('claimRow', key='a', command='command-a', claim='overflow', now=200, until=300).rowcount)
        self.assertEqual(0, self.q('claimRow', key='foreign', command='command-a', claim='foreign', now=200, until=300).rowcount)
        self.assertEqual(0, self.q('touch', key='a', now=99).rowcount)
        self.assertEqual(1, self.q('touch', key='a', now=200).rowcount)
        self.assertEqual(0, self.q('acknowledge', key='a', command='command-a', claim='attempt', now=199).rowcount)
        self.assertEqual(1, self.q('pruneExpired', now=9000).rowcount)

    def test_erasure_blocks_late_completion_and_preserves_foreign_live_intent(self):
        self.gate(); self.pending()
        self.gate('b', 'owner-b'); self.pending('b', 'command-b')
        self.assertEqual(1, self.q('claimRow', key='a', command='command-a', claim='old', now=200, until=300).rowcount)
        # These two production statements run in Room's erase transaction.
        self.db.execute('BEGIN IMMEDIATE')
        self.q('blockOwner', local='owner-a', now=150, until=20000)
        self.q('eraseRows', local='owner-a')
        self.db.commit()
        self.assertEqual(0, self.q('touch', key='a', now=200).rowcount)
        self.assertEqual(0, self.q('acknowledge', key='a', command='command-a', claim='old', now=201).rowcount)
        self.assertEqual(0, self.q('failClaim', key='a', command='command-a', claim='old', now=201, retry=250).rowcount)
        self.assertIsNone(self.q('pending', key='a').fetchone())
        self.assertIsNotNone(self.q('pending', key='b').fetchone())
        self.assertEqual(1, self.q('gate', key='a').fetchone()[3])

    def test_failed_claim_backoff_and_gate_prune_keep_pending_scope(self):
        self.gate(); self.pending()
        self.q('claimRow', key='a', command='command-a', claim='one', now=200, until=300)
        self.assertEqual(1, self.q('failClaim', key='a', command='command-a', claim='one', now=201, retry=500).rowcount)
        self.assertEqual(0, self.q('claimRow', key='a', command='command-a', claim='two', now=499, until=600).rowcount)
        self.assertEqual(1, self.q('claimRow', key='a', command='command-a', claim='two', now=500, until=600).rowcount)
        self.assertEqual(0, self.q('pruneGates', now=10000).rowcount)
        self.q('pruneExpired', now=10000)
        self.assertEqual(0, self.q('pruneGates', now=10000).rowcount)
        self.assertEqual(1, self.q('keyErased', key='a', now=10000).rowcount)
        self.assertEqual(1, self.q('pruneGates', now=10000).rowcount)


if __name__ == '__main__':
    unittest.main()
