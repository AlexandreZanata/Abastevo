# MAP-13 — End-to-end data gate

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (real Rust→Go/PostGIS→HTTP→Kotlin chain plus native package)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `721582b8` plus the uncommitted
MAP-09–13 changes. Date: 2026-10-24.

## Gate chain actually executed

1. **Rust registry preparation** from the admitted synthetic 13-column fixture:
   `prepare_registry` produced a file-backed batch with `input=12 accepted=8
   duplicates=1 quarantined=3 assertion_bytes=10291`. The batch manifest,
   assertions, candidates and quarantine files were emitted locally; no network,
   database or clock input was used.
2. **Go/PostGIS directory import** through `station-load --publish` into a fresh
   disposable database after migrations 000001–000122:
   `state=complete accepted=8 duplicates=1 rejected=3`,
   `publication=complete registry_assertions=7`.
3. **Go/PostGIS map-overlay import** through `map-load`:
   generation `3ca88db9-19cb-4f86-969d-d69fb3fb506f`, `status=ACTIVE`,
   `source_revision=overlay-2026-10-24.1`.
4. **Real HTTP API process** on that database returned three in-viewport
   assertions for `source=ANP&product=GASOLINE_REGULAR`: the combined ANP site
   with exact `5890` milli-BRL/L and period `2026-10-18..2026-10-24`, the
   community combined site with no ANP price, and the OSM mapped reference.
   The same process returned five published directory stations for `state=SP`.
5. **Kotlin decoders** on both the prepared corpus and the **actual HTTP
   response bytes**:
   `:data:testDebugUnitTest --tests '...MapPoiCodecTest'` and
   `...MapDiscoveryResponseCodecTest` → BUILD SUCCESSFUL. The API page decoder
   reads `generation/items/next_cursor/generated_at`, preserves the station UUID
   and exact `5890` price, and a cross-check asserts the API item equals the
   prepared corpus item.
6. **Native package** verification with the MAP-08 Curitiba package plus
   World/WorldCoasts: `mapverify` reported `{"packages":3,"bytes":72067998}`.

## Fixture coverage

- ANP combined fuel/charging site with exact per-station price and period.
- Community combined site with a distinct community projection.
- OCM charging site with no price, unknown power/tariff, unlinked identity.
- OSM reference POIs: one unknown/missing location (list-only) and one
  `osm-mapped` approximate point.
- No-price and unknown-value cases remain empty/null, never zero/free.
- The API projection sets `capabilities.price_known` from the *projected* price
  array, so a community-only price hidden in ANP mode is not reported as a known
  ANP price. The actual response fixture is
  `contracts/testdata/map/assertions-response-v1.json`.
- A blocked-license corpus was rejected end to end:
  `map-load: $.assertions[0].rights.license: rights not admitted`, exit 1;
  the active generation count stayed 1 and no partial generation was written.
- Disputed identity is exercised by MAP-07 review tests (`REVIEWED`,
  `UNRESOLVED`, `REJECTED`); unlinked OCM/OSM points stay visibly unlinked here.

## Commands and results

```sh
prepare_registry contracts/testdata/station-prep/registry-13col-sample.csv \
  contracts/testdata/station-prep/ibge-mapping-sample.json \
  /tmp/map-data-gate/registry registry-13col gate-2026-10-24 \
  11111111-1111-4111-8111-111111111111 2026-10-24T00:00:00Z
# input=12 accepted=8 duplicates=1 quarantined=3

ANPFUEL_DATABASE_URL=... go run ./cmd/migrate
ANPFUEL_DATABASE_URL=... go run ./cmd/station-load --emit-dir ... --publish
ANPFUEL_DATABASE_URL=... go run ./cmd/map-load --envelope ... --assertions ...
ANPFUEL_DATABASE_URL=... ANPFUEL_MAP_DIST_DIR=... go run ./cmd/api
# GET /v1/map/assertions -> 3 items, exact 5890 price
# GET /v1/stations?state=SP -> 5 items
go run ./cmd/mapverify --manifest ... --root ...
# {"packages":3,"bytes":72067998}
```

## Limits

- The map-overlay corpus is the frozen MAP-03 golden corpus, not a new national
  Rust map-assertion producer. Rust produces the station registry and OSM/ANP/
  OCM references; the contract corpus is the admitted cross-language fixture.
- The native package is one pilot region; no signed CDN publication or Android
  Room import is claimed here (MAP-12/15 own those boundaries).
- Physical Android interaction, staging, national coverage and release remain
  OWED.
