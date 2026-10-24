# MAP-04 — OSM reference and location candidates in Rust

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (source, synthetic PBF and one real regional sample)
Integration: INTEGRATION_PENDING

Tested source: `codex/map-ev-implementation` after MAP-03 plus this change.
Date: 2026-10-24.

## Behavior rules and use cases

- Rust reads one admitted **local** `.osm.pbf`; no network, database, engine or
  original-source scraping occurs inside the preparer.
- Fuel (`amenity=fuel`/`fuel=yes`) and charging
  (`amenity=charging_station`) nodes, ways and relations keep their OSM type/id,
  name, address/access/hours/operator/brand tags and every unknown tag.
- Node geometry is `osm-mapped` with an `osm_node` display point. Way/relation
  centroids are `osm-mapped` with `osm_centroid`, explicitly approximate and
  distinct from Abastevo-reviewed geometry. Missing geometry stays `unknown`
  with null coordinates; no city centroid is invented.
- OSM identity remains `osm:<type>:<id>`; nearby elements are never merged.
  Duplicate source IDs are recorded once and counted as duplicates.
- CNPJ-like `ref:vatin`/`ref:CNPJ` is normalized only when valid; otherwise the
  raw tag is preserved and the CNPJ field stays null.
- Tags over 64 entries or 256 bytes, NUL bytes, incomplete geometry and
  out-of-region coordinates are quarantined with a reason. Census input,
  matched, accepted, duplicates, quarantined and missing geometry always
  reconcile.
- Cancellation is checked per blob and per candidate; a cancelled parse returns
  no run. The output writer builds a complete temporary generation and swaps it
  atomically, preserving the previous directory on failure.
- ODbL attribution travels in every reference and in the manifest.

## Implementation

- `tools/station-prep/src/osm_reference.rs`: bounded parser, deterministic
  NDJSON/manifest output and atomic replacement.
- `tools/station-prep/src/bin/prepare_osm.rs`: local CLI
  (`--input/--output/--bbox/--max-input-bytes/--max-matched`).
- Contract extension: `osm-mapped` quality plus `osm_node`/`osm_centroid`
  display points in the MAP-03 schema and Go/Rust/Kotlin readers. Tests still
  reject a city centroid used as a pin.
- Dependency: `pbf-craft 1.0.3` (MIT), pinned in `Cargo.lock`. Transitive
  licenses reviewed from `cargo metadata`: MIT/Apache-2.0, BSD-2/3, Zlib,
  Unlicense and Unicode-3.0; no GPL-only or unknown-license dependency. It is a
  local reader/writer only; it does not replace the native map generator.
- No new Go/PostGIS table, endpoint, migration or public artifact was added.

## Verification performed

```sh
cargo fmt --manifest-path tools/station-prep/Cargo.toml --all -- --check
cargo test --locked --manifest-path tools/station-prep/Cargo.toml --lib osm_reference
# 4 passed: node/way/relation, unknown tags, missing geometry, outside bbox,
# duplicate ids, oversized tags, truncated input, cancellation, atomic output
cargo test --locked --manifest-path tools/station-prep/Cargo.toml
# full station-prep suite passed
cargo clippy --locked --manifest-path tools/station-prep/Cargo.toml --all-targets -- -D warnings
# finished, no warnings
cargo build --locked --manifest-path tools/station-prep/Cargo.toml --release --bin prepare_osm
git diff --check
```

### Real regional sample outside Git

- Source: `https://download.bbbike.org/osm/bbbike/Curitiba/Curitiba.osm.pbf`
- Last-Modified: 2026-10-17; bytes 9,069,699; SHA-256
  `3ab1872056d2f6c87f8d58595c626cfa25f1a4d82dd49468b9dad13e5faac266`.
- ODbL attribution retained; extract stored only at
  `/data/dev/src/osm/Curitiba.osm.pbf` (not committed).
- Command:
  `prepare_osm --input Curitiba.osm.pbf --output reference --bbox -49.5,-25.7,-49.0,-25.2`
- Result: 831,646 input elements; 256 matched; 256 accepted; 0 duplicates;
  0 quarantined; 0 missing geometry; 245 fuel and 11 charging.
- Output hashes: `references.ndjson` 142,536 bytes
  `860ac0cfc4f6a3952f26fad1dd81145032b93142c21c92d727cfd5e1024a331c`;
  `decisions.ndjson` 25,376 bytes
  `5e49deb9196c14678000daedc0431c93d7c5319fb3195073722ee10944cbb186`;
  `manifest.json` 794 bytes
  `75fe7db25ae9896829ac4f698be4b39f9227f04062cc20b33595b151f2ee6394`.
- Output directory: `/data/dev/src/osm/reference` (outside Git).
- Inspection confirmed names such as “Posto Ipiranga”/“Shell”, way centroids
  (`osm_centroid`), retained `brand`/`opening_hours` tags and ODbL attribution.

## Limits

- Only a city extract was qualified; no national Brazil extract, national
  coverage, completeness or production publication is claimed.
- Relation handling covers direct and nested node/way/relation dependencies via
  the pinned reader; circular relations are rejected by that reader.
- The reader creates a `.pif` index beside the input. Go staging must use a
  disposable writable working directory; the admitted source file itself is not
  modified by this repository.
- No PostGIS staging, API, map package, Android UI or device evidence exists yet.
- MAP-05/06 prepare prices/charging; MAP-08/09 generate, publish and serve.
  This task does not authorize source activation or a public overlay.

Next: MAP-05 exact per-station ANP price facts in Rust.
