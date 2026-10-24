# MEX-33 — Private end-to-end regression and free boundary

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `c9c7d13a` (maintained dev, upgrade-test repair). Date: 2026-10-23.
Depends MEX-07–16/32; reuses PRM08/09 journeys. Risk: critical
privacy/money/data loss.

## Chains executed (real PostGIS, disposable)

- Private ledger: two-device conflict/delete/replay, consent
  separation/revoke, stale cascade/expiry/foreign owner, late charge
  correction invalidating reports, report money/erase, owner-lock
  correction serialization.
- Cloud: full backend journey, negative order/proof paths, backup
  manifest lapse/isolation, privacy campaign delete-during-activity
  and restore, free read/export/erasure/inventory with foreign
  session refusal.
- Android: MEX-08 admission, MEX-11/12/13/14/15 ledger/outbox/sync
  ViewModels and domain premium selectors; real SQLite MEX-15 script.

## Actual results

- `go test -tags=integration ./internal/modules/ledger/...
  ./internal/modules/cloudsync/...` PASS (15.6 s / 29.8 s packages).
- Targeted verbose: `TestLedgerTwoDeviceConflictDeleteReplay`,
  `TestEE16LateCorrectionInvalidates`, `TestMEX10ReportMoneyAndErase`,
  `TestLedgerConsentSeparateAndRevoke`,
  `TestJourneyFullBackend`, `TestJourneyNegatives`,
  `TestBackupManifestLapseAndIsolation`,
  `TestPrivacyCampaignDeleteDuringActivityAndRestore`,
  `TestCloudFreeReadExportErasureInventoryAndForeignSession` PASS.
- JVM selectors for sync/ledger/vehicle/premium PASS.
- `python3 scripts/tests/test_mex15_local_sql.py` — 6 tests PASS
  (actual SQLite 24→25 upgrade, private-row preservation, first
  erasure epoch, owner/dataset SQL, rollback, legacy held deletes,
  two-connection worker serialization). Schema JSON removed after use.
- Upgrade-expectation test repaired to derive every append-only
  migration ≥000109 instead of pinning 109–111; PASS with the current
  chain.

## Limits

- Two-device, signed staging and private-object receipts remain OWED.
  No production customer data, deployment or provider call.
- Native physical Room/process death remains OWED under the standing
  deferral; JVM/SQLite evidence is local source proof.
- Free boundary is proven by backend tests; no commercial activation.

Next: MEX-34 official source-to-mobile acceptance.
