# iOS workstream — archived, explicit resumption only

State: **DEFERRED_EXPLICIT_RESUME_ONLY**, 2026-10-01. Authority: maintainer explicitly requested archival because no Mac is available. [ADR-016](../../adr/016-android-commercial-community-ios-deferred.md) owns platform/release scope.

This is a parked workstream, not a completed or cancelled product. No automatic resumption, native implementation, iOS release or acceptance claim is permitted. Preserve `iosApp/`, shared Kotlin ports, platform fixtures and `docs/assets/brand/`; Android/shared work may continue while maintaining portability. Do not delete iOS code or weaken its tests. Existing optional macOS CI is evidence only; it does not authorize resumed work.

## Preserved backlog

- P12 native framework/Swift host/toolchain rows: the current host is SPM-only; create a real Xcode application target and integrate assets/frameworks on resumption.
- P13/P17 native email/Google/Apple login, callback/Keychain, revocation/recovery/erasure and free account parity.
- P15/P17 camera encoding, KiB/memory limits, secure temporary storage, lifecycle cleanup and 24-hour expiry across every app-owned copy.
- P16/P17 Core Location simulation/integrity, denied/unknown/proximity and foreground/background behavior; no universal fake-GPS guarantee.
- P18 native functional/accessibility/performance/device rows. PR #67's Linux/JVM/Android evidence does not fill them; issues #60–#62 remain open for real acceptance evidence.
- P00-T04 icon asset catalog: exact-size opaque RGB exports prepared; no Xcode app target/build/device proof. Apply `ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon` only in the future app target.
- Future parity with P19–P24 commercial Android flows, after their contracts are stable. Do not clone an obsolete UX merely to mark parity complete.

## Resume checklist, only after an explicit request

1. Record the new request/date/scope in PROGRESS and open one bounded owning phase branch/issue/PR; preserve this archive as history.
2. Audit supported macOS/Xcode/Swift/Kotlin/Gradle combination and distribution/signing requirements using current official documentation. Obtain a Mac and supported iPhone test devices; record access without secrets.
3. Reconcile current Android/backend contracts with native adapters and define exact build/device/security/accessibility/performance commands and support matrix before implementation.
4. Run native negative/lifecycle/offline/provider/media/location tests, and platform acceptance on the pinned candidate. Recover missing or failed proofs rather than relying on mocks.
5. Certify iOS separately with the applicable full production and store/provider/privacy requirements. Android/backend G09 evidence may be reused only where its inputs and scope actually match; no blanket acceptance inheritance.
