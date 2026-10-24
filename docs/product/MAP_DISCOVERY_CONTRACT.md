# Map discovery behavior and source admission contract

Date: 2026-10-24. Status: FROZEN_FOR_IMPLEMENTATION (MAP-01).
Scope: Android map discovery, public station/charging overlays and pricing
projection. It does not authorize deployment, external feeds or source
activation.

Companions: [execution plan](../planning/MAP_EV_EXECUTION_PLAN.md),
[phase registry](../planning/MAP_EV_PHASES.json),
[source register](../planning/MAP_EV_SOURCE_REGISTER.md),
[ADR-022](../adr/022-comaps-derived-native-map-engine.md) and
[contract fixtures](../../contracts/testdata/map-discovery/cases-v1.json).

## Information products

Two independently versioned public products share admission/provenance:

1. **Native reference map**: roads, places, search index and routing graph
   derived from admitted OSM data (ODbL), rendered by the pinned native engine.
2. **Abastevo overlay**: canonical station UUIDs, exact ANP price observations,
   community projections, charging identities/connectors, location quality and
   attribution. Price updates never rebuild the routing graph.

Private state (favorites, trips, precise location, charge history) is a third,
account-scoped product and is never published.

## Frozen artifact formats (MAP-03)

- `contracts/map/poi-assertions-v1.schema.json` freezes source-specific site
  assertions: canonical UUIDs, CNPJ text, station/charging identities, separate
  display/entrance/road points, exact `amount_milli_brl`, product/unit pairs,
  clocks, rights and derived capability flags. One site may have several
  source-specific assertions merged by explicit identity, never by proximity.
- `contracts/map/native-map-manifest-v1.schema.json` freezes the pinned engine
  and generator SHA, `.mwm` format version, region/bbox, package digests,
  overview/neighbor dependencies and ODbL/Apache attribution. Missing or
  mismatched dependencies block activation.
- `contracts/map/prepared-envelope-v1.schema.json` reuses the existing
  `charging-prepared-v1` manifest/hash/census fields. `product_class` and
  `visibility` keep `REFERENCE`, `PUBLIC_OVERLAY` and `PRIVATE_STATE` artifacts
  separate; private state can never be `PUBLIC`.
- Goldens live in `contracts/testdata/map/`. Rust, Go and Kotlin reject unknown
  fields, duplicate IDs, wrong hashes/rights/profiles, product/unit mismatches,
  overflow, centroid-as-pin, truncated streams and unsupported versions.
  Validation is all-or-nothing; no partial overlay is published.

## Mandatory behavior rules

### B-BR-MAP-01 — Identity and location

- A station pin requires an admitted public geometry with a known quality.
- `reviewed` coordinates may be shown as the station/entrance display point.
  `osm-mapped` node/centroid geometry may be shown as an explicitly approximate
  OSM reference pin, visually and semantically distinct from a reviewed station.
  `unknown` and `city-centroid` geometry never become an exact pin; they remain
  reachable through the accessible result list with an explicit location warning.
- Display point, entrance and road-snap are separate values. Navigation targets
  the admitted entrance/access when present; otherwise it warns that the last
  segment may be imprecise.
- CNPJ stays alphanumeric text with leading zeros. Station UUID and charging
  place/equipment/connector identities are preserved. Proximity alone never
  merges two records.

### B-BR-MAP-02 — Price source

- `ANP` mode shows only individual official station observations. `Comunidade`
  mode shows only the current validated community projection.
- Switching the selector atomically changes pins, quick sheet, list and cache
  identity. A response produced for an older selection/generation is discarded,
  even if it arrives later.
- There is no silent fallback between sources. Absence is displayed as absence.
- Public OSM reference-only POIs have no invented ANP/community linkage.

### B-BR-MAP-03 — Money, units and product

- Preserve exact decimal values, currency, product, sale unit, tariff condition,
  observed date and survey/reference period.
- Municipal averages, wholesale/distribution prices, LPG cylinder prices and
  electric kWh/min/session tariffs are never relabeled as a station per-liter
  price.
- Unknown is not zero and never means free.
- Premium/additive products remain distinct from regular products.

### B-BR-MAP-04 — Electric stations in ANP mode

- Chargers remain discoverable in `ANP` mode.
- Their quick sheet states that ANP does not supply electric tariffs and offers
  an explicit action to switch to `Comunidade`.
- Source mode and service-type filter are independent; the selector is never
  silently disabled.

### B-BR-MAP-05 — Service types

- Fuel, charging and combined sites have stable identities and distinct
  accessible symbols (fuel pump and electric plug/bolt). Text labels are always
  present; color alone is not the signal.
- A fuel marker does not certify fuel quality; a charging marker does not
  certify current availability.
- Co-located services expose both service types without duplicate selectable
  pins or duplicated prices.

### B-BR-MAP-06 — Clocks and status

- Geometry, price, community reports and any future live status have separate
  `observed_at`, `published_at`, `fetched_at`, source and generation values.
- Static reference status, dated community working reports and live availability
  are visually and semantically distinct.
- No fixed expiry or confidence percentage is invented. The interface always
  shows source/date; cached data never masquerades as current.
- Tapping a marker never starts a network scrape of the original source.

### B-BR-MAP-07 — Rights

- Absent, mismatched or revoked rights block publication.
- Attribution remains visible offline and in exports.
- Source IDs and provenance survive deduplication and reprocessing.
- OSM data is ODbL, not MIT. Code licenses, data licenses and artwork licenses
  are reported separately.

### B-BR-MAP-08 — Privacy

- Public POI coordinates are distinct from private home/route/favorite/history
  coordinates.
- Public map reads are anonymous. Signed writes and owner reads require the
  existing proof/ownership rules.
- No telemetry of raw coordinates or routes. No implicit background recording.
- Browsing, search and manual city selection work without location permission.

### B-BR-MAP-09 — Persistence and updates

- Map binaries live outside Room; Room stores overlay generations/metadata only.
- Overlay and native map packages activate atomically after manifest signature,
  digest, size, path and dependency checks. The last valid generation survives
  crash, cancellation and disk-full.
- A price/overlay generation updates independently from the routing graph.
- Account erasure/revocation survives private restore; public packages never
  enter the private media bucket.

### B-BR-MAP-10 — Route truth

- Roads, restrictions and entrances govern routing; straight-line distance is
  never presented as a drivable route.
- Missing regional/neighbor packages are shown explicitly with a download
  action.
- No live-traffic claim without an admitted traffic feed.

### B-BR-MAP-11 — Electric trip truth

- Compatibility, useful battery, initial SOC, consumption, reserve and road
  detours govern stop estimates.
- A failed primary alternative must be reachable under the stated assumptions.
- Insufficient evidence is displayed. No guaranteed-arrival badge or invented
  exact charging time.

### B-BR-MAP-12 — Accessibility and free use

- Retain an accessible result list, 48dp targets, labels, 200% font, narrow
  screens, TalkBack semantics and light/dark/dynamic themes.
- Fuel/charging symbols have independent shapes.
- Basic discovery and community remain free; no paid trust, contribution
  privilege or safety-warning paywall.

### B-BR-MAP-13 — Distribution security

- Authenticated manifests and package digests are verified before activation.
- Paths, hosts, bytes, formats and dependency sets are constrained.
- Private media permissions are not weakened for public map delivery.
- Package deletion cannot race an active route.

### B-BR-MAP-14 — Native boundary

- Keep `com.anpfuel`, MIT notices and existing modules.
- Native C++/JNI types stay behind a narrow Android adapter. No parallel
  canonical account/station/price database and no unreviewed proprietary SDK.
- Engine failure is reported; it never justifies a silent WebView/MapLibre/
  Google substitution.

## Use cases

### BUC-MAP-01 — Anonymous browse without GPS

GIVEN a user with no account and location permission denied
WHEN the map opens
THEN the last public region/camera and admitted overlays render, a manual city
search is available, and no location prompt blocks browsing.

### BUC-MAP-02 — Source/product/period switch

GIVEN the map is showing ANP gasoline for a survey period
WHEN the user switches to Comunidade
THEN only community projections replace the prices, the older response is
discarded, and pins/list/sheet/cache share the new source identity.

### BUC-MAP-03 — Pin → quick price → profile

GIVEN a visible reviewed station with a known selected-fuel price
WHEN the user taps it
THEN a bottom sheet shows name, service type, exact product/unit price, source,
date/period, location quality and actions for route, full profile and report.
The profile route is the existing station profile.

### BUC-MAP-04 — Public contribution recovery

GIVEN an authorized user submits a price/status report offline
WHEN connectivity returns or the process restarts
THEN the signed outbox item is retried idempotently or visibly fails; no
unvalidated report is shown as public truth.

### BUC-MAP-05 — Regional download/update/rollback

GIVEN a compatible regional package is available
WHEN the download is interrupted or its digest/signature fails
THEN the partial file is not activated, the previous generation remains usable,
and the user can resume/retry or roll back explicitly.

### BUC-MAP-06 — Offline search/route/guidance

GIVEN all required region/neighbor packages are active and the device is offline
WHEN the user searches and starts a car route
THEN the real native search/route/guidance works, and missing packages or
restrictions are shown instead of a straight-line substitute.

### BUC-MAP-07 — Electric plan with failed primary

GIVEN a vehicle with useful battery, SOC, reserve and consumption
WHEN the primary compatible charger is unavailable
THEN the app shows the reachable alternative under stated assumptions, or states
that no evidenced alternative exists. It does not guarantee arrival.

### BUC-MAP-08 — Private save/export/backup/erase

GIVEN an owner saves a favorite/trip or records a trip explicitly
WHEN exporting, restoring or erasing
THEN ownership is enforced, public data notices are retained, and no home/route
coordinate is published by default.

### BUC-MAP-09 — Source revocation/moderation

GIVEN a source or record is withdrawn, corrected or rights-blocked
WHEN a new public generation is published
THEN the map stops serving it without rewriting history, and caches/cursors
bound to the old generation are invalidated.

### BUC-MAP-10 — Upgrade and optional car host

GIVEN an existing installation and optional Android Auto host
WHEN the app upgrades or the host connects/disconnects
THEN public/private data survives, phone navigation remains independent, and
missing host certification is reported, not fabricated.

## Fixture-derived decisions

The versioned fixtures in
`contracts/testdata/map-discovery/cases-v1.json` freeze the following decisions:

- ANP-only, community-only, no-price and combined sites are all representable.
- Missing and city-centroid geometry never render as exact pins.
- A wrong product/unit pair and a municipal aggregate are rejected.
- A delayed response for an old source generation is dropped.
- A rights-blocked record is excluded.
- Unknown charging power/tariff remains unknown; it is not zero/free.

## Out of scope

No national completeness, live availability, operator feed, payment/reservation/
OCPP control, upstream OSM editing, cloud voice, traffic forecast, iOS/CarPlay or
guaranteed safe arrival is contracted here.

## MAP-14 T03 release-build recovery (2026-10-24)

- **B-BR-MAP-BUILD-01:** a distributable engine AAR comes from the pinned
  `:sdk:assembleRelease` task with minSdk 26. Debug Java classes mixed with a
  release native library are probe inputs, never accepted release artifacts.
- **B-BR-MAP-BUILD-02:** the modern NDK uses LLD with ThinLTO retained; source
  patches are recorded and reapplicable. Generated Ninja files are not edited.
- **B-BR-MAP-BUILD-03:** artifact verification checks actual release BuildConfig,
  native ABI/stripping/LOAD alignment and hashes. It does not certify runtime
  16 KB compatibility, transitive rights or the other ENGINE_GATE obligations.
- **BUC-MAP-BUILD-01:** GIVEN CMake 3.22.1 and NDK 28.2, WHEN a C/C++ ThinLTO
  library links without the source fix, THEN the obsolete gold option fails;
  WITH the recorded source fix, the library links using NDK LLD with 16 KB
  alignment and ThinLTO still enabled.
- **BUC-MAP-BUILD-02:** GIVEN a debug/hybrid AAR or wrong minSdk/ABI, WHEN
  release candidate verification runs, THEN it refuses publication and retains
  any prior output. A valid Gradle release AAR publishes to a new output only.
