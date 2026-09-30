# Kotlin and multiplatform audit

Status: source inspection and target plan, 2026-09-30. No version upgrade or iOS implementation performed by this audit.

## Repository findings

Source: gradle/libs.versions.toml, gradle/wrapper/gradle-wrapper.properties, settings.gradle.kts and the four module build files at baseline c51fa03 plus backend correction 8c5e795 (no mobile source changes).

- Kotlin 2.0.21; AGP 8.7.3; Gradle 8.10.2; KSP 2.0.21-1.0.28; JDK 17.
- Android minimum API 26, compile/target API 35. Compose BOM 2024.12.01, coroutines 1.9.0, Room 2.6.1, Hilt 2.53.1, OkHttp 4.12.0 and WorkManager 2.10.0.
- :domain and :application are Kotlin JVM; :data and :app are Android. No existing iOS app or KMP common source sets are established by this inspection.
- Domain uses java.time, BigDecimal/RoundingMode, UUID and Normalizer. Application uses Java time, IO and SSL exceptions. These need portable contracts or platform adapters, not a blind source-set move.
- Room, WorkManager, Hilt, OkHttp, jsoup and POI are platform/data adapters. Existing parser/calculation/offline behavior must retain fixture parity.

## Official compatibility findings

Kotlin 2.4.20 is the current stable release, dated 2026-09-07. It is a migration candidate, not an approved dependency pin. [Kotlin release history](https://kotlinlang.org/docs/releases.html).

The published KMP matrix for 2.0.21 lists Gradle through 8.8 and AGP through 8.5; the repository's newer Gradle/AGP are outside that documented KMP range. This does not assert the current Android build is broken. KMP 2.4.20 lists Gradle 7.6.3–9.7.0, AGP 8.5.2–9.3.1 and Xcode 26.4. P12-T01 must reconcile KSP, Compose, Android KMP plugin and actual host tooling before selecting pins. [KMP compatibility guide](https://kotlinlang.org/docs/multiplatform/multiplatform-compatibility-guide.html).

Swift export is Alpha; prefer framework interop plus a Swift wrapper for the first supported app. [Swift export status](https://kotlinlang.org/docs/native-swift-export.html), [iOS integration options](https://kotlinlang.org/docs/multiplatform/multiplatform-ios-integration-overview.html).

Target iosArm64 and iosSimulatorArm64. Apple binary/device validation requires Apple hosts; a Linux JVM test or native intermediate artifact is not an iPhone build. [Native target support](https://kotlinlang.org/docs/native-target-support.html).

## Migration acceptance

P12 freezes a tested compatibility matrix, dependency review and baseline device set before changing build plugins. Move pure logic incrementally; exact milli-BRL operations include overflow/rounding vectors, and time/ID/text representations have shared fixtures. Keep Hilt out of common code; inject ports explicitly. Android regression must pass for every migrated slice. Build the shared framework and Swift shell on macOS, then verify lifecycle/error mapping, cancellation, concurrency, Keychain proof parity and device capabilities.

Measure cold start, list reads, photo capture/processing, outbox synchronization, retained heap and frame responsiveness on supported low-resource Android and iPhone devices. Budgets are acceptance hypotheses until the P12 baseline freezes them; no claim of high performance follows compilation. UI refinement and branding remain later work.
