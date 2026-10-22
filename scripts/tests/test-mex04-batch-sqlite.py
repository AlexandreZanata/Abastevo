#!/usr/bin/env python3
"""MEX-04 real-SQLite oracle for batch catalog/availability reads.

Builds a disposable SQLite database mirroring the Room schema
(municipality_catalog + average_price subset), inserts accent/case spelling
variants, unknown states and unmatched rows, then executes the exact new
batched SQL from AveragePriceDao/MunicipalityCatalogDao:

- one grouped availability query (week + ever flags together);
- one indexed state read per involved state (no per-row/per-result lookup).

Asserts resolution sets against a brute-force expectation using the same
normalization as MunicipalitySearchTextNormalizer, plus exact query counts
and EXPLAIN QUERY PLAN shapes. No device, network or real user data.
Host SQLite only; not a device performance certification.
"""
import sqlite3
import sys
import unicodedata


def normalize(text):
    text = unicodedata.normalize("NFD", text.strip())
    text = "".join(c for c in text if unicodedata.category(c) != "Mn")
    text = text.replace("'", "").replace("-", " ").upper()
    return " ".join(text.split())


GROUPED_SQL = """
SELECT state AS state, municipality AS municipality,
       MAX(CASE WHEN survey_week_id = ? THEN 1 ELSE 0 END) AS hasWeekData
FROM average_price
GROUP BY state, municipality
"""

FIND_BY_STATE_SQL = "SELECT * FROM municipality_catalog WHERE state = ? ORDER BY municipality COLLATE NOCASE ASC"


def main():
    db = sqlite3.connect(":memory:")
    db.execute(
        "CREATE TABLE municipality_catalog("
        "id TEXT PRIMARY KEY, ibge_code TEXT, state TEXT, municipality TEXT, "
        "normalized_name TEXT, anp_alias TEXT)"
    )
    db.execute("CREATE UNIQUE INDEX idx_catalog_state_norm ON municipality_catalog(state, normalized_name)")
    db.execute("CREATE UNIQUE INDEX idx_catalog_ibge ON municipality_catalog(ibge_code)")
    db.execute(
        "CREATE TABLE average_price("
        "survey_week_id TEXT, state TEXT, municipality TEXT)"
    )
    catalog = [
        ("sp-3550308", "3550308", "SP", "São Paulo", normalize("São Paulo"), None),
        ("sp-3509502", "3509502", "SP", "Campinas", normalize("Campinas"), None),
        ("ac-1200013", "1200013", "AC", "Acrelândia", normalize("Acrelândia"), None),
        ("rj-3304557", "3304557", "RJ", "Rio de Janeiro", normalize("Rio de Janeiro"), None),
    ]
    db.executemany("INSERT INTO municipality_catalog VALUES(?,?,?,?,?,?)", catalog)
    week = "2026-06-07_2026-06-13"
    prices = [
        # Raw ANP spelling variants of the canonical catalog names.
        (week, "SP", "SAO PAULO"),
        (week, "SP", "campinas"),
        (week, "RJ", "Rio de Janeiro"),
        ("2026-05-31_2026-06-06", "SP", "São Paulo"),
        ("2026-05-31_2026-06-06", "AC", "ACRELANDIA"),
        # Unknown state abbreviation: must be dropped, as before.
        (week, "XX", "Nowhere"),
        # Spelling with no catalog counterpart: dropped, as before.
        (week, "SP", "Atlantida"),
    ]
    db.executemany("INSERT INTO average_price VALUES(?,?,?)", prices)

    queries = [0]

    def run(sql, params=()):
        queries[0] += 1
        return db.execute(sql, params).fetchall()

    rows = run(GROUPED_SQL, (week,))
    assert queries[0] == 1, f"grouped availability must be one query, ran {queries[0]}"
    assert len(rows) == 7, f"expected 7 distinct locations, got {len(rows)}"

    grouped_plan = " ".join(r for row in db.execute("EXPLAIN QUERY PLAN " + GROUPED_SQL, (week,)) for r in map(str, row))
    assert "SCAN" in grouped_plan and "average_price" in grouped_plan, grouped_plan

    states = sorted({row[0] for row in rows if row[0] in ("SP", "AC", "RJ")})
    index = {}
    for state in states:
        for entity in run(FIND_BY_STATE_SQL, (state,)):
            index[(entity[2], entity[4])] = (entity[3], entity[1])
    assert queries[0] == 1 + len(states), f"expected 1+{len(states)} queries, ran {queries[0]}"

    state_plan = " ".join(
        r for row in db.execute("EXPLAIN QUERY PLAN " + FIND_BY_STATE_SQL, ("SP",)) for r in map(str, row)
    )
    assert "USING INDEX" in state_plan or "USING COVERING INDEX" in state_plan, state_plan
    assert "SCAN municipality_catalog" not in state_plan.replace("SCAN municipality_catalog USING", ""), state_plan

    with_data, ever = set(), set()
    for state, raw, has_week in rows:
        key = (state, normalize(raw))
        if key not in index:
            continue
        canonical, _ = index[key]
        ever.add((state, canonical))
        if has_week:
            with_data.add((state, canonical))
    assert with_data == {("SP", "São Paulo"), ("SP", "Campinas"), ("RJ", "Rio de Janeiro")}, with_data
    assert ever == with_data | {("AC", "Acrelândia")}, ever

    # No-week variant: impossible week id keeps ever set, empties week set.
    queries[0] = 0
    rows_now = run(GROUPED_SQL, ("",))
    assert queries[0] == 1
    assert all(r[2] == 0 for r in rows_now), rows_now
    assert len(rows_now) == 7, rows_now

    print(
        f"MEX-04 SQLite oracle PASS: grouped=1 query, states={len(states)} indexed reads, "
        f"week={len(with_data)} ever={len(ever)} unmatched/unknown dropped, "
        f"catalog reads use index, no per-row lookup."
    )


if __name__ == "__main__":
    try:
        main()
    except AssertionError as exc:
        print(f"MEX-04 SQLite oracle FAILED: {exc}", file=sys.stderr)
        sys.exit(1)
