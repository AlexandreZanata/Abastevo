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
