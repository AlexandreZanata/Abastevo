# MAP-00 — Actual baseline and upstream inventory

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (inventory scope only)
Integration: INTEGRATION_PENDING

Date: 2026-10-24
Tested source: `bf32027d4e5408fb530b521942e24c7b29c8ceef` on
`codex/map-ev-implementation`, derived from maintained `dev` after integrating
planning commit `7cd99ef5`.
Upstream source inspected: `comaps/comaps`
`4587b8ad4d12c9f59982709fdaf03cc925948827` (2026-10-23), pinned locally outside
Git at `/data/dev/src/comaps-4587b8ad`.

## State separation

- Tested source: the commit above plus the tests named in this record.
- Deployed source: API/worker `53719336`, schema `120`, only on
  `https://teste.abastevo.com.br`; this is a test environment, not production.
- Device/manual: not executed here. The historical Android manual batch remains
  OWED; no device result is inferred.
- Production, public pilot and release: NOT CLAIMED.

## Actual repository versions

| Item | Actual value | Source |
| --- | --- | --- |
| Android compile/target SDK | 35 / 35 | `gradle/libs.versions.toml` |
| Android minSdk | 26 | `gradle/libs.versions.toml` |
| AGP / Kotlin / Gradle | 8.7.3 / 2.0.21 / 8.10.2 | version catalog, wrapper |
| Room | 2.6.1, schema version 25 | catalog, `AnpFuelDatabase.kt` |
| PostgreSQL migrations | append-only head `000120` | `backend/db/migrations` |
| Go | `go 1.27`, toolchain `go1.27.3` | `backend/go.mod` |
| Rust | `1.99.0` | `backend/README.md` |
| Installed Android NDK/CMake | 28.2.13676358 / 3.22.1 | local SDK |

The local shell currently resolves Go 1.22.2; backend commands must request the
pinned toolchain (`GOTOOLCHAIN=go1.27.3`). Missing pinned tools are a failed
prerequisite, not a skip.

## Upstream capability inventory

Pinned source evidence:

- `LICENSE` SHA-256 `09e8a9bcec8067104652c168685ab0931e7868f9c8284b66f5ae6edae5f1130b`
  (Apache-2.0).
- `NOTICE` SHA-256 `307a6cfee51a6a2c9f8adf7d1e1a2e8f0cfa4dbe7e7ce5ab97d24558eb3e6acc`
  (My.com/Organic Maps/CoMaps copyrights; third-party directories excluded from
  the core license).
- `data/copyright.html` SHA-256
  `5d0156f5db0b4ec9c3e281b4fb239393e7f4484fa8ff750497ecbe62aa08121e`.
- 22 Git submodules, including Boost, protobuf, ICU, FreeType, HarfBuzz,
  Vulkan-Headers, Expat, GLM, googletest, osmctools and kothic. Exact gitlinks
  are recorded by the pinned commit; individual transitive licenses and the
  copyright inventory remain `ADMISSION_PENDING` until MAP-01/MAP-14 review.
- Android module `android/sdk` exists as an in-repository Gradle library
  (`namespace app.organicmaps.sdk`, minSdk 21 in upstream, NDK 28.2.13676358,
  CMake target `organicmaps`). It is **not** a published drop-in SDK artifact
  and still compiles the full native core plus submodules/resources.
- Native build entry: repository `CMakeLists.txt`, `configure.sh`,
  `tools/unix/build_omim.sh`; map generation: `tools/python/maps_generator`,
  with `data/countries.txt`, styles, fonts, borders and world/coastline inputs.
- Map package format: `.mwm` (versioned region files); sample
  `data/minsk-pass.mwm` exists for tests. Downloader URLs are configurable; no
  upstream CDN entitlement is assumed.
- Full upstream build guidance: 4 GB RAM and 20–30 GB disk, 2–5 GB downloads;
  this workstation currently has 31 GB RAM but only ~4 GB free on the root
  filesystem. Any native build must use `/data` (323 GB free) and a measured
  budget; this is not yet a build proof.

### Kept, adapted, excluded

| Capability | Disposition |
| --- | --- |
| Offline vector render, pan/zoom/compass | Keep, behind Android adapter |
| Offline search and car routing/guidance | Keep as candidate; proof required at MAP-14/MAP-22 |
| Regional `.mwm` download/update | Adapt to Abastevo signed manifests and host |
| Custom selectable POI/overlay/hit test | Adapt; required at ENGINE_GATE |
| Favorites, saved trips, KML/GPX | Adapt later, private by default |
| Night style/accessibility controls | Adapt to app theme and TalkBack |
| Upstream reviews/editor/OSM write | Exclude |
| Hiking/cycling/transit/tourism encyclopedia | Exclude |
| Upstream CDN/demo services as production dependency | Exclude |
| iOS/CarPlay, payments/OCPP, live traffic | Exclude from this scope |

## Reuse already present in Abastevo

- Bottom navigation and single-top tab behavior:
  `app/.../navigation/AbastevoBottomBar.kt`, `TabNavigation.kt`.
- Station identity/quality and directory transport:
  `domain/.../discovery/ServerStation.kt`,
  `domain/.../discovery/DirectoryStationRepository.kt`.
- Rust deterministic preparers and EV envelopes: `tools/station-prep`.
- Charging snapshots/compatibility/Room import:
  `data/.../local/charging`, `ChargingDiscoveryRepository`.
- Existing community/private ledger and signed outbox.
- The latest documented runtime gaps (unregistered charging handlers, incomplete
  charge selectors, unpublished charging catalog) remain MAP-02 work; they were
  not re-proved by this read-only inventory.

## Risks and limits

- No CoMaps native build, AAR, APK, render, search or route was executed here.
  The upstream size and the current root-disk constraint make the engine proof
  a separate gated task, not an assumed success.
- Transitive license/admission, third-party artwork/fonts/styles and hosting
  terms are not accepted merely by the Apache-2.0 core license.
- OSM data remains ODbL; ANP/OCM/provider terms remain separate.
- No national coverage, live availability, production readiness or device pass
  is claimed.

## Exit

A scoped, reproducible source/feature/dependency inventory now identifies the
next task (MAP-01 behavior/source admission), the exact upstream revision and
the build constraints. No migration number was allocated from the plan.
