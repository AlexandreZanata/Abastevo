# Current state audit

Audit date: 2026-09-28, **P00 import snapshot**. This is a historical source audit, not a certification of runtime correctness or current implementation status. Later P01 foundation work and delivery-flow activation status are tracked in [PROGRESS](planning/PROGRESS.md).

## Provenance and scope

- Source: https://github.com/AlexandreZanata/brazil-fuel-prices-app
- Branch: `main`; clean clone at `b8a52049e0294cd2d07612cedcc52b3017c5271e`.
- Latest change: merge #6 (2026-09-21), structured station-address geocoding cascade. Preceded by nearest best-price station/radius work and direct navigation from home cards.
- 670 tracked files were imported using `git archive` from a complete clone. The new repository's existing `.git` and `origin` (`AlexandreZanata/brazil-fuel-prices`) were preserved. Upstream Git history is in the source repository, not grafted into the new repository.
- Destination initially had no commits or project files. Import does not create a commit, push, release or deployment.
- The supplied commercialization text is reference material. Its suggested ordering was changed to honor the user's explicit backend/infrastructure-first request. This delivery imports the base and creates planning documents; it does not implement the backend.
- No existing Android source, Gradle configuration, schema, application ID or license is changed by the planning work.

## Actual stack and boundaries

`settings.gradle.kts` includes `:domain`, `:application`, `:data`, `:app`; package root is `com.anpfuel`, application ID `com.anpfuel.app`. App version is 3.1.0 / versionCode 4.

Versions in `gradle/libs.versions.toml`: Kotlin 2.0.21, AGP 8.7.3, Compose BOM 2024.12.01, Room 2.6.1, Hilt 2.53.1, WorkManager 2.10.0, OkHttp 4.12.0, Jsoup 1.18.3. Gradle wrapper is 8.10.2; Kotlin JVM toolchains request JDK 17; Android minSdk 26, compile/targetSdk 35. These are observed versions, not upgrade recommendations.

- `domain`: 108 production Kotlin files, 57 unit-test files. Entities, value objects, business rules, events and repository ports; no Android dependency. `DomainId.generate()` calls UUID directly: reuse concepts, not every implementation choice, for deterministic Go tests.
- `application`: 39 production Kotlin files, 29 unit-test files. Use cases and coroutine orchestration; depends on domain.
- `data`: 103 production Kotlin files, 30 unit-test files, 15 instrumentation-test files. Room, preferences, import/parser, geocoding and workers. Actual dependencies include BOTH domain and application.
- `app`: 80 production Kotlin files, 18 unit-test files, 9 instrumentation-test files. Compose, ViewModels, permissions, navigation and DI.

Counts are files matching `*Test.kt`, not executed test cases or coverage: 134 unit files and 24 instrumentation files total.

## Implemented capabilities to preserve

UC-001…009 cover ANP discovery/import, onboarding, location selection, municipality search, averages, history, station details, settings and survey-week selection. UC-010…015 add up to three local vehicles, tank-fill estimates, optional device location, external navigation, local price-drop notifications and nearest best-price station selection.

Evidence paths:

- `data/src/main/kotlin/com/anpfuel/data/parser/`: custom streaming XLSX plus summary/station parsers; POI is test-only.
- `data/src/main/kotlin/com/anpfuel/data/worker/SyncWorker.kt`: WorkManager calls application sync use cases.
- `data/src/main/kotlin/com/anpfuel/data/local/AnpFuelDatabase.kt`: schema version **4**, seven entity types; `data/schemas/` has versions 1–4.
- `data/src/main/kotlin/com/anpfuel/data/local/AnpFuelDatabaseMigrations.kt`: 1→2, 2→3, 3→4 migrations.
- `domain/.../PriceAmount.kt`: BigDecimal rounded HALF_UP to two places; Room prices are Double/REAL.
- `domain/.../Cnpj.kt`: numeric-only normalization, exactly 14 digits, no check-digit validation.
- `application/.../FindNearestBestPriceStationUseCase.kt`: at most eight candidates, structured/free-text address cascade; BR-028 applies a 2% price band and configurable 3/5/10/15 km radius.
- `data/samples/`, parser tests, `data/src/androidTest/`: reusable input samples, import/idempotency and migration checks.

The current app is self-contained. It has no community backend, contributor authentication, photo upload, OCR contribution flow, billing verification or cloud synchronization.

## Documentation reconciled with implementation

**A01 — FTS mismatch.** README, architecture and stack say FTS5. `MunicipalityFtsEntity.kt` uses Room `@Fts4`. Record FTS4 as baseline; do not replace search during backend work.

**A02 — Schema/dependency examples are stale.** `docs/architecture.md` contains a version-1 Room example and says data depends only on domain; actual schema is 4 and data also depends on application for worker orchestration. Keep the working dependency graph; reconcile prose in P01-T01.

**A03 — Correction history differs from BR-003.** Glossary promises a new history record for corrected ANP input. ADR-002 and the deterministic IDs plus `OnConflictStrategy.IGNORE` keep the first record. A reimport is idempotent but does not version corrections. Backend must use import revisions; Android must not silently acquire different semantics before P10.

**A04 — Units and precision need an explicit bridge.** `PriceAmount` has two decimals, while backend ingestion must preserve up to three price decimals and raw source precision. Do not migrate local REAL columns now. `LPG_P13` is per 13 kg cylinder and CNG per cubic metre, not price per litre.

**A05 — Product vocabulary carries a legacy misnomer.** `GASOLINE_PREMIUM` currently maps `GASOLINA ADITIVADA`. Backend should use `GASOLINE_ADDITIVED` and a documented compatibility map, not silently claim premium/octane properties.

**A06 — CNPJ cannot remain numeric-only.** Current normalization drops letters. New catalog must support numeric and alphanumeric CNPJ, preserve leading zeroes, and quarantine invalid identifiers. The [Receita Federal CNPJ program](https://www.gov.br/receitafederal/pt-br/acesso-a-informacao/acoes-e-programas/programas-e-atividades/cnpj-alfanumerico) documents the new format. Android compatibility work is deferred to P10; no letter-to-digit coercion is acceptable.

**A07 — Spreadsheet documentation is not a parser contract.** `docs/data-sources.md` describes station header/data rows 7/8; the actual parser constants are 10/11. Fixed columns, sheet name and header assumptions need fixture-based layout detection in Go. Verify samples, not prose offsets.

**A08 — Geocoding scope changed.** ADR-003 describes one-shot reverse geocoding; UC-015 now performs forward-geocoding cascades. Per-device throttling does not establish a safe aggregate production load. Server-side bulk catalog geocoding requires a permitted provider and global quotas; public Nominatim is not the default bulk source.

**A09 — Privacy documents describe the old product.** Current policy says there is no app-owned backend, no photo collection and no account. It is valid context for the imported app, not a policy for the planned network. New backend data flows require a separately reviewed notice before a pilot.

**A10 — Generic agent rules conflict with this product.** Existing rules demand authentication/tenant identification for every API, events-only cross-context communication and tests without databases. The target has public reads, no tenants, explicit application interfaces and real PostgreSQL integration tests. ADR-004 resolves the scope; unit-test isolation remains mandatory.

**A11 — Legacy publishing/docs references.** README indexes only UC-001…014 although UC-015 exists; it links ignored `.local/PROJECT_PLAN.md`. Several documents reference the old `TABELA-ANP-COMBUSTIVEIS` name. Release scripts/descriptions still target v2.0.0 in places despite version 3.1.0. Do not run release/tag scripts to establish baseline.

**A12 — License summary is imprecise.** `docs/license.md` suggests a repository link alone suffices, while the actual MIT file requires retaining its notice. Preserve `LICENSE` verbatim and use it as the authoritative text; no relicensing is part of this task.

**A13 — Secret scanner coverage is limited.** Existing scanner covers tracked files and excludes documentation/test paths. On an unborn destination with untracked imports its success is not a meaningful full scan. It was also executed against the tracked upstream clone; broader changed-file scanning belongs in the new CI.

## Validation evidence

- PASS: source clone clean; import file provenance checked against the source commit.
- PASS: `bash scripts/validate-repo-baseline.sh` in destination validates ignore patterns and required rule/docs files.
- PASS: `bash scripts/scan-secrets.sh` in tracked upstream clone, with its documented limited pattern/path coverage.
- Android unit/build baseline: attempted `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon` using an isolated Gradle cache. Initial attempt failed downloading Gradle due to sandbox networking. Final retry outcome is recorded in [baseline evidence](planning/BASELINE_VALIDATION.md).
- Environment observed: active Java 21, requested toolchain 17 not installed in the inspected Java locations; SDK symlink resolves to `/data/dev/android/sdk`, but platform directory was absent. Do not claim APK/tests/coverage passed.
- Instrumentation, actual UI behavior, live ANP import, Play publishing and production security were not executed.
- Existing CI `.github/workflows/ci.yml` runs `./gradlew test --no-daemon` with JDK 17; it does not explicitly run instrumentation, APK, lint, vulnerability scanning or a Go pipeline. Remote CI status was not used as a substitute for local results.

## Reuse and migration risks

Preserve the four modules, 15 use cases, offline Room cache, navigation, local alerts, domain tests and sample spreadsheets. Port domain meaning through shared fixtures; Kotlin classes are not directly shared with Go. The highest-risk interfaces are fuel names/units, CNPJ identity, ANP revisions, local precision, and source/freshness presentation.

Start implementation with [P01-T01](../ROADMAP.md#p01-t01) to close prerequisite and baseline gaps. Existing functionality is the regression reference. New Android functionality is blocked until the backend release gate G09.
