# Backend toolchain and inherited baseline

Status: P01-T01 evidence. Resolves D01 (runtime versions) for local/CI use; digests and
remaining tool pins land in their owning P01 tasks. No Android version was changed.

## 1. Inherited Android baseline (observed, unchanged)

Source commit `b8a52049e0294cd2d07612cedcc52b3017c5271e`, branch `main`.
`gradle/libs.versions.toml`: Kotlin 2.0.21, AGP 8.7.3, Compose BOM 2024.12.01,
Room 2.6.1, Hilt 2.53.1, WorkManager 2.10.0, OkHttp 4.12.0, Jsoup 1.18.3.
Gradle wrapper 8.10.2; `minSdk` 26, `compileSdk`/`targetSdk` 35; JVM target 17
(`domain`/`application` via `jvmToolchain(17)`); app `versionName` 3.1.0 /
`versionCode` 4 (`app/build.gradle.kts:42-43`).

Execution environment on 2026-09-28:

- Active JVM: Eclipse Temurin 21 (`java -version` → `21+35-LTS`). Gradle
  auto-provisioned Eclipse Temurin JDK 17.0.19+10
  (`/data/dev/caches/gradle-home/jdks/eclipse_adoptium-17-amd64-linux.2`,
  `./gradlew -q javaToolchains` → auto-detection/download enabled), which
  satisfies `jvmToolchain(17)`. No JDK install or Gradle change was needed.
- Android SDK: `ANDROID_HOME`/`ANDROID_SDK_ROOT` =
  `/data/dev/android/sdk/Sdk` holds `platforms/android-34, android-35,
  android-36, android-36.1` and `build-tools/35.0.0` among others.
  The earlier "no platforms directory" observation inspected the parent
  symlink path `/data/dev/android/sdk/platforms` instead of `$ANDROID_HOME`.
- Full baseline `./gradlew :domain:test :application:test
  :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`
  → BUILD SUCCESSFUL in ~1m55s (88 tasks: 81 executed, 2 from cache,
  5 up-to-date). `bash scripts/validate-repo-baseline.sh` → exit 0.

## 2. Backend toolchain pins (D01: decided)

- Go: minimum supported **1.26**, preferred **1.27.x** (checked 2026-09-28:
  latest 1.27.1 per go.dev; Go supports the two most recent majors).
  Installed `go1.22.2` is **out of support** and must not be used beyond this
  planning task. P01-T02 pins the exact version in `backend/go.mod`
  (`GOTOOLCHAIN=auto` may fetch it; a local 1.26+ install is preferred).
- PostgreSQL **18** + PostGIS **3.6.x** (stable 3.6.2, Feb 2026; supports
  PG 12–18; PG 14–18 are supported majors per postgresql.org).
  PostGIS 3.7 is still RC — do not use for the pilot. P01-T07 pins the exact
  image **digest** (no `latest` tags) in `infra/compose.dev.yml`.
- Docker **29.1.3** + Compose **v2.27.0** (verified `docker version` server
  reachable, `docker compose version`). Daemon access confirmed.
- Go stack per ADR-005: `net/http`, chi routing, pgx, sqlc, slog, explicit SQL
  migrations. Exact library versions are pinned in `backend/go.mod` by P01-T02
  with a recorded license/security review; Caddy image digest by P01-T10.
- Static/contract tooling (`staticcheck`, `govulncheck`, `sqlc`, OpenAPI
  linter) is pinned in P01-T11/P01-T12, not here.

## 3. Licenses and support

Go BSD-3-Clause; PostgreSQL under the permissive PostgreSQL Licence;
PostGIS GPL-2.0 (copyleft — server-side use, no client redistribution);
Docker Engine/Compose Apache-2.0; Gradle/AGP Apache-2.0; Room/Compose/WorkManager
Apache-2.0. Support windows rechecked 2026-09-28 via go.dev, postgresql.org
and postgis.net; recheck at image/dependency procurement (P01-T02/T07).

## 4. Inherited documentation corrections (A01/A02/A11)

Baseline truth; `docs/architecture.md` R-ARCH-03 and the README scope/version
lines below are corrected in this task. Remaining historical references
(tech-stack/data-sources/ADR-001 FTS5 prose, old `TABELA-ANP-COMBUSTIVEIS`
URLs, `docs/releases/v1.0.0|v2.0.0.md`) stay as history and are recorded here,
not rewritten.

- **A01 — FTS baseline is FTS4.** `MunicipalityFtsEntity.kt:5,14` uses Room
  `@Fts4`; `AnpFuelDatabaseMigrations.kt:12,102` create
  `municipality_fts USING FTS4`. Prose claiming FTS5
  (`docs/architecture.md:285,331-338`, `docs/tech-stack.md:17,34,105,188,191`,
  `docs/data-sources.md:121`, `docs/adr/001-kotlin-compose-stack.md:27,37`)
  is stale. Do not replace search during backend work.
- **A02 — Schema v4; `:data` also depends on `:application`.**
  `AnpFuelDatabase.kt:30` → `version = 4` with schemas 1–4 under
  `data/schemas/`; `data/build.gradle.kts:57-58` implements both
  `project(":domain")` and `project(":application")` (worker orchestration).
  `docs/architecture.md` R-ARCH-03 and its version-1 schema example are
  corrected/annotated accordingly in this task.
- **A11 — Legacy publishing references.** `README.md` indexed UC-001…UC-014
  while UC-015 exists (`FindNearestBestPriceStationUseCase`, architecture
  mapping table row UC-015); linked git-ignored `.local/PROJECT_PLAN.md`;
  release quick-start commented `(v2.0.0)` while the release is 3.1.0
  (`app/build.gradle.kts:42-43`). Scope/version lines are corrected in this
  task; historical release notes keep their original text. Never run
  release/tag scripts to establish a baseline.

## 5. Validation evidence

```sh
java -version  # Temurin 21+35-LTS (launcher); JDK 17.0.19 auto-provisioned by Gradle
go version     # go1.22.2 linux/amd64 — out of support, plan-only
docker version # Server 29.1.3 reachable
docker compose version  # v2.27.0
bash scripts/validate-repo-baseline.sh  # exit 0
./gradlew :domain:test :application:test :data:testDebugUnitTest \
  :app:testDebugUnitTest :app:assembleDebug --no-daemon  # BUILD SUCCESSFUL ~1m55s
./gradlew -q javaToolchains --no-daemon  # Temurin 17.0.19 provisioned; 21 current
git diff --check  # clean (run after edits)
```

Limitation: instrumentation (`connectedDebugAndroidTest`), coverage/lint gates
and live ANP import were not executed in this task.

## 6. Next

P01-T02 (Go module and process roots) consumes the Go ≥1.26 pin from §2.
P01-T07 pins the Postgres/PostGIS digest. Backend code starts in P01-T02;
no Android source was touched here.
