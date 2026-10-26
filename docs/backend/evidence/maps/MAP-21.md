# MAP-21 — Electric community operational reports

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Rules: [MAP-21–23 contract](../../../product/MAP_21_23_CONTRACT.md).
Source follows the merged `eee1b33f` baseline and inherited local minimum map.
No payment gate, photo intake, live feed or provider activation was introduced.

## Behavior

Optional dated WORKED / DID_NOT_WORK / IN_USE / ACCESS_RESTRICTED events travel
through domain, append-only migration 132, sqlc, signed command handlers, public
history, owner reads, Kotlin transport, durable outbox and the existing community
form. SITE references an actual place; connector groups must belong to it and
match the connector. EQUIPMENT is explicitly a source reference with quantity
exactly one, not an invented physical equipment identity. Future instants refuse;
legacy price/experience reports remain valid. Labels explain recency and unavailable
live status in all eight existing locales. No score or arbitrary expiry was added.

Submit/correct/withdraw bind owner + stable command UUID + canonical digest to
accepted revision inside the mutation transaction. Lost replies replay exactly;
different bodies, foreign owners, stale edits and resurrection refuse. Corrections
preserve moderation. An owner advisory lock covers quota and receipts. Owner place
filtering happens before the limit. Applied migrations remain unchanged.

The existing 50-command outbox now refuses overflow without evicting old work,
serializes enqueue/drain, persists before network, checkpoints each acknowledgement
and parks FIFO on failure. Failed preference commits and corrupt data refuse visibly;
failed in-memory commits must flush before replay. Owner/origin partitions and
before/after owner fences prevent private receipts leaking after account changes.
Owner-scoped discard explains that local removal does not withdraw accepted content.
Connected WorkManager chains append behind running work, bound retry at 25 attempts
and park conflicts/refusals. Startup repairs the persist-before-schedule crash window;
reconnection/manual retry can reschedule parked network work. No credentials or
report bodies enter WorkManager inputs.

## Evidence executed

- Domain RED: permissive stub failed shape/clock tests; implementation GREEN.
- Explicit disposable PostGIS at loopback port 55439: `go test -tags=integration
  -race ./internal/modules/charging/... ./internal/modules/mapdata
  ./internal/modules/payments/adapters` PASS. Quota race rerun PASS: eight distinct
  simultaneous commands at quota-1 accept exactly one. Identical concurrent create,
  cancelled transaction, future/foreign group, altered replay, lost edit/withdraw
  reply and moderation/tombstone cases PASS. Map/payment cumulative upgrades now
  explicitly include migration 132 and replay idempotently.
- A **real Go test process + real HTTP + disposable PostGIS** was consumed by
  `ChargingOperationalWireTest`, using the production Kotlin client and serialized
  pending codec. Submit -> mine -> identical correction twice -> public event ->
  identical withdrawal twice -> empty public history PASS (1 test, zero skips).
  The opt-in Go test binary alone provides synthetic session verification; no test
  authentication path exists in the shipped API and this is not a live account or
  staging acceptance. The ordinary suite skips only that explicitly opted-in harness.
- `:application:test`, `:data:testDebugUnitTest`, `:app:testDebugUnitTest` PASS;
  focused final outbox cases cover overflow, FIFO, lost reply, process reopening,
  storage failure, owner change and startup recovery.
- Owned `anpfuel-16k` API 35 emulator, 16,384-byte pages: four
  `ChargingObservationWorkerTest` instrumented cases PASS: missing owner, former
  owner, storage failure/bounded retry and in-flight chain scheduling. Physical
  connected devices were not used. UI form/viewmodel/codec tests and compilation
  PASS; this does not claim a live station/user field report on a phone.
- `:app:lintDebug` PASS after supplying all six missing locale resources; no
  missing-translation check was disabled. Final startup injection compilation PASS.
- `sqlc generate`, `sqlc vet`, affected `go vet`, backend API/worker compilation,
  `GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/charging/...`, OpenAPI vacuum
  (zero errors; existing broad warning catalog), `gofmt`, diff-check PASS.
  The default Go toolchain exposed the documented staticcheck export-format issue;
  the repository's prescribed compatible toolchain passed. Secret surface reviewed:
  synthetic test proof only; no credentials, contributor GPS or photos added.

Local logs: `/tmp/map21-{domain-red,domain-green,wire-go,wire-kotlin,backend,
 integration-final,quota,worker-final,owner-final,android-recheck}.log`.
The loopback harness shut down and its per-run database was dropped.

## Limits and next phase

New PR, staging rollout, source admission and production certification remain
pending. Existing EE session/moderation is reused, not recertified as a provider or
commercial gate. Native routes and explicit foreground guidance are MAP-22/23;
MAP-24–33 remain outside this batch. Local minimum-map data acceptance does not
establish staging coverage, live charging availability or production distribution.
