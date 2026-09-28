# Technology Stack

> **Status:** Accepted (see [ADR-001](adr/001-kotlin-compose-stack.md))  
> **Last updated:** 2026-06-18

This document is the **single source of truth** for all technology choices in the ANP Fuel Prices project. Implementation must follow this stack and `.cursor/rules/`.

---

## Stack summary

| Concern | Technology |
|---------|------------|
| **Language** | Kotlin 2.0+ |
| **UI** | Jetpack Compose + Material Design 3 |
| **Architecture** | Clean Architecture — `:domain` / `:application` / `:data` / `:app` |
| **Database** | Room + SQLite + FTS5 |
| **Background sync** | WorkManager + OkHttp |
| **HTML discovery** | OkHttp + Jsoup |
| **XLSX parsing** | Streaming XLSX parser (custom primary; Apache POI for validation/tests only) |
| **DI** | Hilt |
| **Async** | Kotlin Coroutines + Flow |
| **Navigation** | Navigation Compose |
| **i18n** | Android string resources (`values/` = `en`, `values-pt-rBR/`) |
| **Unit tests** | JUnit 5 + MockK + Turbine |
| **Instrumented tests** | JUnit 4 + Compose UI Test (AndroidX) |

---

## Platform targets

| Setting | Value | Rationale |
|---------|-------|-----------|
| `minSdk` | 26 (Android 8.0) | Room FTS5, WorkManager, modern TLS |
| `targetSdk` | 35 | Current Google Play requirement |
| `compileSdk` | 35 | Latest stable APIs |
| JVM target | 17 | Kotlin 2.x default |

---

## Gradle modules

```
anp-fuel-prices/          (root)
├── app/                  → :app        (Interfaces / UI)
├── domain/               → :domain     (Domain)
├── application/          → :application (Application)
├── data/                 → :data       (Infrastructure)
├── docs/
└── data/samples/         (local ANP reference files)
```

### Dependency graph (strict)

```
:app  ──────────►  :application
  │                      │
  └──────────►  :data     │
       (DI only)          ▼
                    :domain  ◄── :data
```

| Module | May depend on | Must NOT depend on |
|--------|---------------|-------------------|
| `:domain` | Kotlin stdlib only | Android SDK, Room, OkHttp, Hilt, Compose |
| `:application` | `:domain` | `:data`, `:app`, Android framework |
| `:data` | `:domain`, Android SDK | `:app`, Compose |
| `:app` | `:application`, `:data`, Compose, Hilt | Direct `:domain` imports in UI (use use cases) |

---

## Libraries by module

### `:domain` (pure Kotlin JVM)

```kotlin
// build.gradle.kts — no Android plugin
dependencies {
    // No external runtime dependencies in v1
    testImplementation("org.junit.jupiter:junit-jupiter:5.11.4")
    testImplementation("io.mockk:mockk:1.13.13")
}
```

### `:application` (pure Kotlin JVM)

```kotlin
dependencies {
    implementation(project(":domain"))
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.9.0")

    testImplementation("org.junit.jupiter:junit-jupiter:5.11.4")
    testImplementation("io.mockk:mockk:1.13.13")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")
    testImplementation("app.cash.turbine:turbine:1.2.0")
}
```

### `:data` (Android library)

```kotlin
dependencies {
    implementation(project(":domain"))

    // Room + SQLite FTS5
    implementation("androidx.room:room-runtime:2.6.1")
    implementation("androidx.room:room-ktx:2.6.1")
    ksp("androidx.room:room-compiler:2.6.1")

    // Network
    implementation("com.squareup.okhttp3:okhttp:4.12.0")

    // HTML scraping (ANP page discovery)
    implementation("org.jsoup:jsoup:1.18.3")

    // Background sync
    implementation("androidx.work:work-runtime-ktx:2.10.0")

    // Coroutines
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")

    // DI (Hilt worker support)
    implementation("com.google.dagger:hilt-android:2.53.1")
    ksp("com.google.dagger:hilt-compiler:2.53.1")
    implementation("androidx.hilt:hilt-work:1.2.0")
    ksp("androidx.hilt:hilt-compiler:1.2.0")

    // XLSX — test/validation only (POI is too heavy for production import)
    testImplementation("org.apache.poi:poi-ooxml:5.3.0")

    testImplementation("org.junit.jupiter:junit-jupiter:5.11.4")
    testImplementation("io.mockk:mockk:1.13.13")
    androidTestImplementation("androidx.room:room-testing:2.6.1")
}
```

### `:app` (Android application)

```kotlin
dependencies {
    implementation(project(":application"))
    implementation(project(":data"))

    // Compose BOM (pin versions via BOM)
    val composeBom = platform("androidx.compose:compose-bom:2024.12.01")
    implementation(composeBom)
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
    implementation("androidx.navigation:navigation-compose:2.8.5")

    // Hilt
    implementation("com.google.dagger:hilt-android:2.53.1")
    ksp("com.google.dagger:hilt-compiler:2.53.1")
    implementation("androidx.hilt:hilt-navigation-compose:1.2.0")

    // Coroutines
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")

    debugImplementation("androidx.compose.ui:ui-tooling")
    debugImplementation("androidx.compose.ui:ui-test-manifest")

    testImplementation("org.junit.jupiter:junit-jupiter:5.11.4")
    testImplementation("io.mockk:mockk:1.13.13")
    testImplementation("app.cash.turbine:turbine:1.2.0")
    androidTestImplementation(composeBom)
    androidTestImplementation("androidx.compose.ui:ui-test-junit4")
}
```

> **Note:** Version numbers are baseline targets at project initialization. Update via Dependabot or explicit ADR when bumping.

---

## Technology decisions

### UI — Jetpack Compose + Material 3

- Declarative UI aligned with `.cursor/rules/android-system-design.mdc`
- `MaterialTheme` with dynamic color (Material You)
- Light + dark theme from day one
- UiState / UiEvent pattern in ViewModels — no business logic in Composables

### Database — Room + SQLite + FTS5

- **Room** for type-safe DAOs and migrations
- **FTS5** virtual table for UC-004 municipality search (`municipality_fts`)
- **WAL journal mode** enabled for concurrent read during import
- Batch inserts: 1 000 rows per transaction
- Indexes defined in [architecture.md](architecture.md#database-schema)

### Sync — WorkManager + OkHttp

- `SyncWorker` (Hilt-injected) implements UC-001
- Constraints: `NetworkType.UNMETERED` when `autoSyncOnWifi` (BR-014)
- Periodic sync: weekly (ANP publishes weekly)
- OkHttp: download XLSX with progress, connection timeouts, retry interceptor
- Jsoup: parse ANP listing page for `arquivos-lpc/` URLs

### Parser — Streaming XLSX (custom)

| Approach | Use |
|----------|-----|
| **Custom streaming parser** | Production import in `:data:parser` — reads XLSX as ZIP + SAX-style XML, row-by-row, low memory |
| **Apache POI** | Test fixture validation only — compare custom parser output against POI on `data/samples/` |

Rationale: weekly station file ~2 MB / ~20K rows; POI loads entire workbook into heap — risky on low-end devices.

Parser location: `data/src/main/kotlin/.../parser/StreamingXlsxPriceTableParser.kt`

### DI — Hilt

| Module | Provides |
|--------|----------|
| `DatabaseModule` | Room database, DAOs |
| `NetworkModule` | OkHttp client |
| `RepositoryModule` | Binds repository interfaces → implementations |
| `UseCaseModule` | Use case bindings (if interfaces used) |
| `WorkerModule` | WorkManager worker factory |

- ViewModels: `@HiltViewModel`
- Workers: `@HiltWorker`
- Domain and Application layers have **zero** Hilt annotations

### Tests — JUnit 5 + MockK + Turbine

| Layer | Test type | Tools |
|-------|-----------|-------|
| `:domain` | Unit | JUnit 5, GIVEN/WHEN/THEN, no mocks needed for VOs |
| `:application` | Unit | JUnit 5, MockK (repository ports), Turbine (Flow use cases) |
| `:data` | Unit + integration | MockK, in-memory Room (`room-testing`) |
| `:app` | UI | Compose UI Test, Hilt test rules |

**Coverage target:** Domain Layer ≥ 90% (per `.cursor/rules/agent-core-practices.mdc`).

---

## Cross-cutting concerns

### Coroutines & Flow

- Use cases expose `Flow<Result<T>>` or suspend functions
- ViewModels collect flows with `stateIn` / `collectAsStateWithLifecycle`
- Import pipeline runs on `Dispatchers.IO`
- Single-threaded DB writes during batch import

### Error model

Structured errors in Application layer — never raw exceptions in UI:

```kotlin
sealed class AppError {
    data object SyncNetworkError : AppError()
    data object SyncParseError : AppError()
    data object SearchNoResults : AppError()
    // maps to i18n keys in :app
}
```

### Logging

- `timber` in `:data` and `:app` only (not in `:domain`)
- No PII in logs (per agent core principles)
- Sensitive fields (CNPJ) logged only at debug, truncated

### i18n

- All user strings in `app/src/main/res/values/strings.xml` (English)
- Brazilian Portuguese in `values-pt-rBR/strings.xml`
- ANP Portuguese labels normalized to English `FuelProduct` enum at import boundary

---

## What is explicitly excluded

| Technology | Reason |
|------------|--------|
| Backend / API server | 100% on-device (product requirement) |
| Firebase / analytics SDK | No PII collection in v1; add via ADR if needed |
| Retrofit | No REST API — OkHttp sufficient for file download + HTML |
| KMP / Flutter | Android-only v1 (ADR-001) |
| Apache POI in production | Memory footprint on Android |
| RxJava | Coroutines + Flow are standard |
| Dagger (without Hilt) | Hilt reduces boilerplate for Android |

---

## Related documents

- [architecture.md](architecture.md) — layers, packages, data flow
- [adr/001-kotlin-compose-stack.md](adr/001-kotlin-compose-stack.md) — decision record
- [user-business-logic.md](user-business-logic.md) — product contract
- [data-sources.md](data-sources.md) — ANP file formats
