# DRV-03–07 — Isolated driver batch acceptance

Date: 2026-10-26
Status: LOCAL_DONE / INTEGRATION_PENDING
Functional validation: PASS
Global Android coverage: FAIL (also reproduced on the unmodified dev base)
Remote CI: UNVERIFIED before publication; never inferred from local results

## Scope and tested sources

The owner requested the next five phases and publication to the existing
**driver-only PR #152**, `driver-suite/drv-00-02` → `dev`.
Local construction branch: `codex/driver-suite-drv-03-07`.
Published dev base `507d915e` was normally merged in `7ed41679`; the diff against
that base contains driver work only. Main's separate landing batch is not
imported. PR #151 and foreign primary/MAP checkouts are untouched.

- [DRV-03](DRV-03.md), behavior `97b42c60`: exact earnings, cash, recorded and modeled results; matched coverage and weighted ratios.
- [DRV-04](DRV-04.md), behavior `6a66f414`: replay-safe shifts, monotonic clock gaps/recovery and multiapp online-time union.
- [DRV-05](DRV-05.md), behavior `fadfca59`: distinct distance bases, paid/empty partitions and pure GPS quality estimates.
- [DRV-06](DRV-06.md), behavior `d21e5a34`: dated vehicle cost meaning and exact centavo allocation.
- [DRV-07](DRV-07.md), behavior `5b03801d`, corrected by `93aa405a`: private constrained earnings, typed revision history/adjustments, source deduplication, tombstones and erasure epochs.

Final compatibility correction: `2b4ef76c`; both runtime tests PASS.
Additional negative-boundary tests: `9adecfc5`; precise expected-error assertions:
`7d830547`. Contracts were recorded before behavior. Evidence preserves each
RED result and its GREEN checks. [Private-data registry](../../DRIVER_PRIVATE_DATA.md)
records ownership, export, retention and future admission boundaries.

## Actual validation

- Go: all backend unit tests, driver domain race tests, vet and staticcheck PASS.
- Real disposable PostGIS: **11 tests PASS under -race**. Empty/upgrade schemas,
  actual SQL failure after DDL and recovery, concurrent migrators/importers,
  ownership, overlap, foreign revision FK, exact money, restart, stale revisions,
  cancellation after writes, authorization revocation, rollback, bounded export
  pages, source tombstones and soft/physical account erasure are exercised.
- SQLC generation diff/vet, Go formatting, mobile/security static guards,
  secret-surface review and `git diff --check` PASS. No dependency was added.
- Full JVM results: domain **846**, application **489**, data **617**, app **556**;
  zero failures/errors. Data has **4 inherited external/environment skips**;
  these are not counted as acceptance of those external paths.
- Debug app compilation, data/app instrumented APK compilation and `:app:lintDebug`
  PASS in the final matrix. `--continue` completed these independent tasks even
  though the global coverage task failed.
- Original `premium/financial-v1.json`, planned financial fixture and manual PSV
  remain byte-identical to checkpoint `842ed809`. Production Go/Kotlin functions
  execute the shared manual earnings/union examples; the expected values were
  not rewritten to match implementations.

Local logs: `/tmp/drv07-final-exact.log`, `/tmp/drv03-07-go-unit.log`,
`/tmp/drv03-07-vet.log`, `/tmp/drv03-07-staticcheck.log`,
`/tmp/drv03-07-mobile-static.log`, `/tmp/drv03-07-security-static.log`,
`/tmp/drv03-07-final-android.log`, `/tmp/drv03-07-final-models.log`, `/tmp/drv03-07-final-api26-models.log` and
`/tmp/drv03-07-error-contracts.log`. Logs are local evidence, not runtime dependencies.

## Coverage limitation — no aggregate PASS

`:domain:jacocoTestCoverageVerification` fails: **81.81%**, required **90%**.
An isolated control from pure dev `507d915e` also fails at **80.31%**. The new
`com/anpfuel/domain/driver` package is **93.48%**. Thresholds and exclusions are
unchanged. Existing uncovered code spans other fronts; this driver request does
not authorize unrelated behavior or artificial coverage inflation.

The final Android command therefore exits nonzero **only for this coverage
verification**, despite the functional/build/lint tasks passing. This is a
recorded integration blocker, not a waived requirement or a merge-ready claim.

## Android runtime regression and limits

On the owned API 35 / 16 KB x86_64 emulator, actual Room **8/8** and Compose
**5/5** tests PASS with **4096 MB** guest memory. The earlier 2048 MB attempt
passed Room but Android killed the Compose process: current-run `exit-info`
records reason **LOW_MEMORY**. Only the owned emulator RAM was increased;
APK hashes, image, test assertions and application behavior were unchanged.
The successful rerun has no app crash-buffer entries. This explains that run,
not historical unexplained crashes, and does not certify low-memory performance.

Logs: `/tmp/drv03-07-low-memory-exit-info.log`,
`/tmp/drv03-07-device-4gb-room.log`, `/tmp/drv03-07-device-4gb-compose.log`,
`/tmp/drv03-07-device-4gb-crashes.log`; identical APK hashes are in the local
`/tmp/drv03-07-device-apks.json` receipt.

On the owned API 26 emulator, both new production arithmetic/clock tests PASS
(**2/2**). The first run reproduced `NoSuchMethodError` for
`BigInteger.longValueExact()` (native availability starts after the minimum
runtime). A shared checked Long-range conversion now replaces all three calls;
no truncation or overflow waiver. JVM boundary cases include both Long limits
and the immediately adjacent rejected values. The same instrumented assertions
then passed on API 26; its emulator was shut down afterwards.
Logs: `/tmp/drv03-07-api26-runtime-red.log` and
`/tmp/drv03-07-api26-runtime-green.log`.

## Deliberate boundaries and next action

No new driver hub/UI, live GPS collector, Uber/bank integration, public HTTP
route, Premium admission or cloud synchronization was activated. Pure models
and private storage are implemented; durable shift adapters, orchestration,
reporting and mobile journeys remain in DRV-08 onward. Do not present this
checkpoint as a complete driver suite or commercial/production acceptance.

Stop at DRV-07. Publish the cumulative driver-only PR, capture one current-head
CI snapshot, and leave integration pending. No merge, deploy, VPS change, wiki
publication or release is authorized in this batch. Disposable PostGIS and
owned test emulators are stopped after evidence collection; foreign resources
are preserved.
