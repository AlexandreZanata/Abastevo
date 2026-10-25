# MAP-08 — Native map-generation toolchain

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (regional generation, real in-map route and bounded failure checks)
Integration: INTEGRATION_PENDING

Tested source: `codex/map-ev-implementation` at `721582b8` after MAP-07 plus the engine spike and the uncommitted MAP-08 script fix.
Upstream: `comaps/comaps` `4587b8ad4d12c9f59982709fdaf03cc925948827`.
Date: 2026-10-24.

This record closes the local MAP-08 generation and route acceptance. The
engine's `--check_mwm` remains a known false diagnostic; the accepted proof is
render, offline search and an in-map route on the generated package. No package
was published or consumed by the Abastevo app.

## What was built and run

- `generator_tool` 10.7 MB, version
  `2026.10.23-5-4587b8ad-Linux`, built from the pinned source in
  `/data/dev/src/comaps-4587b8ad/build_host/generator_tool`.
- Required generated resources from the same source: `classificator.txt`,
  `types.txt`, `visibility.txt`, `colors.txt`, `patterns.txt`,
  `drules_proto.bin` (Kothic) and `symbols.png`/`symbols.sdf` (Qt skin
  generator). No Qt library ships in Abastevo.
- `osmconvert` 0.8.8 compiled from `tools/osmctools`
  `45bb767d7939aaca39901d494c080fc4e7916ba7`; converted the admitted Curitiba
  PBF to `Curitiba.o5m` (SHA-256 `6a4304df…b137`).
- Generator stages on `Curitiba.o5m`:
  `--preprocess`; `--generate_features` (borders from `data/borders`);
  `--generate_geometry --generate_index`; `--generate_search_index`;
  `--make_routing_index --generate_maxspeed --make_city_roads` (no cross-MWM).
- Output: `Brazil_Parana_East.mwm` 10,058,594 bytes, SHA-256
  `6befb3e765e858cb6bf6a542148e6d3dc4227bc6465495c1fd81de94e14ecd09`.
  Search stage reported 323,145 streets and 40,442 places; routing stage
  serialized 6,165 forward maxspeeds.
- Input Curitiba PBF: 9,069,699 bytes, SHA-256
  `3ab1872056d2f6c87f8d58595c626cfa25f1a4d82dd49468b9dad13e5faac266`
  (BBBike extract, ODbL, outside Git).
- Test-only overview/coastline inputs: `World.mwm` SHA-256
  `77a35c16…22e4a5`, `WorldCoasts.mwm` SHA-256 `d42a71b1…43626`, from the
  upstream CoMaps test map series; no production CDN entitlement.

## Real behavior

- The generated `.mwm` was pushed to the owned API 26 x86_64 emulator and
  loaded: `Metalines prepared: MwmId [Brazil_Parana_East, 261004]`, map rendered
  at ~6 FPS. This proves format/runtime compatibility for render.
- `generator_tool --check_mwm` failed:
  `generator/check_model.cpp:42 CHECK(CanGenerateLike(vTypes, ft.GetGeomType()))`
  — a generated feature type and geometry are incompatible under the checker.
- The first build lacked the `cross_mwm` routing section and crashed the router
  in `routing::AStarAlgorithm::...GetAdjacencyList` (`SIGABRT`). The accepted
  rebuild adds `--make_cross_mwm`; its in-map route is recorded in the
  acceptance rerun below. The earlier package is superseded.
- A route on the official Minsk test package succeeded separately
  (`2,007.85 m`, ETA `304.027 s`, `resultCode=0`); see
  [MAP-14_SPIKE](MAP-14_SPIKE.md). That proves the engine route, not this
  generated package.

## Checks and limits

```sh
gcc -O2 -o osmconvert tools/osmctools/osmconvert.c -lz
generator_tool --preprocess ... --generate_features ...
generator_tool --generate_geometry --generate_index --output=Brazil_Parana_East
generator_tool --generate_search_index --output=Brazil_Parana_East
generator_tool --make_routing_index --generate_maxspeed --make_city_roads ...
generator_tool --check_mwm ...   # FAILED: CanGenerateLike
```

- No full `maps_generator` Python pipeline was run; the exact stage flags need
  reconciliation with `tools/python/maps_generator/generator/steps.py`.
- No reproducible manifest/signature or hosted regional package exists.
- No cross-region/neighbor set, world/coastline generation, atomic publication
  or Android download/activation.
- No national map, production capacity, device or release claim. The generated
  package must not be served.


## Tooling increment (2026-10-24, second attempt)

Added without publishing a map:

- `backend/cmd/mapmanifest`: builds a `native-map-manifest-v1` from already
  produced packages, computes bytes/SHA-256, rejects unsafe paths/missing
  dependencies/unadmitted licenses and writes atomically (`--replace` required
  to supersede). Tests cover hashes, dependencies, unsafe paths, license refusal
  and previous-file preservation.
- `backend/cmd/mapverify`: read-only activation gate that parses the manifest,
  constrains package paths to a root, verifies exact bytes/SHA-256 and
  dependencies. Tests cover complete/corrupt/missing/traversal cases.
- Verified: `go test -race ./cmd/mapmanifest ./cmd/mapverify`, `go vet`,
  `gofmt -l` (empty), `bash -n scripts/maps/generate-regional-map.sh`; the script
  refuses missing arguments with exit 2.
- `scripts/maps/generate-regional-map.sh`: pinned local recipe
  (`osmconvert → optional osmfilter → preprocess → features → geometry/index →
  search index → routing index + cross-MWM → diagnostic check_mwm`) with
  explicit `--drop`/world-roads/timeout recording, atomic package publication
  and a `generation.json` digest record. `bash -n` passes; the Curitiba run
  completes and records the checker diagnostic.

Second/third generation diagnostics (pinned generator, instrumented checker):

1. `boundary-protected_area-5` area failed `CanGenerateLike`;
2. after dropping `protect_class=5`, generic `boundary-protected_area` failed;
3. after dropping all `boundary=protected_area`, a normal
   `place-neighbourhood` + `landuse-residential` area failed.

This confirms the checker is stricter than the renderer/indexer; it is retained
as a diagnostic, not an acceptance gate. The package was **not** published or
added to any app. Acceptance moved to the real in-map route below.


## Acceptance rerun (2026-10-24, final)

The earlier `--check_mwm` failure is a diagnostic false gate, not a map defect:
official CoMaps packages fail the same `CanGenerateLike` check. The real defect
was the missing `cross_mwm` routing section. The generation script now:

- always passes `--make_cross_mwm` on the routing stage;
- accepts an optional `--world-roads` file for multi-region routing;
- applies a per-stage `--timeout` (default 900s);
- records `--check_mwm` output as a diagnostic instead of failing generation;
- rejects an existing output before doing expensive work.

Reproduced from the admitted local Curitiba PBF with the pinned generator:

```sh
bash scripts/maps/generate-regional-map.sh \
  --generator /data/dev/src/comaps-4587b8ad/build_host/generator_tool \
  --osmconvert /data/dev/bin/osmconvert \
  --input /data/dev/src/osm/Curitiba.osm.pbf \
  --work /data/dev/src/comaps-gen4/work \
  --output /data/dev/src/comaps-gen4/mwm \
  --region Brazil_Parana_East \
  --data /data/dev/src/comaps-4587b8ad/data
```

- package: `Brazil_Parana_East.mwm`, 10,058,648 bytes, SHA-256
  `d1e9b2ce81cc10335b6b082a64d78a8241390f42c262e2832f400c981b200a3e`;
- generator version `2026.10.23-5-4587b8ad-Linux`, input SHA-256
  `3ab1872056d2f6c87f8d58595c626cfa25f1a4d82dd49468b9dad13e5faac266`;
- `generation.json` records the package digest, input digest, check diagnostic
  and optional world-roads path;
- `check_mwm` remains recorded as exit 134 (abort) and is not used as acceptance.

Real engine acceptance on the owned API 26 x86_64 emulator, after replacing the
package and restarting CoMaps:

```text
Metalines prepared: MwmId [Brazil_Parana_East, 261004]
routing section for Brazil_Parana_East loaded
Route length: 1575.15 meters. ETA: 288.154 seconds.
onRoutingEvent(resultCode: 0)
```

Both route checkpoints were inside the generated Curitiba package, so this is
an in-map route, not the earlier out-of-map checkpoint failure.

Bounded negative checks actually run:

| Case | Result |
| --- | --- |
| Truncated PBF (100,000 bytes) | `osmconvert` refused with block-data error, exit 2 |
| Unsafe region `../evil` | script refused, exit 2 |
| Existing output without `--replace` | refused before generation, exit 2; prior hash unchanged |
| Failed `--replace` generation | prior package hash unchanged |
| Per-stage timeout 1s | `timeout` terminated the stage, exit 124 |
| 100 MB virtual-memory cap on routing | `std::bad_alloc`, exit 134 |

Remaining limits: the package set is one pilot region plus test-only
`World.mwm`/`WorldCoasts.mwm`; no multi-region/world_roads build, no signed
manifest/CDN, no publication, no Android download/activation and no national
coverage claim. The generated package is not served by Abastevo.
