#!/usr/bin/env python3
"""Real SQLite acceptance of catalog/ANP production SQL; native Room is a separate gate."""
import concurrent.futures
import json
from pathlib import Path
import re
import sqlite3
import statistics
import tempfile
import threading
import time
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCHEMAS = ROOT / 'data/schemas/com.anpfuel.data.local.AnpFuelDatabase'
MIGRATIONS = (ROOT / 'data/src/main/kotlin/com/anpfuel/data/local/AnpFuelDatabaseMigrations.kt').read_text()
QUERY_PATTERN = re.compile(r'@Query\(\s*(?:"""([\s\S]*?)"""|"([^"\n]+)")\s*,?\s*\)\s*suspend fun (\w+)')
QUERIES = {}
for name in ['ServerStationCacheDao', 'StationPriceDao']:
    source = (ROOT / f'data/src/main/kotlin/com/anpfuel/data/local/dao/{name}.kt').read_text()
    for multiline, line, method in QUERY_PATTERN.findall(source):
        QUERIES[method] = multiline or line


def schema(db, version):
    for entity in json.loads((SCHEMAS / f'{version}.json').read_text())['database']['entities']:
        db.execute(entity['createSql'].replace('${TABLE_NAME}', entity['tableName']))
        for index in entity['indices']:
            db.execute(index['createSql'].replace('${TABLE_NAME}', entity['tableName']))
    db.execute(f'PRAGMA user_version={version}')


def migration(source, target):
    section = MIGRATIONS.split(f'val MIGRATION_{source}_{target}:', 1)[1].split('\n    val MIGRATION_', 1)[0]
    statements = []
    for match in re.finditer(r'db\.execSQL\(\s*(?:"""([\s\S]*?)"""\.trimIndent\(\)|"([^"\n]+)")\s*,?\s*\)', section):
        statements.append(match.group(1) or match.group(2))
    return statements


def upgrade(db):
    for source, target in [(13, 14), (14, 15), (15, 16)]:
        if db.execute('PRAGMA user_version').fetchone()[0] != source:
            continue
        db.execute('BEGIN IMMEDIATE')
        try:
            for sql in migration(source, target):
                db.execute(sql)
                if target == 16 and 'ADD COLUMN `address_search`' in sql:
                    # Same Unicode folding as the production Kotlin helper, with a Portuguese fixture oracle.
                    rows = db.execute('SELECT id, legal_name, trade_name, brand, address FROM station_price')
                    for row in rows:
                        db.execute('UPDATE station_price SET legal_name_search=?, trade_name_search=?, brand_search=?, address_search=? WHERE id=?',
                                   tuple((value or '').lower() for value in row[1:]) + (row[0],))
            db.execute(f'PRAGMA user_version={target}')
            db.commit()
        except BaseException:
            db.rollback()
            raise


def q(db, name, **params):
    return db.execute(QUERIES[name], params)


def directory_row(db, station, scope='page', position=0, name='Synthetic station'):
    db.execute('INSERT OR REPLACE INTO server_station_cache (station_id,display_name,location_quality,position,saved_at_millis,address_json,scope_key) VALUES (?,?,?,?,?,?,?)',
               (station, name, 'unknown', position, 1000, '{}', scope))


def meta(db, scope, cursor):
    db.execute('INSERT OR REPLACE INTO server_catalog_meta VALUES (?,?,?)', (scope, cursor, 1000))


def replace(db, scope, station, cursor):
    db.execute('BEGIN IMMEDIATE')
    try:
        q(db, 'clearScopePageRows', scopeKey=scope)
        directory_row(db, station, scope)
        q(db, 'clearScopeMeta', scopeKey=scope)
        meta(db, scope, cursor)
        db.commit()
    except BaseException:
        db.rollback()
        raise


class MobilePaginationSQL(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='mobile-pagination-sql-')
        self.path = Path(self.temp.name) / 'actual.db'
        self.db = sqlite3.connect(self.path, isolation_level=None, timeout=10)
        schema(self.db, 15)
        self.db.execute("INSERT INTO survey_week VALUES ('w1','2026-01-01','2026-01-07',1000,NULL)")
        self.db.execute("INSERT INTO station_price (id,survey_week_id,cnpj,address,municipality,state,fuel_product,price) VALUES ('anp1','w1','04218406000104','RUA SÃO JOÃO','SAO PAULO','SP','GASOLINE_REGULAR',5.99)")
        self.db.execute("INSERT INTO server_station_cache (station_id,display_name,location_quality,position,saved_at_millis,address_json) VALUES ('legacy','Original','unknown',0,1000,'{}'),('detail','Detail','unknown',2147483647,1000,'{}')")
        meta(self.db, 'page', 'legacy-cursor')
        self.db.execute("INSERT INTO quality_owner_gate VALUES ('owner-key','synthetic-owner','https://synthetic.invalid',0,100,10000,0)")
        upgrade(self.db)

    def tearDown(self):
        self.db.close()
        self.temp.cleanup()

    def test_previous_schema_upgrade_preserves_quality_anp_details_and_matches_fresh_schema(self):
        fresh = sqlite3.connect(':memory:')
        schema(fresh, 16)
        try:
            for entity in json.loads((SCHEMAS / '16.json').read_text())['database']['entities']:
                name = entity['tableName']
                self.assertEqual(fresh.execute(f'PRAGMA table_info(`{name}`)').fetchall(), self.db.execute(f'PRAGMA table_info(`{name}`)').fetchall(), name)
                left = sorted(fresh.execute(f'PRAGMA index_list(`{name}`)').fetchall(), key=lambda row: row[1])
                right = sorted(self.db.execute(f'PRAGMA index_list(`{name}`)').fetchall(), key=lambda row: row[1])
                self.assertEqual([(x[1:]) for x in left], [(x[1:]) for x in right], name)
            self.assertEqual([('legacy', 'page'), ('detail', '')], self.db.execute('SELECT station_id,scope_key FROM server_station_cache ORDER BY position').fetchall())
            self.assertEqual('legacy-cursor', q(self.db, 'findMeta', key='page').fetchone()[1])
            self.assertEqual('synthetic-owner', self.db.execute('SELECT owner_local FROM quality_owner_gate').fetchone()[0])
            self.assertEqual(5.99, self.db.execute("SELECT price FROM station_price WHERE id='anp1'").fetchone()[0])
            self.assertEqual('rua são joão', self.db.execute("SELECT address_search FROM station_price WHERE id='anp1'").fetchone()[0])
            upgrade(self.db)
            self.assertEqual(16, self.db.execute('PRAGMA user_version').fetchone()[0])
        finally:
            fresh.close()

    def test_upgrade_from_13_keeps_both_maintained_quality_migrations(self):
        prior = sqlite3.connect(':memory:', isolation_level=None)
        try:
            schema(prior, 13)
            upgrade(prior)
            self.assertEqual(16, prior.execute('PRAGMA user_version').fetchone()[0])
            self.assertIn('key_erased', [row[1] for row in prior.execute('PRAGMA table_info(quality_owner_gate)')])
            self.assertIn('scope_key', [row[1] for row in prior.execute('PRAGMA table_info(server_station_cache)')])
        finally:
            prior.close()

    def test_failed_page_metadata_rolls_back_rows_and_cursor_without_touching_another_city(self):
        replace(self.db, 'city:a', 'a1', 'a-next')
        replace(self.db, 'city:b', 'b1', 'b-next')
        self.db.execute("CREATE TRIGGER reject_meta BEFORE INSERT ON server_catalog_meta WHEN NEW.key='city:a' BEGIN SELECT RAISE(ABORT,'synthetic fault'); END")
        with self.assertRaises(sqlite3.DatabaseError):
            replace(self.db, 'city:a', 'replacement', 'bad')
        self.assertEqual('a1', q(self.db, 'loadScopePage', scopeKey='city:a').fetchone()[0])
        self.assertEqual('a-next', q(self.db, 'findMeta', key='city:a').fetchone()[1])
        self.assertEqual('b1', q(self.db, 'loadScopePage', scopeKey='city:b').fetchone()[0])

    def test_window_and_public_clear_preserve_namespaced_details(self):
        for index in range(125):
            directory_row(self.db, f'p{index:04}', 'origin:synthetic:city:a', index)
        directory_row(self.db, 'namespaced-detail', 'origin:synthetic:', 2147483647)
        window = q(self.db, 'loadScopePageWindow', scopeKey='origin:synthetic:city:a', limit=21, offset=20).fetchall()
        self.assertEqual(21, len(window))
        self.assertEqual('p0020', window[0][0])
        before = q(self.db, 'estimatedBytes').fetchone()[0]
        self.assertGreater(before, 60_000)
        q(self.db, 'clearPublicPageRows')
        self.assertLess(q(self.db, 'estimatedBytes').fetchone()[0], before)
        self.assertEqual({'detail', 'namespaced-detail'}, {row[0] for row in self.db.execute('SELECT station_id FROM server_station_cache')})

    def test_twelve_concurrent_appends_allocate_distinct_positions_and_reopen(self):
        barrier = threading.Barrier(12)
        def append(index):
            connection = sqlite3.connect(self.path, isolation_level=None, timeout=10)
            try:
                barrier.wait(timeout=10)
                connection.execute('BEGIN IMMEDIATE')
                position = q(connection, 'nextPosition', scopeKey='city:concurrent').fetchone()[0]
                directory_row(connection, f'c{index}', 'city:concurrent', position)
                meta(connection, 'city:concurrent', f'cursor-{index}')
                connection.commit()
            finally:
                connection.close()
        with concurrent.futures.ThreadPoolExecutor(max_workers=12) as pool:
            list(pool.map(append, range(12)))
        reopened = sqlite3.connect(self.path)
        try:
            rows = q(reopened, 'loadScopePage', scopeKey='city:concurrent').fetchall()
            self.assertEqual(12, len(rows))
            position_index = [x[1] for x in reopened.execute('PRAGMA table_info(server_station_cache)')].index('position')
            self.assertEqual(list(range(12)), [row[position_index] for row in rows])
        finally:
            reopened.close()

    def test_anp_search_unicode_literal_wildcards_and_global_stable_price_pages(self):
        params = dict(surveyWeekId='w1', state='SP', municipality='SAO PAULO', fuelProduct='GASOLINE_REGULAR', search='são', limit=20, offset=0)
        self.assertEqual('anp1', q(self.db, 'findPageByPrice', **params).fetchone()[0])
        self.assertEqual(1, q(self.db, 'countPage', **params).fetchone()[0])
        params['search'] = r'\%'
        self.assertEqual([], q(self.db, 'findPageByPrice', **params).fetchall())
        params['search'] = ''
        self.db.execute("BEGIN")
        self.db.executemany("INSERT INTO station_price (id,survey_week_id,cnpj,address,municipality,state,fuel_product,price) VALUES (?,'w1','04218406000104','RUA','SAO PAULO','SP','GASOLINE_REGULAR',5.99)", [(f'p{i:05}',) for i in range(10000)])
        self.db.commit()
        self.db.execute('ANALYZE')
        first = q(self.db, 'findPageByPrice', **params).fetchall()
        params['offset'] = 20
        second = q(self.db, 'findPageByPrice', **params).fetchall()
        self.assertEqual(20, len(first)); self.assertEqual(20, len(second))
        self.assertFalse({x[0] for x in first} & {x[0] for x in second})
        plans = [row[3] for row in self.db.execute('EXPLAIN QUERY PLAN '+QUERIES['findPageByPrice'], params)]
        self.assertTrue(any('idx_station_discovery_price' in plan for plan in plans), plans)
        self.assertFalse(any('TEMP B-TREE' in plan for plan in plans), plans)
        samples = []
        for _ in range(30):
            start = time.perf_counter()
            q(self.db, 'findPageByPrice', **params).fetchall()
            samples.append((time.perf_counter()-start)*1000)
        print(json.dumps({'fixture':'10001-price-tied-rows','query':'production ANP page 2','returned':len(second),
            'median_ms':round(statistics.median(samples),3),'p95_ms':round(sorted(samples)[28],3),'plan':plans}))


if __name__ == '__main__':
    unittest.main(verbosity=2)
