# P12-T01 — Toolchain and feature baseline

Status: LOCAL_DONE on `codex/phase-12-kmp-foundation`, base `6f4f048`.
Issue: #16. Phase: P12 — Kotlin Multiplatform foundation (milestone 3).
Entry gate: G09-LOCAL INTEGRATED (PR #14 merged as `6f4f048`, Quick verification
SUCCESS plus fast/integration/test SUCCESS on `2b1a55a`, wiki `136c6f9`).
This task performs no version upgrade and no behavior change.

## B-BR / BUC

No B-BR changed here. Preservation scope for the whole P12 migration:

- Existing ANP offline reads, search/filter/sort, comparisons/calculations,
  three free local vehicles, favorites, settings, navigation, local alerts and
  offline/outbox behavior stay intact (MIGRATION_PLAN + docs/user-business-logic.md).
- Future targets stay owned elsewhere and are NOT IMPLEMENTED here:
  B-BR-F01…F08 / BUC-F01…F05 (P14/P17), B-BR-A01…A05 / BUC-A01…A04 (P13),
  B-BR-M01…M06 / B-BR-L01…L04 (P15/P16). v1 contracts stay compatible.

## Frozen pins (source of truth)

From `gradle/libs.versions.toml`, `gradle/wrapper/gradle-wrapper.properties`
and module build files at base `6f4f048` (same as audit baseline `c51fa03`
plus backend-only `8c5e795`):

- Gradle 8.10.2, AGP 8.7.3, Kotlin 2.0.21, KSP 2.0.21-1.0.28.
- Java: `jvmToolchain(17)`, Android `source/target 17`, `kotlinOptions.jvmTarget 17`.
  Gradle daemon on this host runs on JDK 21 (Temurin 21+35); toolchain 17 is
  what compiles `:domain`/`:application`.
- Android: minSdk 26, compileSdk/targetSdk 35, versionCode 4 / versionName 3.1.0.
- Compose BOM 2024.12.01, coroutines 1.9.0, Room 2.6.1, Hilt 2.53.1,
  OkHttp 4.12.0, WorkManager 2.10.0, datastore 1.1.1, junit5 5.11.4,
  mockk 1.13.13, turbine 1.2.0, POI 5.3.0.
- Modules today: `:domain`/`:application` Kotlin JVM, `:data`/`:app` Android.
  No KMP `commonMain`, no `iosApp`, no shared framework yet (by design).

## KMP compatibility decision (no upgrade in T01)

Per `docs/mobile/KOTLIN_MULTIPLATFORM_AUDIT.md`:

- Kotlin 2.4.20 (2026-09-07) is a migration *candidate*, not an approved pin.
- KMP 2.0.21 matrix lists Gradle ≤8.8 / AGP ≤8.5; repo Gradle 8.10.2 / AGP 8.7.3
  are outside that KMP range. This does not claim the Android build is broken.
- KMP 2.4.20 lists Gradle 7.6.3–9.7.0, AGP 8.5.2–9.3.1, Xcode 26.4.
- Swift export is Alpha: first iOS app uses framework interop + Swift wrapper,
  one umbrella framework, narrow DTO/use-case API.
- Targets when implemented: `iosArm64`, `iosSimulatorArm64`.
- Trial upgrade + plugin-compatibility + rollback builds belong to P12-T02…T04;
  T01 freezes the above as the rollback baseline.

## Environment results (this task)

Host: Linux x86_64, JDK 21 daemon, Android SDK platforms 34/35/36/36.1 present.
Base SHA `6f4f048`, clean tree at start (git-flow `start --phase 12`).

Exact validation command (from ROADMAP P12-T01):

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
```

Outcome on this host (rerun, not UP-TO-DATE):

- `:domain:test` + `:application:test`: 87 suites, 430 tests, 0 failures/errors.
- `:data:testDebugUnitTest` + `:app:testDebugUnitTest`: 49 suites, 167 tests,
  0 failures/errors, 1 skipped.
- `:app:assembleDebug`: SUCCESS, `app-debug.apk` produced.
- Total: 597 unit tests, 1 skipped, 0 failed.

`git diff --check` clean. `make quick-verify` result is recorded in the task
commit evidence (bounded gate, not a release certification).

## Supported-device baseline (frozen as proposal, validation deferred)

- Android: min API 26 preserved; compile/target 35. Low-resource reference
  device and current-device matrix are frozen at G12/P12-T05 with measured
  cold-start/list/photo/outbox/heap/frame budgets (today hypotheses only).
- iOS: `iosArm64` device + `iosSimulatorArm64`, Xcode 26.4 on macOS.
  Explicit limit: this Linux run certifies **no** iPhone build. Framework/shell
  compile + simulator/device evidence require macOS and belong to P12-T04/T05.
- Instrumentation (`connectedDebugAndroidTest`), performance profiling and
  accessibility/device acceptance belong to P10-T08/P12-T05/P18, not claimed here.

## Dependency need/license/security note

Review is inventory-only in T01: no new dependency added. Hilt/Room/WorkManager/
OkHttp/jsoup/POI stay Android/data adapters behind ports; Hilt stays out of
future common code with explicit DI. Full dependency/security drift evidence
belongs to P12-T05; known-vulnerability remediation never waits for release.

## Rollback / recovery

T01 changes docs/ledger/delivery tooling only; product rollback = no-op.
Future plugin/source-set moves keep the Android baseline green per slice and
revert to these pins on failure. Append-only migrations; prior local data kept.

## Delivery evidence

- Branch `codex/phase-12-kmp-foundation` from `origin/main 6f4f048`.
- Issues #16–#20 (milestone 3) created via fixed `scripts/issues.sh`
  (gh 2.45 `issue create` has no `--json`; parse trailing number from URL/bare
  output; harness `test-issues` still 23/23). Missing labels
  `phase:P12`/`type:task`/`priority:must`/`risk:standard` created.
- Files: this baseline doc, `phase-ledger.json` current-phase fields,
  `PROGRESS.md` pointer, `scripts/issues.sh` compatibility fix.
- Tests: Android baseline above; `make quick-verify` + `git diff --check`.
- Next: P12-T02 portable domain/money contracts. Issue #16 stays open until
  the P12 phase PR merges.
