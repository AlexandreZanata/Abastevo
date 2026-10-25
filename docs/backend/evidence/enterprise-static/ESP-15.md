# ESP-15 — Wire automatic refresh and correction recovery

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-14 `43574f7a`

Rules: B-BR-ESP-09–15; BUC-ESP-06/07.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-15--wire-automatic-refresh-and-correction-recovery).
[ESP-14 hosting/promotion](ESP-14.md).
[Operator contract](../../../operator/portal-static-hosting.md#automatic-refresh-and-correction-esp-15).

## Delivered

- **Publication intents:** `FineFileImport` gained an optional `AfterPublish`
  hook; the worker wires it (default off) to enqueue an export build with the
  source observation time. The official import hook now records the same
  observation time. A hook failure is visible but never rolls back the
  published revision; the export reconcile recovers.
- **Build completion intent:** `Build` gained an optional `AfterBuild` hook and
  `State.ObservedAt`/`BuildPayload.ObservedAt`. A new accepted export enqueues a
  portal refresh; an unchanged build is a no-op and does not re-run the hook.
- **Durable refresh job:** `enterprise-portal-refresh` reads the export state,
  generates a portal candidate, verifies and promotes it with the current
  release fence, and records `portal-refresh-state.json` in the private portal
  root. It has its own flock, a 15-minute recovery schedule and a dedicated
  dispatcher slot. No DB/API/source call is made by the generator or publisher.
- **No-op and recovery:** an unchanged export/source pin or a generated content
  hash equal to the current release records `noop` without switching. A failed
  generation/promotion preserves the current release and records `failed`.
  Orphans older than 24 hours are removed; a crash after promotion is recovered
  as a no-op on the next refresh.
- **Correction withdrawal:** if the current FQ official revision is no longer
  published, the refresh withdraws the affected release; it uses a newer release
  without official records as fallback when available, otherwise removes
  `current` (404) rather than serving a withdrawn slice.
- **Release provenance:** `portalgen.Release` now records the official revision,
  lease, edition and source checksum so withdrawal can identify affected
  releases. `CurrentReleaseHeader` is the bounded metadata read for refresh;
  full `portalgen.Verify` still runs before every promotion/copy and on
  `portalrelease verify`.
- **Configuration:** `ANPFUEL_ENTERPRISE_PORTAL_REFRESH_ENABLED`,
  `..._ROOT`, `..._SITE_URL`, `..._INDEXABLE`, `..._SEARCH_JS`; defaults off,
  requires the export builder, an absolute private root and an http(s) origin.

## Refresh path and timing receipt

```text
fine/official publication
  -> enterprise-export-build (ObservedAt from the import)
  -> enterprise-portal-refresh (AfterBuild; 15-minute recovery)
  -> generate candidate under portal/candidates
  -> portalgen.Verify + portalrelease.Promote (fence on current)
  -> current symlink switch; old release retained
  -> portal-refresh-state.json
```

The receipt records `trigger`, `observed_at`, `export_built_at`,
`portal_built_at`, `released_at`, `withdrawn_at`, source
revision/lease/edition/checksum/clock, `export_content_sha256`,
`portal_content_sha256`, generation seconds, export-to-release seconds and
observed-to-release seconds. `observed_at` is the local source/build detection
time, not the ANP publication delay; the ANP source clock is recorded separately
and no ANP delay is inferred. The receipt stays in the private portal root and is
never served.

## Tests and actual results

Unit/race, from `backend/`:

```sh
go build ./...
go test -race -count=1 ./internal/modules/enterpriseexport/... \
  ./internal/modules/quality/adapters/jobs ./cmd/worker ./internal/platform/config
```

PASS: jobs 7.821 s, portalgen 18.941 s, other packages green. Final refresh
tests cover promotion/coalescing, content-addressed asset invalidation,
failed-build preservation, restart/orphan recovery, source-change release,
`awaiting-export`, price-only output without the official source, withdrawal
with a safe fallback, withdrawal to 404, broken lock, and the real official
snapshot.

Integration, disposable PostGIS:

```sh
go test -race -count=1 -tags=integration \
  ./internal/modules/enterpriseexport/adapters/jobs
```

PASS 12.054 s. `TestRefreshPromotesBuiltCandidateAndCorrection` builds an export
from PostGIS, refreshes it to a release, no-ops the unchanged candidate, applies
a price correction, and promotes a second release. `Build` integration also
proves `AfterBuild` runs only for a new state and `ObservedAt` is preserved.

Fine publication hook (real Rust/PostGIS):

```sh
ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/tmp/fq-native-rust-20261024/debug/prepare_sanctions \
ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH=/tmp/fq-native-rust-20261024/debug/prepare_fine_history \
go test -race -count=1 -tags=integration \
  ./internal/modules/quality/adapters/jobs \
  -run TestNativeFineFileRustPostGISRecoveryCorrectionAndCleanup
```

PASS 4.440 s. The hook observes the original and correction revisions; replay
converges on the same revision dedupe key. The published history and watermark
remain correct when the hook is not invoked on superseded replays.

Real official refresh/withdrawal:

```sh
ANPFUEL_TEST_REAL_OFFICIAL_DIR=/tmp/fq-portal-exports \
ANPFUEL_TEST_PORTAL_SEARCH_JS=landing/dist/assets/search.2828d19c.js \
go test -race -count=1 -tags=integration \
  ./internal/modules/enterpriseexport/adapters/jobs \
  -run TestRefreshRealOfficialSnapshotAndWithdrawal
```

PASS. The real 2025 official export (revision
`fc18397c-cf1c-41aa-a155-a3d5c4bbfa5c`, lease
`a18283c151938ddbb56574ddbd897629cda050c887fefbc8ce81e0dd6c150a88`, 3,375
all-unmatched records) over the synthetic station/price fixture produced a
**3,401-page / 3,411-file / 15,881,532-byte** release with content
`sha256:ccd426695c6c5bcfd7aaf050f7dc1221713b554db154ace0673002ae800899ee`.
The indexable artifact used the real compiled search JS and a 3,392-entry
sitemap. A price correction produced a second release; a second unchanged
refresh recorded `noop`; a withdrawn official revision removed the current
official release (no official-free fallback) and recorded `withdrawn`. The race
run passed in 165.40 s; the final non-race run passed in 24.75 s.

Config/dispatcher:

```sh
go test -race -count=1 ./internal/platform/config ./cmd/worker
```

PASS. New config defaults/rejections and the dedicated
`enterprise-portal-refresh` dispatcher are covered.

Static:

```sh
go vet ./internal/modules/enterpriseexport/... ./internal/modules/quality/adapters/jobs \
  ./internal/platform/config ./cmd/worker ./cmd/portalgen ./cmd/portalcheck ./cmd/portalrelease
GOTOOLCHAIN=go1.27.1 staticcheck ...   # same scope
sqlc vet
sqlc generate                           # byte-stable
gofmt -l <changed files>
git diff --check
```

All PASS. The quality unit matrix also PASS. No credential, PII or source row
was added.

## Limitations and OWED

- **Production activation is OWED.** The refresh is default-off; no source
  schedule, DNS, VPS, Caddy or Compose change was made. Production
  host/CDN/observability, multi-host locking, quotas and capacity remain
  ESP-18/ESP-16.
- Real prices remain blocked by ESP-07/rights. The real refresh used the real
  official 2025 slice with the synthetic station/price fixture; the PostGIS
  integration uses synthetic price data. No price publication is claimed.
- The FQ snapshot remains one current edition; a historical multi-edition
  official snapshot is OWED.
- Fine association live revalidation/rebound remains OWED (FQ-07R6D).
- `observed_at` is local detection/build time; it is not ANP publication delay
  and must not be reported as such.
- The refresh's no-op/status path reads release metadata without rehashing every
  file; promotion still verifies the candidate and copied tree, and
  `portalrelease verify` remains the explicit full integrity check.
- A replay may invoke the fine hook more than once for the same revision; the
  worker's revision-ID dedupe key converges the export intent.
- No `main` merge, PR, production activation or publication occurred.

## Next

ESP-16 — capacity and browser-performance campaigns over the verified combined
artifacts. Not implemented here.
