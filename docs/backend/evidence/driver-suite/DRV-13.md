# DRV-13 — Owner-scoped Android state and migrations

Status: LOCAL_DONE
Validation: PASS (local JVM + owned Android emulator; no production/UI claim)
Integration: INTEGRATION_PENDING
Parent: 6d665c3b. Behavior: 062a4030. Contract: [DRV-13](../../../product/DRV_13_CONTRACT.md).

T01: Room schema 29 adds seven append-only driver tables keyed by
`owner_account` + `origin`: shift, shift gap, earning, immutable earning
revision, distance, local command and draft. DAO reads are owner/origin scoped
with no unscoped query. Earning+revision+command, shift+gaps and distance+
command save in Room transactions; a duplicate revision aborts and rolls back.
Draft rows retain integer metres/centavos/seconds and no location/GPS-start
column.

T02: `MIGRATION_28_29` creates only the new tables/indexes. Real Room 28→29 on
the emulator seeded a ledger draft, a vehicle and a map saved place; all rows
survived and validation against exported schema 29 passed. A JVM SQL test
asserts no DROP/DELETE/ALTER and all seven tables/indexes. Fresh install and
close/reopen preserve driver drafts. Existing ledger odometer preview/CAS/receipt
repair remains unchanged and its `LedgerDraftStoreImplTest` passed.

T03: close/reopen returns owner A's draft with exact 12345 m, 6789 centavos and
90 seconds; owner B and guest reads return null. The driver-table `pragma_table_info`
contains no gps/latitude/longitude columns, so rehydration cannot restart GPS.
Atomic earning/shift tests run against real Room on the emulator.

## Actual validation

- `./gradlew :data:compileDebugKotlin` PASS (schema JSON 29 generated).
- `./gradlew :data:testDebugUnitTest --tests 'com.anpfuel.data.local.DriverMigration28To29UnitTest' --tests 'com.anpfuel.data.repository.LedgerDraftStoreImplTest'` PASS.
- `./gradlew :data:testDebugUnitTest :app:compileDebugKotlin` PASS.
- `ANDROID_SERIAL=emulator-5554 ./gradlew :data:connectedDebugAndroidTest`
  with the driver classes: **4/4 PASS** on owned API35 x86_64 emulator
  (1 migration + 3 persistence/isolation tests).
- `scripts/check-mobile.sh --static-only` PASS (including no background
  location tracking guard).

## Deliberate limits

No driver Compose screen, owner-session UI, GPS recording, WorkManager, network
sync, cloud backup or command outbox dispatcher. The command table is durable
local state only; dispatch is DRV-14. API26/device-matrix and the inherited
coverage gate were not re-run in this phase. No push, PR update, merge, deploy
or production certification.
