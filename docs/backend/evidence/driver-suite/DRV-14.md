# DRV-14 — Durable outbox, leases and Go/Kotlin wire contract

Status: LOCAL_DONE
Validation: PASS (local JVM/Go + owned API35 emulator; no live sync claim)
Integration: INTEGRATION_PENDING
Parent: b5957f53. Behavior: 72f8bd46. Contract: [DRV-14](../../../product/DRV_14_CONTRACT.md).

T01: the DRV-13 transaction now exposes PENDING/SYNCED/CONFLICT command states.
`DriverOutbox` claims are owner/origin/operation scoped and lease-bound; a
restart preserves PENDING rows. A duplicate revision/storage transaction abort
rolls back the complete local save (DRV-13 test) and the queue is not deleted.

T02: migration 29→30 adds only `lease_until_millis` (default 0). Claiming is
refused for a foreign owner/guest and while the lease is live; the same owner
can reclaim after lease expiry. Retry uses 5s base, 15min cap and six attempts,
then parks as CONFLICT with `retry-limit`; server conflict/409 is parked without
auto-replay. A first instrumentation install attempt hit an emulator
`package ... Broken pipe`; the rerun passed 7/7. A real disk-full pressure test
was **not** executed; the transactional rollback path is proven by the
duplicate-revision failure, but disk-full remains an explicit evidence gap.

T03: the real Go HTTP handler asserts exact shared fixtures for a success
receipt (`id`, `revision`, `epoch`, `replayed`), a 409 `driver.conflict`, and a
401 `driver.auth-required`; the Kotlin `DriverHttpCodec` reads the same files and
asserts the exact values/ranges.

## Actual validation

- Go: `go test ./internal/modules/driver/adapters/http/ -run TestDriverHTTPWireFixtures` PASS.
- JVM: `:data:testDebugUnitTest` plus driver migration/codec tests PASS.
- Android API35 x86_64 emulator: **7/7 instrumentation PASS** across migration
  28→29, store draft/atomic/shift, migration 29→30 and outbox claim/backoff/
  conflict tests.
- `:app:compileDebugKotlin` PASS; `scripts/check-mobile.sh --static-only` PASS.

## Deliberate limits

No network dispatcher, cloud consent, UI, GPS or production claim. Disk-full was
not simulated on device (recorded above). The inherited coverage gate is
unchanged. No push/PR/merge until the delivery step.
