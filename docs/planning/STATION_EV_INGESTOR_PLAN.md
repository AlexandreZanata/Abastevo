# Station and EV charger ingestor plan

Status: PLANNED (2026-11-03). Owner request:

1. Stations show the **trade name (nome fantasia)** as the main name and the
   **legal name (razão social)** as a secondary name. People recognise the
   sign at the forecourt, not the company's registered name.
2. Stations without an official ANP address must show a readable address in
   the mobile app. Today they show raw coordinates.
3. The ingestor maps every EV charger in Brazil that the open sources know,
   by cross-matching **Open Charge Map** (location, connectors, power,
   operators) with **OpenStreetMap** (fuel stations, chargers, geometry).
   This is Part B (EVC-xx). It runs after SNA-01..SNA-03 because it reuses
   the station names and the address formatter.

Part A task IDs: **SNA-xx**. One bounded task at a time, B-BR/BUC before
behaviour, RED → GREEN → REFACTOR, append-only migrations with real-PostGIS
tests, additive API changes only (older APKs stay in use).

## 1. Findings (origin/main, 2026-11-03)

Names:

- `backend/internal/modules/directory/adapters/registry/stage.go`
  `parseRecord`: `DisplayName = RAZAO_SOCIAL`. `NOME_FANTASIA` is an accepted
  header (`policy.go`) but never read.
- `registry/api_official.go`: `DisplayName = razaoSocial`; `nomeFantasia`
  (present in `contracts/testdata/registry/registry-api-sample.json`) is
  not parsed.
- `registry/loadbatch.go` (station-prep batches): `DisplayName =
  business_name_normalized` (the legal name). The 13-column ANP registry
  used by `tools/station-prep` has no trade-name column. `anp_prices.rs`
  (weekly survey) and the RFB establishment data already used by
  `prepare_open_geocode` both carry a trade name.
- `directory_stations` (`000002`) has only `display_name`; the public
  `Station` schema in `contracts/openapi/v1.yaml` exposes `display_name` and
  `official.display_name` ("Posto Central LTDA").
- The official price import (`official/adapters/jobs/import.go`) and the
  legacy offline `RetailStation` already prefer the trade name. Only the
  canonical directory path, which feeds server stations, map, portal and
  admin, shows the legal name.

Addresses:

- `app/.../ui/stations/ServerStationDetailSheet.kt` prints
  `"%.5f, %.5f"` (latitude, longitude) where an address should be. It never
  renders `station.address`.
- `app/.../ui/map/MapDiscoveryViewModel.kt` `ServerStation.addressLabel()`
  shows `municipalityCode/state`, which is an IBGE number such as
  `5103403/MT`, as the marker subtitle.
- `stage.go` (registry CSV) keeps only `municipio_ibge` and `uf`. It drops
  `LOGRADOURO`, `NUMERO`, `COMPLEMENTO`, `BAIRRO`, `CEP` and `MUNICIPIO`.
- Address sources already present in the pipeline: ANP registry (CSV/API),
  RFB establishment by CNPJ, OSM `addr:*` tags on a matched POI, and IBGE
  CNEFE address points (`address-point` tier).

## 2. Business rules (write first as B-BR entries)

**B-BR-STATION-NAME-01, name precedence.** `display_name` is, in order:

1. an approved community `display_name` correction (`000144`);
2. the trade name, after normalisation;
3. the legal name.

`legal_name` is always the legal name from the official source, kept
verbatim. The UI shows it as a secondary line only when it differs from
`display_name` after case and whitespace normalisation. A trade name counts
as absent when it is any of:

- empty or punctuation only;
- a placeholder (`NAO POSSUI`, `SEM NOME`, `NAO INFORMADO`, `-`, `.`, `*`;
  keep the list as tested data);
- the CNPJ digits;
- equal to the legal name.

Casing stays verbatim from the source. Title-casing is a separate owner
decision. Official and community names stay distinct:
`official.display_name` remains the source's own value.

**B-BR-STATION-ADDRESS-01, address precedence.** The address shown is the
first available of:

1. ANP official address (registry API/CSV);
2. RFB establishment address for the same active CNPJ;
3. OSM `addr:*` of the POI matched to this station;
4. CNEFE address point;
5. municipality **name** + UF.

Each stored address carries `address_source` (`anp`, `rfb`, `osm`, `cnefe`,
`municipality`). The app labels non-ANP addresses as approximate. Raw
coordinates are never the address text. They stay available only as an
explicit secondary action ("copiar coordenadas" / navigate). Reverse
geocoding with public Nominatim is not a bulk source: ADR-003 and the
Nominatim usage policy apply. Verify ADR-003 before using any geocoder. No
new geocoding service without an ADR.

## 3. Tasks

### SNA-01: Ingestor parses names and full addresses

- CSV (`stage.go`): read `NOME_FANTASIA` and the address columns into the
  assertion (`street`, `number`, `complement`, `district`, `postal_code`,
  `municipio`, plus `municipio_ibge`, `uf`). Use the keys that
  `read/export.go` already uses.
- API (`api_official.go`): parse `nomeFantasia`.
- Assertion gains `LegalName` and `TradeName`. `DisplayName` becomes the
  output of a pure domain function implementing B-BR-STATION-NAME-01.
- Station-prep batch: additive `trade_name` and `legal_name` fields in the
  batch assertion contract (schema version bump in `contracts/`). Rust prep
  fills `trade_name` from RFB by CNPJ, else from the latest ANP survey row.
  Fixtures are synthetic (`[P25-TEST]`), never real contributor data.
- Tests: table tests for the name rule (placeholders, equality, CNPJ,
  blanks), CSV/API parsing with and without the trade name, quarantine
  behaviour unchanged, Rust contract tests.

### SNA-02: Storage and projection (append-only migration)

- New migration: `directory_stations.legal_name TEXT NULL`,
  `trade_name TEXT NULL`, `address_source TEXT NULL` with a CHECK on the
  enum. Add the matching staging and assertion columns where the projection
  reads them.
- The projection computes `display_name` and the address with the two
  rules. Source rows stay immutable.
- Backfill: an idempotent ops/worker job re-projects existing stations from
  retained assertions. It is resumable, batched, and safe with two
  dispatchers (`SKIP LOCKED`). No destructive rewrite.
- Tests: migration recovery test, real-PostGIS projection tests (trade name
  wins; correction wins; legal-only; placeholder), backfill idempotency and
  concurrency.

### SNA-03: API contract (additive)

- `Station`: add `legal_name` (nullable) and `address_source` (nullable
  enum). `display_name` keeps its type; only its value improves. `address`
  keeps the documented component keys.
- OpenAPI, handler and contract tests. Shared-cacheable invariance tests
  still pass. Older APKs ignore the new fields.

### SNA-04: Fill addresses for stations without an ANP address

- In station-prep, attach RFB, OSM and CNEFE addresses by the precedence
  rule, with `address_source` and the source reference in provenance.
- Report coverage: count of stations per `address_source` before and after,
  nationally and per UF. Store it in evidence, with no personal data.
- STOP if a source's licence or terms do not allow this use. Record it and
  skip that source.

### SNA-05: Mobile app

- `ServerStation` (domain): add `legalName` and `addressSource`. Add a
  formatted `addressLine` such as "Av. X, 123 – Bairro, Cidade/UF", built
  from the components and never from coordinates.
- `ServerStationDetailSheet`: title is `displayName`, subtitle is
  `legalName` when it differs, then the address line plus an "endereço
  aproximado" label for non-ANP sources. Coordinates move to a secondary
  copy/navigate action.
- `MapDiscoveryViewModel`: the marker subtitle uses the address line, falling
  back to the municipality name, never the IBGE code.
- List rows, the capture station picker (`CaptureScreen` `addressLabel`) and
  search match both names.
- Strings in all 8 locales. Domain unit tests, ViewModel tests, Compose UI
  test for the detail sheet, and light/dark screenshots.

### SNA-06: Other surfaces

The admin panel shows both names and `address_source`. The portal
generator (`enterpriseexport/portalgen`) and landing use `display_name` and
the address line. Check that nothing still prints coordinates as an address.

### SNA-07: Staging delivery and acceptance

Merge through the protected dev → main flow. The CD agent deploys to staging
(see `CD_ZERO_DOWNTIME_PLAN.md`). Run the backfill with owner authorization.
Acceptance on staging, with evidence in `docs/backend/evidence/sna/`:

- 20 sampled stations across 5 UFs show the trade name first.
- Legal-only stations still show a name.
- Stations formerly shown with coordinates now show an address or the
  municipality name.
- Coverage numbers per `address_source` are recorded.

## 4. Never

- Invent a name or address.
- Show coordinates as the address.
- Overwrite official source rows.
- Mix community corrections into `official.*`.
- Put real CNPJ or contributor data in fixtures beyond the public registry
  test samples.
- Bulk-query public Nominatim.
- Make breaking API changes.

---

# Part B: national EV charger mapping (OCM × OSM)

Task IDs: **EVC-xx**. Same delivery rules as Part A. This part extends the
existing EV work and does not replace it: EE-00..EE-31
(`docs/planning/EV_FULL_SUPPORT_EXECUTION_PLAN.md`), MAP-EV
(`MAP_EV_EXECUTION_PLAN.md`, `MAP_EV_SOURCE_REGISTER.md`) and
`EV_SOURCE_RESEARCH.md` (EVS-06..EVS-09).

## B1. What already exists (origin/main, 2026-11-03)

- **OCM reader**: `tools/station-prep/src/ev_ocm.rs` and
  `bin/prepare_charging.rs`.
  - Profile `ocm-br-20261021-v1` (EE-04) holds 1,298 Brazilian POIs and
    2,089 connection groups, pinned to an `ocm-export` commit.
  - Only open-licence providers are admitted: 1 (CC BY 4.0, 768 POIs),
    40 (CC-0, 422) and 57 (CC0, 108).
  - Contacts, comments and media are minimised away.
- **OSM reader**: `tools/station-prep/src/osm_reference.rs` and
  `bin/prepare_osm.rs` read local PBF files.
  - They already classify `amenity=fuel`, `amenity=charging_station` and
    `COMBINED_SITE`.
  - They keep operator and socket tags, and carry the
    `ODbL-1.0` / `© OpenStreetMap contributors` attribution.
  - So far they have only been run on a regional (Curitiba) extract.
- **Backend**: `backend/internal/modules/charging` covers prepared import,
  linking, read, community observations and operational commands. The
  related migrations are `000115`–`000118`, `000121` (identity links),
  `000122` (map overlays) and `000132`.
- **App**: `ChargingExplore`/`ChargingCommunity` screens,
  `ChargingMarkersSource`, the offline `ChargingSnapshotImport`, and the EV
  trip planner.
- **Rule already designed** (`MAP_EV_SOURCE_REGISTER.md`, "Charger
  confidence"): an OSM ∩ OCM match within 50 m with the same socket family
  is labelled "Confirmado por 2 fontes"; a single source stays unconfirmed.
  It has not been implemented.
- **Register drift to fix**: SRC-OCM still says `PENDING` although EE-04
  recorded the user authorisation. SRC-OSM is waiting on a pinned national
  extract.

## B2. Coverage reality

- ABVE/Tupi Mobilidade report about 25 thousand public and semi-public
  charge points (mid-2026), with news reporting close to 30 thousand by
  September 2026. Those figures count **points**; OCM and OSM count
  **sites**.
- OCM has 1,298 Brazilian sites. The OSM national count has not been
  measured yet (EVC-01 does it).
- OCM + OSM will therefore cover only part of the real network. The product
  must say "carregadores mapeados por fontes abertas", never "todos os
  carregadores do Brasil".
- The rest is filled by:
  - community contributions through the existing charging community flow;
  - later, licensed sources (operator OCPI feeds, Neoenergia,
    EDP, Shell Recharge…), which need written rights and a separate source
    admission.

## B3. Business rules (B-BR first)

**B-BR-EV-SRC-01, admission.**

- OCM records are admitted only from providers listed in the rights manifest
  with a licence and attribution (today: 1, 40, 57). The `opendata` flag
  alone is not enough, and a new provider requires a manifest amendment.
- OSM is admitted from a pinned Geofabrik Brazil extract (URL, date, bytes,
  SHA-256), never from live Overpass queries in production.
- Excluded in both sources:
  - `access=private` or `no`;
  - OCM status Removed or Decommissioned;
  - `disused:`, `abandoned:` or `construction=` objects.

**B-BR-EV-SITE-01, site unit.**

- A site is one OSM `amenity=charging_station` node, way or relation, or
  one OCM POI. `man_made=charge_point` children aggregate into their
  parent site and are never counted twice.
- Connector groups keep the socket family and the quantity each source
  reports. A reported quantity is not proof of physical EVSEs.

**B-BR-EV-MATCH-01, cross-match.** Matching is deterministic and
one-to-one between an OSM site and an OCM POI:

- **Confirmed by 2 sources**: distance ≤ 50 m and at least one shared
  socket family (Type 2, CCS2, CHAdeMO, GB/T, Type 1, Tesla, domestic NBR
  14136).
- **Probable**: distance from 50 to 150 m with the same normalised operator,
  network or name. Shown as single-source and queued for review.
- **Separate**: anything else. The site appears once per source.
- Ties break by distance, then by source IDs. Two OSM sites never merge into
  one OCM POI, or the reverse.
- Conflicting connectors (no shared family within 50 m) go to a conflict
  queue, never auto-merge.

**B-BR-EV-FIELDS-01, field precedence on a confirmed site.**

- Geometry: OSM, which carries way/relation footprints.
- Connectors and power: shown per source. Values are never summed and no
  maximum is invented.
- Operator: OSM `operator`/`network`, else the OCM operator.
- Address: the Part A precedence (OSM `addr:*`, OCM AddressInfo, CNEFE,
  municipality). Coordinates are never shown as the address.
- Lineage: both source IDs, versions and clocks.
- Status: open-source status is static metadata, never "available now".
  Community status reports stay separate and dated.

**B-BR-EV-STATION-01, link to fuel stations.**

- A charger links to an ANP fuel station when any of these holds:
  - OSM tags the same object as both fuel and charging (`COMBINED_SITE`);
  - the charger is within 75 m of a pinned station (reviewed, osm-mapped or
    anp-reported tier) with a matching normalised brand or trade name;
  - a reviewed community link exists.
- It is a link in `000121` identity links, never an identity merge. The
  station page shows "Recarga elétrica" with connectors and power.

**B-BR-EV-RIGHTS-01, licences.**

- The published charger dataset, API and offline pack contain
  OSM-derived data, so they follow the recorded T07 ODbL derived-database
  determination:
  - visible attribution `© OpenStreetMap contributors (ODbL)`;
  - per-POI OCM provider attribution and licence;
  - the derived-database offer at release.
- OCM data is CC BY 4.0, CC0 or CC-0 (OCM moved its own data from CC BY-SA
  to CC BY 4.0). It may be combined with attribution.
- Missing or mismatched rights block publication of that generation.

## B4. Tasks

### EVC-00: Inventory and register reconciliation (no behaviour change)

- Read the EE, MAP-EV and EVS docs. Confirm what charging data is live on
  staging (overlay generation, counts).
- Fix the SRC-OCM/SRC-OSM register rows against EE-04 and T07.
- Baseline counts go to `docs/backend/evidence/evc/EVC-00.md`.

### EVC-01: National source pins

- **OCM**: refresh the Brazil snapshot from a newly pinned `ocm-export`
  commit using sparse paths (`data/BR/` plus `data/referencedata.json`;
  never clone the world). Verify blob SHAs and the provider licences.
  - If the export is stale or experimental-broken, the alternative is the
    OCM API with `countrycode=BR`. The owner obtains the API key, which stays
    on the VPS or the owner machine and never goes into Git.
- **OSM**: pin the Geofabrik `brazil-latest.osm.pbf` (date, bytes,
  SHA-256). Run `prepare_osm` nationally with new, versioned streaming
  limits for disk, memory and time. Never silently raise or truncate the EV
  limits.
- Record per UF: charging sites, combined sites and fuel sites.
- No network access in tests; fixtures are synthetic.

### EVC-02: Normalisation

- One canonical charging-site model for both sources.
- Socket-family mapping table: OSM `socket:*` keys ↔ OCM `ConnectionTypeID`,
  kept as tested contract data.
- Fields: power (kW per group), access, fee, opening hours, operator,
  network and name.
- Quarantine reasons are explicit, with per-reason counts.

### EVC-03: Cross-match (conflation)

- Implement B-BR-EV-MATCH-01 and B-BR-EV-FIELDS-01 in station-prep (Rust)
  as a pure, deterministic step with golden outputs. Use a spatial grid
  index, not O(n²).
- Output: canonical sites with `sources[]`, `confidence`
  (`two-source` / `single-source`), a review queue and a conflict queue.
- Tests cover every band: ≤ 50 m with a shared socket, 50–150 m with the
  same operator, no shared socket, one-to-one ties, `charge_point`
  aggregation, private and removed records.
- Real-run evidence: matched, OSM-only, OCM-only, probable and conflict
  counts nationally and per UF, plus a 30-site manual spot check against
  satellite or street imagery recorded by the owner.

### EVC-04: Link chargers to fuel stations

- Implement B-BR-EV-STATION-01 using the Part A trade names, through the
  existing linking adapter (`000121`).
- Real-PostGIS tests: the 75 m band, brand mismatch, combined sites, and a
  re-run that stays idempotent.

### EVC-05: Rights manifest and attribution

- A per-generation rights manifest carries the OSM extract identity and the
  OCM provider licences.
- Attribution strings go into the API response (additive `sources` field),
  the app detail sheet and the offline pack.
- Prepare the ODbL derived-database offer artefact for release.
- Publication is blocked when rights are missing (negative test).

### EVC-06: Backend import and publication

- Reuse the prepared import (`000115`/`000116` batch order).
- Add only the minimum append-only columns needed for confidence, source
  lineage and the station link.
- The generation swap is atomic. The kill switch keeps the last good
  generation.
- Anomaly guard: refuse publication when the site count drops by more than
  15 % per UF versus the previous generation, unless the owner overrides.
- OpenAPI changes are additive (`confidence`, `sources`, `linked_station_id`).
  Shared-cache invariance tests stay green.

### EVC-07: App

- Map charger pins with a "Confirmado por 2 fontes" badge.
- Filters by connector family and minimum power.
- Charger detail:
  - name and operator;
  - address line from the SNA-05 formatter;
  - connector groups with power per source;
  - source and licence attribution;
  - "reportar status" (existing community flow).
- The fuel station detail shows linked chargers.
- Offline snapshot update.
- Strings in all 8 locales; unit, ViewModel and Compose UI tests; light and
  dark screenshots.

### EVC-08: Refresh cadence

- A scheduled worker/ops job: OCM weekly, OSM monthly (new pins).
- Every run produces a diff report (added, removed, moved > 50 m, connector
  changes).
- A site disappears only when its source drops it. A confirmed site that
  loses one source becomes single-source.
- No live public Overpass or OCM polling from the API path.

### EVC-09: Coverage and honesty

- A coverage report per UF and municipality: sites mapped versus the
  ABVE/Tupi benchmark (points ≠ sites, so the unit difference is stated).
- In-app wording reads "mapeados por fontes abertas", with an
  "adicionar carregador" prompt in gap areas through the existing community
  flow.
- Candidate licensed sources are listed for the owner; none is contacted or
  admitted without authorisation.

### EVC-10: Staging delivery and acceptance

Deploy through the CD agent (`CD_ZERO_DOWNTIME_PLAN.md`). Owner-authorised
import of the national generation. Acceptance:

- national counts match the manifest;
- 30 spot-checked sites are correct;
- confirmed badges show only on two-source matches;
- linked fuel stations show their chargers;
- attribution is visible;
- the kill switch and rollback work.

Evidence goes in `docs/backend/evidence/evc/`.

## B5. Never

- Claim complete national coverage.
- Treat static open-source status as live availability.
- Sum connector counts across sources.
- Merge identities instead of linking them.
- Poll public Overpass or the OCM API from production paths.
- Clone the world OCM export.
- Admit an OCM provider without a recorded licence.
- Drop OSM or OCM attribution.
- Put contacts, comments or photos from OCM into Abastevo content.
- Put an API key in Git.
