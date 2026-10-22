# MEX-25 — Public occurrence Android types and bounded persistence

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `a5b08ef5` (maintained dev, post-PR-#145 merge). Date: 2026-10-22.
Depends MEX-23/24 (accepted APIs on main), accepted. User-authorized
Android scope (G09/G24) for this unit. Rules B-BR-MEX-10/11/14/17;
BUC-MEX-07–09. Risk: privacy/cache truth (critical: official cache
must never hold private data, stale must disclose, cursors bind).

Before behavior: the app reads official occurrences through strict
typed DTOs over the accepted enforcement APIs, reusing the shared
public-cache contract. Unknown enums are retained, never crashed on;
malformed, oversize, foreign or private bodies fail closed without
disclosure. Transport is cancellable with timeouts and no credentials;
ETag/304, 404 clearing and no-store are honored. Cache keys bind
origin and filters; revision vectors ride every result; offline
fallback discloses staleness instead of claiming freshness. Private
reports are refused before any cache write. Recovery: disable affected
cache generation, never wipe other modules.

## Delivered

- `domain/.../occurrence/Occurrence.kt`: strict DTOs (records,
  stations, coverages, leases, revision vectors) with shape
  validation; typed `OccurrenceScope` sealed interface; unknown enums
  retained as raw strings.
- `domain/.../repository/OccurrenceGateway.kt`: bounded anonymous
  occurrence reads (records, coverages, stations) with revision
  vectors.
- `data/.../remote/occurrence/OccurrenceHttpClient.kt`: strict client
  over the 23/24 endpoints reusing the public-cache contract —
  facet params, cursor/ETag binding, 1 MB cap, 20 s timeout, no
  redirects/credentials, cancellable calls, 304 reuse, 404 clearing,
  no-store respect, transport-failure stale fallback, and
  validation-before-cache (private JSON never stored).
- `OccurrenceTest` (domain, 5 cases): verbatim retention, unknown
  kinds, malformed refusal, typed scopes/vectors, active-only
  stations.
- `OccurrenceHttpClientTest` (data, 5 cases): credential-free reads
  with unknown-kind retention, foreign-revision/repeat-cursor
  refusal, oversize/corrupt/private refusal, stations/coverage with
  wrong-origin refusal, and cancellation without caching.

## Actual checks

From this worktree (2026-10-22, parent `a5b08ef5`):

- `./gradlew :domain:test --offline` → PASS (643 tests, including 5
  new occurrence cases).
- `./gradlew :data:testDebugUnitTest --offline --tests
  "com.anpfuel.data.remote.occurrence.*"` → PASS, 5 tests
  (MockWebServer: pages, coverage, facets, 304/404/no-store paths,
  cancellation, private rejection).
- `./gradlew :data:lint --offline` → PASS, no new warnings.
- `./gradlew :data:testDebugUnitTest --offline --tests
  "com.anpfuel.data.local.*"` → PASS (Room/file-cache regression,
  no schema change, no new migration).
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- No backend, contract, worker or iOS changes; no source activation,
  no PII/secrets in Git/fixtures; `com.anpfuel` and MIT notices
  preserved.

## Limits

- This unit ships types, transport and cache only. No UI, navigation,
  ViewModel or screen wiring (MEX-26); no backend changes; the
  accepted 23/24 APIs are consumed as-is.
- Cache reuses the shared file-based public contract (7-day
  retention, 1 MB body, 64 entries/16 MB total); no Room entities
  added, no migration needed. Private data is refused by shape
  validation before any write — the cache itself is unchanged.
- Offline fallback serves stale bodies with the lease flagged stale;
  a cache hit never renews source validity and never claims fresh
  verification. Expired (>7 days) or corrupt entries are deleted,
  never served.
- Unknown enum values are retained verbatim; unknown FIELDS are
  ignored (forward-compatible); missing REQUIRED fields refuse.
- No device farm, no production, no Play release and no release
  acceptance is claimed.

Next: MEX-26 (national occurrence explorer UI) needs its own
authorization; it consumes these tested types.
