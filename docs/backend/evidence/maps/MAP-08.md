# MAP-08 — Native map-generation toolchain

Status: IN_PROGRESS / BLOCKED_ON_ROUTING_VALIDATION
Validation: PARTIAL (tool build, regional generation and render; check/route failed)
Integration: INTEGRATION_PENDING

Tested source: `codex/map-ev-implementation` after MAP-07 plus the engine spike.
Upstream: `comaps/comaps` `4587b8ad4d12c9f59982709fdaf03cc925948827`.
Date: 2026-10-24.

This is not MAP-08 acceptance. A generated regional map renders, but the
engine's own `--check_mwm` refuses it and an offline route attempt on the first
generated build crashed. No package was published or consumed by Abastevo.

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
- The first build (with cross-MWM routing) crashed the router in
  `routing::AStarAlgorithm::...GetAdjacencyList` (`SIGABRT`) when a route was
  requested. The map was rebuilt without `--make_cross_mwm`; a new route proof
  was not completed. The map's search/index/render are usable, but its routing
  package is not accepted.
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

Next: compare the manual stages with `maps_generator`, fix the feature/geometry
check failure, rebuild with the correct routing/cross-MWM sections, and pass a
real offline route on the generated Curitiba package before any publication.
