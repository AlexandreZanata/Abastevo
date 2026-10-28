# DRV-02 — Legacy odometer review and financial oracle

Status: LOCAL_DONE
Validation: PASS
Scope: local; not production, store or full-suite acceptance
Integration: INTEGRATION_PENDING
Gate: DRIVER_FOUNDATION_GATE — PASS locally (DRV-00, DRV-01, DRV-02)

Parent: d4223b5c (contract frozen before behavior: [DRIVER_ODOMETER_REVIEW_CONTRACT](../../../product/DRIVER_ODOMETER_REVIEW_CONTRACT.md)). B-BR-DRV-02/03/17/18/21.

## T01 — Classify legacy provenance safely

Real RED first (`/tmp/drv02-red.log`): two legacy refuels with 10000/10500 and no unit proof still produced a measured consumption as if trusted. GREEN: Room `body_json` carries a versioned local-only `_abastevo_odometer` object (UNVERIFIED / VERIFIED_METRES). Missing or unknown metadata fails closed. No schema, migration or backend SQL change; the added parameterized SQLite CAS is proven by real Room. The recharge session-context column is untouched. Outgoing commands are rebuilt from `LedgerDraft.bodyJson()`, so the envelope never reaches the server (command/codec test plus Room test assert this).

Unverified refuel/charge odometers set `odometer-unit-unverified` and block measured consumption, energy efficiency and cost/km, while purchases, expenses and rows stay readable. An unverified *expense* odometer does not block consumption derived only from verified refuel anchors (dedicated test). The list shows a review entry per affected row; the monthly card shows the review message instead of a fake zero.

## T02 — Owner-reviewed repair

`LegacyOdometerReview` previews AS_METRES / AS_KILOMETRES with the dataset's existing bounds (charge keeps its stricter ceiling; overflow refused) and fingerprints the canonical body (SHA-256). Commit is a parameterized SQLite compare-and-set on owner, origin, dataset, record_id, revision, previous body and draft_id. Receipt keeps original number, base hash, revision, choice and result hash — no old note text. Identical retry converges; a different choice, edit after preview, foreign owner/origin or deleted row is refused without overwriting. The editor locks the odometer field and refuses save until an explicit choice is made; editing a note never attests a legacy distance. A queued body that no longer equals the corrected local draft is shown as CONFLICT and the dispatcher refuses to send it (marked `ledger.local-record-changed`). No blanket multiplication; magnitude/date never imply a unit.

Real Room on owned emulator `emulator-5568` (AVD codex-driver-foundation, API 35, 16 KB pages): `LegacyOdometerRoomTest` 6/6 on the initial and the final APK — review with restart and exact receipt, concurrent identical reviews, concurrent different choices (one winner, one conflict), late edit survives, cancellation then lost-acknowledgement replay, fresh proof persists and legacy rows are not mass converted. Logs: `/tmp/drv02-room-device.log`, `/tmp/drv02-room-final.log`.

Real Compose on the same emulator: `DriverFoundationComposeTest` 5/5 — rendered 12,5 km/L and R$ 0,52/km for verified anchors; with unverified units the review message replaces ratios while the R$ 260,00 purchase remains; two previews, no callback before an explicit click, failure message. Synthetic identified rows only. Logs: `/tmp/drv02-compose-device.log`, `/tmp/drv02-compose-final2.log`, `/tmp/drv02-compose-final3.log`. Disclosure: the first run after installing fresh APKs ended with `Process crashed` (no FATAL EXCEPTION captured) twice — once alongside my own assertion bug — and the three later runs on the same build were 5/5; the cause was not isolated.

In-flight limit: an operation already sent to the server is not made atomic with a later local review; the correction is local until explicitly queued again.

## T03 — Cross-language oracle

`contracts/testdata/driver-suite/driver-unit-v1.psv` (19 rows) executes in Go (`TestDriverUnitVectors`, client-only operations listed explicitly and counted) and Kotlin (`DriverUnitVectorsTest`, plus BRL display in `LedgerValueFormatterTest`). `planned-financial-v1.json` is unchanged; Go asserts V02–V05 still match it. Mutating a Go expectation made the test fail. Original `financial-v1` Go/Kotlin corpora run unchanged and pass. At the initial checkpoint, earnings/time/GPS vectors V06–V20 had no runtime consumer. The follow-up below adds explicit test-only earnings/time references; production earnings, shifts and GPS remain planned until their phases.

## Verification

Final matrix (`/tmp/drv02-final-matrix.log`, BUILD SUCCESSFUL, 1m49s; `:data:testDebugUnitTest` was UP-TO-DATE from an identical-input green run): `./gradlew --offline --no-daemon --max-workers=1 :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :data:assembleDebugAndroidTest :app:assembleDebugAndroidTest :app:assembleDebug :app:lintDebug :data:lintDebug` — domain 707, application 472, data 616 (4 pre-existing environment-gated skips), app 549 tests, zero failures/errors. Go: `go test ./internal/modules/metrics/... ./internal/platform/apicontract/...`, gofmt and vet clean. `git diff --check` clean; changed-surface secret/PII review found none (synthetic IDs and example.test origin only). Backend SQL/PostGIS unchanged, so no PostGIS test was invented; the changed SQL is SQLite and is proven by real Room.

## DRV-02 completion review — 2026-10-26

Reviewed source: `822b5dd2`. Manual oracle: `566c3516`. Final tested behavior:
`29f6c4fe`. This follow-up supersedes the earlier acceptance where affected;
it preserves the earlier RED/GREEN history and does not claim remote delivery.

### T01 — Provenance and malformed proof

The real JVM regression accepted proof version `1.5` as version `1` through
JSONObject numeric coercion (`/tmp/drv02-proof-red.log`, four tests, one failure).
Version 1 now requires an integer JSON value. Fractional/string/boolean values
leave the decoded financial record intact and its odometer UNVERIFIED. The
same three malformed variants pass on Android's actual JSON/Room implementation.
Fresh exact-UI entries remain VERIFIED_METRES; an old ambiguous row stays
UNVERIFIED until the owner's explicit choice supplies a reviewed receipt.
No date/value heuristic, user database inspection or mass conversion occurred.

### T02 — Review, reversal and held commands

Five RED regressions exposed transport after a suspended draft read when the
owner/login or device/dataset consent changed, and a same-body/different-revision
command (`/tmp/drv02-followup-red.log`). Final dispatch rechecks live account,
login family and consent after that read, then uses the current token; a
changed local revision/body is held as conflict. Revocation keeps the bounded
claim without sending, conflict mutation or acknowledgement. A positive test
proves matching records still send once after a same-family token refresh.

A separate RED (`/tmp/drv02-intents-red.log`, 21 tests, one failure) exposed the
inherited guard trying to parse `ledger-gap`/`ledger-reset` as stored-entry
datasets. The guard now applies to refuel/expense/charge only. Actual factory
commands for both existing intents dispatch and acknowledge normally; they do
not query a nonexistent draft row. Final dispatcher suite: 21/21 PASS.

Real Room suite expanded from six to **eight tests**, all passing on owned AVD
`codex-driver-foundation`, `emulator-5568`, API 35, 16 KB pages. Added: an
explicit subsequent owner edit restores the original number, preserves money,
context/revision and the numeric receipt, survives restart, keeps one row,
and refuses replay of the previous conversion; malformed proof cannot certify
units. Original retry, concurrent identical/different choices, late edit,
cancel/lost acknowledgement, owner/origin isolation and deletion cases remain.
Final log: `/tmp/drv02-accepted-room.log` (8/8).

Actual Compose suite passes on the final app/test APKs with network disabled:
`/tmp/drv02-accepted-compose.log` (5/5). Crash collection for this final run is
empty (`/tmp/drv02-accepted-device-crashes.log`); the earlier unexplained process
exits remain disclosed above, not reclassified as a proven fixed defect. Only
the owned emulator was used and shut down; other emulators/phone were untouched.
The previously disclosed already-in-flight server-operation limit still applies.

### T03 — Executed independent manual examples

`driver-unit-v1.psv`: Kotlin executes all 19 rows; Go executes seven existing
financial operations and explicitly identifies the 12 client-only form/review
rows. The app formatter owns BRL display. No client-only test is counted as
backend runtime acceptance.

New `driver-manual-reference-v1.psv`: **15 rows executed in both Go and Kotlin**.
Eight frozen planning examples (V06–V11, V17, V20) plus seven edges cover earned
versus inclusive net, separate payouts, recorded versus modeled result, signed
loss, exact fractional ratios, overlapping/contained/duplicate/adjacent/disjoint
time intervals, refunds and missing/incomplete denominators. Go merges spans;
Kotlin independently sweeps time boundaries. Integers/exact rationals only;
manual derivations are in the fixture README. Go also checks the eight planning
expectations. Changing V06 from 45000 to 45001 in a disposable copy fails with
that exact mismatch (`/tmp/drv02-manual-mutation.log`); canonical files were
never mutated for that proof.

These are explicitly **test-only reference consumers**, not production earning,
shift or time-union features. Their real consumers remain DRV-03 onward. The
existing unit/consumption rows still execute actual production rules.

Byte-identical to planning baseline `42250934`:

- `premium/financial-v1.json`: SHA-256 `e7e1568d441ea1da847a0645bb4dd295b838ad735aa1280b82eb1a8275c3f0b6`.
- `driver-suite/planned-financial-v1.json`: SHA-256 `629c734101de4f82c45b69ff26bbdc90c32fa458d055943b92e65693a46a06b5`.
- Original financial wire/execution CSVs also compared byte for byte unchanged.

### Final verification and boundary

- `/tmp/drv02-followup-matrix.log`: broad regression/build/lint PASS, 7m27s,
  domain 722, application 477, data 616 (four inherited environment-gated skips),
  app 549 tests. This preceded the final malformed-proof/intent fixes.
- `/tmp/drv02-final-scoped.log`: affected final checks PASS, 6m05s; full
  application 479, affected data 25 and app 30 tests, zero failures/errors/skips.
  Actual command (repository root, existing JDK/SDK, offline, one worker):

```sh
./gradlew --offline --no-daemon --max-workers=1 \
  :application:test \
  :data:testDebugUnitTest --tests '*Ledger*' \
  :app:testDebugUnitTest --tests '*Ledger*' --tests '*Monthly*' --tests '*DriverFoundation*' \
  :data:assembleDebugAndroidTest :app:assembleDebugAndroidTest \
  :app:assembleDebug :app:lintDebug :data:lintDebug
```

- Domain 722-test result includes the 15 new manual and 19 unit rows, unchanged
  executable financial corpus and existing rules; no domain source changed
  afterward. Go metrics/API contract tests and vet PASS; gofmt output asserted
  empty. The changed behavior has real Room proof rather than a fake PostGIS
  claim; no backend SQL/schema/dependency/provider change was introduced.
- Diff, fixture immutability, changed-document file links and secret-surface
  checks PASS. Only synthetic records/example.test origins in new tests.
- Maintained dev in isolated clone `/tmp/abastevo-driver-suite-planning-20261026`.
  Primary and active MAP checkout were inspected read-only and not altered.
  No push, PR, merge, wiki, VPS, provider call or release certification.

DRV-02-T01/T02/T03: LOCAL_DONE / INTEGRATION_PENDING. Foundation gate passes
locally for DRV-00–02; DRV-03–35 remain PLANNED and unauthorized in this batch.
