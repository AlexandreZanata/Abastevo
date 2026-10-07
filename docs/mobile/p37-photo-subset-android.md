# P37-PC04B transactional review and signed subset outbox (Android)

Status: LOCAL_DONE / INTEGRATION_PENDING
Parent: 79005fe on maintained dev. Backend slice PC04A remains as committed.

B-BR-PC04/PC06 and BUC-PC03: only human-confirmed supported rows enqueue as one
atomic review batch. Every row is validated before the atomic enqueue; one
photo/capture/station/time, STANDARD condition, live receipt, distinct row and
fuel ids, fixed owner/origin scope. Disabled never reports queued. Double tap,
edit during save, process death and retry cannot duplicate. Retake remains
possible; inputs and crop freeze after queued. Queued photo ids are protected
on retake. Legacy single submit path stays separate and is not used by the
review screen.

Room 9 adds receipt fields and durable photo_upload_sessions. Freeze is
transactional and immutable, atomic 5-minute lease claim, persisted
receipt/status, 30s poll, fail by revision plus nonce. ACKED with receipt is
valid. Cancel only clears QUEUED/FAILED rows without receipt and never deletes
shared media. Scoped enqueue is frozen; unscoped legacy rows stay quarantined.
Schema 9 is generated; migration 8->9 preserves legacy payload without scope
and without data loss.

Signed transport requires scope, canonical wire/money/fuel mapping, durable
sessions, short HTTPS presigned PUT without auth/cookies/redirect/retry,
complete/READY before observations, no /bytes fallback, no unsigned POST, no
expired-photo metadata fallback. READY evidence persists before observation;
lost ACK recovers without reupload. RECEIVED only means pending. Signed GET
checks state with exact schema. Dispatch is pure: atomic lease, pending
receipt/retry, refresh instead of repost, real terminal expiry/rejection,
cancellation rethrow. Worker uses the use case. Repository schedules only
after durable commit; MainActivity resumes owned non-terminal work on STARTED.
No cross-account or cross-origin transfer. Owner/environment mismatch refuses.

## Local evidence (2026-10-07)

Final serial Gradle run passed after the post-GREEN guard additions
(signed GET, owner filtering, no implicit retries, cancel/receipt guards):
`BUILD SUCCESSFUL in 6m 52s, 268 tasks`.
Private log: `/tmp/abastevo-photo-review/pc04-android-final-build.log`.
Offline serial flags: `--offline --no-daemon --no-parallel --max-workers=1
--console=plain -Pkotlin.compiler.execution.strategy=in-process`.

Focused suites, zero failures/errors/skips:
- application: DispatchContributionUseCaseTest 4, EnqueueContributionUseCaseTest 8,
  GetOwnedContributionsUseCaseTest 3 (15 total)
- domain: ContributionStateRuleTest 9 (filter `*ContributionState*` resolves here)
- data unit: RoomContributionOutboxRepositoryTest 7,
  ContributionUploadHttpClientTest 7, PhotoProofTransportTest 1 (15 total)
- app: CaptureOcrViewModelTest 22
Total focused: 61. RED preconditions (batch/Room, unsafe unscoped transport,
missing dispatch class) were proven before GREEN; no test was weakened.

Lint debug: zero errors. Release `app-release-unsigned.apk` 4,226,245 bytes.
Release dex contains no `PhotoEvaluationActivity`, no `ocr-evaluation`, no
controlled proximity hooks; the only `evaluation` hit is the unrelated
`anp_price_drop_evaluation` key. `git diff --check` passed.
`scripts/scan-secrets.sh` passed.

POCO 2311DRK48G / API 36 (owner-selected serial, not recorded in Git):
reinstalled current `data-debug-androidTest.apk` (20M, built 13:52) and ran
isolated synthetic-DB suites only, never the main app data:
`adb -s <poco> shell am instrument -w -r -e class
com.anpfuel.data.repository.PhotoReviewOutboxDeviceTest,com.anpfuel.data.local.V8ToV9PhotoOutboxMigrationTest
com.anpfuel.data.test/androidx.test.runner.AndroidJUnitRunner`
Result: `OK (3 tests)` in 0.198s — frozen batch rollback plus 8-way concurrent
claim, received receipt plus cancelled sibling across reopen with shared READY
session, and 8->9 migration preserving legacy payload unscoped.

No live photo, proof, account, GPS or price was sent. Staging TLS/profile and
private-S3 process limitations from PC04A remain unchanged and are not claimed
here. Next: PC05 versioned consent/promotion/deletion, then expanded PC06
acceptance. No merge, wiki, deployment or production certification.
