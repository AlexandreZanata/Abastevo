# MAP-03 — Map artifacts and cross-language contracts

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (contracts, goldens and readers)
Integration: INTEGRATION_PENDING

Tested source: `codex/map-ev-implementation` after MAP-02. Date: 2026-10-24.

## Behavior frozen

- A site assertion is source-specific. `ANP`, `COMMUNITY`, `OSM` and `OCM`
  assertions are merged only by explicit station/place/equipment/connector
  identity, never by distance.
- Money is exact integer `amount_milli_brl` (1..1,000,000) with an explicit
  product/unit pair. `CNG=M3`, `LPG_P13=KG_13`; all other products use `L`.
  Unknown price is an empty list, never zero.
- ANP prices require a survey period; community projections forbid one.
- Reviewed coordinates may be a station/entrance display point. Unknown geometry
  is list-only. A city centroid is never a pin. Display point, entrance and road
  access remain separate.
- ANP/OSM assertions cannot publish charging facts. OCM/community charging
  assertions keep unknown power/tariff null; a condition without a tariff is
  rejected. `live_status` is false in the open-only launch.
- Rights must be admitted, source-matched, attributed and hash-pinned. Unknown
  rights reject the whole generation.
- Native manifests pin engine/generator SHA, format generation, region/bbox,
  package bytes/SHA-256, safe paths, complete dependencies and commercial
  offline redistribution rights. Missing/unsafe dependencies reject activation.
- Prepared envelopes reuse the existing manifest/hash/census discipline and keep
  `REFERENCE`, `PUBLIC_OVERLAY` and `PRIVATE_STATE` separate. Private state can
  never be public.

## Artifacts

- `contracts/map/poi-assertions-v1.schema.json`
- `contracts/map/native-map-manifest-v1.schema.json`
- `contracts/map/prepared-envelope-v1.schema.json`
- `contracts/testdata/map/poi-assertions-v1.json`
  SHA-256 `bbec233539752e242f6e64ee42ee7923cf0db0825a0fb8a6e65a5cfe67ab2d68`
- `contracts/testdata/map/native-map-manifest-v1.json`
  SHA-256 `c490ebbeaf32b2d376d6bc081d2ac90296217ebad94235c28decb4491b87093c`
- `contracts/testdata/map/prepared-overlay-v1.json`
  SHA-256 `b263e34210ec1419f9edd3063ccfa8e517b8f5e6f41581238245e85ece4c58ce`
- Rust reader: `tools/station-prep/src/map_contract.rs`
- Go reader: `backend/internal/platform/mapcontract`
- Kotlin domain values: `domain/.../map/MapDiscovery.kt`; strict wire decoder:
  `data/.../mapper/MapPoiCodec.kt`

## Verification performed

```sh
# JSON Schema + goldens
python3 -m json.tool ...            # all valid
jsonschema Draft202012 check/validate
# poi-assertions-v1, native-map-manifest-v1 and prepared-envelope-v1 OK

# Rust
cargo fmt --manifest-path tools/station-prep/Cargo.toml --all -- --check
cargo test --locked --manifest-path tools/station-prep/Cargo.toml --lib map_contract
# 4 passed
cargo clippy --locked --manifest-path tools/station-prep/Cargo.toml --all-targets -- -D warnings
# finished, no warnings

# Go
GOTOOLCHAIN=go1.27.3 go test ./internal/platform/mapcontract/...
GOTOOLCHAIN=go1.27.3 go vet ./internal/platform/mapcontract/...
# ok

# Kotlin
GRADLE_USER_HOME=<pinned cache> ./gradlew :domain:compileKotlin :data:compileDebugKotlin
GRADLE_USER_HOME=<pinned cache> ./gradlew :data:testDebugUnitTest \
  --tests 'com.anpfuel.data.mapper.MapPoiCodecTest'
# BUILD SUCCESSFUL
```

Rejections exercised in at least one reader per case: unsupported version,
duplicate assertion ID, unknown field, malformed/truncated JSON, wrong
product/unit, money overflow, missing station identity, ANP charging facts,
rights/source mismatch, city-centroid pin, CNPJ leading-zero preservation,
unknown charging power/tariff, missing manifest dependency, unsafe path, bad
digest, unpinned engine, inverted bbox and denied commercial/offline rights.

## Limits

- No producer or public API consumes these contracts yet (MAP-04 onward).
- No map data, native engine, database, deployment or device behavior was added.
- Kotlin manifest decoding is not yet wired into downloads (MAP-12/MAP-15).
- No national coverage, production or release claim.

Next: MAP-04 OSM reference preparation in Rust.
