# DRV-14 — Durable commands, bounded outbox and Go/Kotlin wire contract

Date: 2026-10-26. B-BR-DRV-01/02/17. Freeze before behavior.

## Scope

This phase makes the DRV-13 local command table a bounded, owner-scoped outbox
and freezes the first Go→Kotlin receipt/error wire fixtures. It does not start a
network sync loop, cloud opt-in, GPS recording or UI state.

## Durable intent and states

- A confirmed local save writes the record/revision and the first command in one
  Room transaction. Success is returned only after commit; a duplicate revision
  or storage failure rolls the whole transaction back.
- Command states are PENDING, SYNCED and CONFLICT. A process restart or owner
  switch cannot lose a PENDING row or expose another owner's queue.
- Claiming is owner+origin+operation scoped and lease-bound. A live lease blocks
  a second worker/device; an expired lease can be reclaimed. A foreign owner or
  guest partition cannot claim another owner's command.
- Retry attempts are bounded and use deterministic exponential backoff (5 s
  base, 15 min cap, six attempts). Exhaustion parks the command as CONFLICT with
  `retry-limit`; it is not deleted or silently dropped. A 409/server conflict is
  parked as CONFLICT and is not auto-replayed.
- Migration 29→30 only adds `lease_until_millis` with default 0; existing
  PENDING rows remain immediately claimable.

## Go/Kotlin wire fixtures

- Shared fixture files under `contracts/testdata/driver-suite/http` are produced
  and asserted by the real Go HTTP handler and read by the Kotlin codec test.
- Success receipts preserve exact operation id, revision, epoch and replayed
  flag. Conflict and auth error bodies preserve status/code and the no-store
  contract. The Kotlin decoder is strict about required fields and numeric
  ranges and does not invent missing values.

## Evidence boundary

Real Room on an owned emulator must prove 29→30 migration, restart, owner/guest
isolation, lease expiry, backoff, retry-limit and conflict parking. The Go test
must compare exact bytes to the shared fixtures; Kotlin/JVM must decode them.
Disk-full behavior is protected by the same Room transaction but requires a
separate storage-pressure test; this phase must state whether that test actually
ran. No network dispatcher/cloud/UI acceptance is claimed.
