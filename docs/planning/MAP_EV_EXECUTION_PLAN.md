# Fuel and electric map — complete execution plan

Date: 2026-10-24. Status: PLANNING_LOCAL_DONE / IMPLEMENTATION_PLANNED.
Audited application source: `b90a0cd1277f0728d60fdf47bb1b62bc91613416` on maintained dev.
Last documented test deployment: `53719336`, schema 120; this is not a claim about any later deployment.
Scope authority: the user requested complete planning and two prepared map pictograms, not runtime implementation, a VPS deployment, external contacts, paid APIs or remote publication in this turn.

[Planning/asset validation](MAP_EV_PLAN_VALIDATION.md) records the checks actually performed.

Canonical companions: [phase registry](MAP_EV_PHASES.json), [source register](MAP_EV_SOURCE_REGISTER.md), [execution prompt](MAP_EV_AGENT_PROMPT.md), [icons](../assets/map-icons/README.md).

## Product outcome and delivery boundaries

Add **Explorar | Comunidade | Mapa | Perfil** to the Android bottom bar, in that exact order. The map presents fuel and electric stations with distinct icons. A top **ANP / Comunidade** selector controls displayed prices. Selecting a station opens a fast bottom sheet with exact source/date/product/unit prices and actions to navigate, open the existing full station profile and report a price. A list equivalent keeps discovery accessible.

First finish information ingestion and serving: admitted raw sources -> Rust preparation -> Go validation/publication -> PostgreSQL/PostGIS and immutable regional packages -> actual HTTP contracts -> Android decoding/persistence. Only after DATA_GATE should the agent start Android map functionality. After ENGINE_GATE, deliver the minimum map UI; then complete offline search, driving routes, guidance, favorites, electric stops and acceptance. A hidden unfinished button is not a delivered capability.

The map is an **Abastevo product built on an owned, pinned CoMaps-derived native core**, not a second app, an upstream SDK add-on or a wholesale fork of the CoMaps UI. We vendor the minimal core/JNI host at an exact audited SHA, keep a small documented patch set, build our own AAR in a reproducible builder, and expose a narrow `MapEngine` Kotlin port. Abastevo owns the application identity, Compose UI, station identities, prices, community data, download/manifest policy and map style; the native core owns rendering, the compatible map format, offline search and routing. ENGINE_GATE (MAP-14) proves that extracted artifact before any UI depends on it. Its Apache-2.0 core does not license all third-party code/art/maps/services automatically. Never ship the CoMaps app, branding, CDN or service key.

### Engine ownership boundary

- **Abastevo app/UI:** navigation, Compose theme, map shell, quick sheet, existing station profile, filters, accessible list, favorites and trip UI. No CoMaps screen or widget is shipped.
- **`MapEngine` port and adapter:** the only native surface the app sees; initialization config, lifecycle, camera, overlay/markers, selection, search, route and guidance events, cancellation and error mapping. Kotlin owns Activity/Compose lifecycle, FileProvider, storage, permissions and location source.
- **Native core:** renderer, compatible map package reader, offline search index, routing graph and guidance state machine. JNI types and threads stay inside the adapter.
- **Data:** Go/PostGIS/HTTP owns canonical station identity, ANP/community prices and charging facts; Room/files own overlay generations, map packages and download state. The engine never becomes a second station/price database.
- **Assets:** admitted Abastevo map style, fonts, symbols, voices and the generated pictograms; reference OSM and ODbL attribution stay visible. No CoMaps brand/logo is reused.

The extraction is the committed architecture, not a fallback spike: MAP-14 must produce the real artifact, build recipe and measurements. An alternative engine still requires an explicit revised ADR; never silently substitute MapLibre, a WebView or a proprietary SDK.

This is an authorized planning extension beyond the old EV data/cost plan, which explicitly excluded route feasibility. Keep EE/MEX completed code and evidence; do not rename old pending obligations as accepted or restart all of EE-00–31. iOS/CarPlay stay archived. Source merge, staging acceptance, device validation and production RELEASE_CERTIFIED are different results.

## Verified starting points and reuse map

- Current Android tabs are in `app/src/main/kotlin/com/anpfuel/app/navigation/AbastevoBottomBar.kt`; `TabNavigation.kt` deliberately opens base routes with singleTop and without restoring child stacks. Update the IA rather than adding a fourth item outside the navigation contract.
- `domain/.../discovery/ServerStation.kt` already carries canonical UUID, optional coordinates and `unknown`, `city-centroid`, `reviewed` quality. `DirectoryStationRepository.kt` already has directory/city/search/nearby/detail transport. Its cache is network/last-good memory, not a durable complete offline map database.
- Existing ANP acquisition/import is partly Android-side (`AnpFileDownloader`, `PriceTableBatchImporter`, product/date mappers). The requested server information chain is an extension: do not pretend all ANP prices already travel Rust -> Go, and do not remove the working importer until parity/migration is proven.
- `tools/station-prep` already contains `prepare_registry`, `prepare_charging`, `prepare_vehicle` and EV envelope/types. Preserve its local-file/no-network/no-DB deterministic boundary. Go owns remote acquisition and publication. The CoMaps generator is a separately pinned native build tool; map generation is not reimplemented in Rust.
- ChargingSnapshot/Compatibility/Freshness, Room charging import, ChargingDiscoveryRepository, community outbox/screens and private ledger exist. The latest documented runtime gaps include unmounted charging command/history/rank handlers, incomplete charge read/delete selectors and unpublished charging catalog. Reproduce on the actual starting head before deciding the fix still applies.
- Use `docs/mobile/UI_CONTAINER_GUIDELINES.md` and `AnpContainerDefaults` for content cards; preserve the app theme, spacing, typography, sheets and existing state treatments. Do not outline every Box, marker, button or navigation bar.
- The real Git remote is currently `git@github.com:AlexandreZanata/Abastevo.git`; backend module import paths still use the historical brazil-fuel-prices name. Verify origin before any delivery; do not infer repository identity from a Go import string.

## Commercial-use and open-resource policy

New runtime/build dependencies and reference inputs must be open-source/open-data with commercial-use rights appropriate to actual copying, redistribution and offline storage. A free endpoint, public website or open protocol is insufficient. Use the [source register](MAP_EV_SOURCE_REGISTER.md) and record exact revision/license evidence before admission. Unknown/NC/no-derivatives/offline-prohibited inputs cannot enter the shipped artifact.

Preserve MIT project notices; ship Apache notices and third-party obligations for actually included CoMaps code. OSM data remains ODbL with required attribution and derived-database obligations; code licensing stays distinct. Isolating tables/layers helps provenance but is not by itself a decision about ODbL derivative databases. Provide the required source/derived-data offer, and document whether the overlay is an independent/collective/derived database based on the actual integration. Never erase source IDs to obscure derivation.

The default design does not require Google Maps/Places, Mapbox proprietary services, PlugShare scraping, paid-only operator feeds, Open-Meteo's noncommercial free API, cloud voice, Google location services, public tile bulk download or a public Nominatim autocomplete. No upstream CoMaps CDN or public demo server is an assumed production entitlement. Host admitted regional assets yourself through the existing infrastructure; generating and serving open data still has CPU/storage/traffic costs.

No new source account, purchase, partner outreach or public OSM write is authorized by the plan. If a required resource is not admitted, stop only dependent work and present concrete licensing/technical evidence. Optional PBE/manufacturer/live-operator inputs cannot become mandatory blockers for manually entered vehicle data and open static discovery.

## CoMaps-inspired capability disposition

Adopt or adapt for this product: regional offline maps and updates; pan/zoom/compass/location controls; local address/place search; driving route preview and stops; offline guidance, voice and recalculation; private favorites and saved trips; safe KML/GPX exchange; optional explicit local track recording; night styling and accessibility; custom station overlays and useful nearby amenities. These are product requirements to implement and test, not claims of current Abastevo support.

Replace generic reviews/editor screens with the existing Abastevo community, moderation and correction flows. ANP and community prices are Abastevo layers, never upstream CoMaps facts. Prevent duplicate conflicting fuel/charging POIs in the native reference layer and the overlay; preserve reference-only OSM points honestly when unlinked. Custom POI selection/overlay/native hit testing is part of ENGINE_GATE.

Exclude dedicated hiking/cycling modes, transit/subway planning, offline encyclopedia/tourism articles, social feeds, external review imports and an upstream OSM write editor. Terrain/elevation is an energy-model input where admitted, not a hiking UI. Android Auto is a separate optional client/host acceptance unit; phone functionality remains independent. No iOS or CarPlay work, charging payments/reservations/OCPP control, OEM telemetry, traffic forecast or guaranteed safe arrival is included.

Upstream references, researched 2026-10-24: [README](https://github.com/comaps/comaps/blob/main/README.md), [recent feature differences](https://www.comaps.app/support/how-do-the-features-differ-from-those-of-organic-maps/), [license](https://github.com/comaps/comaps/blob/main/LICENSE), [notices](https://github.com/comaps/comaps/blob/main/NOTICE), [build guide](https://github.com/comaps/comaps/blob/main/docs/INSTALL.md), [map-server model](https://www.comaps.app/support/how-can-i-host-a-custom-map-server-for-downloads/). Codeberg is upstream; GitHub was the accessible official mirror used for source inspection. Pin and reconcile the actual SHA at MAP-00; this plan does not invent a pinned upstream SHA.

## Mandatory behavior rules to formalize at MAP-01

- **B-BR-MAP-01 identity/location:** a pin requires admitted usable public geometry with known quality. City centroids and missing coordinates do not become exact station pins. Approximate OSM candidates remain visibly distinct from reviewed locations; car navigation requires usable road access or an explicit precision warning. Display point, entrance and road snap are separate.
- **B-BR-MAP-02 price source:** ANP mode contains individual station facts only; Comunidade contains the community projection only. Changing the selector cancels/invalidates older results and updates pins, sheet, list and cache identity together. No silent fallback between sources.
- **B-BR-MAP-03 money/units:** preserve exact monetary values, fuel taxonomy, sale units, tariff conditions, dates and reference periods. Municipal averages, wholesale prices, LPG cylinder price and electric kWh/min/session charges are never relabeled as a station's per-liter price. Unknown is not zero/free.
- **B-BR-MAP-04 electric ANP behavior:** chargers stay discoverable under ANP; the sheet explains the absence of ANP electric tariffs and offers explicit community selection. Source mode and service-type filter are independent.
- **B-BR-MAP-05 service types:** fuel, charging and combined sites have stable identities and distinct symbols. A combustion marker does not certify fuel quality; an electric marker does not certify current availability. Preserve unmatched open reference POIs as reference-only points with no invented ANP/community linkage.
- **B-BR-MAP-06 clocks:** geometry, prices, reference status, community working reports and future authorized live status have separate source/observed/published/fetched clocks and validity rules. Cached availability never masquerades as current.
- **B-BR-MAP-07 rights:** absent/mismatched/revoked source rights block publication. Keep attribution visible offline and with exports. Source provenance and public-data licensing obligations survive deduplication and reprocessing.
- **B-BR-MAP-08 privacy:** public POI coordinates are distinct from private home locations, precise user location, trips, favorites and charge history. Public reads are anonymous; signed writes and owner-private reads require existing proof. No telemetry of raw coordinates/routes or implicit background recording.
- **B-BR-MAP-09 persistence:** map binaries and public overlays activate atomically with compatible generations and dependency sets. Last good data survives crash/cancellation/disk-full; erasure/account revocation must survive private restores. No destructive Room fallback.
- **B-BR-MAP-10 route truth:** roads, restrictions and entrances govern routing, not straight-line distance. Missing regional packages and unknown access appear explicitly. No live traffic claim without an admitted actual feed.
- **B-BR-MAP-11 EV trip truth:** compatibility, useful battery, initial SOC, consumption, reserve and road detours govern stop estimates. A failed-primary alternative must be reachable under stated assumptions. Insufficient evidence is shown; no guaranteed-arrival badge.
- **B-BR-MAP-12 accessibility/free use:** retain accessible result list, 48dp controls, 200% text, labels, light/dark/dynamic themes and independent icon shapes. Basic discovery and community remain free under existing contracts; no paid trust, contribution privilege or safety-warning paywall.
- **B-BR-MAP-13 distribution security:** verify authenticated manifests and package digests, constrain paths/hosts/bytes, refuse mixed formats or missing dependencies, and preserve private media permissions. Package deletion cannot race an active route.
- **B-BR-MAP-14 native boundary:** preserve com.anpfuel, MIT notices and current modules. Keep native C++/JNI/version details behind ports; no parallel canonical account/station/price database and no unreviewed proprietary SDK.

Existing EV preparation limits remain unchanged (including its 100 MiB and census limits). Large OSM inputs/native map files require their own versioned streaming and disk/memory budgets; do not silently raise EV ceilings or truncate data to fit. The proposed artifact contract must explicitly document this separation.

Core BUCs: MAP-01 anonymous browse/no GPS; MAP-02 source/product/period switch; MAP-03 pin -> quick price -> full profile; MAP-04 public contribution offline/recovery; MAP-05 regional download/update/rollback; MAP-06 offline search/route/guidance; MAP-07 electric plan/failed primary/alternative; MAP-08 private save/export/backup/erase; MAP-09 source revocation/moderation; MAP-10 upgrade/regression and optional car host lifecycle. Write preconditions, success, unknown and failure cases before behavior changes.

## Architecture and format decisions

Two independent public products share admission/provenance: **native reference maps/search/roads** and **Abastevo station identity/price/community overlays**. Prices must not require rebuilding the native routing graph. Keep public overlays independently versioned and licensed; determine actual database legal obligations instead of assuming independence from file layout.

Go acquires pinned sources with existing allowlists/timeouts/size rules, runs trusted local Rust preparers and the separately pinned native generator under budgets, validates all prepared artifacts and publishes transactional PostGIS snapshots plus immutable compatible map packages. Do not load raw Brazil data into the application or run source scraping on a marker tap.

The Android adapter opens compatible CoMaps-format resources and maps native events to app/domain ports; Room stores overlay generations and metadata, not giant map blobs. Kotlin presents existing Compose controls/sheets and an accessible result list. The native engine provides offline search/roads/routing. Public spatial server queries/updates complement the locally downloaded coverage; route positions and histories need not leave the device.

Do not combine CoMaps MWM resources with a MapLibre/PMTiles renderer as if formats were interchangeable. Source or generator resources such as overview maps, coastlines, world search, neighbor connectivity, styles, fonts, voices and region indexes must be inventoried for the selected SHA. Cross-region navigation depends on a coherent required set; arbitrary clipping of one corridor is not assumed supported by the generator.

Proposed API/resource names in task code are not frozen endpoints. Check existing contracts, then version only required additions. Every cursor/cache key includes all relevant filters and source/generation context. Geometry queries must be indexed/bounded and price reads batched. Location quality stays visible even when the marker looks polished.

## How a less capable agent must execute

Read the selected phase completely before coding. Execute one bounded task at a time. If a phase is too broad, split it into MAP-NN-T01/T02/T03 with independent acceptance and commit each tested task. Never mark the whole phase done after implementing only its first bullet.

For each task: inspect actual diff/history -> document B-BR/BUC/risk -> reproduce meaningful failing behavior -> implement the smallest necessary change -> run affected behavior/critical tests -> static/compile/format checks -> git diff/secret review -> evidence with tested tree/commands/results/remaining limits -> atomic English commit -> normal dev backup push only when authorized -> continue. No per-phase PR/CI wait. Documentation-only tasks need link/structure/source checks, not a fabricated full app test run. Preserve all other agents' files.

Evidence states: PLANNED, IN_PROGRESS, LOCAL_DONE / INTEGRATION_PENDING, INTEGRATED, EXTERNAL_PENDING, WIKI_PENDING and RELEASE_CERTIFIED are not synonyms. A phase exit requiring actual native behavior cannot pass on compiled test APKs or mock engine events alone. User/manual/device/staging/host proofs that were not run stay named and OWED. Optional MAP-30 host evidence does not block phone tasks but cannot be included in a full parity claim.

Do not automate `checkpoint/start --phase` against the old branch lifecycle without checking ADR-020; maintained dev policy wins. Actual helper commands and test names must be read from current code. A proposed `new module` path below is a design destination, not an existing executable target. Never guess a Gradle module, generator argument or migration sequence and then weaken validation when it fails.

## Sequential phases (MAP-00–33)

Every phase below is PLANNED. References to PASS describe future exit requirements, not outcomes from this planning turn. The JSON registry is a task-selection aid, never an auto-approval or proof generator.

### MAP-00 — Reconcile the actual baseline and upstream inventory

Scope: Read-only inventory and bounded execution setup.

Dependency: audited baseline.

Implementation steps:

1. Read AGENTS, FAST_EXECUTION, current PROGRESS, DELIVERY_WORKFLOW, this plan, EE/MEX evidence and actual changes. Record tested source, deployed source and device obligations separately.
2. Use maintained dev synchronized normally from main. If occupied, use a managed isolated worktree and coordinate its eventual integration; never stash, reset or overwrite another task. Reconcile stable task IDs before creating trackers.
3. Inventory every relevant CoMaps capability at a pinned upstream SHA, actual native/build entry points, package format, notices and third-party dependencies. Document kept, adapted and excluded capabilities. Do not assume an Android SDK exists.

Required evidence:

- Verify inventory against real code, not just an old checklist.
- Record actual SQL and Room versions; never allocate migration numbers from this plan.

Exit: **A scoped source/feature/dependency inventory identifies the next task and source revisions.**

Code/document targets to verify: `docs/planning/PROGRESS.md`; `docs/planning/MAP_EV_PHASES.json`; `docs/adr/020-maintained-dev-main-delivery.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-00.md`. Commit topic: `docs(maps): reconcile the actual baseline and upstream inventory`; split broad phases into tested task commits.

### MAP-01 — Freeze behavior, source rights and commercial admission

Scope: Contracts only; no UI and no source activation.

Dependency: MAP-00.

Implementation steps:

1. Write B-BR-MAP and BUC-MAP cases from the mandatory rules below. Freeze price-source switching, marker eligibility, offline expiry, permissions and navigation behavior.
2. Complete the source register: exact bytes/revision, code/data/artwork licenses, commercial use, offline redistribution, attribution, derived-database obligations and access terms. Unknown rights means not admitted.
3. Record ADR-022 for the committed CoMaps-derived native engine extraction: pinned source and patch set, Abastevo-owned reproducible AAR/build recipe, narrow `MapEngine` port, app-owned UI/data/station identity, admitted style/font/symbol assets, no CoMaps app/brand/CDN, and ENGINE_GATE as the pre-UI proof. Native C++/JNI is a declared exception to the Rust preparer language, not a backend rewrite. An alternative engine requires an explicit revised ADR; never silently substitute MapLibre or a WebView.

Required evidence:

- Contract fixtures cover ANP-only, community-only, unknown prices, combined sites and missing coordinates.
- Review transitive CoMaps licenses and upstream service endpoints independently from Apache-2.0 core.

Exit: **Behavior and admission rules are versioned; no required input is silently unlicensed.**

Code/document targets to verify: `docs/product/MAP_DISCOVERY_CONTRACT.md (new)`; `docs/planning/MAP_EV_SOURCE_REGISTER.md`; `docs/adr/ (new numbered decision)`.

Evidence destination: `docs/backend/evidence/maps/MAP-01.md`. Commit topic: `docs(maps): freeze behavior, source rights and commercial admission`; split broad phases into tested task commits.

### MAP-02 — Close the existing electric runtime gaps

Scope: Reuse EE/MEX components; repair composition and incomplete selectors.

Dependency: MAP-01.

Implementation steps:

1. Reproduce actual missing charging command, owner, history and rank handlers with process-level tests, then register existing components using narrow ports and original feature gates.
2. Reproduce private charge read/delete failures; complete dataset selectors without changing refuel/expense behavior, ownership or idempotency.
3. Reconcile EE-22/24/25/26-31 obligations. Reuse passing implementation and name outstanding source, scenario, backup and physical proofs; do not restart the old plan or claim full EV acceptance.

Required evidence:

- Actual Go API process + disposable PostGIS: signed writes, owner read/delete, foreign owner, replay, disabled gate and failures.
- Affected Go race suites, SQL/migration tests immediately if changed; Kotlin wire regressions if affected.

Exit: **Current basic electric routes work on the local real API; preserved gaps have explicit owners and dependencies.**

Code/document targets to verify: `backend/cmd/api/main.go`; `backend/internal/modules/charging/`; `backend/internal/modules/ledger/adapters/store.go`; `docs/backend/evidence/TEST_ENV_UPDATE_2026-10-24.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-02.md`. Commit topic: `feat(maps): close the existing electric runtime gaps`; split broad phases into tested task commits.

### MAP-03 — Define map artifacts and cross-language contracts

Scope: Versioned schemas and fixtures before producers/consumers.

Dependency: MAP-02.

Implementation steps:

1. Define map POI, assertion provenance, station/charging aliases, exact price observations, geometry quality, clocks and capability flags. Reuse existing UUID and EV types.
2. Define a map prepared envelope compatible with the existing manifest/hash/census discipline; reuse charging-prepared-v1 where applicable. Separate OSM-derived reference packages, public first-party observations and private state.
3. Define native-map manifest fields: engine/upstream SHA, generator SHA, format/data generation, regions/neighbors, bytes/digests, source rights and attribution. Placeholder field names become frozen only with schema tests.

Required evidence:

- Rust/Go/Kotlin goldens for bytes, units, IDs, unknowns and schema rejection.
- Reject overflow, duplicate keys, wrong hash, wrong rights/profile, truncated streams and unsupported versions.

Exit: **A real golden corpus can be consumed without ambiguous units or fabricated coordinates/prices.**

Code/document targets to verify: `contracts/ (new map schemas/testdata)`; `docs/product/EV_PREPARATION_CONTRACT.md`; `docs/product/EV_TYPED_DATA_CONTRACT.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-03.md`. Commit topic: `docs(maps): define map artifacts and cross-language contracts`; split broad phases into tested task commits.

### MAP-04 — Prepare OSM reference and location candidates in Rust

Scope: Regional OSM data, fuel/charging POIs and access candidates.

Dependency: MAP-03.

Implementation steps:

1. Use a rights-admitted pinned regional PBF/extract from OSM/Geofabrik; fetching stays in the Go acquisition boundary. Rust parses local bounded files without network or DB.
2. Extract nodes/ways/relations for fuel and charging with names, addresses, source IDs, connectors, access, hours and geometry provenance. Preserve unknown tags and distinguish equipment from site; do not trust arbitrary tags as live state.
3. Emit deterministic entities, decisions, aliases and full census. Missing coordinates remain missing; a city centroid is never a station entrance. Validate coordinate ranges and country/region consistency without deleting legitimate border cases.

Required evidence:

- Streaming limit and malformed PBF/tag tests, Unicode, duplicate IDs, nested children and cancellation/old-output preservation.
- Known synthetic node/way/relation fixtures and actual regional sample qualification outside Git.

Exit: **Every input unit is accounted for; the last good output survives failure; OSM licensing accompanies the artifact.**

Code/document targets to verify: `tools/station-prep/ (bounded extension)`; `contracts/testdata/ (synthetic map fixtures)`.

Evidence destination: `docs/backend/evidence/maps/MAP-04.md`. Commit topic: `feat(maps): prepare osm reference and location candidates in rust`; split broad phases into tested task commits.

### MAP-05 — Prepare exact per-station ANP price facts in Rust

Scope: Extend price ingestion without replacing the existing Android ANP importer.

Dependency: MAP-04.

Implementation steps:

1. Inspect current CSV/XLSX and local import contracts first. Add the missing server-side preparer only where necessary; never assume prepare_prices exists.
2. Normalize actual station-level survey facts with observed date, publication/survey period, product, sale price, unit, CNPJ/address identity and source reference. Reuse product taxonomy and exact decimal rules.
3. Keep city/state aggregates separate. Quarantine ambiguous station links and unsupported units; never attach municipal mean to a station or manufacture an ANP premium/electric price.

Required evidence:

- Brazilian decimal/date/encoding/leading-zero CNPJ fixtures, duplicate/correction/out-of-order survey rows and exact unit tests.
- Cross-language comparison with the current Android importer; existing historical price behavior remains valid.

Exit: **ANP station facts have a traceable source and deterministic identity; unchanged current import behavior is proven.**

Code/document targets to verify: `tools/station-prep/`; `data/src/main/kotlin/com/anpfuel/data/local/importing/PriceTableBatchImporter.kt`; `data/src/main/kotlin/com/anpfuel/data/mapper/AnpProductMapper.kt`.

Evidence destination: `docs/backend/evidence/maps/MAP-05.md`. Commit topic: `feat(maps): prepare exact per-station anp price facts in rust`; split broad phases into tested task commits.

### MAP-06 — Prepare admitted charging and vehicle reference data

Scope: Reuse electric producers and admit only open commercial-use subsets.

Dependency: MAP-05.

Implementation steps:

1. Reuse prepare_charging, ev_ocm and trusted profiles; select only individually admitted providers/records, including redistribution and attribution. opendata flags alone do not bypass the rights manifest.
2. Normalize place/equipment/connector, declared power, access and independent status clocks. Unknown tariff or status is not free or available. Do not transform OCM comments/photos into automatically reusable Abastevo content.
3. Reuse prepare_vehicle only for admitted reference inputs. PBE/manufacturer data remain optional until rights/variant matching are established; manual useful capacity/consumption enables core travel without proprietary catalogs.

Required evidence:

- Existing EV preparation suites plus mixed-rights, unknown connector, missing power, child census and exact tariff-unit cases.
- No contributor PII, precise contributor GPS or photos in public artifacts.

Exit: **An admitted regional charging batch can be produced with unknowns intact; optional blocked catalogs do not falsify readiness.**

Code/document targets to verify: `tools/station-prep/src/bin/prepare_charging.rs`; `tools/station-prep/src/ev_ocm.rs`; `tools/station-prep/src/bin/prepare_vehicle.rs`; `contracts/ev/preparation-profiles-v1.json`.

Evidence destination: `docs/backend/evidence/maps/MAP-06.md`. Commit topic: `feat(maps): prepare admitted charging and vehicle reference data`; split broad phases into tested task commits.

### MAP-07 — Resolve identities, co-located services and navigation entrances

Scope: Explicit reconciliation instead of distance-only merges.

Dependency: MAP-06.

Implementation steps:

1. Keep Directory station UUIDs and charging identities; add aliases/relationships with source, method and reversible review state. Preserve alphanumeric CNPJ and leading zeros.
2. Use CNPJ/address/network/geometry as evidence, not automatic proof. Nearby competing stations or multiple chargers in a shopping center must stay distinct.
3. Distinguish display point from road-access entrance and location quality. A combined establishment can expose both fuel and charging without duplicate selectable prices or fictional co-location.

Required evidence:

- Near-neighbor false merge, CNPJ change, franchise, address ambiguity, stale correction and split/unlink tests.
- Real PostGIS concurrent reconciliation/review/rollback tests when storage is added.

Exit: **No unsafe identity/price/location conflation; reviewed and unresolved relationships remain distinguishable.**

Code/document targets to verify: `backend/internal/modules/directory/`; `backend/internal/modules/charging/`; `domain/src/main/kotlin/com/anpfuel/domain/discovery/ServerStation.kt`.

Evidence destination: `docs/backend/evidence/maps/MAP-07.md`. Commit topic: `feat(maps): resolve identities, co-located services and navigation entrances`; split broad phases into tested task commits.

### MAP-08 — Build the native map-generation toolchain

Scope: CoMaps-compatible map assets; no Android UI yet.

Dependency: MAP-07.

Implementation steps:

1. Pin the audited CoMaps SHA, patch set and toolchain selected for the MAP-14 engine, plus admitted source data. Verify the actual generator CLI and required world/regional/search/routing/style resources before writing wrappers.
2. Build the generator as a versioned, reproducible build-time tool from the pinned source; run it alongside the Rust-prepared POI stream. Do not reimplement its map format in Rust. Go orchestrates acquisition, bounded tool execution and publication.
3. Produce a consistent regional map set with routing/search and neighbor/overview dependencies, including the admitted style/font/symbol assets that the MAP-14 artifact expects. Separate immutable map packages from frequently updated Abastevo prices; no full national build on the shared VPS by default.

Required evidence:

- A clean reproducible tool build and regional generation; verify format and dependency manifest against pinned engine documentation.
- Corrupt/truncated input, process timeout, traversal, resource exhaustion and atomic previous-generation preservation.

Exit: **A self-produced, rights-admitted regional package set exists with exact generator/data hashes and required dependencies.**

Code/document targets to verify: `tools/ (new pinned map-generation integration)`; `scripts/ (build-time commands after verifying upstream CLI)`; `docs/planning/MAP_EV_SOURCE_REGISTER.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-08.md`. Commit topic: `feat(maps): build the native map-generation toolchain`; split broad phases into tested task commits.

### MAP-09 — Import and publish map assertions transactionally in Go

Scope: Prepared artifacts to existing PostgreSQL/PostGIS modules.

Dependency: MAP-08.

Implementation steps:

1. Extend existing acquisition/native bridge and publication patterns. Validate all hashes/profiles/census before publishing and keep raw/public/private storage boundaries.
2. Stage assertions and aliases; publish one consistent generation atomically. Use existing module ports and jobs; no speculative microservice or second canonical station table.
3. Allocate append-only migrations from actual HEAD; include source/rights references, geometry indexes and recovery. Support safe replay, cancellation and old-reader compatibility.

Required evidence:

- Mandatory real PostGIS empty install, previous-schema upgrade, replay, two writers, cancelled import, rollback and crash recovery.
- Go race + parameterized SQL/static/sqlc checks; unauthorized/malformed prepared input refused.

Exit: **Failed ingestion cannot partially replace the public catalog or lose existing station/user data.**

Code/document targets to verify: `backend/internal/modules/directory/`; `backend/internal/modules/charging/`; `backend/db/migrations/ (next version verified from actual HEAD)`.

Evidence destination: `docs/backend/evidence/maps/MAP-09.md`. Commit topic: `feat(maps): import and publish map assertions transactionally in go`; split broad phases into tested task commits.

### MAP-10 — Serve bounded spatial discovery and exact price projections

Scope: Viewport/nearby/corridor and quick-detail API contracts.

Dependency: MAP-09.

Implementation steps:

1. Extend current Directory and charging APIs through contracts; add missing bounded spatial endpoints only after checking reuse. Query by viewport/zoom or route corridor with appropriate GiST indexes and stable ordering.
2. Expose marker type, location quality, source-separated selected-fuel prices, time/period, known/unknown state and attribution. Apply query/response/area limits, bounded pagination or server aggregation; never return all Brazil.
3. Bind cursor/cache identity to area, source mode, product, survey period, filters, generation and permissions. Batch price loading; no per-pin N+1. Public reads remain anonymous; writes/owner reads stay signed.

Required evidence:

- PostGIS actual query plans/load fixtures, antimeridian/invalid bbox, huge corridor, pagination tamper, stale generation and timeout/cancellation.
- Price-source, product/unit/condition isolation and stale response generation tests.

Exit: **A bounded API returns honest individual prices and enough metadata for map selection without leaking private data.**

Code/document targets to verify: `backend/internal/modules/directory/`; `backend/internal/modules/charging/`; `backend/internal/platform/apicontract/`; `contracts/openapi/`.

Evidence destination: `docs/backend/evidence/maps/MAP-10.md`. Commit topic: `feat(maps): serve bounded spatial discovery and exact price projections`; split broad phases into tested task commits.

### MAP-11 — Apply freshness, moderation and source lifecycle rules

Scope: Different lifetimes for geometry, price and operational observations.

Dependency: MAP-10.

Implementation steps:

1. Reuse existing refresh/consensus/moderation contracts. Add operational events only through new explicit rules; no invented fixed expiry or confidence percentage.
2. Keep observed_at, published_at, fetched_at and source generation distinct. Revocations/corrections/deletions propagate through snapshots and tombstones without rewriting history.
3. OCM/OSM static status, community reports and authorized dynamic data remain separate; the open-only launch has no assumed operator live feed. Kill switches and rollback retain the last admitted generation.

Required evidence:

- Clock-boundary and malformed/future date cases; stale/withdrawn/retracted values and mixed-generation negative tests.
- Concurrent reports, moderation removal, cursor/cache invalidation and signed owner operations on real PostGIS.

Exit: **A static/cached point never becomes a fictional live availability or fresh price.**

Code/document targets to verify: `backend/internal/modules/charging/`; `domain/src/main/kotlin/com/anpfuel/domain/charging/ChargingSnapshotFreshness.kt`; `docs/product/MAP_DISCOVERY_CONTRACT.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-11.md`. Commit topic: `feat(maps): apply freshness, moderation and source lifecycle rules`; split broad phases into tested task commits.

### MAP-12 — Publish regional manifests and recoverable download contracts

Scope: Public open data distribution with explicit access/integrity rules.

Dependency: MAP-11.

Implementation steps:

1. Expose versioned regional manifests and public immutable packages through existing Go/Compose/Caddy boundaries, separate from private S3 media. Do not make the private media bucket public.
2. Use HTTPS, verified digests and authenticated manifest integrity (signed metadata with app-pinned verification key), fixed allowed origins/path IDs, size limits, resumable Range/ETag semantics and key rotation.
3. Advertise engine-format compatibility and complete dependency sets. Stage files, verify all, atomically activate; retain rollback and reclaim policy. Open-license source/derived-database offers remain accessible as required.

Required evidence:

- Manifest/signature tamper, bad digest, changed ETag/range, disk full, cancelled transfer, missing neighbor/world files and old app compatibility.
- HTTP actual-process tests, invalid path/redirect/SSRF controls and recovery to last valid package.

Exit: **Downloads have a testable integrity and recovery protocol; public map delivery cannot expose private objects.**

Code/document targets to verify: `backend/ (existing download/publication boundaries)`; `infra/ (actual existing Caddy/Compose scope)`; `contracts/ (map distribution manifests)`.

Evidence destination: `docs/backend/evidence/maps/MAP-12.md`. Commit topic: `feat(maps): publish regional manifests and recoverable download contracts`; split broad phases into tested task commits.

### MAP-13 — Prove the data pipeline before Android map work

Scope: Hard DATA_GATE with real prepared bytes, process and local client decoding.

Dependency: MAP-12.

Implementation steps:

1. Execute admitted local input -> actual Rust executable/native generator -> real Go import/PostGIS -> running HTTP API -> Kotlin decoder fixture/Room import contract. No fake HTTP response may stand in for the real Go output.
2. Cover one fuel station with per-station ANP price, community-only station, charging site, combined site, no-price point, unknown/missing location, disputed identity and blocked-license record.
3. Record source/rights/preparer/generator/schema/API bytes and generation. Physical Android interaction is later; exact Kotlin decoding and local DB behavior must pass now.

Required evidence:

- End-to-end all named fixtures with exact IDs/prices/units and rejected inputs; real source process failures and old-generation survival.
- Rust/Go/Kotlin contract checks and SQLite migration tests; required tests must execute rather than skip.

Exit: **DATA_GATE PASS: data is consumable and legally admitted before adding the map tab.**

Code/document targets to verify: `docs/backend/evidence/maps/MAP-13.md`; `contracts/testdata/`; `backend/README.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-13.md`. Commit topic: `feat(maps): prove the data pipeline before android map work`; split broad phases into tested task commits.

Hard gate: **DATA_GATE**.

### MAP-14 — Extract, package and own the pinned CoMaps-derived engine

Scope: Committed extraction, not an optional spike. Build the actual Abastevo-owned map artifact: a pinned, audited CoMaps-derived native core packaged by us behind a narrow Kotlin/JNI port. No CoMaps app, upstream SDK contract, CoMaps UI or upstream CDN. ENGINE_GATE still proves it before any map UI depends on it.

Dependency: MAP-13.

Fixed architecture: Abastevo owns the module layout, build recipe, public Kotlin surface, app identity, Compose UI, station identity and data. The native core owns rendering, compatible map packages, offline search, routing and guidance. The base/reference layer is OSM roads/POIs from the generated packages; the Abastevo overlay is stations, prices, charging and community data from MAP-10. Upstream source is vendored at an exact SHA with a small documented patch set, never consumed as an unpinned Gradle dependency.

Implementation tasks (split into tested commits; all are required for the gate):

**T01 — Pin, vendor and make the build reproducible**

1. Select and record the exact CoMaps upstream commit/tag; vendor only the minimal core/JNI host and required submodules, not the full Android application, under a dedicated path (`third_party/comaps/` or the module name frozen in T02).
2. Pin NDK, CMake, Qt-skin/generator toolchain and every included submodule. Preserve LICENSE/NOTICE/copyright, mark Abastevo patches, and draft the SBOM/third-party notice for exactly included code, fonts, styles, symbols, voices and data.
3. Build twice from a clean checkout in the reproducible builder image; record source/toolchain/artifact hashes. No CoMaps logo, brand asset or upstream service credential is copied.

**T02 — Freeze the Abastevo `MapEngine` port**

1. Define the Kotlin port and value types independent of JNI: initialize/configure, region open/close, camera, overlay/markers, selection, search, route, guidance events, cancellation and release.
2. Freeze ownership: Kotlin owns Activity/Compose lifecycle, application id, FileProvider, storage directories, permissions, location source and threading; native owns renderer, map package reader, search index and routing graph. No native type leaks into `domain` or `data`.
3. Define errors and generations for missing/corrupt/incompatible packages, unsupported ABI, init failure, route failure, stale callbacks and teardown. Write the module/package naming decision in the ADR.

**T03 — Produce and integrate the owned AAR/artifact**

1. Build our AAR (or bounded module artifact) with release `arm64-v8a` and supported `armeabi-v7a`; `x86_64` only for debug/emulator. Enforce minSdk 26, native 16 KB page-size, stripped symbols, no Qt runtime and no debug-only assets.
2. Package only the admitted minimal style/font/symbol/voice set, with a manifest; no upstream CDN, demo server or key is a runtime dependency.
3. Integrate the artifact into the real app build without replacing `com.anpfuel`, application/navigation/DI roots or existing modules; add only the required consumer/R8 keep rules for the JNI surface.
4. Record AAR/APK size delta, build time and memory/disk baseline; declare a provisional budget. Exceeding it needs a recorded decision, not a silent waiver.

**T04 — Implement the JNI host adapter and lifecycle**

1. Implement initialization, resource/storage paths, FileProvider, location callback and lifecycle (create/start/resume/pause/stop/destroy, rotation, process death) behind the port.
2. Confine callback threads; never leak Activity, Context or location listeners. Provide explicit teardown and no hidden/background GPS.
3. Test double init, missing/corrupt resources, invalid paths, permission denied, stale callbacks, rotation and process recreation.

**T05 — Prove render, reference layer, offline search and route**

1. Load an admitted regional package set and render the Abastevo-owned light/dark style with visible OSM attribution; prove pan/zoom, compass/recenter and no network requirement.
2. Prove offline place/address/category search over the loaded region and one offline car route with instructions using roads/entrances and restrictions, not straight-line distance.
3. Verify labels, road classes, fuel/charging reference POIs and scale; no missing native libs and no CoMaps branding.

**T06 — Prove the Abastevo overlay and selection**

1. Render a custom overlay fixture with canonical station UUIDs, fuel/electric types and `unknown`/`city-centroid`/`reviewed` location states; hit-test returns the UUID and never merges by proximity.
2. Keep overlay updates independent from map packages: a new overlay generation must not rebuild the routing graph, and the previous generation survives failure. Selection emits an app-owned event that opens the future Compose quick sheet.
3. Prove co-located fuel/charging, duplicate suppression, cluster/overlap behavior, 48dp targets and a TalkBack-equivalent result list.

**T07 — Complete rights, security and update operations**

1. Complete the transitive audit for the exact included code/assets/data; determine ODbL attribution/derived-database obligations; retain the source/derived-data offer and all notices.
2. Scan the artifact for secrets, credentials and upstream endpoints; document offline redistribution and confirm no required private service.
3. Document patch reapplication and rollback for an upstream engine update; a changed SHA or ABI set invalidates the gate evidence.

**T08 — Close ENGINE_GATE with real evidence**

1. On an owned emulator/device when authorized, demonstrate real render/pan/zoom, custom POI selection, offline search and one offline car route; run JNI/ABI/lifecycle checks and measure the declared budgets.
2. Write `docs/backend/evidence/maps/MAP-14.md` with source SHA, patch set, toolchain, artifact hashes, licenses/SBOM, measured costs, negative cases and unresolved limits; record ADR-022.
3. If extraction fails, stop dependent map UI, keep the exact failure evidence and revise this bounded extraction design. Opening an alternative engine requires a new explicit ADR; never silently swap MapLibre, a WebView, Google or an external intent and call it equivalent.

Required evidence:

- Clean reproducible build and the actual owned artifact/AAR; per-ABI, minSdk 26 and 16 KB page-size proof.
- Real native render/search/route and Abastevo overlay selection on an owned emulator/device, plus JNI/ABI/lifecycle tests.
- No missing native libs, no private upstream service/key assumption, no leaked activity/location callback.
- License/SBOM/notices, measured APK/RAM/disk/build budgets and the patch/update recipe.

Exit: **ENGINE_GATE PASS: an Abastevo-owned, reproducible, licensed CoMaps-derived engine artifact and adapter render, search, route and select Abastevo stations offline within measured budgets.**

Code/document targets to verify: `third_party/comaps/` or the bounded engine module (name frozen at T02); `maps/engine-adapter/` (or current module convention); `app/build.gradle.kts`; `gradle/libs.versions.toml`; `docs/adr/022-*.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-14.md`. Commit topic: `feat(maps): extract and package the pinned comaps engine`; use atomic T01–T08 commits.

Hard gate: **ENGINE_GATE**.

### MAP-15 — Add Android public map persistence and download recovery

Scope: Room/managed file storage; no map UI until verified import.

Dependency: MAP-14.

Implementation steps:

1. Persist public map overlay generations, metadata, attribution and download progress using current DB conventions. Store map binaries outside Room; keep a separate private account namespace for routes/favorites.
2. Implement resumable cancel/retry downloads from MAP-12, signature/digest checks, dependency-aware activation, disk budget/quota warnings and safe deletion excluding active files.
3. Import public snapshots atomically; preserve old packages on failure. Reuse existing refresh/WorkManager policy instead of inventing aggressive periodic jobs or requiring login for public maps.

Required evidence:

- Real SQLite old-schema upgrade/rollback behavior, corruption, process death, partial writes, disk full and simultaneous readers.
- WorkManager retry/cancel/account switch and signed-manifest/key rotation tests.

Exit: **An interrupted download/import cannot corrupt the last working public map or reveal account-private state.**

Code/document targets to verify: `data/src/main/kotlin/com/anpfuel/data/local/AnpFuelDatabaseMigrations.kt`; `data/src/main/kotlin/com/anpfuel/data/local/charging/`; `data/src/main/kotlin/com/anpfuel/data/repository/`.

Evidence destination: `docs/backend/evidence/maps/MAP-15.md`. Commit topic: `feat(maps): add android public map persistence and download recovery`; split broad phases into tested task commits.

### MAP-16 — Integrate the engine with Compose lifecycle and location

Scope: Map host and privacy-controlled platform services.

Dependency: MAP-15.

Implementation steps:

1. Wrap the proven native surface in Compose using verified lifecycle hooks; own resource creation/disposal, pause/resume, configuration change and process recovery. Keep domain/data independent of native SDK types.
2. Use Android platform location, foreground permission and approximate-location behavior; browsing/search/manual city selection work without permission or GPS. No new Play Services dependency or background location for simple browsing.
3. Implement compass/recenter and map camera persistence separately from navigation stack. Cancel obsolete requests by generation; never paint a late source/filter response into a new selection.

Required evidence:

- Lifecycle attach/detach/rotation/leak cases; no permission, approximate location, denied forever, GPS off and stale fix.
- Deterministic view-model cancellation tests and native surface behavior evidence.

Exit: **The map host works with existing app lifecycle, permissions and themes without continuous hidden GPS.**

Code/document targets to verify: `app/ (new map host/DI)`; `domain/ (map ports)`; `data/ (platform adapters)`.

Evidence destination: `docs/backend/evidence/maps/MAP-16.md`. Commit topic: `feat(maps): integrate the engine with compose lifecycle and location`; split broad phases into tested task commits.

### MAP-17 — Add the fourth bottom tab and minimal map shell

Scope: Exact requested navigation order and existing UI language.

Dependency: MAP-16.

Implementation steps:

1. Add Mapa between Comunidade and Perfil: Explorar | Comunidade | Mapa | Perfil. Use the existing bottom-bar icon system, localized label, equal-width cells and permanent visible labels.
2. Add a native MAP route/base following navigateToTabBase: single top, no restoration of a hidden child stack. Preserve station deep links and associate details opened from the map with the correct tab/origin.
3. Minimal shell contains the map, ANP/Comunidade selector at the top, fuel/charging filter entry, recenter, search and existing download affordance. Use the Abastevo-owned map style (brand, typography, light/dark, visible OSM attribution), not CoMaps branding or its app chrome. Honor safe areas, app light/dark/dynamic themes and shared container borders; no app-wide redesign.

Required evidence:

- Actual navigation-controller regressions: four tabs/order, repeat tap, back, deep link, return from details and map child routes.
- Compose semantics/48dp targets/200% font/narrow screen; affected app JVM, lint and debug assembly.

Exit: **Four tabs work, and the real map is reachable without disrupting existing Explore/Community/Profile behavior.**

Code/document targets to verify: `app/src/main/kotlin/com/anpfuel/app/navigation/AbastevoBottomBar.kt`; `app/src/main/kotlin/com/anpfuel/app/navigation/TabNavigation.kt`; `app/src/main/kotlin/com/anpfuel/app/navigation/ (Routes/NavHost)`; `docs/mobile/UI_CONTAINER_GUIDELINES.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-17.md`. Commit topic: `feat(maps): add the fourth bottom tab and minimal map shell`; split broad phases into tested task commits.

### MAP-18 — Render fuel and electric markers with clustering and selection

Scope: Use the supplied generated pictograms; do not overload the map.

Dependency: MAP-17.

Implementation steps:

1. Integrate map-icons raw/provenance/SVG/Android exports through the current asset pipeline. Marker backing/selection is Abastevo-owned and uses theme-aware neutral contrast, not hardcoded white on all maps; reference OSM POIs stay visually distinct from Abastevo stations.
2. Render fuel pump and electric bolt/plug as distinct symbols. Co-located services expose both without overlapping duplicate pins. Show selected-product price only when compatible and known; cluster density deterministically.
3. Preserve stable marker IDs across source changes. Fetch bounded viewports, debounce moves, expand clusters and reconcile selection after generation replacement; never place unknown/city-centroid stations as exact pins.

Required evidence:

- 20/24/48/128px light/dark basemap inspection; generated alpha/ink/provenance and silhouette checks.
- Dense synthetic viewport, overlapping sites, repeated pan/source switch, hit targets, TalkBack equivalent list and unknown geometry cases.

Exit: **Users can distinguish, select and reach a real station without depending only on marker color.**

Code/document targets to verify: `docs/assets/map-icons/`; `app/src/main/res/drawable-nodpi/ (future integration)`; `app/ (map overlay adapter)`.

Evidence destination: `docs/backend/evidence/maps/MAP-18.md`. Commit topic: `feat(maps): render fuel and electric markers with clustering and selection`; split broad phases into tested task commits.

### MAP-19 — Show fast station prices and honest source switching

Scope: Map-to-bottom-sheet-to-existing-profile journey.

Dependency: MAP-18.

Implementation steps:

1. On marker tap show name, service type, selected source, exact product/unit prices, survey/report date, access/location quality and actions to route/open full profile/report price.
2. ANP mode shows station-specific ANP observations for the chosen existing survey period; if absent, say no ANP price for this station. Community mode uses the current validated community projection with its date/conditions. No silent fallback/mixing.
3. Charging markers remain discoverable in ANP mode but state ANP does not supply an electric tariff. Offer explicit switch to Comunidade; never disable the source selector silently. No-price stations remain visible. Reuse existing vehicle/product/week selection semantics after defining persistence scope.

Required evidence:

- Racing ANP/community responses, missing price, stale period, wrong fuel/unit, premium-vs-additive and EV-in-ANP tests.
- Real quick sheet/profile route and data contract; no N+1 request fan-out.

Exit: **Tap -> truthful quick prices -> existing profile/route works under both selected sources.**

Code/document targets to verify: `app/ (map quick sheet/view-model)`; `data/src/main/kotlin/com/anpfuel/data/repository/StationPriceRepositoryImpl.kt`; `app/src/main/kotlin/com/anpfuel/app/ui/components/SourceTimeBadge.kt`.

Evidence destination: `docs/backend/evidence/maps/MAP-19.md`. Commit topic: `feat(maps): show fast station prices and honest source switching`; split broad phases into tested task commits.

### MAP-20 — Complete discovery, accessibility and the minimal release gate

Scope: Search/filter/list equivalents before advanced navigation.

Dependency: MAP-19.

Implementation steps:

1. Support current area, city and name/address search, fuel types, compatible connector, AC/DC, known power, access/hours and date. Unknown metadata is shown and is not automatically excluded.
2. Provide equivalent accessible list of visible results, readable sheet order, focus return, map-control descriptions, app-owned dark/light styles and no permission lockout. Store public last map position locally without precise-coordinate telemetry.
3. Prove the minimum product end to end: open Mapa -> choose source/fuel -> see compatible pins -> quick price -> profile -> route action. A user-chosen installed open-source navigator can be an explicit optional interim action, never claimed as completed native navigation.

Required evidence:

- Native/device or user manual checklist with source dates, offline cached overlay, no GPS and 200% text.
- Full affected navigation/UI/static tests and actual server->Kotlin price decode.

Exit: **MINIMUM_MAP_GATE PASS with real map/prices; advanced navigation may remain planned, not hidden as completed.**

Code/document targets to verify: `app/ (map search/filter/list)`; `docs/backend/evidence/maps/MAP-20.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-20.md`. Commit topic: `feat(maps): complete discovery, accessibility and the minimal release gate`; split broad phases into tested task commits.

Hard gate: **MINIMUM_MAP_GATE**.

### MAP-21 — Complete electric community and operational reports

Scope: Signed contributions with offline recovery and honest recency.

Dependency: MAP-20.

Implementation steps:

1. Complete existing electric price/experience/edit/withdraw/vote/report screens by reuse. Add structured operational events per MAP-01, scoped to actual site/equipment/connector.
2. Reuse account proof, outbox, moderation, anti-abuse and ownership. Recover uploads after process death/network loss; contribution remains free. Do not activate photo intake or collection gates to add status reporting.
3. Separate static reference, dated community working reports and unavailable live feed. Show conditions and independent observations without invented scores or paid trust.

Required evidence:

- Go process/PostGIS + Kotlin outbox replay, duplicate/reordered edits, withdrawn/rejected reports, foreign owner and concurrent moderation.
- Account switch/logout, airplane recovery and timestamp boundary cases.

Exit: **A real authorized user can submit/recover/edit/withdraw a report and see the correct public projection.**

Code/document targets to verify: `app/src/main/kotlin/com/anpfuel/app/ui/charging/`; `data/src/main/kotlin/com/anpfuel/data/remote/charging/`; `backend/internal/modules/charging/`.

Evidence destination: `docs/backend/evidence/maps/MAP-21.md`. Commit topic: `feat(maps): complete electric community and operational reports`; split broad phases into tested task commits.

### MAP-22 — Implement offline route planning and search with the native core

Scope: Driving routes, stops and road constraints.

Dependency: MAP-21.

Implementation steps:

1. Expose the verified core search/route APIs through the adapter. Resolve origin/destination by manual input, map point, current fix or saved POI; support intermediate stops and route preview.
2. Use road access/one-way/turn restrictions and permitted avoidance options proven upstream. Compare only routes actually returned; do not invent alternative paths or real-time traffic.
3. Require all regional/neighbor routing dependencies. If missing, offer the explicit package download; no straight-line route presented as drivable. Include station entrance rather than inaccessible centroid.

Required evidence:

- Offline cross-region/neighbor, no route, inaccessible entrance, one-way/divided road, origin=destination and intermediate stops.
- Native route cancellation/lifecycle and route-option contract cases with deterministic regional fixtures.

Exit: **A real offline driving route can be planned, changed and cancelled with explicit missing-map states.**

Code/document targets to verify: `native engine adapter`; `domain/ (routing port/value objects)`; `app/ (route preview)`.

Evidence destination: `docs/backend/evidence/maps/MAP-22.md`. Commit topic: `feat(maps): implement offline route planning and search with the native core`; split broad phases into tested task commits.

### MAP-23 — Implement foreground navigation, rerouting and voice

Scope: Real guided navigation with explicit start/stop and privacy.

Dependency: MAP-22.

Implementation steps:

1. Add maneuver/ETA/distance display, route progress, off-route recalculation and recoverable guidance state using the proven core. Time estimates are estimates without live traffic.
2. Use Android platform foreground-service/location APIs only during explicit active navigation, notification and stop action. Handle audio focus and offline voice using local OS TTS with optional user-selected engine; do not add a cloud TTS dependency.
3. Keep GPS traces on-device only if explicitly saved; no background tracking for browsing, no automatic external telemetry. Handle tunnel/poor GPS, route cancel, call interruption and permission revocation.

Required evidence:

- Synthetic location replay in owned emulator: progress, wrong turn, GPS loss, reroute and stop releases resources.
- Foreground service API-level rules, notification/permission failure, audio focus and no local TTS engine fallback.

Exit: **Native guided navigation works offline, stops promptly and does not imply recording consent.**

Code/document targets to verify: `native engine adapter`; `app/ (navigation service/UI)`; `AndroidManifest.xml (actual existing location/FGS rules)`.

Evidence destination: `docs/backend/evidence/maps/MAP-23.md`. Commit topic: `feat(maps): implement foreground navigation, rerouting and voice`; split broad phases into tested task commits.

### MAP-24 — Finish the regional offline experience and update UX

Scope: Maps, references and prices keep distinct generations and lifetimes.

Dependency: MAP-23.

Implementation steps:

1. Expose region download sizes/dependencies, Wi-Fi preference, pause/cancel/retry, storage controls and active-route safe deletion/update rules.
2. Activate a complete map generation only when engine and region dependencies match; keep last-good fallback and recover from process death. Overlay/reference/prices update separately and retain their dates.
3. Demonstrate offline browse/search/route/guidance/POI detail with downloaded data and no network endpoint contact. Missing regions or prices get explicit actions rather than false coverage.

Required evidence:

- Airplane mode after clean restart, corrupted package, interrupted update, low storage and cross-region journey.
- No stale price/live-status claim, no mixed native map generations and no file deletion while in use.

Exit: **OFFLINE_GATE PASS covers the complete relevant journey, not only cached screenshots.**

Code/document targets to verify: `data/ (downloads/cache)`; `app/ (offline region management)`; `docs/backend/evidence/maps/MAP-24.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-24.md`. Commit topic: `feat(maps): finish the regional offline experience and update ux`; split broad phases into tested task commits.

Hard gate: **OFFLINE_GATE**.

### MAP-25 — Add private favorites, saved trips and safe file exchange

Scope: Useful CoMaps-inspired personal features, no mandatory tracking.

Dependency: MAP-24.

Implementation steps:

1. Save stations and route plans locally with clear account/guest ownership. Reuse existing private backup/sync contracts and deny cross-account restore by default.
2. Implement needed KML/GPX import/export via a reviewed open-source parser or existing primitives; KMZ only with bounded archive handling. Keep original/source IDs and export license notices for public data.
3. Recording a trip is optional explicit opt-in, local by default, with stop/delete/export controls; no automatic recording during normal navigation and no public route upload.

Required evidence:

- Malicious XML/XXE, oversized/recursive zip, traversal, point-count limits and malformed coordinates.
- Account A/B/guest isolation, logout, stale restore, deleted trip resurrection, process restart and signed private backup cases.

Exit: **Personal data is useful and portable without silently publishing home addresses or paths.**

Code/document targets to verify: `domain/ (private saved trip model)`; `data/ (private storage/backup)`; `app/ (favorites/trip screens)`.

Evidence destination: `docs/backend/evidence/maps/MAP-25.md`. Commit topic: `feat(maps): add private favorites, saved trips and safe file exchange`; split broad phases into tested task commits.

### MAP-26 — Build the electric energy model as a pure domain component

Scope: Conservative configurable calculations with explicit assumptions.

Dependency: MAP-25.

Implementation steps:

1. Use useful battery capacity, current SOC, desired reserve and consumption in exact units. Start with manual inputs/calibration from private sessions; PBE remains optional reference only after admission.
2. Calculate usable energy and segment consumption without claiming laboratory range as real range. Declared charger power is only an input bound; charging time depends on vehicle/curve/SOC and unknown components.
3. Define missing-data behavior and sensitivity inputs. Elevation can use admitted OSM/core/SRTM data later in this same scoped task if necessary; climate is not required and no noncommercial API is admitted.

Required evidence:

- Boundary reserve/SOC/capacity, PHEV usable portion, units, rounding/overflow, negative distance/energy and missing-consumption cases.
- Compare independent hand-calculated vectors and monotonic behavior; never count tests that duplicate implementation only.

Exit: **A documented pure model returns interpretable estimates/unknowns, not guaranteed trip feasibility.**

Code/document targets to verify: `domain/ (EV trip energy rules)`; `backend/ (only if authoritative shared calculations are required)`; `contracts/testdata/ (energy vectors)`.

Evidence destination: `docs/backend/evidence/maps/MAP-26.md`. Commit topic: `feat(maps): build the electric energy model as a pure domain component`; split broad phases into tested task commits.

### MAP-27 — Plan charging stops with a reachable alternative

Scope: Road-distance energy feasibility and explicit coverage gaps.

Dependency: MAP-26.

Implementation steps:

1. Find compatible charging candidates along native road routes with bounded offline overlay/corridor data. Account for road detours, access/hours, useful power, tariff conditions and report recency.
2. Plan energy-constrained legs with reserve and charging stops. Test reachability of a fallback when the primary point fails; expose cases with no evidenced alternative instead of asserting safety.
3. Do not auto-route onto private/closed sites, merge incompatible connectors or optimize unknown prices as zero. Keep algorithm cancellation/limits so large queries cannot hang the phone or API.

Required evidence:

- Single isolated charger, failed primary, unreachable alternate, nighttime closure, incompatible plug, unknown power/tariff, road closure uncertainty and offline missing corridor.
- Deterministic route/energy integration vectors and bounded candidate explosion tests.

Exit: **A plan explains primary/alternate reachability and assumptions; insufficient evidence is visible.**

Code/document targets to verify: `domain/ (EV stop planner)`; `native routing adapter`; `data/ (corridor charging lookup)`.

Evidence destination: `docs/backend/evidence/maps/MAP-27.md`. Commit topic: `feat(maps): plan charging stops with a reachable alternative`; split broad phases into tested task commits.

### MAP-28 — Expose trip planning and complete the electric private journey

Scope: Minimal themed screens over proven contracts.

Dependency: MAP-27.

Implementation steps:

1. Add route inputs, vehicle/SOC/reserve/consumption, stop list, alternative details and estimated arrival charge with understandable assumptions. Preserve current profile visual conventions and shared borders.
2. Allow save/download and replan after a failed charger, maintaining provenance/recency offline. No promise that a trip cannot strand the user.
3. Finish charge history/cost/scenario execution and electric backup fields needed by the trip journey. Preserve ICE/free/PHEV accounting and existing entitlement rules; no new paid safety/contribution restriction.

Required evidence:

- Server->Android price/scenario/private session actual flow, offline restart and two-account/two-device backup/restore when available.
- TalkBack, 200% font, invalid inputs, low SOC, no map/no station and account switching.

Exit: **The EV journey works from vehicle and map through plan, alternative, saved trip and private charge history.**

Code/document targets to verify: `app/ (EV trip flow)`; `app/src/main/kotlin/com/anpfuel/app/ui/ledger/`; `data/ (vehicle/backup transport)`; `domain/ (existing cost rules)`.

Evidence destination: `docs/backend/evidence/maps/MAP-28.md`. Commit topic: `feat(maps): expose trip planning and complete the electric private journey`; split broad phases into tested task commits.

### MAP-29 — Measure performance, native compatibility and operational budgets

Scope: Measured constraints before broader geographic rollout.

Dependency: MAP-28.

Implementation steps:

1. Measure cold/warm map load, frame pacing, native/Java memory, APK/ABI size, package/storage use, GPS battery impact, network bytes, spatial query latency and routing cancellation against the MAP-14 declared budgets.
2. Use representative low-resource device/emulator plus dense and sparse synthetic regions; define explicit budgets from product needs and measured MAP-14 baseline before acceptance, never after failures to declare green.
3. Bound map requests/clusters/caches/workers, verify source ingestion/backpressure and public package traffic. Build maps off the shared VPS unless its measured capacity supports the job; no live stress test without scoped authorization.

Required evidence:

- Document hardware/OS/ABI/data/config/commit/commands and repeatable samples; required ABIs, minSdk and 16KB native page-size checks.
- DB concurrency/load/failure, regional generation resource ceilings, leak/pause/resume and cancellation.

Exit: **PERFORMANCE_GATE PASS has declared budgets and actual measurements; national capacity remains unclaimed.**

Code/document targets to verify: `docs/backend/evidence/maps/MAP-29.md`; `backend/ (query/job metrics)`; `app/ (native performance validation)`.

Evidence destination: `docs/backend/evidence/maps/MAP-29.md`. Commit topic: `feat(maps): measure performance, native compatibility and operational budgets`; split broad phases into tested task commits.

Hard gate: **PERFORMANCE_GATE**.

### MAP-30 — Plan and validate optional in-car integration

Scope: Open-source client adapter; external vehicle-host acceptance separate.

Dependency: MAP-29.

Implementation steps:

1. Assess AndroidX Car App Library license/dependency tree for the pinned version; no new closed-source map/navigation SDK or Google location dependency.
2. If admitted, implement a thin optional Android Auto client over the same local map/routes with host-safe controls, navigation intents and correct start/end metadata. Phone navigation must work without the host.
3. Car host/distribution rules, DHU/device availability and certification are external gates. If strict FOSS host requirements cannot be met, retain explicit EXTERNAL_PENDING/excluded platform obligation and do not block core phone features or claim this capability delivered.

Required evidence:

- Navigation service/host lifecycle, stop request, reconnect and no driving-time text-entry regressions.
- Owned DHU/head-unit evidence only when available/authorized; missing host is not a pass.

Exit: **Optional in-car scope and its actual acceptance are explicit; no false parity or external certification claim.**

Code/document targets to verify: `app/ (optional car adapter after admission)`; `docs/backend/evidence/maps/MAP-30.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-30.md`. Commit topic: `feat(maps): plan and validate optional in-car integration`; split broad phases into tested task commits.

### MAP-31 — Run regression, privacy and failure acceptance

Scope: Mandatory risk tests and final local source union.

Dependency: MAP-29; MAP-30 local disposition; external host proof is nonblocking for the phone.

Implementation steps:

1. Prove isolation between public OSM references, public community and account-private routes/history. Review licensing obligations for actual derived databases; physical table separation alone is not a legal determination.
2. Exercise upgrades from actual prior Room/Postgres release, backup/restore/erasure, missing permissions, revoked account, stale source/manifests, source switch races and native failure recovery.
3. Preserve combustion ANP/community, free accounts/social participation, legacy IDs, existing UI/navigation and all critical security gates. Review dependencies/artifacts for secrets, private GPS and telemetry.

Required evidence:

- Targeted security/money/privacy/SQL suites immediately, then the final appropriate Go/Rust/Kotlin/native union once.
- Two-account/device/manual and rendered accessibility evidence explicit; missing external executions remain OWED.

Exit: **All mandatory local contracts pass with exact artifacts and preserved regressions; evidence distinguishes local and external acceptance.**

Code/document targets to verify: `docs/backend/TEST_STRATEGY.md`; `docs/backend/evidence/maps/MAP-31.md`; `existing EV/MEX test suites`.

Evidence destination: `docs/backend/evidence/maps/MAP-31.md`. Commit topic: `feat(maps): run regression, privacy and failure acceptance`; split broad phases into tested task commits.

### MAP-32 — Prepare scoped staging and manual acceptance runbooks

Scope: Reviewable deployment plan, not automatic production mutation.

Dependency: MAP-31.

Implementation steps:

1. Produce version/source/license manifest, configs/defaults, migration/package compatibility, backup rehearsal, rollback plan and smoke checklist for teste.abastevo.com.br only when separately authorized.
2. Backend checks use synthetic records and bounded requests; no real contributor content or unsanctioned load campaign. Android manual steps are provided to the user unless device execution is explicitly authorized.
3. Check fuel/charging pins, exact source switch, detail/profile/routes, download/airplane restart, no GPS, neighbor maps, failed charger alternative and private restore. Production/G09/public pilot remain distinct.

Required evidence:

- Local restore rehearsal and configuration validation; authorized staging results recorded only if actually executed.
- Manual result sheet identifies device/build/region/source date; pending cases do not become PASS.

Exit: **An operator/user can execute the runbook with explicit rollback and evidence; release stays uncertified until required real proofs.**

Code/document targets to verify: `docs/backend/evidence/maps/MAP-32.md`; `docs/operations/ (new scoped runbook)`; `docs/planning/PROGRESS.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-32.md`. Commit topic: `docs(maps): prepare scoped staging and manual acceptance runbooks`; split broad phases into tested task commits.

### MAP-33 — Deliver the cumulative protected batch and reconcile obligations

Scope: One final dev-to-main integration; no unrelated PR sweep.

Dependency: MAP-32.

Implementation steps:

1. Freeze/reconcile the final source union, complete the phase/feature/license/acceptance matrix, include the engine source/SBOM/patch set/notices, inventory open PRs and prove required commits are included. Merge current main normally if needed; revalidate affected changes.
2. With explicit delivery authorization, push maintained dev and open/reuse one protected cumulative dev -> main PR. Inspect all applicable exact-head/base checks and reviews at finalization, fix failures in new commits, and preserve history.
3. Invoke scripts/git-flow.sh finish with the actual PR and required Quick verification only when guards pass. Missing/failed/skipped/cancelled required checks block merge; no force/admin bypass/direct-main push. Keep dev, verify ancestry and mirror allowed wiki pages once if authorized.

Required evidence:

- git diff --check, secret-surface/dependency review, actual required CI current head/base and meaningful final acceptance evidence.
- Record INTEGRATED/WIKI_PENDING and RELEASE_CERTIFIED separately; do not close incomplete issues or merge unrelated PRs merely because open.

Exit: **Authorized source integration preserves every scoped commit and reports all remaining external/device/operator obligations honestly.**

Code/document targets to verify: `scripts/git-flow.sh`; `docs/planning/DELIVERY_WORKFLOW.md`; `docs/planning/PROGRESS.md`.

Evidence destination: `docs/backend/evidence/maps/MAP-33.md`. Commit topic: `feat(maps): deliver the cumulative protected batch and reconcile obligations`; split broad phases into tested task commits.

Hard gate: **DELIVERY_GATE**.

## Test cadence and command discovery

Use `docs/backend/TEST_STRATEGY.md`, `backend/README.md`, current Gradle/Cargo configuration and actual test selectors. The following are verified command families, not orders to rerun the full matrix after every small task:

```sh
# From repository root; select affected suites first.
cargo test --locked --manifest-path tools/station-prep/Cargo.toml
cargo fmt --manifest-path tools/station-prep/Cargo.toml -- --check
cargo clippy --locked --manifest-path tools/station-prep/Cargo.toml -- -D warnings
./gradlew :domain:test :application:test
./gradlew :data:testDebugUnitTest :app:testDebugUnitTest
./gradlew :app:lintDebug :app:assembleDebug
git diff --check
# From backend, using its pinned tools and verified disposable DB setup.
go test -race ./...
go vet ./...
go test -race -count=1 -tags=integration ./...
```

Do not run integration fixtures against persistent/staging/production databases. Read and use the existing test environment variables/tool pins; missing PostGIS is a failed prerequisite, not a skip that closes SQL acceptance. Native engine checks must use the real pinned build targets discovered at MAP-14; invented commands are prohibited. Physical Android remains user-manual unless a separate instruction authorizes emulator/device execution. If real native behavior is required but no authorized execution surface exists, implement all independent work and report that gate pending.

Required adversarial matrix includes identity/price false joins; exact money and incompatible units; ANP/community source races; unknown price/power/connector; centroid-as-pin refusal; source/license revocation; malformed PBF/ZIP/XML/manifest; corrupted downloads and disk-full; 2-writer publish/replay; real PostGIS upgrades/recovery; stale/foreign cursors; signed auth/foreign owner/replay; account A/B/guest state; erase/restore tombstones; lifecycle/GPS/FGS/TTS failure; missing neighbor maps; impossible EV leg/failed primary; no network/no permission; minSdk/ABI/16KB support; TalkBack/text scaling/theme contrast; existing ICE/free/EV regression.

## Rollback and recovery boundaries

Use feature gates to disable map UI/new APIs without deleting prior application data. Map packages/public overlays roll back to a compatible last-good generation, independently from private account state. Database changes are append-only expand/contract: app rollback must not rely on destructive down migrations. A private data restore must reapply existing erasure/revocation rules before serving. Public ODbL files must not be put in the private contributor-media bucket by relaxing its policy.

A failed extraction or ENGINE_GATE has a concrete deliverable: pinned source/tool logs, incompatible interface/resource evidence, measured costs and a revised bounded extraction proposal. It does not justify rewriting the entire app, changing identifiers, swallowing native errors, replacing real navigation with a mock or silently adding a proprietary SDK. Stop dependent engine UI work; keep independent data/contract work reviewable.

## Final acceptance, not a promise of national coverage

- All required phone phases have their local exits proven; minimum, data, engine, offline, performance and delivery gates are distinguished.
- Demonstrate at least the contracted fuel, charging, combined, unknown-price, unknown-location and disputed-source journeys with actual server outputs and native behavior.
- Maps and data have measured regional coverage and versions, not an invented national completeness claim. An audited pilot corridor is a proposal; Cuiaba–Sorriso–Sinop is not already certified as traversable.
- Every shipped code/data/artifact has explicit commercial/offline rights and attribution. No closed or noncommercial-only service is a required dependency.
- Current UI standards and existing ANP/community/private/free behavior are preserved. Native UI/device/manual cases that were not executed remain pending.
- Optional operator live feeds, external in-car host certification, staging/production and G09/pilot/release obligations are individually reported. Do not label the whole product ready merely because a PR merged.

No runtime implementation, source admission, map download/build, phone change, VPS mutation, remote commit push, PR, merge, wiki or operational certification was performed to create this plan. The two pictograms are prepared assets with actual local export validation, not evidence of an implemented map.
