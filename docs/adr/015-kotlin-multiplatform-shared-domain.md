# ADR-015: Kotlin Multiplatform logic and native device adapters

Date: 2026-09-30. Status: ACCEPTED target; migration NOT IMPLEMENTED.

Share pure domain, application use cases, protocol fixtures, exact money and offline state rules in Kotlin common source sets. Preserve existing modules, com.anpfuel packages, MIT and Android behavior. Platform dependencies stay behind explicit ports: Android Keystore/camera/location/Room/WorkManager/Hilt and iOS Keychain/camera/Core Location/persistence/background execution.

Keep the existing Android presentation patterns; add a thin Swift/SwiftUI iOS shell using a supported Kotlin/Native framework with Objective-C-compatible interop and Swift wrappers. Do not base production on Alpha Swift export. One umbrella framework exposes narrow DTO/use-case APIs; avoid exporting all transitive modules. DDD boundaries do not require one Gradle module for every class.

Audit Kotlin/AGP/Gradle/KSP/Compose and native toolchains together before upgrades. Java time, BigDecimal, UUID, Normalizer and Java exception APIs currently prevent moving files unchanged into commonMain. Define portable representations/ports and golden tests first; preserve precise money, timestamps and Unicode semantics. Review each new dependency's need, license, security and platform support.

ADR-001 remains the historical Android stack decision; its Android-only target is superseded here. The [audit](../mobile/KOTLIN_MULTIPLATFORM_AUDIT.md) records current versions and the migration candidate. Actual iOS build/device evidence requires macOS/Xcode.
