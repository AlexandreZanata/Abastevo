# MEX-03 — Reproducible host performance baseline

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Parent: 5cb8e660. Date: 2026-10-21. Rules B-BR-MEX-19; BUC-MEX-05/09/10.

Before behavior: measure the full pinned IBGE asset, actual Room16/DAO SQLite
queries and actual Kotlin fallback ranking before optimization. Add deterministic
dense synthetic occurrence and private-history workload probes, clearly separate
from unimplemented production endpoints. No product optimization or schema change.
Record first-call/reopened connection versus warm timings, payload/allocation
components and query plans. Host timings cannot certify Android UI/scroll/network.
Native frame, keystroke-to-visible, first-content and battery/network traces stay
OWED. Freeze structural/baseline-relative acceptance before MEX-04–06 changes.
Risk: false performance claim. Run all measurements in disposable local files;
no personal/device/VPS database, network load or private fixture. Recovery: remove
benchmark artifacts only, never product data. One local checkpoint, no publication.

## Measurements and limits

[Host report](MEX-03-host.json) pins asset/Room16/DAO hashes, Python3.12.2,
SQLite3.45.1, parent5cb8e660. Actual full catalog:5,571 rows,27 states.
31 warm samples per SQL query; reopened connection is not a cold OS cache.
Full catalog scan p95=5.5413ms/343,872 JSON-array bytes; FTS SAO prefix
p95=0.6178ms/200 candidates/6,045 bytes; exact Sorriso p95=0.0203ms.
Plans show the full scan, FTS virtual index/temp sorting and state index use.
The source has a composite-expression IN lookup; it must be measured/replaced
carefully by MEX-04 rather than assumed to be a composite-key index seek.

Actual unchanged Kotlin fallback over the full asset, JDK17 Gradle test JVM:
SORRISO first83.47ms/warm p95=20.95ms; typo SORISO first8.66ms/p95=7.61ms;
SAO first19.39ms/p95=20.71ms;128-character absent query first26.34ms/p95=33.87ms.
Eleven warm samples per query; first call includes varying JIT state, not a
startup/device SLA. All result ordering/asset census assertions pass, no skips.

Discovery correction: LocationPickerScreen uses LocationPickerFilter over loaded
states/state municipalities, with no debounce/network per keystroke. Its
ViewModel loads through SelectLocationUseCase. SearchMunicipalityUseCase has no
app consumer at this head. The global Kotlin fallback is therefore a reusable
component baseline, NOT measured current picker typing latency. StationsViewModel
has a separate300ms debounce; do not attribute that delay to the city picker.
MEX-04–06 must target the actual picker and preserve its selection semantics.

Dense model probes:100,000 synthetic occurrences,20,000 private-history rows,
indexed bounded page/month reads p95=0.0381/0.3335ms. These explicitly model-only
SQLite tables are not FQ/PostGIS/private-ledger runtime acceptance. Disposable
DB8,237,056 bytes; Python process peak RSS33,016KiB includes seeding/serialization,
not APK/Room/JVM peak memory. JSON-array lengths are encoded fixture payloads,
not observed HTTP transfer. No customer data or device database is read.

## Budgets frozen before optimization

- SQL: no per-result enrichment; at most3 catalog/availability queries per ready
  global search, excluding one-time seed. Retain current FTS candidate cap200;
  fuzzy supplementation must not re-materialize5,571 rows per keystroke. Use the
  existing indexed state path for state-scoped picker loading where appropriate.
- Matching: exact/accent/UF/code and documented typo results must survive;
 128-character/adversarial input must remain bounded/cancellable. Query bounds
  and UX refusal semantics must be documented in MEX-05 before runtime changes.
- Timing: compare identical workloads on the same host/JVM/config, at least
  these sample counts; unchanged paths must not regress warm p95 without an
  explained accepted tradeoff. Target reduced full-scan/query/allocation work,
  not a promised phone millisecond result. Keep SQL, CPU, debounce and UI separate.
- Persistence/network: never clear retained useful data on refresh failure;
  valid-empty differs from failure. Source currently has15s foreground refresh
  loops, but no request/session trace was recorded here. No invented traffic,
  battery or savings result. OPT acceptance precedes cache/refresh claims.
- Native scrolling, typing-to-visible, first useful content, network bytes/SQL
  on actual Go endpoints and device heap/frame/battery measurements remain OWED
  under MEX-06/35/EE-28 and the existing deferred device matrix. Existing P35
  native evidence is historical, not a rerun of this asset/schema/UI scope.

## Actual checks

`python3 scripts/measure-mobile-baseline.py` PASS (six workloads, correctness and
plans); `./gradlew --offline :domain:test --tests com.anpfuel.domain.rule.MunicipalitySearchBaselineTest`
PASS (one actual test, zero skips/failures,19s build; compile/Jacoco included).
Python source parse, document links, scoped secrets and diff --check PASS.
No product source/dependency/migration changed; native/production not certified.
