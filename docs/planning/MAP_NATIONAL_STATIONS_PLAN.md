# National fuel-station map plan (two fronts)

Status: PLANNED (2026-10-26). Owner request: the most up-to-date open map of
Brazilian fuel stations, cheap to serve, where tapping a pin shows the
station's exact ANP prices. This plan is written for execution by a less
capable agent: every task names its inputs, commands, outputs, checks and a
STOP condition. Do not improvise beyond a task.

It extends, never replaces, [MAP_EV_EXECUTION_PLAN](MAP_EV_EXECUTION_PLAN.md)
(MAP-19 quick prices, MAP-20 minimum gate, MAP-32 staging runbook) and keeps
its rules: B-BR-MAP-01 (no centroid or unknown as an exact pin, OSM candidates
stay distinct from reviewed points), ODbL attribution, exact milli-BRL prices,
append-only migrations, backups before mutation.

## 0. Facts this plan is built on (main @ bbd96568)

| Fact | Where |
| --- | --- |
| ANP registry CSV (45,706 accepted rows) has **no coordinates**; every row is `unknown`. | `tools/station-prep/src/registry.rs:17-31,479-481` |
| ANP Revendedores API **has** lat/lon + SRID (4674/4326) + accuracy/validation fields; already parsed, still stored as `unknown`. | `backend/internal/modules/directory/adapters/registry/api_official.go:85-124` |
| `station-discover` needs `-uf` **and** `-municipio` (or `-cnpj`), 5-minute deadline, and **stages rows in the database**. MAP-20 instead fed the reviewer the raw public API JSON (`/v1/combustivel?uf=PR&municipio=CURITIBA&numeroPagina=1&tamanhoPagina=5000`). | `backend/cmd/station-discover/main.go:1-3,26-40`; `docs/backend/evidence/maps/MAP-20_MINIMUM_LOCAL_20261025.md:28-44` |
| OSM references come from a local PBF (`prepare_osm`), `amenity=fuel`/`charging_station`, CNPJ from `ref:vatin`/`ref:CNPJ`, ODbL constants. | `tools/station-prep/src/osm_reference.rs` |
| The reviewer (`prepare_location_reviews`) needs an ANP API point, equal API/registry address, exactly one OSM candidate with the same street+number, distance <= 50 m. No spatial index, single thread. | `tools/station-prep/src/location_review.rs:20,355-480` |
| `station-review` applies reviews as `reviewed` revisions. | `backend/cmd/station-review/main.go:66-72,206` |
| `directory_location_revisions.quality` allows only `unknown`, `city-centroid`, `reviewed`; map overlays also allow `osm-mapped`. | migrations `000002`, `000122` |
| The Android map reads pins from `/v1/stations/nearby` (radius <= 15 km, limit <= 100); prices from `/v1/stations/{id}/prices`. | `MapDiscoveryViewModel.kt:284` |
| Weekly ANP price import is live on staging since 2026-10-26 (20,926 prices, float-noise rule B-BR-002a). | `docs/backend/DOMAIN_MODEL.md` |
| EV charging is a separate module fed by OpenChargeMap + OSM. | `charging/adapters/http/http.go` |

Measured on staging 2026-10-26: 48,750 directory stations; Rust registry
prepare 45,808 rows in 4.8 s / 113 MB; Go load+publish 164 s.

## 1. Location quality ladder (shared by both fronts)

A station gets the **best** tier it proves; nothing is ever upgraded by
proximity alone, and a lower tier never overwrites a higher one.

| Tier | Quality value | Proof required | Shown as pin? |
| --- | --- | --- | --- |
| L1 | `reviewed` | ANP API point + OSM fuel POI agree: same CNPJ tag, **or** existing MAP-20 rule (same street+number, unique, <= 50 m) | yes |
| L2 | `osm-mapped` | OSM fuel POI with the station CNPJ in `ref:vatin`/`ref:CNPJ`, or unique OSM fuel POI with equal normalized street+number+municipality and compatible name/brand; no ANP point needed | yes (styled "mapeado pela comunidade OSM") |
| L3 | `anp-reported` *(new, decision D1)* | ANP API point inside the station municipality polygon, valid SRID, no OSM confirmation | only if D1 = yes, styled "localização informada pela ANP" |
| L4 | `address-point` *(new, decision D2)* | IBGE CNEFE 2022 establishment/address point with equal street + number + CEP/municipality, and name or category evidence | only if D2 = yes, styled "localização aproximada pelo endereço" |
| L5 | `unknown` | nothing proven | **no pin**; listed in the city list with address |

`city-centroid` stays forbidden as a pin. New values need an append-only
migration widening the CHECK constraints plus the `map_contract.rs` and API
contract update; until the owner decides D1/D2, only L1/L2/L5 ship.

**Decisions owed by the owner (record in PROGRESS before Phase A3/B3):**

- **D1:** may an unconfirmed ANP API point be a pin (L3)? Recommended: yes,
  with distinct styling; ANP is the official source.
- **D2:** may a CNEFE address point be a pin (L4)? Recommended: yes, only when
  the house number matches exactly; never street-only or CEP-only.
- **D3:** Overture Places (CDLA-Permissive-2.0; OSM-derived records ODbL) as
  extra POI evidence? Recommended: evidence only, never sole proof.

## 2. Front A — ANP points x OSM, national, in Rust

Goal: every station that has an ANP API point is matched against Brazil's OSM
fuel POIs in one fast deterministic pass.

### A1 — National input snapshots (parallel lanes, no DB writes)

Worker limits and observed source behavior: [MAP_NATIONAL_SOURCE_LIMITS](MAP_NATIONAL_SOURCE_LIMITS.md).

- **A1.a ANP API harvest (files only, no database).** Build the
  municipality list (UF + municipality name) from the registry CSV. For each
  pair fetch
  `https://revendedoresapi.anp.gov.br/v1/combustivel?uf=<UF>&municipio=<NAME>&numeroPagina=<n>&tamanhoPagina=5000`
  with `curl --fail --max-time 60`, one request at a time, at most one request
  per second, `User-Agent: abastevo-map-harvest/1 (+https://teste.abastevo.com.br)`,
  three retries with 10/30/90 s backoff, following `numeroPagina` until a page
  returns fewer rows than requested. Save each raw response unchanged as
  `api/<UF>/<municipality-slug>-p<n>.json`. Record per file: URL, HTTP status,
  bytes, sha256, rows, rows with a point. Never edit or "fix" a response.
  Optional first probe: try `uf=<UF>` without `municipio` once for one small
  UF (e.g. AC); use it only if it returns the same rows as the per-municipality
  files. Do **not** use `station-discover` in Phase 1 (it writes the database).
- **A1.b OSM Brazil PBF.** Download the Geofabrik `brazil-latest.osm.pbf`
  once, record URL, `sha256`, size and OSM timestamp. Run the existing
  `prepare_osm --input <pbf> --output <dir>`; record wall time, peak RSS,
  counts by kind (FUEL_STATION, CHARGING_SITE, COMBINED_SITE) and how many
  carry a CNPJ tag.
- **A1.c Registry CSV.** Reuse the current registry snapshot from
  `registry-sync` state; record its hash.
- **A1.d Baseline reviewer run (read-only).** Run the existing
  `prepare_location_reviews` for 3 municipalities (Curitiba/PR, Sorriso/MT,
  Sao Paulo/SP) with their API files, the national OSM references and the
  registry assertions; record reviewed/rejected counts per reason and wall
  time. Curitiba must reproduce MAP-20's 62 reviewed from the same inputs
  type (counts may differ with newer sources; explain any difference).
- STOP if any download needs credentials, the API returns 401/403/429
  repeatedly, a hash cannot be recorded, or `prepare_osm` exceeds its
  8 GiB/400M-element limits (report, do not raise limits).

### A2 — `prepare_national_matches` (new Rust binary)

Location: `tools/station-prep/src/national_match.rs` +
`src/bin/prepare_national_matches.rs`. No new crate dependency unless recorded
with license/security review (prefer none).

Design (why it is fast): inputs are small (<100k points each); the cost is
I/O and PBF parsing, already done in A1. Matching uses:

1. A uniform lat/lon grid hash (cell ~0.0025 deg ~ 250 m): `HashMap<(i32,i32),
   Vec<u32>>` over OSM fuel POIs; a query scans the 3x3 neighbourhood. O(n).
2. A CNPJ index `HashMap<cnpj14, Vec<u32>>` over OSM POIs with CNPJ tags.
3. A normalized-address index reusing `location_review.rs` normalization.
4. Haversine distance in f64; deterministic ordering by (CNPJ, OSM id).
5. Optional parallelism only if A1 timing shows >30 s matching (it should
   not): `std::thread::scope` over UF chunks, merged in UF order.

Rules per station (CNPJ from registry): L1 if CNPJ-tag match or MAP-20 rule;
else L2 candidates; else L3 candidate (if API point valid); else L5. Emit
ambiguity reasons (`ambiguous_osm`, `distance_exceeded`, `municipality_mismatch`,
`srid_invalid`) and never pick among ties.

Outputs (same contracts as today): `reviews.ndjson` + manifest for
`station-review` (L1 only), `abastevo.map-assertions-v1` NDJSON for `map-load`
(L2/L3), `decisions.ndjson` with every station's tier and reason, and
`summary.json` (counts per tier per UF, timings, input hashes).

Tests first (RED -> GREEN): CNPJ-tag match, address match, tie -> ambiguous,
51 m -> refused, wrong municipality -> refused, SRID 4674 accepted, point
outside Brazil refused, deterministic output hash across two runs, 1M synthetic
points under `bench_stages` budget. Run `scripts/check-rust-fast.sh`.

Acceptance: national run < 60 s wall on the VPS for matching (excluding A1),
peak RSS < 1 GiB, byte-identical reruns.

### A3 — Apply on staging (VPS, scoped to `abastevo-temp`)

1. Backup DB (`pg_dump -Fc`, verify TOC) + current map generation id.
2. `station-review -reviews ... -manifest ... -dry-run` -> must report 0
   unresolved; then apply. `map-load` the L2/L3 overlay generation.
3. Smoke: nearby for Curitiba, Sao Paulo, Sorriso returns >0 stations with
   quality and canonical UUID; `/v1/stations/{id}/prices` exact or explicitly
   absent; no `unknown` with a point.
4. Rollback: restore dump + previous overlay generation; migrations
   append-only.

## 3. Front B — open-data geolocation of stations without ANP points

Goal: locate registry stations that have no ANP API point (or whose point
failed checks) using only open data, majority fast and assertive.

### B1 — Open sources (parallel lanes, record license + hash)

- **OSM** (from A1.b): CNPJ-tagged and addressed fuel POIs. ODbL.
- **IBGE CNEFE 2022**: ~110M validated address coordinates, including
  establishments, with street, number, CEP, locality. Download per UF; keep
  only the station municipalities' rows to bound size. Positional accuracy
  varies (IBGE note); never use street-only or CEP-only matches.
- **Receita Federal CNPJ open data** (establishments): canonical address and
  CEP for the 14-digit CNPJ; used to normalize the registry address, never
  published beyond the existing public fields (no CPF; esp-02 minimization).
- **Overture Places** (D3): `places` GeoParquet filtered to Brazil + fuel
  categories; evidence only.

Phase 1 scope for B1 (sizes are large): record the official download URL,
license text/link, release date and total size of every source; download
only the pilot UFs **PR, MT, SP** of CNEFE and only the RFB
"Estabelecimentos" files needed to cover those UFs; for Overture record the
release id and, only if `duckdb` is already installed, extract the Brazil
fuel-category places to one GeoParquet. Anything heavier waits for Phase 2.
Store under `/opt/abastevo-temp/map-national-<date>/inputs/open/` (mode 700),
never in Git.

### B2 — `prepare_open_geocode` (new Rust binary)

Location: `tools/station-prep/src/open_geocode.rs` + bin. Inputs: registry,
A2 `decisions.ndjson` (only L5 stations), OSM references, CNEFE extract, RFB
address extract, optional Overture extract.

Pipeline: normalize addresses once (same normalizer as A2); build
`HashMap<(municipality_ibge, street_norm, number), Vec<point>>` over CNEFE;
per station try, in order: OSM CNPJ tag -> L2; OSM unique street+number+name ->
L2; CNEFE exact street+number+municipality with establishment evidence -> L4;
otherwise L5 with reason. Never centroid, never nearest-street guess.

Tests: exact number match, number missing -> L5, two CNEFE points disagreeing
>100 m -> ambiguous, accent/abbreviation normalization ("AV." = "AVENIDA"),
RFB address overriding a stale registry address only when CNPJ equal.

### B3 — Apply (after D1/D2) and coverage report

File-only coverage and apply-plan preparation is implemented in
`prepare_national_coverage`; see
[MAP_NATIONAL_APPLY_GATE](MAP_NATIONAL_APPLY_GATE.md). The actual database apply
remains gated by D1/D2 and explicit authorization.

Same backup/dry-run/apply/smoke/rollback as A3. Publish `coverage.json`:
stations per tier per UF and the remaining L5 list for community
contribution (existing contribution flows, no GPS requirement).

## 4. Serving at very low cost (both fronts)

- Pins stay on the existing `/v1/stations/nearby` + `/v1/map/assertions`.
- Add a static, release-addressed per-UF overlay (`/dados/mapa/<release>/<UF>.json`
  or PMTiles) built after each import, served by the staging web tier with
  `immutable` caching (same pattern as portal search shards), so the CDN
  absorbs map traffic and the DB only answers price taps.
- Price on tap: `/v1/stations/{id}/prices` (official week + date + unit +
  condition), absence explicit. Optional: embed the latest official week's
  prices per station in the UF overlay (~20k stations x 5 fuels) to make taps
  free; the overlay carries the survey week and is rebuilt weekly.

## 5. Parallel execution map

| Phase | Lane | Depends on | Can run in parallel with |
| --- | --- | --- | --- |
| 1 | A1.a ANP API harvest | — | A1.b, A1.c, B1 |
| 1 | A1.b OSM PBF + `prepare_osm` | — | A1.a, A1.c, B1 |
| 1 | A1.c registry snapshot | — | all |
| 1 | A1.d baseline reviewer run | A1.a (3 cities), A1.b, A1.c | B1 |
| 1 | B1 open-source acquisition + license record | — | all |
| 2 | A2 matcher (code + tests) | A1 samples only | B2 code |
| 2 | B2 geocoder (code + tests) | B1 samples only | A2 code |
| 3 | A3 apply L1/L2 | A2, backup | B2 runs |
| 3 | B3 apply | B2, D1/D2, A3 done | — |
| 4 | Static overlay + price embedding | A3 | B3 |

Each lane writes to its own directory and its own branch; no two lanes edit
the same file. Lanes in Phase 2 share only read-only fixtures.

## 6. EV and combined sites

Combustion and EV share the map but keep separate truth: `prepare_osm`
already labels FUEL_STATION / CHARGING_SITE / COMBINED_SITE and the charging
module (OpenChargeMap open providers + OSM) owns chargers. Front A/B outputs
only fuel stations; a COMBINED_SITE links to a charging place only through
the existing reviewed `charging_identity_links` rule, never by proximity.

## 7. Phase 1 definition of done

- Private directory `/opt/abastevo-temp/map-national-<date>/inputs/` with
  every input, `SOURCES.md` (URL, license, retrieval time, sha256, size, rows).
- `phase1-report.md`: per UF registry rows, API rows, API rows with a valid
  point, OSM fuel POIs, OSM POIs with CNPJ; tool timings and peak RSS; every
  failure/truncation with its exact reason.
- No database, deployment, edge or app change. No data committed to Git.

## Appendix A — prompt for the executing agent (Phase 1 only)

The owner's copy-paste prompt lives in the delivery message of 2026-10-26 and
mirrors section 2 A1 + section 3 B1 + section 7. Agents executing Phase 1
must not start A2/B2 code, migrations, deployments or database writes.
