# MAP-08 diagnosis — checker false gate and real integration blockers

Date: 2026-10-24. Upstream: `comaps/comaps`
`4587b8ad4d12c9f59982709fdaf03cc925948827`.

## Short answer

`--check_mwm`/`CanGenerateLike` is **not** a valid acceptance gate for current
CoMaps maps. Official CoMaps packages fail the same check. The real generated
map defect found separately was a missing `cross_mwm` routing section; the
engine aborts when it tries to route without it. Engine integration into
Abastevo is additionally blocked by packaging/API/size/licensing/data work, not
by the checker.

## Why `CanGenerateLike` fails

`libs/indexer/feature_visibility.cpp` accepts a feature only if at least one of
its types has a drawing rule for the exact geometry:

```cpp
bool CanGenerateLike(types, geomType) {
  for (t : types)
    if (IsUsefulStandaloneType(t, geomType) ||
        classif().GetObject(t)->IsDrawableLike(geomType, false))
      return true;
  return false;
}
```

For an area, `IsDrawableLike` only counts `drule::area`; captions/symbols do not
count. Normal OSM types such as `landuse-residential`, `place-neighbourhood`
and `place-suburb` carry caption rules but no area rule in the generated
`drules_proto_*`, so a valid named area is reported as invalid. The checker is
stricter than the renderer/indexer and rejects normal map content.

Proof with official, unmodified CoMaps packages:

```text
minsk-pass.mwm:
BAD_FEATURE ... Types : internet_access-wlan ... geom=0
BAD_TYPE internet_access-wlan

Belarus_Minsk Region.mwm:
BAD_FEATURE ... Types : place-suburb ... geom=2
BAD_TYPE place-suburb
```

Both abort the checker exactly like the generated Curitiba package. Therefore
the Curitiba `boundary-protected_area`/`landuse-residential` failures are not
evidence that the source or generator output is invalid. Filtering those OSM
features was a diagnostic experiment, not the fix; it should not be shipped as
a data policy.

## Real generated-map route defect

The generated Curitiba package **without** cross-MWM rendered at 13 FPS, but a
route request aborted with:

```text
routing/index_router.cpp:378 Can't find path ...:
Reader::OpenException ... Can't find section:
.../Brazil_Parana_East.mwm cross_mwm
SIGABRT
```

The engine requires the `cross_mwm` section even for a self-contained region.
The first crash (`AStarAlgorithm::GetAdjacencyList`) was an out-of-map checkpoint
case (Minsk coordinates while the Curitiba map was active), not proof that the
generated graph itself is corrupt.

Rebuilt with the missing stage:

```sh
generator_tool ... --make_routing_index --generate_maxspeed \
  --make_city_roads --make_cross_mwm --output=Brazil_Parana_East
```

Result: `SerializeCrossMwm(): Cross mwm section generated, size: 29 bytes`,
transitions 0 (a single-region package has no cross-region transitions). A
route retest on this rebuilt package is still pending; it must use two
checkpoints **inside** the Curitiba package. The full official pipeline also
builds `world_roads` with `world_roads_builder_tool` and passes
`--world_roads_path`; that is required for multi-region/cross-border routing.

## Impediment to integrating CoMaps into Abastevo

1. **Not a published SDK.** `android/sdk` is an in-repository Android library
   (`namespace app.organicmaps.sdk`) that compiles the whole native core. It is
   not a stable Gradle artifact and has no compatibility/API contract. The AAR
   is a build output, not the supported distribution.
2. **Build chain.** It needs the pinned submodules, NDK 28.2.13676358, CMake
   `organicmaps` target, generated `classificator/types/visibility/colors/
   patterns`, Kothic drules, skin symbols/fonts and app resources. A clean build
   also needs Qt 6.5.3 + Positioning for the skin generator (build-only).
3. **Host contract.** `OrganicMaps` requires an application id, file-provider
   authority, location provider, version/flavor, writable storage and the
   MapView/lifecycle wiring. Abastevo must implement and test that host adapter;
   CoMaps does not provide a drop-in Compose screen.
4. **Size/ABI.** Measured: x86_64 AAR 26 MB, x86_64 debug APK 144 MB, arm64
   debug APK 229 MB, native `liborganicmaps.so` 45 MB (x86_64). A release
   arm64/arm32/x86_64 package, 16 KB page-size, minSdk 26 and download-size
   budgets are not yet measured.
5. **Custom overlay.** Stations/prices/charging assertions are Abastevo data,
   not OSM. The engine exposes search/place-page/routing and bookmark-like
   markers, but the ANP/Comunidade selector, custom marker symbols, quick sheet,
   station profile and route target need an Abastevo adapter over those APIs.
6. **Data publication.** `.mwm` packages are tied to the engine/map format and
   need a correct `cross_mwm`/world-roads section set. Abastevo still needs the
   signed manifest, download, atomic activation and independent overlay
   generation (MAP-09–13); map prices must not rebuild the road graph.
7. **Rights.** Core is Apache-2.0, but all 22 submodules, styles/fonts/symbols
   and OSM data (ODbL) need the pending transitive/license audit. Official
   CoMaps map packages are test inputs, not an assumed production CDN.

## Correct next gates

- Replace `--check_mwm` with engine-level acceptance: local package load,
  render, offline search and a route between two in-map checkpoints. Keep
  `check_mwm` only as a diagnostic; official maps prove it currently rejects
  valid content.
- Build the region with `--make_cross_mwm` and, for multi-region routing,
  `--world_roads_path` from `world_roads_builder_tool`.
- Then implement the Abastevo adapter (arm64/release/16 KB/lifecycle), the
  signed package manifest/download (the new `mapmanifest`/`mapverify` commands
  are the first pieces), and MAP-09–13 publication.
