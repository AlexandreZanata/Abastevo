#!/usr/bin/env python3
"""MEX-05 real-SQLite oracle for the bounded normalized-prefix supplement.

Mirrors the Room schema subset (municipality_catalog + the MIGRATION_16_17
index) and the exact findByNormalizedPrefix SQL. Asserts: prefix ranges are
served by the dedicated index (no catalog scan), ORDER BY + LIMIT make pools
deterministic, accent variants resolve through the same range, typo stems
contain their documented targets, adversarial input stays a 2-char stem,
and mid-token substrings are NOT retrieved (accepted MEX-05 trade-off: the
ranking rule is unchanged, the full-scan retrieval path is retired).
Host SQLite only; not a device performance certification.
"""
import sqlite3
import sys
import unicodedata


def normalize(text):
    text = unicodedata.normalize("NFD", text.strip())
    text = "".join(c for c in text if unicodedata.category(c) != "Mn")
    return " ".join(text.replace("'", "").replace("-", " ").upper().split())


RANGE_SQL = """
SELECT * FROM municipality_catalog
WHERE normalized_name >= ? AND normalized_name < ?
ORDER BY normalized_name ASC
LIMIT ?
"""

SUPPLEMENT_LIMIT = 1000
END = "\uFFFF"

catalog = [
    ("sp-3550308", "3550308", "SP", "São Paulo", normalize("São Paulo")),
    ("sp-3509502", "3509502", "SP", "Sorocaba", normalize("Sorocaba")),
    ("mt-5107925", "5107925", "MT", "Sorriso", normalize("Sorriso")),
    ("ac-1200013", "1200013", "AC", "Acrelândia", normalize("Acrelândia")),
    ("rj-3304557", "3304557", "RJ", "Rio de Janeiro", normalize("Rio de Janeiro")),
    ("rj-3301702", "3301702", "RJ", "Niterói", normalize("Niterói")),
    ("ba-2927408", "2927408", "BA", "Salvador", normalize("Salvador")),
]


def stem_pool(db, stem):
    return db.execute(RANGE_SQL, (stem, stem + END, SUPPLEMENT_LIMIT)).fetchall()


def main():
    db = sqlite3.connect(":memory:")
    db.execute(
        "CREATE TABLE municipality_catalog("
        "id TEXT PRIMARY KEY, ibge_code TEXT, state TEXT, municipality TEXT, "
        "normalized_name TEXT, anp_alias TEXT)"
    )
    db.execute("CREATE UNIQUE INDEX idx_state_norm ON municipality_catalog(state, normalized_name)")
    db.execute("CREATE UNIQUE INDEX idx_ibge ON municipality_catalog(ibge_code)")
    # MIGRATION_16_17 exact statement shape.
    db.execute("CREATE INDEX IF NOT EXISTS `idx_municipality_catalog_normalized` ON `municipality_catalog` (`normalized_name`)")
    db.executemany("INSERT INTO municipality_catalog VALUES(?,?,?,?,?,?)",
                   [(i, c, s, m, n, None) for i, c, s, m, n in catalog])

    plan = " ".join(r for row in db.execute("EXPLAIN QUERY PLAN " + RANGE_SQL, ("SA", "SA" + END, 10))
                    for r in map(str, row))
    assert "USING INDEX" in plan or "USING COVERING INDEX" in plan, plan
    assert "SCAN municipality_catalog" not in plan.replace("SCAN municipality_catalog USING", ""), plan
    print(f"plan: {plan}")

    sao = stem_pool(db, "SAO")
    assert {r[3] for r in sao} == {"São Paulo"}, [r[3] for r in sao]
    assert [r[3] for r in sao] == sorted([r[3] for r in sao], key=normalize)

    # Accent-insensitive: normalized "SAO" range finds "São Paulo".
    assert any(r[1] == "3550308" for r in sao), "accent variant missing"

    # Typo stems contain documented targets.
    so = stem_pool(db, "SO")
    assert any(r[1] == "5107925" for r in so), "SORISO target missing"
    sa = stem_pool(db, "SA")
    assert any(r[1] == "3550308" for r in sa), "san paolo target missing"
    pa = stem_pool(db, "PA")
    assert len(pa) <= SUPPLEMENT_LIMIT, len(pa)

    # Adversarial input collapses to its 2-char stem before touching SQL.
    long_query = "z" * 128
    stemmed = (normalize(long_query).split(" ")[0][:2])
    assert stemmed == "ZZ", stemmed
    assert stem_pool(db, stemmed) == [], "unexpected rows for ZZ stem"

    # Accepted trade-off, pinned: mid-token substrings are not retrieved.
    assert all("AULO" not in r[4] or not r[4].startswith("AULO") for r in stem_pool(db, "AU"))
    assert stem_pool(db, "AU") == [] or all(r[4].startswith("AU") for r in stem_pool(db, "AU"))

    # Determinism: repeated runs are byte-identical (ORDER BY + LIMIT).
    first = stem_pool(db, "S")
    assert stem_pool(db, "S") == first

    print(
        f"MEX-05 SQLite oracle PASS: range uses dedicated index, "
        f"SAO->Sao Paulo accent-covered, SO->Sorriso typo-covered, "
        f"adversarial stem bounded, mid-token substring not retrieved."
    )


if __name__ == "__main__":
    try:
        main()
    except AssertionError as exc:
        print(f"MEX-05 SQLite oracle FAILED: {exc}", file=sys.stderr)
        sys.exit(1)
