# MAP-05 — Exact per-station ANP price facts in Rust

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (synthetic official-layout workbook and cross-language parity)
Integration: INTEGRATION_PENDING

Tested source: `codex/map-ev-implementation` after MAP-04. Date: 2026-10-24.

## Behavior rules and use cases

- Rust reads the local ANP `POSTOS REVENDEDORES` station sheet only; it never
  reads the `MUNICIPIOS` average sheet as a station price and never uses network,
  database or provider scraping.
- Exact source identity: normalized 14-character CNPJ (leading zeros preserved),
  municipality/UF, brand, trade/legal name and source row. Canonical station UUID
  linkage remains MAP-07; this task emits unlinked station facts.
- Products use the frozen taxonomy and unit pair:
  `GASOLINA COMUM→GASOLINE_REGULAR/L`, `GASOLINA ADITIVADA→GASOLINE_ADDITIVED/L`,
  `GASOLINA PREMIUM→GASOLINE_PREMIUM_GRADE/L`,
  `ETANOL HIDRATADO→ETHANOL_HYDRATED/L`,
  `DIESEL S500/S10→DIESEL_S500/S10/L`, `GNV→CNG/M3`, `GLP→LPG_P13/KG_13`.
- Prices convert from the source lexeme to exact milli-BRL with at most three
  decimals. Zero, negative, more than three decimals, out-of-range values and
  unit mismatches quarantine; nothing is rounded or zero-filled.
- The explicit survey period (`YYYY-MM-DD`) must contain the row's collected
  date. Rows outside it quarantine.
- Duplicate station/product/date rows with the same price count as duplicates;
  conflicting prices for the same key quarantine instead of picking one.
- Any header shift, wrong sheet name, truncated workbook or invalid period
  refuses the whole run without partial output. The existing Android importer
  remains authoritative for its offline screens; no store migration occurred.
- An additive Android mapping was added for `GASOLINA PREMIUM`/
  `GASOLINA PREMIUM GRADE` → `GASOLINE_PREMIUM_GRADE`; existing labels keep
  their previous mapping.

## Implementation

- `tools/station-prep/src/anp_prices.rs`: bounded XLSX reader, exact parser,
  reconciliation, typed quarantine and atomic output.
- `tools/station-prep/src/bin/prepare_anp_prices.rs`: local CLI with explicit
  period and source revision.
- `tools/station-prep/src/xlsx.rs`: absolute worksheet targets (`/xl/...`) are
  normalized alongside relative targets; the workbook profile is otherwise
  unchanged.
- `contracts/testdata/map/anp-station-prices-v1.xlsx`: synthetic workbook
  generated with openpyxl 3.1.5 (no real station data), 15 data rows covering
  valid/missing/wrong/duplicate/out-of-period/conflicting cases. SHA-256
  `d87fc74def27e107a7c7a8108c6fa3e014bfbc4352217ac7afd8703f5f6d68a8`.
- Android parity fixture: `AnpStationPriceFixtureTest` parses the same workbook
  with `StationDetailSheetParser` and maps the same products.

## Verification performed

```sh
cargo fmt --manifest-path tools/station-prep/Cargo.toml --all -- --check
cargo test --locked --manifest-path tools/station-prep/Cargo.toml --lib anp_prices
# 4 passed
cargo test --locked --manifest-path tools/station-prep/Cargo.toml
# full station-prep suite passed
cargo clippy --locked --manifest-path tools/station-prep/Cargo.toml --all-targets -- -D warnings
# finished, no warnings

cargo build --locked --manifest-path tools/station-prep/Cargo.toml --release --bin prepare_anp_prices
prepare_anp_prices --input contracts/testdata/map/anp-station-prices-v1.xlsx \
  --output /data/dev/src/anp-prices --period-start 2026-10-18 \
  --period-end 2026-10-24 --source-revision survey-2026-10-24
# {"accepted":7,"duplicates":1,"quarantined":7,"input":15}

GRADLE_USER_HOME=<pinned cache> ./gradlew :data:testDebugUnitTest \
  --tests 'com.anpfuel.data.parser.AnpStationPriceFixtureTest' \
  --tests 'com.anpfuel.data.mapper.MapPoiCodecTest'
# BUILD SUCCESSFUL
GRADLE_USER_HOME=<pinned cache> ./gradlew :domain:test --tests 'com.anpfuel.domain.rule.*'
# BUILD SUCCESSFUL
git diff --check
```

Observed exact facts in `/data/dev/src/anp-prices`: 5.999 L gasoline →
`5999` milli-BRL; 4.499 M3 CNG → `4499`; 109.99 KG_13 LPG → `109990`;
6.450 L diesel → `6450`. Output hashes: `prices.ndjson` 3,551 bytes
`b5bbe330…540cce2b`; `duplicates.ndjson` 509 bytes `33914ac9…1d6625`;
`quarantine.ndjson` 1,482 bytes `65de9888…675c999`; manifest records the source
SHA-256 and period.

## Limits

- The workbook is synthetic; no real ANP station file was downloaded or
  redistributed in Git. Exact current ANP source terms/bytes remain
  `PENDING` in the source register for a real admitted run.
- No canonical station UUID linkage, PostGIS publication, API or Android map
  consumption yet (MAP-07/09/10/13).
- No municipal average, distributor price, electric tariff or LPG cylinder
  mislabeling is accepted.
- No national coverage, production deployment or release claim.

Next: MAP-06 admitted charging and vehicle reference data.
