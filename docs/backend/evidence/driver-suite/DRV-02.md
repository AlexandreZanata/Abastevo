# DRV-02 — Legacy odometer review and financial oracle

Status: LOCAL_DONE
Validation: PASS (local; not production, store or full-suite acceptance)
Integration: INTEGRATION_PENDING
Gate: DRIVER_FOUNDATION_GATE — PASS locally (DRV-00, DRV-01, DRV-02)

Parent: d4223b5c (contract frozen before behavior: [DRIVER_ODOMETER_REVIEW_CONTRACT](../../../product/DRIVER_ODOMETER_REVIEW_CONTRACT.md)). B-BR-DRV-02/03/17/18/21.

## T01 — Classify legacy provenance safely

Real RED first (`/tmp/drv02-red.log`): two legacy refuels with 10000/10500 and no unit proof still produced a measured consumption as if trusted. GREEN: Room `body_json` carries a versioned local-only `_abastevo_odometer` object (UNVERIFIED / VERIFIED_METRES). Missing or unknown metadata fails closed. No schema, migration or SQL change; the recharge session-context column is untouched. Outgoing commands are rebuilt from `LedgerDraft.bodyJson()`, so the envelope never reaches the server (command/codec test plus Room test assert this).

Unverified refuel/charge odometers set `odometer-unit-unverified` and block measured consumption, energy efficiency and cost/km, while purchases, expenses and rows stay readable. An unverified *expense* odometer does not block consumption derived only from verified refuel anchors (dedicated test). The list shows a review entry per affected row; the monthly card shows the review message instead of a fake zero.

## T02 — Owner-reviewed repair

`LegacyOdometerReview` previews AS_METRES / AS_KILOMETRES with the dataset's existing bounds (charge keeps its stricter ceiling; overflow refused) and fingerprints the canonical body (SHA-256). Commit is a parameterized SQLite compare-and-set on owner, origin, dataset, record_id, revision, previous body and draft_id. Receipt keeps original number, base hash, revision, choice and result hash — no old note text. Identical retry converges; a different choice, edit after preview, foreign owner/origin or deleted row is refused without overwriting. The editor locks the odometer field and refuses save until an explicit choice is made; editing a note never attests a legacy distance. A queued body that no longer equals the corrected local draft is shown as CONFLICT and the dispatcher refuses to send it (marked `ledger.local-record-changed`). No blanket multiplication; magnitude/date never imply a unit.

Real Room on owned emulator `emulator-5568` (AVD codex-driver-foundation, API 35, 16 KB pages): `LegacyOdometerRoomTest` 6/6 on the initial and the final APK — review with restart and exact receipt, concurrent identical reviews, concurrent different choices (one winner, one conflict), late edit survives, cancellation then lost-acknowledgement replay, fresh proof persists and legacy rows are not mass converted. Logs: `/tmp/drv02-room-device.log`, `/tmp/drv02-room-final.log`.

Real Compose on the same emulator: `DriverFoundationComposeTest` 5/5 — rendered 12,5 km/L and R$ 0,52/km for verified anchors; with unverified units the review message replaces ratios while the R$ 260,00 purchase remains; two previews, no callback before an explicit click, failure message. Synthetic identified rows only. Logs: `/tmp/drv02-compose-device.log`, `/tmp/drv02-compose-final2.log`, `/tmp/drv02-compose-final3.log`. Disclosure: the first run after installing fresh APKs ended with `Process crashed` (no FATAL EXCEPTION captured) twice — once alongside my own assertion bug — and the three later runs on the same build were 5/5; the cause was not isolated.

In-flight limit: an operation already sent to the server is not made atomic with a later local review; the correction is local until explicitly queued again.

## T03 — Cross-language oracle

`contracts/testdata/driver-suite/driver-unit-v1.psv` (19 rows) executes in Go (`TestDriverUnitVectors`, client-only operations listed explicitly and counted) and Kotlin (`DriverUnitVectorsTest`, plus BRL display in `LedgerValueFormatterTest`). `planned-financial-v1.json` is unchanged; Go asserts V02–V05 still match it. Mutating a Go expectation made the test fail. Original `financial-v1` Go/Kotlin corpora run unchanged and pass. Earnings/time/GPS vectors V06–V20 remain PLANNED until their phases.

## Verification

Final matrix (`/tmp/drv02-final-matrix.log`, BUILD SUCCESSFUL, 1m49s; `:data:testDebugUnitTest` was UP-TO-DATE from an identical-input green run): `./gradlew --offline --no-daemon --max-workers=1 :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :data:assembleDebugAndroidTest :app:assembleDebugAndroidTest :app:assembleDebug :app:lintDebug :data:lintDebug` — domain 707, application 472, data 616 (4 pre-existing environment-gated skips), app 549 tests, zero failures/errors. Go: `go test ./internal/modules/metrics/... ./internal/platform/apicontract/...`, gofmt and vet clean. `git diff --check` clean; changed-surface secret/PII review found none (synthetic IDs and example.test origin only). Backend SQL/PostGIS unchanged, so no PostGIS test was invented; the changed SQL is SQLite and is proven by real Room.
