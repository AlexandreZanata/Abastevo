# MEX-05 — Bound typo search and retire obsolete computation

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Parent: 431b935a49acc7793614175cf402800db0690b21. Date: 2026-10-21.
Depends MEX-04 (LOCAL_DONE); existing Room/FTS schema upgraded 16→17.
Rules B-BR-MEX-07/19; BUC-MEX-05.

Before behavior: replace the per-keystroke full-catalog (`findAll`) fallback
with bounded indexed candidate selection, run approximate ranking off the
main thread over normalized queries, and keep results bounded and reusable
per call. Remove no documented matching (exact/accent/UF/code, documented
typos) and no minSdk behavior. One additive index migration; no table
rewrite, no ranking-rule change, no network/DB/producer change. Risk:
performance/search correctness. Recovery: keep the FTS fast path first;
drop back to exact/prefix retrieval by capping the supplement to zero.

## Delivered

- `findAll` is deleted from `MunicipalityCatalogDao`; nothing else used it.
  The fuzzy fallback now reads one indexed range per stem
  (`findByNormalizedPrefix`, `ORDER BY normalized_name ASC LIMIT 1000`)
  over the new `idx_municipality_catalog_normalized` (migration 16→17,
  additive `CREATE INDEX IF NOT EXISTS`, schema `17.json` exported).
- Stems derive from the first two normalized tokens (length ≥ 2): two
  characters for tokens of length ≥ 4, one character otherwise.
  Single-character tokens stay with FTS prefix matching. National pools
  measure 756 rows max (`S*`), so the 1,000-row cap keeps every stem fully
  covered with margin; a benchmark test pins this bound against catalog
  growth.
- `MunicipalityFtsMatchExpression` normalizes tokens up front (the FTS4
  simple tokenizer folds ASCII case only, so raw accented queries matched
  inconsistently); accent-aware retrieval is completed by the supplement
  pool over the folded index.
- Candidate assembly and `IntelligentSearchRankingRule.rank` run on
  `Dispatchers.Default` with cancellation points after each DAO call; a
  cancelled search raises `CancellationException` with no partial result.
  Per-call pools are independent (rapid reversal returns each query's own
  correct results); no cross-call cache was added because the catalog/FTS
  can change within the process via alias merge + reindex.
- FTS fast path unchanged: when ranked FTS matches already fill the limit,
  no supplement read happens.

Accepted, documented trade-off: mid-token substring retrieval (e.g. a query
matching only inside a token) is no longer fetched; the ranking rule still
ranks it if fed. All documented probes (exact, prefix, accent, `SORISO`,
`san paolo`, 128-char adversarial, unknown) keep their visible results.

## Actual checks

From the worktree (2026-10-21, HEAD 431b935a before this commit):

- RED: `normalizesAccentedTransformedTokensForStableMatching` failed on the
  old expression builder before the fix; GREEN after.
- `./gradlew --offline :data:testDebugUnitTest` PASS: 449 tests, 0
  failures/errors, 2 pre-existing skips. Includes 6 new supplement tests
  (exact stems/LIMIT passed to the DAO, FTS-dedupe, 128-char input reduced
  to its 2-char stem, cancellation), 4 FTS expression tests, the
  `Migration16To17UnitTest` (exactly one additive statement; no
  DROP/ALTER/DELETE/UPDATE) and the asset-backed benchmark test.
- Benchmark (host JVM, 5,571-row asset): parity of visible top-3 between
  the retired full scan and the bounded pools for SORRISO/SORISO/
  `san paolo`/SAO/`São Paulo`/CAMPINAS/SO/xyz/128×z; `SORISO`→Sorriso
  (5107925) and `san paolo`→São Paulo top-3 preserved; national max pools
  812 (1-char stems) / 654 (2-char stems) rows ≤ 1,000 cap. Host timings
  printed in-test, not asserted as device proof.
- `PYTHONDONTWRITEBYTECODE=1 python3 scripts/tests/test-mex05-prefix-sqlite.py`
  PASS on host SQLite 3.50.6: the exact range SQL uses
  `SEARCH ... USING INDEX idx_municipality_catalog_normalized`, accent
  (`SAO`→São Paulo) and typo (`SO`→Sorriso, `SA`→São Paulo) stems resolve,
  the 128-char input collapses to its `ZZ` stem, ORDER BY + LIMIT runs are
  byte-identical, and mid-token substrings are not retrieved (pinned).
- `./gradlew --offline :application:test` PASS (390, 0 failures; use-case
  behavior unchanged), `:domain:test` PASS (547, 0 failures; rule
  unchanged), `:app:testDebugUnitTest` PASS (367, 0 failures);
  `:app:assembleDebug`, `:app:assembleDebugUnitTest`, `:app:lintDebug`
  and `:data:lintDebug` PASS.
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS; no
  PII/device/VPS data in fixtures or tests.

Limits: host JVM/SQLite evidence only; Room SQL validity comes from
compile-time Room verification plus schema 17 export. Native
keystroke-to-visible timing, scroll and device execution remain OWED under
MEX-06/35 and the deferred device matrix. No backend, migration-chain
(replay verified by statement test only), FTS ranking or selection change.
Next: MEX-06 fast state/city UI.
