# National map — national run and staging apply receipt (2026-10-27)

Status: **APPLIED on staging** (`abastevo-temp`, `https://teste.abastevo.com.br`)
/ INTEGRATION_PENDING. No production namespace, DNS or release was touched;
G09/P10 remain untouched. Every registry station is classified into exactly
one tier with a reason; every station with a provable open-data point is a
map pin.

Builds on [MAP_NATIONAL_STATIONS_PLAN](MAP_NATIONAL_STATIONS_PLAN.md),
[MAP_NATIONAL_APPLY_RECEIPT](MAP_NATIONAL_APPLY_RECEIPT.md) and
[MAP_NATIONAL_STATIC_OVERLAY_RECEIPT](MAP_NATIONAL_STATIC_OVERLAY_RECEIPT.md).

## Owner decisions (delegated 2026-10-27)

The owner asked to unblock everything and take the best decisions. The plan's
recommended options were adopted:

- **D1 = yes:** an ANP Revendedores API point is a pin as `anp-reported`,
  labelled "Localização informada pela ANP".
- **D2 = yes, exact number only:** an IBGE CNEFE 2022 establishment
  (`COD_ESPECIE` 6) at the exact street and house number with an
  address-level coordinate (`NV_GEO_COORD` 1-3) is a pin as `address-point`.
  Block-face, locality and sector centroids (levels 4-6), street-only and
  CEP-only matches are never pins.
- **D3 = evidence only:** Overture is not used as proof.

## Gaps found and closed

| Gap | Evidence | Fix |
| --- | --- | --- |
| A1.a blocked by HTTP 429 after 12 municipality requests | Phase 1 STOP | `uf=<UF>` without `municipio` returns the same records (AC: 60/60 identical); 29 requests instead of 5,525 |
| Only `reviewed` reached `/v1/stations/nearby` and the app | `current_point` projected reviewed only | B-BR-MAP-02 ladder, migration `000139`, `station-locate` |
| CNEFE centroids could become L4 pins | B2 ignored `NV_GEO_COORD` | levels 4-6 refused (`cnefe_centroid_only`) |
| "POSTO DE SAUDE" etc. counted as station evidence | B2 ignored `COD_ESPECIE` | only species 6 is evidence |
| L3 decisions had no coordinates | A2 decision schema | exact ANP point text on valid-point decisions |
| `station-review` cannot apply a national batch | 10k rows / 4 MiB / per-row | `station-locate`: COPY + set-based, one transaction |
| App failed a whole response on a new `location_quality` | `fromWire` threw | unknown values degrade to `unknown` |
| Static overlay integration test failed (inherited, recorded as PASS) | jsonb fixture typed as text | fixture cast |
| `current/manifest.json` cached `immutable` for a year | live headers | `current/*` 300 s, releases immutable |
| Map handler existed only in the live ConfigMap | repo Caddyfile drift | repo file byte-identical to live (`5d0b2939…`) |
| Overlay served 565 OSM assertions, not the national pins | manifest | every directory pin with prices, OSM duplicates dropped |

## Pipeline timings (VPS `sorrimobi-vps`, 8 vCPU)

| Step | Input | Wall | Peak RSS |
| --- | --- | --- | --- |
| A1.a ANP API by UF (5 s spacing) | 29 requests, 52 MB | 136 s | 62 MB |
| CNEFE download (24 remaining UFs, 3 parallel) | 2.4 GB | 86 s | — |
| National RFB + CNEFE extract (5 workers) | 73.8M RFB + 111.1M CNEFE rows | 294 s | — |
| A2 `prepare_national_matches` | 45,807 API, 28,968 OSM, 45,688 registry | 1.56 s | 200 MB |
| B2 `prepare_open_geocode` | 6,714 L5, 124,545 CNEFE, 6,714 RFB | 1.19 s | 159 MB |
| `prepare_national_coverage` | A2 + B2 | 0.68 s | 100 MB |
| Migration `000139` (Job) | 48,750 stations | 4.4 s | — |
| `station-locate` dry-run | 40,388 rows | 9.6 s | 135 MB |
| `station-locate` apply | 40,027 applied | 2.7 s | 136 MB |
| `station-locate` rerun | 40,388 unchanged | 0.66 s | 135 MB |
| `mapoverlay` | 40,388 pins, 27 UFs | 2.0 s | 137 MB |

A2/B2 reruns were byte-identical. The full chain after the cached OSM
`prepare_osm` (7m14s, unchanged) takes about 9 minutes, dominated by the
polite API spacing and the 5.4 GB RFB scan.

Before/after against the pilot: B2 went from 10.12 s / 679 MB for three
municipalities to 1.19 s / 159 MB for the whole country, because the extract
keeps only rows that can decide an L4 (L5 municipality, species 6, levels
1-3, equal house number) — 124,545 of 111.1M rows.

## Coverage (registry 45,688 stations)

| Tier | Quality | Stations |
| --- | --- | --- |
| L1 | `reviewed` | 1,972 |
| L2 | `osm-mapped` | 66 |
| L3 | `anp-reported` | 36,941 |
| L4 | `address-point` | 1,409 |
| **Pins** | | **40,388 (88.4%)** |
| L5 | no point | 5,300 |

L5 reasons: 3,441 no CNEFE establishment at the exact address, 1,758 without
a usable address/number (mostly highway `S/N`), plus small OSM/name/municipality
conflicts. They stay list-only with their address, never a centroid.

Per UF, pinned share of registry stations ranges from 67.2% (MA) to 99.5%
(AL); SP 89.6%, MG 95.0%, PR 82.9%, BA 77.1%.

The directory also holds 3,003 stations without a registry UF: 2,924 are LPG
(P13) resellers from the weekly ANP survey (not fuel stations) and 79 priced
fuel stations absent from the current registry and API. Of the fuel
stations with a price in the latest official week (2026-10-18..24), 4,522 of
4,895 (92.4%) now have a pin.

## Staging apply

- Images `abastevo-{api,worker,migrate}:stg-7c7d776f` = this branch merged
  with `codex/vps-staging-landing-perf` (the previous staging source), so the
  landing/portal work stays deployed.
- Backup before mutation: `pg_dump -Fc` 38.5 MB, TOC verified (167 table
  data entries), SHA-256 prefix `1b1c8a5480dbe0a9`, plus the active map
  generation id, under the private VPS root `work/backup-20261027/`.
- `000139` applied; both new constraints validated on the live table.
- `station-locate`: 40,027 applied (reviewed 1,611, osm-mapped 66,
  anp-reported 36,941, address-point 1,409), 361 unchanged, 0 unknown or
  inactive; rerun all unchanged. 0 pins with a non-pin quality.
- `map-load`: generation `0fce50e6-7f4d-4b36-95f7-ff49a4086fbf`,
  source revision `national-20261027`, 66 assertions, ACTIVE.
- Overlay release `national-20261027-b3fec38c`: 40,388 pins, 27 UFs, 4,522
  with embedded official prices; 22.0 MB JSON, 2.6 MB precompressed
  (SP 4.48 MB -> 498 KB).
- Rollback: restore the dump, re-activate generation
  `02ea6e7d-19ce-4942-a369-1aa7efdd03dd`, switch `current` back to
  `phase3-coverage-sample-20261026-5e8d5a74`, redeploy `stg-ab20fda8`.
  Migration `000139` only widens checks; the old binaries tolerate it.

## Serving performance

`/v1/stations/nearby` through the API NodePort, 20 sequential requests per
case (p50 / p95):

| City, radius | Before (361 pins) | After (40,388 pins) |
| --- | --- | --- |
| São Paulo 2 km | 3.4 / 6.4 ms, 10 items | 7.4 / 9.2 ms, 36 items |
| São Paulo 15 km | 7.6 / 13.9 ms, 100 items | 14.3 / 16.4 ms, 100 items |
| Curitiba 15 km | 6.1 / 9.2 ms, 68 items | 8.9 / 14.5 ms, 100 items |
| Manaus 15 km | 1.9 / 2.6 ms, 0 items | 7.5 / 9.2 ms, 100 items |
| Rio Branco 15 km | 2.0 / 4.3 ms, 0 items | 4.6 / 5.9 ms, 60 items |

The densest query (São Paulo 15 km) uses the GiST index, examines 1,916
candidates, keeps 1,384 and top-N sorts 101 in 26 ms under EXPLAIN ANALYZE,
all buffers in cache. References resolve in one batch per page.

Edge (TLS) mix with k6 at a constant 100 req/s for 60 s (40% per-UF overlay
files of 10 UFs, 60% nearby in 8 cities at 2 km and 15 km), k6 pinned to two
cores on the VPS:

| Run | Requests | Failures | Overlay p50 / p95 / p99 | Nearby p50 / p95 / p99 |
| --- | --- | --- | --- | --- |
| Cold (first after deploy) | 5,883 | 0 | all kinds: 18.7 / 352 / 1,940 ms | |
| Warm | 5,958 | 0 | 21.8 / 78.2 / 372.5 ms | 23.0 / 207.4 / 657.7 ms |

The overlay tail is absorbed by the edge micro-cache. The nearby tail at
~60 req/s is API capacity (one CPU limit on the API container under the
namespace quota), not the query plan; the static overlay is the low-cost
path for map traffic and the API keeps price taps.

`precompressed gzip` was tried and rejected: Caddy 2.10.2 answered `206
Partial Content` to a plain GET for the precompressed files, even without the
global encoder. The global gzip encoder plus the edge micro-cache serve the
overlay; the `.gz` files stay in each release for a future CDN.

## Device validation

Device: POCO `2311DRK48G` (serial `VKCQGAUCGYAMR4W4`), 2026-10-27, debug APK
built from merge head `b1b85f94` (dev map UI plus the national pin tiers) and
pointing at `https://teste.abastevo.com.br`. Evidence and per-class P38 matrix:
`docs/backend/evidence/maps/map-national-device-20261027/`.

| Capture | What it shows |
| --- | --- |
| `curitiba-pins.png` | Base map plus 100 pins after the admitted `World`, `WorldCoasts` and `Brazil_Parana_East` MWM files were pushed to app-private `files/261004` (hashes match MAP-23). |
| `curitiba-anp-sheet.png` | Bottom sheet for an `anp-reported` station: "Localização: Localização informada pela ANP". |
| `sorriso-pins.png` | 22 pins at the Sorriso viewport over a blank base (no regional pack for MT); query completes after the camera moves. |
| `sorriso-address-sheet.png` | Bottom sheet for an `address-point` station: "Localização: Localização aproximada pelo endereço (IBGE)". |
| `sao-paulo-pins.png` | 100 items and two clusters with the compact dev controls. |
| `manaus-pins.png` | 100 items, two clusters and named pins; API reports 99 `anp-reported` + 1 `address-point` for this viewport. |

The compact panel, city selector, grouped filters and action cards are the dev
layout; the pin labels and `MapStationLocationQuality` plumbing are preserved.
The base map has no tiles outside the admitted Paraná fixture: `mapDistBaseUrl`,
`mapDistRegion` and `mapDistKeys` are empty in every local build and no staging
distribution origin exists (BLOCKED_CONFIG). Pins still render because the
world fixture initialises the camera; the three requested cities are therefore
UI/pin evidence, not base-tile evidence.

Merged-head device run (`b1b85f94` + test fix `ed4f685f`, same device;
`merged-head-device-matrix.txt`): `MapScreenInstrumentedTest` 13/13,
`SavedPlacesDeviceTest` 3/3, every `maps:engine` host/overlay class, and
`MapDownloadWorkerTest` 7/7 pass. `MapDownloadWorkerTest` previously expected
the pre-MAP-24 `CONNECTED` default; MAP-24 made downloads Wi-Fi-aware
(`UNMETERED` by default), so the test now asserts both branches.
`MapRouteInstrumentedTest` (recenter control never leaves the tree after a tab
switch) and `NativeRouteScreenInstrumentedTest` (`native-route-open` is only
composed in the expanded panel) fail identically on pure `origin/dev`
`6f8b82b5`: they come from the dev persistent-host and compact-panel UI, not
from the national merge, and stay open for that UI work. Bottom-tab, home and
training-consent failures are the same pre-existing rows as below. No failure
touches the pin tiers.

The matrix below ran on a different tree and is P38 evidence only, not
evidence for this merge. P38 matrix, branch
`origin/codex/p38-acceptance-20261026` (`4cb221d5`, PR #157)
run with `ANDROID_SERIAL=VKCQGAUCGYAMR4W4 ./gradlew connectedDebugAndroidTest`:

| Module | Cases | Passed | Failed | Skipped |
| --- | --- | --- | --- | --- |
| `:app` | 120 | 87 | 24 | 9 |
| `:data` | 117 | 112 | 3 | 2 |
| `:maps:engine` | 10 | 9 | 1 | 0 |

The 24 app failures are not waived: two `PhotoReviewDeviceTest` cases are
BLOCKED_MEDIA (private corpus absent), 13 route/guidance/performance cases need
the admitted regional fixture (private runtime, not shipped), and nine are
device/Compose environment cases (scroll/keyboard/WorkManager timing and the
Appendix A2 sync timeouts). In the data module the failures are the 15 MB
database-size boundary, `RealImageOcrDeviceTest` BLOCKED_MEDIA, and
`MapDownloadWorkerTest` expecting the pre-MAP-24 `CONNECTED` default (fixed
in `ed4f685f`, 7/7 on the merged head).
`MapOfflineDistributionInstrumentedTest` passed 4/4 once the synthetic
distribution server and `adb reverse tcp:18766` were provided; the engine
`MapHostRealEngineInstrumentedTest` passed in isolation and the remaining
engine failure is the same missing admitted fixture.

## Not done / still open

- 5,300 L5 stations remain without a point (no open evidence); the
  community location flow is their path.
- The app reads pins from `/v1/stations/nearby`; consuming the static overlay
  in the app is a separate product step.
- `mapDistBaseUrl`, `mapDistRegion` and `mapDistKeys` remain unset
  (BLOCKED_CONFIG), so the base map and offline packages are unavailable
  outside the admitted Paraná fixture. Device rendering of the new tiers and
  the remaining P38 rows are recorded in [Device validation](#device-validation).
