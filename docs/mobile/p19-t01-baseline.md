# P19-T01 — Inherited feature and toolchain baseline

Status: LOCAL_DONE on `codex/phase-19-product-identity-foundation` (base `origin/main bad31b7`). Issue: #71. No production code changed in this task; inventory only.

## Implemented screens (actual, `app/src/main/kotlin/com/anpfuel/app`)

Routes (`navigation/Routes.kt`): onboarding, auth, home, search, location, prices, history, stations, stations/{fuelProduct}, settings, vehicles, week_picker, capture.

Nav graph (`navigation/AnpNavGraph.kt`) wires: Onboarding, Home, Search, LocationPicker, History, Prices, Settings, Stations, Vehicle, WeekPicker, Capture (+ Auth route). Community panels present as embedded components (`community/CommunityPricePanel`, `CommunityVotePanel`, `FeedbackDisplay`), not standalone tabs. No Explorar/Comunidade/Perfil shell yet — that is P19-T02/T04 work.

## Preserved free/offline contracts

- Three free vehicles: `domain/.../rule/MaxRegisteredVehiclesRule.kt` (`MAX_VEHICLES = 3`); enforced in `VehicleViewModel`.
- Offline/outbox: P10-T05/T08 integration preserved (cached reads, stable outbox IDs, retry); no new queue semantics in this task.
- Auth/social/media/location backend rules (P13–P16) reused as authoritative; this task adds no API change.
- Package `com.anpfuel`, existing modules (`domain`, `application`, `data`, `app`), MIT notices untouched.

## Toolchain pins (recorded, no upgrade)

`gradle/libs.versions.toml`: Kotlin 2.0.21, AGP 8.7.3, KSP 2.0.21-1.0.28, compileSdk 35, minSdk 26, targetSdk 35, Compose BOM 2024.12.01, Room 2.6.1, Hilt 2.53.1, OkHttp 4.12.0. Backend `go.mod`: go 1.27 (toolchain go1.27.1). Host observed: go1.22.2, Temurin JDK 21. No novelty upgrade performed; upgrades only on measured need.

## P18 gaps carried forward (not waived)

Per `p18-t01-acceptance-matrix.md` and local exit: full G18 NOT_ACCEPTED. Only 2 Android instrumented regressions passed (cached-home 382 ms, not cold start); full device matrix, cold-process/frame/heap/battery profiling, real S3-compatible storage matrix, and iOS native/device proof remain missing. iOS is archived (`archive/IOS_DEFERRED.md`, ADR-016) — resume only on explicit request. Outstanding Android/backend proof carries into P21-T04/P24, not claimed here.

## B-BR-C / BUC mapping (inherited state)

B-BR-C01–C06 / BUC-C01–C05 (`docs/product/COMMUNITY_EXPERIENCE.md`) describe the planned commercial UX, not shipped screens. Current backend contracts (consensus, feedback votes, 24 h media, location integrity) remain authoritative. New Explorar/Comunidade/Perfil/Atualizar-preço journeys are P19-T02..T04 and P20+ scope.

## Validation

- `git diff --check` clean (docs-only slice).
- Scoped secret review: no secrets/PII/GPS/photo content in this file.
- Baseline still compiles: `./gradlew :app:assembleDebug --no-daemon` (record outcome in task commit evidence).
