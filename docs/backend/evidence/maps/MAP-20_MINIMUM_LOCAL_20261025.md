# MAP-20 minimum map — local real-data acceptance (2026-10-25)

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS for the local scoped checks below
Gate: MINIMUM_MAP_GATE **LOCAL PASS**; STAGING and PRODUCTION remain PENDING

This record supersedes only the earlier statement that real reviewed geometry and
station prices were unavailable in the current construction chain. It does not
claim staging/production activation, MAP-21–33, native guided navigation or
public release certification.

## Data chain actually executed

1. **Official ANP registry** (`dados-cadastrais-revendedores-varejistas-combustiveis-automoveis.csv`):
   `https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/arquivos-dados-cadastrais-dos-revendedores-varejistas-de-combustiveis-automotivos/dados-cadastrais-revendedores-varejistas-combustiveis-automoveis.csv`
   fetched 2026-10-25, 7,899,851 bytes, SHA-256
   `46d38fbe402fbb15a8c8e73ae3d78f126d040230a5acc55d56077a2aecc1dabe`.
   `prepare_registry` produced 45,706 accepted stations / 85 quarantined;
   `station-load --publish` loaded them into a disposable PostGIS database
   (all coordinates initially `unknown`).
2. **Official ANP station survey** (`revendas_lpc_2026_10_11-2026_10_17.xlsx`):
   `https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/revendas_lpc_2026_10_11-2026_10_17.xlsx`
   fetched 2026-10-25, 1,875,300 bytes, SHA-256
   `e148758e496dfa3166210358185d05c27ac4cd8c20e5071979d5559e3faaa6d6`.
   The bounded Rust parser accepted 19,561 exact facts / quarantined 928
   (4.5%; 877 are Excel binary-float artifacts, 51 invalid CNPJ). No value was
   rounded or invented. The source workbook is **not** committed.
3. **ANP registry API for Curitiba**:
   `https://revendedoresapi.anp.gov.br/v1/combustivel?uf=PR&municipio=CURITIBA&numeroPagina=1&tamanhoPagina=5000`,
   fetched 2026-10-25, 412,967 bytes, SHA-256
   `687edec00adf428a18713f3992ac1c7fb09eca7b2fd7993282bff789f6b1dbd4`.
   It supplied 348 official station records with reported points and provenance;
   `validacao = "Posição Geográfica não validada"` is retained, not relabeled as
   field-verified.
4. **Independent OSM geometry** from the admitted Curitiba extract
   (`/data/dev/src/osm/Curitiba.osm.pbf`, SHA-256
   `3ab1872056d2f6c87f8d58595c626cfa25f1a4d82dd49468b9dad13e5faac266`,
   ODbL-1.0, 256 fuel/charging references). `prepare_location_reviews` matched
   ANP records only by exact normalized street + house number, required a
   unique OSM candidate within 50 m, and retained both observations,
   `brand_match`, original ANP validation/origin and timestamp:
   348 input → **62 reviewed** / 286 rejected (276 no OSM match, 4 ambiguous,
   3 no ANP point, 2 registry-address mismatch, 1 over distance).
   Review artifact SHA-256 `2abe8c48439218870f86930d72f4eb860e4b0bed8194f1870603e3703c4598ea`.
   `station-review` applied all 62 to canonical Directory UUIDs; no centroid or
   proximity-only link was created.
5. **Exact prices**: `station-prices` loaded only the 33 reviewed CNPJs that
   occur in the survey week into the Official module (quarantine 0, published
   revision `394ea0a5-8004-43b4-be3c-aef23669568b`). Unreviewed stations remain
   visible with no price; no municipal average is substituted.
6. **Real local API** served Directory and Official reads. The app build used the
   local proof switch (`mapApiLocalProof=true`) and the admitted MAP-14 MWM
   packages (Brazil_Parana_East `d1e9b2ce…00a3e`, World `77a35c16…e4a5`,
   WorldCoasts `d42a71b1…3626`; ODbL-1.0) through the existing signed local
   proof server. The release build keeps empty distribution settings and
   `MAP_API_LOCAL_PROOF=false`.

## Native/UI proof

Android 15 / API 35, 16 KB page-size x86_64 AVD `anpfuel-16k`, SwiftShader,
4 GiB, read-only/no-snapshot, ARM64 native bridge, OpenGLES3 renderer.
The first Vulkan render attempt produced severe SwiftShader artifacts; setting
`PreferredGraphicsAPI=OpenGLES3` produced the clean render below. This is a
local emulator profile, not a new all-device certification.

- [Map with 62 native fuel pins](MAP-20-minimum-local-20261025/map-pins.png)
  (no technical UUID label; marker labels use station names and cluster counts).
- [Exact quick sheet](MAP-20-minimum-local-20261025/map-sheet-price.png):
  `Regular gasoline R$ 6,79 L`, `Standard`, `2026-10-11..2026-10-17
  (2026-10-13)`, `Reviewed location`, ANP source.
- [Existing station profile](MAP-20-minimum-local-20261025/map-profile.png).
- Tapping a native pin without a survey price opened the sheet with the explicit
  `No price for this station.` state (no fallback/average).
- [Offline after process restart](MAP-20-minimum-local-20261025/map-offline.png):
  Wi-Fi/data disabled, API reverse removed, app force-stopped and reopened;
  62 pins/list items came from the separate Room public-page cache with the
  visible `Showing last known data (offline cache).` banner.
- [Offline quick sheet](MAP-20-minimum-local-20261025/map-offline-sheet.png):
  cached exact price is labeled `Cached price (offline)` with its survey date.
  The native MWM package and the public overlay cache are distinct generations.
- GPS was denied during the flow; the default Curitiba city/viewport still
  loaded pins. No “open now” claim is made; opening-hours metadata is unknown.

## Tests and checks

```sh
cargo test --locked --manifest-path tools/station-prep/Cargo.toml        # PASS
cargo fmt --manifest-path tools/station-prep/Cargo.toml --all -- --check # PASS
cargo clippy --locked --manifest-path tools/station-prep/Cargo.toml \
  --all-targets -- -D warnings                                           # PASS
GOTOOLCHAIN=go1.27.3 go test ./cmd/station-review ./cmd/station-prices \
  ./internal/modules/official/...                                        # PASS
ANPFUEL_TEST_DATABASE_URL=... go test -tags=integration -count=1 \
  ./internal/modules/official/adapters -run TestFailedRunRetriesSameIdentity # PASS
./gradlew :maps:engine:testDebugUnitTest :data:testDebugUnitTest \
  :app:testDebugUnitTest                                                 # PASS
# engine 32, data 612 (2 known external skips), app 537; 0 failures
ANPFUEL_MAP_PROOF_BASE_URL=http://127.0.0.1:18080 \
  ./gradlew :data:testDebugUnitTest --tests '*MapRealApiProofTest'       # PASS (real HTTP)
./gradlew :app:lintDebug                                                 # PASS, no suppression
./gradlew :app:assembleRelease :app:verifyReleaseApkSize                 # PASS
```

APKs:

- proof debug: 100,294,961 bytes, SHA-256
  `793e278a7bbb410a959199d0cae4b9a45b6870dea979f1042b77cd4fdf6fae08`;
  local API + ephemeral proof origin/key only.
- unsigned release: 78,121,062 bytes / 74.50 MiB, SHA-256
  `5937649c14078bf82460376ee22656241ca80b804aea4566e0ef672f54deabdd`;
  below the unchanged 80 MiB ceiling; `MAP_API_LOCAL_PROOF=false`,
  `MAP_DIST_BASE_URL=""`, `MAP_DIST_KEYS=""`. The engine AAR is unchanged from
  the admitted MAP-14 receipt, so its 16 KB ELF/ZIP acceptance remains the
  earlier verified artifact.

## Behavior changes in this slice

- ANP workbook parser accepts the official full state names and
  money-per-quantity unit labels (`R$ / litro`, `R$ / m³`, `R$ / 13 kg`) and
  left-pads only short numeric CNPJs whose checksum validates.
- `station-review` requires a manifest-verified review artifact and an existing
  active canonical CNPJ; it records the evidence string and never creates a
  station.
- `station-prices` loads Rust-prepared facts through the Official importer and
  retries a failed run under its existing identity (unique-index retry bug fixed).
- Native KML no longer exposes `ABASTEVO:<uuid>`/cluster IDs; selection maps the
  exact rendered point back to the canonical UUID, and ambiguous points fail
  closed.
- Search is a mode: native camera sync cannot replace searched pins; clearing
  the query returns to nearby results at the latest camera.
- `KeyFactory("Ed25519")` resolving AndroidKeyStore now falls back to the owned
  Monocypher verifier instead of treating the provider mismatch as a bad
  signature. The signed local MWM download verified and activated.
- Network-off map uses the existing Room `ServerStationCache` public-page scope
  and the BackendPriceCache, with an explicit cached banner/label.

## Remaining boundaries

- Staging `teste.abastevo.com.br` was not changed; staging still needs the real
  reviewed geometry/prices and the production distribution origin/keys.
- Production distribution, release signing, public pilot and G09 remain
  separate gates. The local proof key/private key are not production material.
- Opening-hours metadata is not supplied by the current ANP/community
  contracts; the UI keeps it unknown and no open-now filter is claimed.
- Charging/OCM offline acceptance, MAP-21–33, guided navigation/voice/trips/EV
  planning and performance budgets remain owned by their phases.
- The 16 KB native render here used OpenGLES3/SwiftShader; Vulkan artifacts on
  this emulator profile were rejected, not accepted as a device result.

## Integration with maintained main — MAP-21–23 continuation

The base merged PR 150 includes a newer atomic Official import contract: a
terminal failed identity is retained as a no-op; only changed source bytes or
a changed parser identity opens a new attempt. Its transaction/advisory-lock,
window fencing and frozen revisions supersede this branch's earlier standalone
retry patch. The continuation retains that production implementation and tests
terminal replay preservation plus a distinct corrected-parser publication. This
is not permission to retry away quarantine verdicts or revive revoked sources.
