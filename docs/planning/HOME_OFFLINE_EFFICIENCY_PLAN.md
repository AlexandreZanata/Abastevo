# Stage A — Home offline and operational efficiency

State: PLANNED / NOT_IMPLEMENTED_BY_THIS_PLAN, 2026-10-14.
Owner: [master](APP_CONSOLIDATION_PLAN.md). Tasks OPT-A00–A07.
Rules: B-BR-OPT-01–05/12/18. Use cases: BUC-OPT-01/02/10.

## Outcome and contracts

Show a useful scoped local summary immediately, refresh without erasing it on
network failure and avoid redundant server work. Historical values are not live
offers. Keep Community/ANP distinct; public Community needs no account/GPS/ANP
week cache. Entry: reconcile community-first and later safety fixes, including
existing slow-request/expiry guards.

Freeze/test backend public-cache semantics before the Android persistence/
revalidation consumer. Never relax no-store on owner/proof/command responses.

- Cache key: backend origin/environment, municipality code/UF, source, product,
  unit and full condition/qualifier scope, with schema/projection/policy version.
  Current summary is STANDARD-only; add no invented condition filter.
- Persist typed milli-BRL amounts, counts, source generated_at/updated_at/
  expires_at and local saved_at/last_checked_at. Formatting stays in UI.
  Save/check/retry time cannot renew the original price validity.
- Atomically replace a complete city snapshot/generation across fuels. A valid
  empty result updates availability; transport failure does not. Historical
  retention is bounded and explicitly labelled.
- Room is the local read model. Separate initial loading, refreshing, live data,
  historical/stale data, no data and failure. Keep cancellation/city/origin/
  source-generation guards and single-flight equivalent loads.
- Expired aggregates cannot be recomputed correctly from old average/min/max
  alone. Re-fetch the eligible population aggregate or show dated history;
  never silently extend freshness or substitute ANP.
- Refresh by useful opening/city/reconnection/contribution/manual events. Evaluate
  60 seconds as initial foreground candidate, not frozen SLA; align server hints
  and client tolerance. Stop periodic network offscreen/offline, use bounded
  backoff/jitter, and update age labels locally.
- ETag binds exact scope/stable semantic snapshot. Use snapshot generation time
  rather than request-time entropy; expiration can change population without a
  write. Revalidation/reuse must invalidate at next relevant expiry/change.
  A 304 updates check state, never source validity.
- Freeze explicit public HTTP caching before relying on persisted responses;
  do not simply store responses still contractually no-store. Caller overlays,
  credentials and error bodies never enter public/shared caches.

## Sequential tasks

### OPT-A00 — Reconcile and measure current Home/summary

- **Depends:** master current-head/app-first prerequisites.
- **Inputs/areas:** Home/summary/client/Room/SQL and existing offline/safety tests.
- **Deliver:** exact code/config inventory and synthetic baseline requests,
  bytes, SQL executions, foreground/expiry/failure behavior.
- **Tests/acceptance:** demonstrate session clearing, failure/empty distinction,
  one full-population query and existing slow-request guard; no stale live claim.
- **Recovery:** audit does not mutate product data; known failures precede changes.

### OPT-A01 — Freeze cache and lifecycle contract

- **Depends:** OPT-A00.
- **Inputs/areas:** target, OpenAPI/HTTP DTO/domain summary and Room design.
- **Deliver:** keys, generation, live/history/empty/error/expiry/304 semantics,
  compatibility vectors and measured-budget protocol.
- **Tests/acceptance:** clock rollback, same amounts/different validity, origin/
  city separation, empty versus outage, unknown enums/schema.
- **Recovery:** revise unpublished contract without weakening private no-store.

### OPT-A02 — Backend conditional GET and bounded reuse

- **Depends:** OPT-A01. Critical SQL/read validity.
- **Inputs/areas:** existing summary handler/read SQL, httpapi/OpenAPI.
- **Deliver:** ETag/304 and public policy; bounded reusable snapshot only where
  the baseline justifies it. No new service/dependency by default.
- **Tests/acceptance:** immediate real PostGIS snapshot/expiry-without-write,
  moderation/deletion/correction/city invalidation, parallel fill/stampede,
  outage, validators and noncached errors. Measure SQL separately from bytes.
- **Recovery:** disable reuse/revalidation, keep exact uncached source filters.

### OPT-A03 — Room snapshot persistence and migration

- **Depends:** OPT-A02 accepted backend contract.
- **Inputs/areas:** existing database/DAO/repository/DI and shared fixtures.
- **Deliver:** bounded typed public cache, atomic replacement, origin scoping,
  forward migration and explicit historical-data handling.
- **Tests/acceptance:** actual Room upgrade/reopen, partial-write rollback,
  multi-fuel/empty/old-schema cases, exact amounts, concurrent refresh and bounds.
- **Recovery:** stop affected-generation writes, repair forward/refetch public
  data; never wipe the whole user database or other modules.

### OPT-A04 — Local-first repository and Home states

- **Depends:** OPT-A03.
- **Inputs/areas:** current repository/use case/mapper/ViewModel/Compose.
- **Deliver:** local-first observation, non-destructive refresh/error, visible
  dates/source and live/history/unknown states.
- **Tests/acceptance:** resume offline, process death, response inversion,
  expired/empty/failed result, locale/source/city/origin switch and cancellation.
  Existing slow-refresh/expiry regressions remain green.
- **Recovery:** disable added path/generation with truthful retained data display.

### OPT-A05 — Event-driven refresh and convergence

- **Depends:** OPT-A04.
- **Inputs/areas:** resumed lifecycle/connectivity/manual and selected event hooks.
- **Deliver:** one in-flight equivalent load, policy interval, local age tick and
  bounded retries; no periodic background 15-second loop.
- **Tests/acceptance:** no offscreen/offline polling; burst events converge,
  manual/city change wins, slow request finishes, clock changes do not revive data.
- **Recovery:** event-only/manual refresh retaining local data.

### OPT-A06 — Measure savings and failure behavior

- **Depends:** OPT-A02–A05.
- **Inputs/areas:** synthetic HTTP/SQL workloads and actual app consumers.
- **Deliver:** before/after requests/session, bytes, SQL, latency, memory, reuse
  and source-change/expiry propagation for pinned environments.
- **Tests/acceptance:** cold/warm, unchanged/changed city, many readers, DB/network
  outage and bounded memory. A 15→60-second timer predicts 75% fewer timer calls;
  battery/CPU/capacity savings require actual measurement.
- **Recovery:** select simpler measured strategy; invalid reuse never stays enabled.

### OPT-A07 — Stage acceptance and handoff

- **Depends:** OPT-A06 and affected task checks.
- **Inputs/areas:** pinned API/app/config/cache schema and native obligations.
- **Deliver:** local checkpoint, accessibility/offline matrix and scoped rollback.
- **Tests/acceptance:** complete Home city/source live/empty/history/error journey;
  affected JVM/lint/APK now, real native/battery checks when authorized/due.
- **Recovery:** scoped disable preserving existing prices and user records.

## Exit and references

Exit: truthful cached lifecycle plus measured bounded optimization, LOCAL_DONE /
INTEGRATION_PENDING; no provider/native/release inference. Continue authorized B
without stage PR/CI waits. Exact task command selectors are recorded when added.

Design references: [Android offline-first](https://developer.android.com/topic/architecture/data-layer/offline-first)
and [HTTP caching](https://www.rfc-editor.org/rfc/rfc9111.html).
