# Premium next-ten execution evidence

Date: 2026-10-13. Branch codex/premium-backend; parent 8553b7a; PR131 to main. User authorized the next ten tasks: PRM01-T07, PRM02-T01..T05 and PRM03-T01..T04. One commit per task, no merge/deploy/mobile edits. Real provider/payment module remains deferred; paid production ports stay disabled. The occupied dev checkout is excluded.

## PRM01-T07 — Opening

Rules PR02–PR05: a missed notification must not permanently prevent canonical reconciliation. Select at most 25 indexed due purchases per worker tick (maximum source batch 100), checkpoint next attempt durably, serialize owner then token locks, enqueue digest-only refresh atomically. Five-minute due interval with worker minute ticks; only configured payment ports enable it. Canonical refresh, not scheduler output, grants rights. Existing durable queue provides retries/dead work and audited replay. Pending operation token material expires at 30 days; completed command/provider receipts at 90 days. Retired ownership fences are retained separately until an explicit audited ownership-retention migration; they are not counted as exported financial history. No indefinite cloud data retention or paid activation is claimed.

Status: LOCAL_DONE / INTEGRATION_PENDING (source)
Validation: PASS
Checks: subscription unit/race plus cmd/ops and worker compilation; real PostGIS TestReconciliation*, TestTerminalRetention*, TestSubscriptionUpgrade* (2.996s); affected vet/staticcheck; sqlc vet/diff; diff/secret review. Twelve parallel ticks produce one job; failed enqueue leaves due checkpoint intact; restart recovers; deleted owners excluded; bounded cleanup removes expired pending/terminal receipts and terminal purchase material while retaining opaque retirement fence; restricted operator replay refuses anonymous/nondead work. Real provider sandbox remains PROOF_OWED. No production paid scheduler runs with nil ports. Next PRM02-T01.

## PRM02-T01 — Owned vehicle identity and three-slot model

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
RED→GREEN domain tests pin legacy fuel mapping, UUID stability, integer capacity precision, incompatible units/empty names/source requirements. Append-only 000078 plus sqlc private records; account/session/signing-key locks and an explicit live subscription transaction port. Twelve competing creates admit exactly the two remaining slots; archived records count. Real SQL covers same UUID in different owners, identical import convergence/divergence, stale revision, foreign session, missing/dirty/expired capability, precommit lapse rollback, unavailable gate and synchronous deletion bytes. Main-before-075 upgrade/replay now verifies every actual append-only migration, including 000078. No app/UUID rewrite or fourth slot.

Checks: domain unit/race; real cloud vehicle SQL/race (4.169s); subscription upgrade SQL (1.803s in initial run); affected vet/staticcheck; sqlc vet/diff; secret/diff review. Next PRM02-T02.

## PRM02-T02 — Typed favorites and portable preferences

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested candidate on 7276fca. Strict schema/version/duplicate-field allowlist tests reject credentials, permissions, Wi-Fi settings and GPS. Actual PostGIS verifies known active station selection, unknown/retired refusal, UUID ownership, repeat favorite convergence, preference singleton/revision/IDOR and no public fact creation. Explicit canonical-directory transaction port; no existing backend follows were duplicated. Unit/race and actual cloud SQL tests PASS (3.378s), affected vet/staticcheck, sqlc vet/diff, secret/diff review PASS.
Next PRM02-T03. Real payments OFF; no merge/deploy/mobile edit.

## PRM02-T03 — Bound device references and irreversible revocation

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested candidate on 6c49f0e. Random device UUIDs bind to account/session family/signing contributor/key/server origin; no hardware identifier or extra bearer secret. Explicit revocation ledger fences restored old rows; free signed revoke preserves records. Actual PostGIS tests PASS (12-way cap race admits three, repeat registration, origin crossover, foreign revoke, restored revoked UUID, deleted owner); unit/race, vet/staticcheck, sqlc vet/diff and diff/secret checks PASS.
Next PRM02-T04. Real payments OFF; no merge/deploy/mobile edit.

## PRM02-T04 — Atomic versioned cloud commands and deletion epochs

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested candidate on d69a177. Append-only 000081 adds owner epochs, consent-OFF state, monotonic change sequence, immutable bounded command receipts and erased-ID fences. Typed batch validation limits 50 operations/64 KiB/8 KiB records; 600 commands/hour and 10000 receipts. Real SQL/race PASS (2.525s): exact/divergent replay, 12-way one-effect race, full batch/receipt rollback, no false last-success, revoked device replay, disabled consent, deleted account epoch, malformed bounds. Unit/race, vet/staticcheck, sqlc vet/diff and secret/diff checks PASS. Vehicle hard-delete refuses until dependency preview exists; archive remains available. Consent activation is still PRM03-T03, paid ports remain nil.
Next PRM02-T05. Real payments OFF; no merge/deploy/mobile edit.

## PRM02-T05 — Private read APIs and free cloud privacy exits

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested candidate on 9054478. Mounted actual session-derived status/inventory, bounded records/devices/export pages, signed device/command/free-erasure routes; production paid gate remains disabled. Free erasure transaction clears record/change bytes, disables consent, fences epoch/devices and retains minimal anti-replay metadata. Real API subprocess/account-login/signature/PostGIS proofs PASS (4.372s), free export/IDOR/unknown inventory/erasure/retry and injected rollback tests PASS, actual EXPLAIN uses owner index with 10000 foreign rows. HTTP/race tests cover no-store/no ETag, exact signed body, duplicate/unknown JSON and owner/origin/epoch cursor isolation. OpenAPI/goldens PASS (zero errors; warnings retained); affected vet/staticcheck and sqlc checks PASS; secret/diff review PASS. Existing contributor export remains explicitly scoped; cloud account export registers its separate typed inventory.
Next PRM03-T01. Real payments OFF; no merge/deploy/mobile edit.

## PRM03-T01 — Incremental change feed and safe cursor lifecycle

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 5a2059a. Store.Pull with owner-global cloud_changes sequence, fixed high-water watermark, 1..100 limit and current-consent filtering; signed 15-minute owner/origin/device/schema-bound cursor carries epoch/consent/after/watermark. Session/key/device/capability revalidated in the same transaction; expired/compacted/stale epoch or consent returns resnapshot without discarding local pending edits. POST /v1/cloud/sync/pull mounted with no-store/no ETag, strict bodies and 409 resnapshot mapping. Real PostGIS PASS (21.252s full cloudsync adapters suite): RED fixed-watermark/interleaving/new-cycle/compacted case plus paginated interleaved/same-instant/version/future/reversed/limit bounds and isolation/revocation/epoch/consent-filter/owner-sequence EXPLAIN with 10000 foreign rows. Unit/race PASS cloudsync/http/apicontract/cmd/api; HTTP cursor lifecycle/isolation/expiry plus forged/other-owner/device/origin/unsigned/bad-limit cases PASS. OpenAPI CloudSyncPullRequest/CloudChange/CloudSyncPullResponse plus three golden vectors PASS. Affected vet/staticcheck clean; sqlc vet/diff clean; git diff --check clean; secret review synthetic-only. No migration (000077-000081 append-only preserved; cloud_changes pkey already indexes owner/sequence). No merge/deploy/mobile edit.
Next PRM03-T02. Real payments OFF.

## PRM03-T02 — Atomic write batches and recoverable conflicts

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop f12d5b4. Reused Apply/receipts; POST /v1/cloud/sync/push returns transient base/current/proposed conflict data with full-batch rollback and no last-arrival overwrite. Resolution is a new command at the current revision with a fresh operation ID; only independent distinct-UUID operations merge. Changed operation-ID reuse conflicts; identical replay converges. Conflict receipts keep only metadata, never deleted private content. Real PostGIS PASS (14.554s cloudsync adapters): two-device same-record stale conflict with base 1/current 2 plus preserved server value and fresh-revision resolution, independent two-record merge, preferences replay/divergent reuse, favorites delete-versus-update erased fence with nil current payload, receipt content scan, partial-batch rollback with unchanged counts/success stamp, and expired-entitlement denial. Unit/race PASS cloudsync/http/apicontract/cmd/api; HTTP push success/unsigned/unknown/duplicate/invalid-device plus denied/resnapshot mapping and detailed/erased/generic 409 bodies with no-store/no-ETag PASS. OpenAPI CloudConflict/CloudSyncPushConflictResponse plus three push goldens PASS. Affected vet/staticcheck clean; sqlc vet/diff clean; diff-check clean; secrets synthetic-only. No migration. No merge/deploy/mobile edit.
Next PRM03-T03. Real payments OFF.

## PRM03-T03 — Initial import, resnapshot and sync consent control

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 15b8dbe. Explicit POST /v1/cloud/consent per dataset with server-computed mask and revision bump; no direct mask/epoch control. Revocation stops future commands/dispatch while preserving local and held records. POST /v1/cloud/import/preview dry-runs guest batches with no writes, preserving UUIDs and the three-vehicle ceiling (create/update/delete/converge/conflict/quota/invalid/erased per operation with batch-quota simulation and identical-repeat convergence). Confirmation reuses Apply with fresh proof/versions. POST /v1/cloud/sync/snapshot returns revision-consistent full-record pages with owner/device/origin/dataset/revision/epoch/consent-bound 15-minute cursors; intervening writes require explicit restart. Real PostGIS PASS (8.552s cloudsync adapters): preview no-upload/converge/divergent-conflict/quota/fourth-slot refusal and UUID preservation, cancel-no-write/stale-epoch-resnapshot/stale-consent-denied/account-switch-unauthorized/revoke-denied-with-preserved-list/regrant/obsolete-preview-denied, and snapshot first-page/interleaved-resnapshot/explicit-restart/foreign-isolation/device-origin checks plus invalid dataset/mask guards. Unit/race PASS cloudsync/http/apicontract/cmd/api; HTTP consent/preview/snapshot success/unsigned/unknown/duplicate/device binding plus denied/resnapshot mapping and forged/crossover cursors PASS. OpenAPI CloudConsentRequest/CloudPreviewItem/CloudImportPreviewResponse/CloudSnapshotRequest/CloudSnapshotResponse plus five goldens PASS. New SetConsent sqlc query vet/diff clean; no migration (cloud_state already carries mask/revision). Affected vet/staticcheck clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM03-T04. Real payments OFF.

## PRM03-T04 — Tombstone compaction and anti-resurrection recovery

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop b205c54. Bounded indexed Compact with durable compacted-through checkpoint in one transaction; owner row lock serializes parallel workers, crash rolls back floor and deletes for retry. Floor advance forces resnapshot of stale cursors. Fences in cloud_erased_ids and cloud_device_revocations never cleaned. All writes including foundation putTyped respect erased fences after physical tombstone removal. Vehicles use archive; shared hard-delete still refused; personal erasure distinct. Restored infrastructure must replay the external erasure ledger; a table inside its own backup never proves recovery. Unknown proof never purges; missing config/outage never implies expiry. Append-only 000082 adds cloud_changes_retention_idx; sqlc CompactFloor/DeleteOldChanges/DeleteOldReceipts/AdvanceFloor vet/diff clean. Real PostGIS PASS (22.190s cloudsync adapters): offline edit after 31-day compaction with nil-change/nil-receipt cleanup and retained fences, stale After resnapshot, erased-UUID push and foundation recreation both conflicted, live vehicle preserved, crash-trigger rollback with unchanged floor plus parallel-worker convergence, erase-all account recreation with resnapshot and fenced vehicle foundation/update, and retention EXPLAIN using indexes. Unit/race PASS cloudsync/http/apicontract/cmd/api. Affected vet/staticcheck clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM03-T05. Real payments OFF.

## PRM03-T05 — Actual two-client protocol and failure acceptance

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 1d4733c. Real HTTP handlers with real Store and disposable PostGIS, synthetic session and signing maps proving backend protocol only. Device A creates, B edits while A offline, stale A push conflicts with base/current/proposed and fresh-revision resolution preserves edits. Identical operation-ID retry converges with replayed receipts. Pinned cursors hide interleaving until the next cycle. Foreign cursor and origin crossover fail closed. Revoked B dispatch denied. Nil paid ports deny pushes and pulls. Twenty-revision bounded stream pages with fixed watermark. Worker restart uses new Store and Handler instances against durable state; 31-day-aged compaction forces resnapshot with snapshot restart. Real PostGIS PASS (two-client suites plus full cloudsync adapters). Unit/race PASS cloudsync/http/apicontract/cmd/api. No new routes or migrations. Affected vet/staticcheck clean; sqlc vet/diff clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM04-T01. Real payments OFF.

## PRM04-T01 — Snapshot manifest and consistent dataset read

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop e8991e5. POST /v1/cloud/snapshots/manifest returns versioned acknowledged revision with per-dataset counts, SHA256 checksum over sorted records, enabled datasets and forbidden exclusion list; no database dump. Consistent reads reuse revision-bound Snapshot with explicit restart on intervening writes. Real PostGIS PASS (backup suites): manifest counts/checksum/datasets/excluded, pending preview unchanged, write-during resnapshot refusal with new manifest, direct-record count, unknown professional dataset refusal at the check constraint, forbidden payload scan, plus foreign/origin/lapsed isolation. Unit/race PASS cloudsync/http/apicontract/cmd/api; HTTP manifest success/unsigned/unknown/duplicate/device binding plus denied/unauthorized mapping PASS. OpenAPI CloudBackupManifestRequest/Response plus three goldens PASS. No migration (records already bounded; future professional/rule ports register before inclusion). Affected vet/staticcheck clean; sqlc vet/diff clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM04-T02. Real payments OFF.

## PRM04-T02 — Private encrypted snapshot storage and inventory

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 3f8c955. Append-only 000083 adds cloud_snapshots with PENDING, READY and FAILED states plus ready and retention indexes; sqlc Insert, Get, ListReady, MarkReady, MarkFailed and DeleteFailed vet/diff clean. Metadata-first StoreSnapshot with immutable snapshots/ keys, 10 MiB JSON bounds, SHA256 integrity readback before READY, duplicate convergence, late-writer immutability and owner-scoped inventory. Only READY is selectable. Real PostGIS PASS (snapshot storage suites): missing objects fail closed with no metadata, corrupt bytes stay FAILED with no listing or download, lost PUT retries converge, duplicate jobs converge, late writers coexist, cross-owner reads stay empty, and crash-trigger retry repairs. Unit/race PASS cloudsync/http/apicontract/cmd/api. No HTTP or OpenAPI surface; staging provider proof remains owed. Affected vet/staticcheck clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM04-T03. Real payments OFF.

## PRM04-T03 — Automatic schedule, quotas and retention jobs

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 6525f85. TickSnapshots with 24-hour cadence, 25-account bound, seven-snapshot retention and ten-megabyte bounds; consent opt-in gates ticks, lapsed or missing capability skips with no cost, owner locks serialize restarts and quota races, failures leave tracked orphans, and SnapshotStatus exposes last READY revision, unprotected changes, ready count and opt-in flag. New snapshots wait while cleanup holds the lock. Real PostGIS PASS (schedule suites): concurrent restart convergence, offline empty snapshot, no-consent and lapsed skips, storage-failure backlog with retry, seven-count retention trim, and cancelled or unbounded tick refusal. Unit/race PASS cloudsync/http/apicontract/cmd/api. No HTTP or OpenAPI surface; no migration. Affected vet/staticcheck clean; sqlc vet/diff clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM04-T04. Real payments OFF.

## PRM04-T04 — Snapshot list, compatibility and restore preview

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 9f32ffe. Owner-only ListSnapshots of READY rows plus PreviewRestore with bounded JSON decoding, canonical payload checks, erased fences and current-conflict diffing. Unknown datasets, duplicate IDs, invalid payloads and old schemas report incompatible issues; deleted and erased records count as excluded. Previews carry 15-minute expiry and current-revision binding for commit; cancellation writes nothing. New POST /v1/cloud/snapshots/list and POST /v1/cloud/restore/preview with no-store responses. Real PostGIS PASS (restore preview suites): foreign NotFound, corrupt Unavailable, oversize Invalid, old-schema incompatible, deleted excluded, malicious duplicate and credential payloads incompatible, stale preview conflicts after intervening edits, and cancelled preview with unchanged counts. Unit/race PASS cloudsync/http/apicontract/cmd/api; HTTP list and preview success, unsigned, validation and NotFound mapping PASS. OpenAPI list and preview schemas plus three goldens PASS. No migration. Affected vet/staticcheck clean; sqlc vet/diff clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM04-T05. Real payments OFF.

## PRM04-T05 — Confirmed restore commit and interruption recovery

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 0c290fd. Idempotent POST /v1/cloud/restore/commit with preview-revision binding and 15-minute expiry, live session, key, device and capability checks, explicit create-or-converge merge with conflict abort, erased and deleted exclusions, quota guards and metadata-only receipts in cloud_restores. Append-only 000084 with snapshot index; sqlc GetRestore and SaveRestore vet/diff clean. Real PostGIS PASS (restore commit suites): double confirmation replay with unchanged counts, stale and expired preview refusal, receipt metadata scan, crash-trigger rollback with retry, concurrent-edit resnapshot, revoked-device unauthorized, lapsed denied, and malicious commit conflict without writes. Unit/race PASS cloudsync/http/apicontract/cmd/api; HTTP commit success, unsigned, validation, resnapshot, denied and detailed conflict mapping PASS. OpenAPI commit request and response plus three goldens PASS. Affected vet/staticcheck clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM04-T06. Real payments OFF.

## PRM04-T06 — Snapshot erasure, expiry and infrastructure recovery proof

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 642f0c0. Erase purges snapshot and restore metadata with record fences in one transaction and deletes objects before commit; outages fail closed, old IDs and stale devices stay blocked, restored bytes remain fenced, and unconfirmed bytes stay inventoried for retry. New ListSnapshotKeys, PurgeSnapshots and PurgeRestores queries vet/diff clean; no migration. Real PostGIS PASS (erasure suites): stale-device plus old-snapshot blocking with object purge counts, retry convergence, outage inventory preservation with retry, restored-byte fencing, and idempotent erase. Unit/race PASS cloudsync/http/apicontract/cmd/api. No HTTP or OpenAPI surface; staging provider proof remains owed. Affected vet/staticcheck clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM05-T01. Real payments OFF.

## PRM05-T01 — Pure refuel and expense domain values

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 22ac5b3. New ledger domain with exact integer centavos, milli-units and metres, IANA timezones, full, partial and unknown markers, seven frozen expense categories and 280-rune notes; no station or GPS required. Stated unit prices preserve authoritative totals with explicit discrepancy flags. Unit/race PASS ledger domain (exact values, bounds, units, incomplete inputs, time, station, notes and categories). Affected vet/staticcheck clean; sqlc vet/diff clean; diff-check clean; secrets synthetic-only. No migration, DB, HTTP or OpenAPI surface. No merge/deploy/mobile edit.
Next PRM05-T02. Real payments OFF.

## PRM05-T02 — Owned ledger schema and conditional commands

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 0a2449f. Append-only 000085 adds ledger_records with period, vehicle-period and live indexes plus ledger_operations receipts; sqlc ledger package vet/diff clean. Conditional Apply with 1..50 operation and 64 KiB batch bounds, operation-ID replay, revision guards and void and delete intents; archived vehicles stay writable, foreign vehicles refused, and no fact reaches public community tables. Real PostGIS PASS (ledger suites): foreign and archived handling with replay and divergent conflict, simultaneous-edit single winner, backdated acceptance, full-batch rollback, max-value bounds, indexed period EXPLAIN and community-table isolation. Unit/race PASS ledger domain. Affected vet/staticcheck clean; diff-check clean; secrets synthetic-only. Auth rides PRM05-T04. No merge/deploy/mobile edit.
Next PRM05-T03. Real payments OFF.

## PRM05-T03 — Odometer segments, gaps and vehicle lifecycle

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop c24af7d. Append-only 000086 adds ledger_segments and ledger_gaps plus segment_no on ledger_records with per-segment odometer indexes; sqlc MaxSegmentNo, OpenSegment, MaxSegmentOdometer, DeclareGap vet/diff clean. Segment-scoped monotonic odometers allow identical readings, refuse regressions and cross-segment edits, and reopen UNKNOWN boundaries on reset or replacement. Gaps declare informational incompleteness without blocking inserts. Histories survive vehicle archive with no cascade delete; cascades ride PRM05-T05 preview and erasure. Real PostGIS PASS (odometer suites): regression refusal with identical acceptance, reset with new-segment monotonicity, cross-segment edit refusal, gap declaration with correction, archive preservation with continued writes, concurrent odometer single winner, and foreign or dangling reference refusal. Unit/race PASS ledger domain. Affected vet/staticcheck clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM05-T04. Real payments OFF.

## PRM05-T04 — Private CRUD list filter API and authorization

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 2b13093. Owner-only POST /v1/ledger/commands with batch atomicity and paid writes, POST /v1/ledger/records with HMAC cursors and vehicle, time and category filters, and free POST /v1/ledger/delete; all no-store without ETags. Free reads and deletes stay available after lapse while new writes refuse. Real PostGIS PASS (ledger suites): vehicle, time and category filters with time-ordered pagination, tombstone exclusion, unknown dataset and category refusal, plus idempotent unconditional deletes. Unit/race PASS ledger domain, http, apicontract and cmd/api. OpenAPI ledger commands, records and delete schemas plus four goldens PASS. Wired professional.write capability in cmd/api. Affected vet/staticcheck clean; sqlc vet/diff clean; diff-check clean; secrets synthetic-only. Private prices never reach public tables. No merge/deploy/mobile edit.
Next PRM05-T05. Real payments OFF.

## PRM05-T05 — Ledger sync, snapshots and privacy registration

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop 45654c8. Separate POST /v1/ledger/consent for refuel and expense uploads with server-computed mask; revoking stops uploads while cloud and held data stay intact and deletes stay free. Bulk POST /v1/ledger/erase tombstones records and clears segments and gaps with consent revoked. Deltas reuse time-bounded List, snapshots reuse verified storage, and restores reuse conditional Apply with tombstone blocking. Append-only 000087 adds ledger_consent with Ensure, Get and Set queries; sqlc vet/diff clean. Real PostGIS PASS (sync privacy suites): separate opt-in and revoke with held-data reads, two-device conflict with delete versus update fencing and replay, stale deleted restore refusal, archive preservation, lapsed paid denial with free reads, foreign reference refusal, and bulk erasure inventory. Unit/race PASS ledger domain, http, apicontract and cmd/api; HTTP consent and erase validation PASS. OpenAPI consent and erase schemas plus three goldens PASS. Affected vet/staticcheck clean; diff-check clean; secrets synthetic-only. No photo or credential inclusion. No merge/deploy/mobile edit.
Next PRM05-T06. Real payments OFF.

## PRM05-T06 — Real ledger HTTP SQL recovery and adversarial acceptance

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Tested working tree atop c146899. Real HTTP handlers with real Store and disposable PostGIS using synthetic session maps. Concurrent create and correct converge with single winner and no lost correction; identical retries return replayed receipts. Trigger-injected outages return 503 with unchanged counts and succeed after retry with new handler and store instances. Erasure empties lists, purges operations so old retries cannot falsely restore, and stays idempotent. Revoked capability denies writes while reads stay free. Month and vehicle scans page bounded with cursors and no duplicates. Unit/race PASS ledger domain, http, apicontract and cmd/api. Affected vet/staticcheck clean; sqlc vet/diff clean; diff-check clean; secrets synthetic-only. No merge/deploy/mobile edit.
Next PRM06-T01. Real payments OFF.
