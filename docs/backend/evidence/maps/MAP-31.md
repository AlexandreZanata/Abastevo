# MAP-31 — Final local regression, privacy and failure acceptance

Status: LOCAL_DONE (local source union); external/device/real-API cases remain OWED
Validation: PASS for the local union below
Integration: INTEGRATION_PENDING
Gate: DELIVERY_GATE is handled by MAP-33; PERFORMANCE_GATE remains PARTIAL (MAP-29)

Dependency: MAP-29 local proxy checkpoint and MAP-30 disposition. This record is the final
local union for the MAP batch; it does not certify staging, production, the Play release,
national capacity, a live charger network or a commercial SLA.

## Source union

`origin/dev` `507d915e` was merged into the MAP branch, then `origin/main` `4eeccfbf` was
merged normally (PROGRESS conflict resolved by retaining the landing entry and the MAP
history). The MAP engine files, patches, AAR, `tools/station-prep/src/location_review.rs` and
the source-offer/SBOM tree were checked present after the merge. The previous PR #150 is not
reused.

## Executed local union

- **Kotlin unit union:** `:domain:test :application:test :data:testDebugUnitTest
  :maps:engine:testDebugUnitTest :app:testDebugUnitTest` — 2,466 tests, 0 failures,
  4 opt-in skips (domain 777; application 481; data 616 with 4 skips; maps/engine 37;
  app 555). The four data skips are the explicit cross-runtime/live harnesses
  `TrainingConsentBackendInteropTest`, `ChargingOperationalWireTest`,
  `AnpListingLiveCatalogValidationTest` and `MapRealApiProofTest`; they require an external
  Go/PostGIS/TLS/API process and are not counted as passed.
- **Rust:** `cargo test --locked` on `tools/station-prep` — 38 tests, 0 failures;
  `cargo fmt --check` clean; `cargo clippy --locked --all-targets -- -D warnings` clean.
- **Go unit/race:** `go test -race ./...` all packages `ok`, 0 failures.
- **Real PostGIS integration:** fresh disposable database `anpfuel_map2831` in the synthetic
  PostGIS container, `go run ./cmd/migrate` applied 132 migrations, then
  `go test -race -count=1 -tags=integration ./...` with the five trusted Rust preparers —
  all packages `ok`, including `mapdata`, `mapdist`, `official`, `ledger`, `quality`,
  `payments`, `privacy` and `subscription`; no FAIL in the captured run.
- **Backend static:** `go vet ./...`, `gofmt -l` empty, `sqlc vet` clean,
  `GOTOOLCHAIN=go1.27.1 staticcheck ./...` clean (pinned workaround for the go1.27.2 export
  format), `govulncheck ./...` — no vulnerabilities found.
- **Android static/format:** `:app:lintDebug :data:lintDebug :maps:engine:lintDebug` BUILD
  SUCCESSFUL; `git diff --check` clean; baseline validation/secret scan passed.
- **Release:** `:app:assembleRelease :app:verifyReleaseApkSize` BUILD SUCCESSFUL. Final
  universal APK 78,396,538 bytes / 74.76 MiB, SHA-256
  `feadd9f34d60c26b57a2025e96a1b799c04b52c90da9d47c9784a430363536e0`; arm64-v8a + armeabi-v7a;
  every packaged `.so` LOAD `0x4000`; `zipalign -c -P 16 4` PASS; no MWM asset; minSdk 26
  manifest. Debug runtime evidence is not release-runtime proof.
- **Device/native:** MAP-23 phone guidance 3/3 + service 1/1 + engine 1/1 (off-route replay
  `RouteRebuilding`, recoverable after >60 s); MAP-27 real engine phone 1/1; MAP-28 real
  app/UI `EvTripUiInstrumentedTest` and 200%/320 dp accessibility passed on the owned 16 KB
  Android emulator; MAP-29 performance 3 emulator samples plus 3 physical-phone samples.
  Synthetic route/fix/charger fixtures are labelled; no private trace is stored.
- **Local restore rehearsal:** `pg_dump` of the disposable migrated DB restored into a new
  disposable database; both sides reported 132 migrations and 163 public tables. The staging
  database was not touched.
- **Privacy/licensing:** no new dependency or provider; no car library; no private GPS/route
  in logs, fixtures or Git; MAP-25 owner scope and MAP-24 map-generation leases remain intact;
  OSM/ODbL attribution, engine notices/SBOM/SOURCE_OFFER and the admitted AAR receipts were
  preserved. `:app`/`:data`/`:maps:engine` lint passed with MissingTranslation intact.

## OWED / not claimed

- The four opt-in Kotlin wire tests above: real Android↔Go↔PostGIS/TLS consent and charging
  flows, live ANP catalog and real map API decode. The Go/PostGIS side is green; the
  server→Kotlin→UI execution remains OWED with its exact environment variables.
- Two-account/two-device private backup/restore; manual TalkBack traversal; physical 16 KB
  runtime (the phone is 4 KB, the 16 KB proof is the emulator); clean ARMv7 runtime; API 26
  runtime; frame pacing/time-to-useful-content; current-head offline network-byte count;
  physical battery/GPS; dense/national regions; staging smoke; release R8 runtime.
- `PERFORMANCE_GATE` remains PARTIAL; MAP-30 is EXTERNAL_PENDING; MAP-32 is STAGING_PENDING.
  These are not converted into pass by this local union.
