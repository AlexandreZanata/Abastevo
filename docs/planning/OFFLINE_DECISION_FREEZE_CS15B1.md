# CS15-B1 — Safety-rows decision freeze (planning only)

Date: 2026-10-11. Status: PLANNING — frozen decisions for the
community-safety rows (helpfulness, consent, training, settings
UX). Unpublished by policy (planning-only work does not
publish). Each row is locally complete, durable intent pending
sync, or explicitly dependent — verified against the cited
contract, never invented. Remaining areas ride CS15-B2.

## Frozen values referenced (all verified in code)

- Quota v1: register 10/h, write 200/h, challenge 60/h
  (`identity/domain/quota.go`).
- Idempotency TTL 7 days (`identity/domain`, `IdempotencyTTL`).
- Queue `maxAttempts` 5 at enqueue call sites
  (`platform/jobs.Enqueue`).
- Consent HTTP body cap 64 KiB, strict decode
  (CS09C `privacy/adapters/http`).
- Training sweep daily; revoked-copy TTL 24 h (CS10C).
- Per-device consent revisions are u64 monotonic, server-ordered
  (CS09A); no cross-device counter exists.

## Rows

### Helpfulness (CS08A/B)

| Decision | Frozen |
|----------|--------|
| Default | Flag OFF hides vote actions; recorded votes preserved server-side |
| Guest | Reads tallies anonymously; writes need a free account (no IO for guests) |
| Offline | Votes/removals fail as transport (fixed kind); no client queue, no retry by the client |
| Stale/expired | Decision-sequence binding; stale refuses 409, client rebinds current sequence |
| Queue/receipt conflicts | Stable `client_submission_id` per intent; identical retries converge server-side (`replayed`), quota 429 surfaces as QUOTA_EXCEEDED |
| Class | Locally complete |

### Consent grant/revoke/status (CS09A/B/C, CS11A/B)

| Decision | Frozen |
|----------|--------|
| Default | Absent consent denies (default OFF) on device and server |
| Guest | Zero IO, claims nothing |
| Offline | Grant refuses without queueing (fail-closed ON); revoke queues as pending revocation, uploaded by `syncPending` on next connection |
| Stale/expired | Revision ordering (greater moves, identical converges, divergent conflicts, smaller stale); terms rotation invalidates old grants until fresh explicit grant (`needs_regrant`) |
| Queue/receipt conflicts | Per-device u64 revisions; same-revision divergent decisions conflict 409 and surface for reload; replayed receipts converge |
| Cache | No client cache: server authoritative, reload every foreground; switch never shows an unconfirmed claim |
| Class | Durable intent pending sync (offline revocation only); otherwise locally complete |

### Promotion propose (CS10A/B)

| Decision | Frozen |
|----------|--------|
| Default | Nothing promotes without granted consent + PASS verdict + per-image scope |
| Guest | No offer path exists for guests (worker-only, owner-bound) |
| Offline | No device-side queue exists; offers arrive via future review producer (CS11-screen work) |
| Stale/expired | Commit-time consent re-check; revoke between queue and commit refuses with no write; pre-grant captures never covered |
| Queue/receipt conflicts | Image-plus-revision key: identical replays converge, divergent hashes conflict |
| Class | Locally complete (contract); producer integration explicitly dependent on unavailable review-submission wire |

### Purge sweep (CS10C, SOAK_RUNNING)

| Decision | Frozen |
|----------|--------|
| Default | Daily sweep; rows past 24 h delete-then-confirm; rows stay REVOKED |
| Guest | N/A (server job, no caller) |
| Offline | N/A (server job); namespace outage fails the pass for retry, never drops rows |
| Stale/expired | Already-missing copies report missing (observability); resisting copies report failures |
| Queue/receipt conflicts | Idempotent deletes; replayed sweeps converge |
| Class | Dependent on wall-clock proof (due 2026-10-12T16:11:11Z); mechanics locally complete |

### Training erasure scope (CS10D)

| Decision | Frozen |
|----------|--------|
| Default | Consent REVOKED past stored revision (baseline when absent); derivatives tombstoned; bytes purged immediately (never waits for the sweeper) |
| Guest | Erasure requires the owner account (identity scope runs first) |
| Offline | Server-side; byte outage fails the scope for FAILED receipt + audited retry |
| Stale/expired | Re-runs converge to zero; receipts stay for the restore filter |
| Queue/receipt conflicts | Ledger row per scope (6 total); replay re-stamps without duplicating |
| Class | Locally complete |

### Settings UX + per-photo opt-in (CS11A/B/C)

| Decision | Frozen |
|----------|--------|
| Default | Switch OFF; photos start disallowed; components unplaced (no screen calls) |
| Guest | Panel renders nothing; opt-in state holds nothing claimable |
| Offline | Grant refuses; revoke pends with explicit Pending state; opt-in checks refuse without granted consent |
| Stale/expired | Switch reloads server-exact state after every decision; clearing consent wipes photo checks |
| Queue/receipt conflicts | Rejected kinds fixed + sanitized; retry reloads |
| Class | Locally complete (components); placement + submission wire explicitly dependent on screen work |

## Explicitly NOT frozen here

All non-safety areas (prices, sync, vehicles, stations, onboarding,
alerts, feedback, intake, location, profile, directory) ride
CS15-B2. No manifests/seeds/devices/budgets (CS15-C). No exit.
