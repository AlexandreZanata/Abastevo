# P38-T02 end acceptance — what remains

Status: INTEGRATION_PENDING (2026-10-27) for the test-only fixes; acceptance
itself is PARTIAL / BLOCKED_DEVICE. PR #157 (base `dev`) is merged into
`codex/phase-integration-20261027`; no release, deployment or acceptance
claim; G09/P10 untouched.

## Context

- Run record: `docs/mobile/p38-t02-acceptance-20261026.md` (from `origin/dev`
  `6f8b82b5`); PROGRESS entry "P38-T02 end acceptance 2026-10-26".
- Later P38 matrix on the PR #157 tree (physical POCO, 2026-10-27):
  `docs/backend/evidence/maps/map-national-device-20261027/p38-device-matrix.txt`,
  summarised in `docs/planning/MAP_NATIONAL_FINAL_RECEIPT.md`: app 87/120
  (24 failed, 9 skipped), data 112/117, engine 9/10.

## Blockers retained in #157

- Two `PhotoReviewDeviceTest` cases time out: the owner-authorized private
  corpus (`ocr-0001.jpeg`) is not on the device (BLOCKED_MEDIA); same root
  for `RealImageOcrDeviceTest.privateCorpusPixels`.
- API 35 16k emulator aborted with `DeadSystemException` during a data
  migration test (emulator system failure, not an app stack); the two fixed
  harness tests could not be re-run there.
- App native code is ARM-only (`arm64-v8a`/`armeabi-v7a`): the API 26 x86_64
  AVD has 0 compatible devices; the ARMv7 AVD is unsupported by QEMU2.
- Physical POCO was not connected to the 2026-10-26 runner (it was used on
  2026-10-27 for the matrix above).

## Remaining work

1. Re-run the two harness fixes on a device. Acceptance:
   `BottomTabNavigationDeviceTest` and `HomeScreenTest` executed on a
   compatible device. The 2026-10-27 matrix shows bottom-tab 5/5 but
   `HomeScreenTest` 8/11 with three different failures
   (`performScrollTo`, `heroContributionSelectsAStationBeforeCapture`) —
   triage them as defects or environment, never waive.
2. Private media corpus. Acceptance: corpus pushed with `run-as` to the
   app-private directory on the test device only (never Git, per the OCR
   no-training-data rule); `PhotoReviewDeviceTest` 4/4 and
   `RealImageOcrDeviceTest` 2/2; corpus removed afterwards.
3. Admitted regional fixture. 13 route/guidance/performance cases (and the
   engine `GuidanceEngineInstrumentedTest`) assert "admitted regional
   fixture is required". Acceptance: fixture provided to the device per the
   MAP-23 hashes; those classes green or each failure triaged.
4. Environment/Compose cases (9 in app per the receipt, which counts the
   three `HomeScreenTest` rows of item 1): `TrainingConsentDeviceTest` scroll
   semantics, `TrainingConsentRecoveryDeviceTest` WorkManager `ENQUEUED`,
   `AnpScaffoldTest` keyboard overlap, `AppendixA2*` sync timeouts.
   Acceptance: each classified as product defect (fix with RED→GREEN) or
   harness/environment (fix the harness), none skipped.
5. Data module: `DatabasePocGateTest` "under 15 MB" reports 15 MB after a
   one-week import. Acceptance: owner decides whether the budget or the
   import changes; test stays meaningful.
6. Low-end/compatibility rows C1–C2 and D1–D2. Acceptance: a real low-end
   ARM device (or ARM-translation-capable emulator) and a signed release
   build on a device; results per row.
7. Manual journeys A1–A7 and novice/accessibility B1–B2. Acceptance:
   executed and recorded per row with synthetic accounts.
8. Staging rows. E1 staging TLS trust from the runner (Fortinet middlebox CA,
   P34-T01); E2 provider email/Google/Apple delivery and callbacks (OWED);
   E3 private media / 24 h live photo path on the VPS (BLOCKED_MEDIA).
   Acceptance: each row PASS or an owner-accepted retained blocker.
9. E4 P25–P33 rows: those phases remain PLANNED; select or explicitly defer
   them before the full rerun.
10. Full P38-T02 rerun once 1–9 are resolved. Acceptance: complete matrix on
    one tested revision, recorded in a new dated acceptance doc; only then may
    P38-T02 leave PARTIAL.

## Blockers and owner decisions

- Stable compatible ARM device attached to the runner (owner hardware).
- Private corpus handling and admitted regional fixture (owner-authorized,
  private runtime only).
- Provider credentials (E2) and TLS trust path (E1).
- Item 5 budget decision; E4 scope decision.

## Delivery: one branch, one PR

One branch `codex/p38-acceptance-rerun` from the updated `dev`/`main`, one PR
carrying every test/harness fix and the new acceptance record. Product
defects found in item 4 go on the same branch unless they need a different
owner (record the reason if split).
