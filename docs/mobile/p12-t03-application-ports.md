# P12-T03 — Application ports and offline state

Status: LOCAL_DONE on `codex/phase-12-kmp-foundation` (phase PR #21, draft).
Issue: #18. Dependency P12-T02 LOCAL_DONE satisfied. No behavior change to
existing screens, no Room schema change, no KMP Gradle plugin yet.

## B-BR / BUC

Contract for P10-T05 (outbox/direct media) and P10-T02 (community reads plus
local cache): stable command IDs with a fresh nonce per retry, explicit
queued/in-flight/failed/cancelled/acked states, stale-revision protection and
honest offline/stale/empty reads. Owning integration stays in P10; this task
freezes the portable state machines and the persistence shape.

## What changed

Portable-ready pure Kotlin in `:application`
(`com.anpfuel.application.portable`, zero `java.*`/Android/Hilt imports,
guard-tested for the future `commonMain` move). Explicit constructor
injection only: platforms implement `PortableClock`/`PortableNonceSource`
behind the boundary; tests use inline ticks.

- `PortablePorts.kt` — `PortableClock` (monotonic device ticks, no
  wall-clock math) and `PortableNonceSource` (fresh opaque value per
  dispatch, never reused).
- `PortableOutbox.kt` — FIFO enqueue with same-ID dedupe (revision bump,
  payload replaced, attempts kept so an old draft is never relabelled
  fresh); `selectNext` skips in-flight/terminal and uneligible backoff and
  dispatches nothing while cancelled; `markInFlight` attaches the fresh
  nonce; `markAcked` removes only the exact ID+revision (stale acks
  ignored); `markFailed` applies overflow-safe bounded exponential backoff
  (`BASE 1000` ticks, doubling, capped at `MAX 3600000`); `cancel` is
  terminal. `toRecord`/`fromRecord` freeze the migration-compatible shape
  (unknown keys ignored, missing/corrupt keys fail loudly).
- `PortableSyncState.kt` — online remote wins as `REMOTE_FRESH`; otherwise
  cache serves as `CACHE`/`CACHE_STALE` by explicit age bound (clock skew
  never marks fresh data stale); absent remote+cache is `EMPTY`, never a
  silent fresh empty list.
- `data/.../local/outbox/OutboxCommandMapper.kt` — native boundary: Room
  adapter persists the portable record field-for-field (delegation, no
  translation drift). Entity/migration introduction stays a later task.
- Tests: `PortableOutboxTest` (8), `PortableSyncStateTest` (5),
  `PortableAppParityTest` (2: platform-import guard + record-shape freeze),
  `OutboxCommandMapperTest` (2) — 17 new tests.

Room remains the Android adapter; Hilt stays out of portable code; existing
coroutines/Flow gateways (`NetworkConnectivityGateway`, `SyncExecutionLock`)
are untouched boundary APIs.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test --no-daemon
./gradlew :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
make quick-verify
git diff --check
```

Outcome:

- `:domain` + `:application`: all suites pass including 17 new portable
  application tests, 0 failures (full counts in commit evidence).
- `:data` + `:app` unit tests incl. `OutboxCommandMapperTest` and
  `assembleDebug`: SUCCESS (no schema change; regression only).
- `make quick-verify`: ok; `git diff --check` clean; secrets scan clean.

## Limits (not claimed)

- No `commonMain` wiring, no WorkManager/Room entity, no retry timers on
  device; the processor loop and DAO land in P10-T05 against this contract.
- No Swift counterpart yet (fixture-shaped records are JSON-ready strings;
  Swift parity belongs to P12-T04/T05).
- Instrumentation and device acceptance stay deferred to P12-T05/P18.

## Rollback

Pure addition behind no flag: previous behavior is bit-identical with the
portable packages unused. Remove the new packages to roll back; no data
migration involved (record shape is forward-only additive).

## Next

P12-T04 Swift framework and native shell (#19): umbrella framework plus thin
iPhone host executing a shared-fixture use case on macOS.
Issue #18 stays open until the P12 phase PR merges.
