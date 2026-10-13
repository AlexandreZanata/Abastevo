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
