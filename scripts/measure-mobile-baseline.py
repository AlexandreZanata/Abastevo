#!/usr/bin/env python3
"""MEX-03 host probes; actual Room SQL and explicitly synthetic model workloads.

Reopened means a fresh SQLite connection, not a purged OS disk cache. No Android
UI, network latency, battery, actual Go API or production PostGIS claim is made.
"""
import hashlib
import json
import math
from pathlib import Path
import platform
import re
import resource
import sqlite3
import statistics
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
ASSET = ROOT / "data/src/main/assets/ibge_municipalities.json"
SCHEMA = ROOT / "data/schemas/com.anpfuel.data.local.AnpFuelDatabase/16.json"
DAO = ROOT / "data/src/main/kotlin/com/anpfuel/data/local/dao"
PATTERN = re.compile(r'@Query\(\s*(?:"""([\s\S]*?)"""|"([^"\n]+)")\s*,?\s*\)\s*suspend fun (\w+)')


def run():
    report = {"scope": "HOST_COMPONENTS_NOT_ANDROID_ACCEPTANCE", "python": platform.python_version(),
              "sqlite": sqlite3.sqlite_version, "catalog_sha256": hashlib.sha256(ASSET.read_bytes()).hexdigest(),
              "schema_sha256": hashlib.sha256(SCHEMA.read_bytes()).hexdigest(),
              "source_head": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()}
    catalog = json.loads(ASSET.read_text())
    assert len({x["ibgeCode"] for x in catalog}) == len(catalog) and len({x["state"] for x in catalog}) == 27
    report["catalog_rows"] = len(catalog)
    queries = {}
    for name in ["MunicipalityCatalogDao", "MunicipalityFtsDao"]:
        path = DAO / (name + ".kt")
        report[name + "_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        for multiline, line, method in PATTERN.findall(path.read_text()):
            queries[method] = multiline or line
    assert {"search", "findAll", "findByStateAndMunicipality", "findDistinctStates"} <= queries.keys()
    measurements = {}
    with tempfile.TemporaryDirectory(prefix="abastevo-mex03-") as folder:
        path = Path(folder) / "synthetic.db"
        db = sqlite3.connect(path)
        entities = json.loads(SCHEMA.read_text())["database"]["entities"]
        for entity in entities:
            if entity["tableName"] not in ("municipality_catalog", "municipality_fts"):
                continue
            db.execute(entity["createSql"].replace("${TABLE_NAME}", entity["tableName"]))
            for index in entity["indices"]:
                db.execute(index["createSql"].replace("${TABLE_NAME}", entity["tableName"]))
        db.executemany("INSERT INTO municipality_catalog VALUES(?,?,?,?,?,NULL)",
                       [(x["ibgeCode"], x["ibgeCode"], x["state"], x["municipality"], x["normalizedName"]) for x in catalog])
        db.execute("INSERT INTO municipality_fts(municipality_fts) VALUES('rebuild')")
        # Model-only data: these tables do not purport to be existing app ledgers.
        db.execute("CREATE TABLE model_occurrences (source TEXT, station INTEGER, revision INTEGER, record_key INTEGER PRIMARY KEY, kind TEXT)")
        db.execute("CREATE INDEX model_scope ON model_occurrences(source,station,revision,record_key)")
        db.executemany("INSERT INTO model_occurrences VALUES('synthetic',?,1,?,'FINE_RECORD')", ((i % 1000, i) for i in range(100000)))
        db.execute("CREATE TABLE model_private_history(owner TEXT,vehicle INTEGER,day INTEGER,amount_centavos INTEGER)")
        db.execute("CREATE INDEX model_owner_month ON model_private_history(owner,day)")
        db.executemany("INSERT INTO model_private_history VALUES('synthetic-owner',?,?,100)", ((i % 3, i % 365) for i in range(20000)))
        db.commit()

        def measure(name, sql, params=()):
            plan = [r[3] for r in db.execute("EXPLAIN QUERY PLAN " + sql, params)]
            with sqlite3.connect(path) as reopened:
                start = time.perf_counter_ns()
                rows = reopened.execute(sql, params).fetchall()
                first = (time.perf_counter_ns() - start) / 1e6
            times = []
            for _ in range(31):
                start = time.perf_counter_ns()
                warm = db.execute(sql, params).fetchall()
                times.append((time.perf_counter_ns() - start) / 1e6)
                assert rows == warm
            payload = json.dumps(rows, ensure_ascii=False, separators=(",", ":")).encode()
            measurements[name] = {"reopened_ms": round(first, 4), "warm_p50_ms": round(statistics.median(times), 4),
                                  "warm_p95_ms": round(sorted(times)[math.ceil(.95 * len(times)) - 1], 4),
                                  "rows": len(rows), "json_array_bytes": len(payload), "plan": plan}
            return rows

        assert len(measure("catalog_full_scan", queries["findAll"])) == len(catalog)
        assert len(measure("states", queries["findDistinctStates"])) == 27
        assert len(measure("fts_sao", queries["search"], {"matchQuery": "SAO*", "limit": 200})) <= 200
        rows = measure("exact_sorriso", queries["findByStateAndMunicipality"], {"state": "MT", "municipality": "SORRISO"})
        assert len(rows) == 1 and rows[0][1] == "5107925"
        assert len(measure("model_occurrences_page", "SELECT record_key FROM model_occurrences WHERE source=? AND station=? AND revision=? AND record_key>? ORDER BY record_key LIMIT 100", ("synthetic", 7, 1, 0))) == 100
        assert measure("model_private_month", "SELECT count(*),sum(amount_centavos) FROM model_private_history WHERE owner=? AND day>=? AND day<?", ("synthetic-owner", 0, 31))[0][0] > 0
        report["temporary_database_bytes"] = path.stat().st_size
    report["measurements"] = measurements
    report["process_peak_rss_kib_linux"] = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    report["native_ui_network_postgis_status"] = "OWED_NOT_MEASURED_BY_HOST_PROBES"
    print(json.dumps(report, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    run()
