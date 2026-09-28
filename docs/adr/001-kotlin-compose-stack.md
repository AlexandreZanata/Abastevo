# ADR-001: Kotlin Native + Jetpack Compose Stack

| Field | Value |
|-------|-------|
| **Status** | Accepted |
| **Date** | 2026-06-18 |
| **Deciders** | Project team |

## Context

The ANP Fuel Prices app must:

- Download and parse weekly ANP XLSX files on-device (no backend)
- Store and query up to millions of price records locally
- Support offline-first reading (BR-004)
- Follow Clean Architecture with strict layer boundaries (`.cursor/rules/`)
- Ship as an international open-source Android app with i18n (`en`, `pt-BR`)

We evaluated: Kotlin native, Flutter, React Native, and Kotlin Multiplatform.

## Decision

Adopt **Kotlin native Android** with:

- **UI:** Jetpack Compose + Material Design 3
- **Modules:** `:domain` / `:application` / `:data` / `:app`
- **Database:** Room + SQLite + FTS5
- **Sync:** WorkManager + OkHttp (+ Jsoup for URL discovery)
- **Parser:** Custom streaming XLSX parser (Apache POI for test validation only)
- **DI:** Hilt
- **Tests:** JUnit 5 + MockK + Turbine

Full specification: [tech-stack.md](../tech-stack.md)

## Rationale

1. **Local data at scale** — Room + FTS5 + WAL is the most mature path for indexed SQLite on Android.
2. **Background sync** — WorkManager is the platform-standard scheduler with network/battery constraints (BR-014).
3. **Clean Architecture** — Pure Kotlin `:domain` module with zero framework deps enables 90%+ unit test coverage.
4. **XLSX import** — Streaming parser avoids POI heap pressure (~2 MB files, ~20K rows/week, multi-year retention).
5. **Material 3** — First-class Compose support matches `.cursor/rules/android-system-design.mdc`.
6. **Open source** — No vendor lock-in; entire stack is Apache 2.0 / MIT licensed libraries.

## Consequences

### Positive

- Strong alignment with agent core principles (layer isolation, TDD, domain events)
- Predictable performance for UC-004 FTS search and UC-005 price queries
- Standard hiring/onboarding path for Android developers

### Negative

- No iOS in v1 — separate codebase required if iOS is added later
- Custom XLSX parser requires upfront investment (mitigated by POI cross-validation in tests)
- Multi-module Gradle setup adds initial complexity

### Neutral

- Kotlin 2.0 and Compose BOM versions must be pinned and updated deliberately
- Min SDK 26 excludes ~2% of legacy devices (acceptable)

## Alternatives considered

| Alternative | Rejected because |
|-------------|------------------|
| Flutter | Weaker FTS5/SQLite tooling; XLSX parsing less mature on mobile |
| React Native | Poor fit for millions of local rows and reliable background sync |
| KMP | Premature for Android-only v1; adds build complexity |
| Apache POI (production) | JVM heap usage unsuitable for low-end Android during import |

## Compliance

This decision satisfies:

- Agent core principles §1 (layered architecture)
- Agent core principles §2 (domain as pure core)
- Agent core practices §12 (TDD with JUnit 5)
- Project i18n rules (Android string resources)
- User business logic (offline-first, no server)
