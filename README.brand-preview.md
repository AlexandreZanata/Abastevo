<!-- Approved composition preview; canonical README uses the same SVG. -->

![abastevo — original blue and green A logo and reference ABASTEVO lettering](docs/assets/brand/abastevo-readme-banner-v2.svg)

# abastevo

This repository starts from [brazil-fuel-prices-app](https://github.com/AlexandreZanata/brazil-fuel-prices-app), commit `b8a52049e0294cd2d07612cedcc52b3017c5271e`. The existing Android app is preserved. The project name is **abastevo**, selected by the maintainer on 2026-10-01. See the [brand identity and README artwork proposal](docs/brand/IDENTITY.md).

**Current status:** Backend P01–P09 phase history and local correction evidence are recorded; real production is not release-certified. G01-FLOW helpers and required Quick verification are active. New accounts, feedback, 24-hour all-copy media and Kotlin/Swift functionality remain planned; see [current state](docs/planning/PROGRESS.md).

**Execution order:** integrated local backend (G09-LOCAL) → modular Kotlin Multiplatform and native Swift app (P12–P18/P10) → real-production release G09 → public pilot and optional paid benefits. Next implementation task after correction integration: [P12-T01](ROADMAP.md#p12-t01). [Functional delivery plan](docs/planning/MOBILE_DELIVERY_PLAN.md) includes free email-code/Google/Apple accounts, 280-character station/fuel feedback, 24-hour lightweight photos and location-integrity controls. These new features are planned, not implemented. Focused TDD/risk checks run per task; quick CI gates each phase; actual G09 production certification waits until functional G18.

- [Roadmap with executable microtasks](ROADMAP.md)
- [Fast phase delivery: branches, PRs, issues and wiki](docs/planning/DELIVERY_WORKFLOW.md), [execution card](docs/planning/FAST_EXECUTION.md) and [CI cadence](docs/planning/CI_PLAN.md)
- [Documentation index](docs/README.md) and [current audit](docs/CURRENT_STATE_AUDIT.md)
- [Target architecture](docs/backend/TARGET_ARCHITECTURE.md), [API plan](docs/backend/API_PLAN.md) and [infrastructure plan](docs/backend/INFRASTRUCTURE_PLAN.md)
- [Product scope](docs/product/PRODUCT_CONTRACT.md), [privacy/security](docs/security/SECURITY_PRIVACY.md) and [decisions](docs/planning/DECISIONS.md)
- [Agent entry point](AGENTS.md) and [execution progress](docs/planning/PROGRESS.md)

Official ANP prices and community observations remain distinct. Offline Android functionality is preserved; paid convenience never affects contribution trust. Existing MIT notices remain unchanged. Code may be public; production data, photos, credentials and contributor location stay private.

## Imported Android documentation

The following original README describes the upstream Android application and its release links. Its “no backend” description applies to that imported release; target platform plans are above. Known stale details (FTS version and UC index) are documented in the audit, not silently treated as implementation truth.

---

# ANP Fuel Prices

[![CI](https://github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS/actions/workflows/ci.yml/badge.svg)](https://github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/badge/release-v3.1.0-blue.svg)](https://github.com/AlexandreZanata/brazil-fuel-prices-app/releases/tag/v3.1.0)

Open-source Android app to browse Brazilian ANP (National Petroleum Agency) weekly fuel price surveys — fully offline-capable, no backend required.

**100% open source and free to use** under the [MIT License](LICENSE). The only requirement when reusing or redistributing this project is to **reference the original source** — see [docs/license.md](docs/license.md).

## Download

Install the signed release APK directly from GitHub — lightweight sideload, no Play Store required:

- **[Download v3.1.0 APK](https://github.com/AlexandreZanata/brazil-fuel-prices-app/releases/download/v3.1.0/app-release.apk)**
- **[GitHub Releases](https://github.com/AlexandreZanata/brazil-fuel-prices-app/releases)** — release notes and previous versions

See [docs/releases/v3.1.0.md](docs/releases/v3.1.0.md) for the full v3.1.0 changelog.

## Features

- **Vehicle profiles & tank fill cost** — register up to 3 vehicles; Home shows estimated full-tank price (UC-010, UC-011)
- **Optional device location** — one-shot GPS + Nominatim reverse geocode after onboarding (UC-012)
- **Station navigation** — open Maps/Waze from station list (UC-013)
- **Local price drop alerts** — on-device notifications after weekly sync (UC-014)
- **Survey week picker** — choose latest or any historical ANP week (gov.br catalog parity)
- **National city search** — ~5 570 IBGE municipalities with FTS ranking and typo tolerance
- Automatic download of ANP price tables from [gov.br/anp](https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/levantamento-de-precos-de-combustiveis-ultimas-semanas-pesquisadas)
- Browse average prices by fuel type, state, and city — **vector icon per fuel**
- Optional per-station detail (CNPJ, address, brand, price)
- Historical price trends (local storage)
- Edge-to-edge UI with system bar safe areas
- 100% on-device processing — no server

## Stack (definitive)

| Concern | Technology |
|---------|------------|
| Language | Kotlin 2.0+ |
| UI | Jetpack Compose + Material Design 3 |
| Architecture | Clean Architecture — `:domain` / `:application` / `:data` / `:app` |
| Database | Room + SQLite + FTS5 |
| Background sync | WorkManager + OkHttp + Jsoup |
| XLSX parsing | Streaming XLSX parser (custom; Apache POI test-only) |
| DI | Hilt |
| i18n | Android string resources (`en` default, `pt-BR`) |
| Tests | JUnit 5 + MockK + Turbine |

Full specification: **[docs/tech-stack.md](docs/tech-stack.md)** · ADR: **[docs/adr/001-kotlin-compose-stack.md](docs/adr/001-kotlin-compose-stack.md)**

## Project structure

```
:app/               # Compose UI, ViewModels, theme, navigation
:application/       # Use cases (UC-001…UC-015)
:domain/            # Entities, Value Objects, rules, events, ports
:data/              # Room, OkHttp, parser, WorkManager, repositories
docs/               # Architecture, use cases, glossary
data/samples/       # Reference ANP spreadsheets for parser tests
.cursor/rules/      # Agent engineering contract
```

Package root: `com.anpfuel`

See **[docs/architecture.md](docs/architecture.md)** for package layout, data flow, and database schema.

## Sample data

Reference files for week **2026-06-07 → 2026-06-13** are in `data/samples/`:

| File | Size | Rows | Purpose |
|------|------|------|---------|
| `resumo_semanal_lpc_*.xlsx` | ~287 KB | ~3.9K (municipalities) | Averages by region/state/city |
| `revendas_lpc_*.xlsx` | ~1.8 MB | ~19.7K | Per-station prices |

See [docs/data-sources.md](docs/data-sources.md) for column schemas and download URL patterns.

## Documentation

| Document | Purpose |
|----------|---------|
| [docs/user-business-logic.md](docs/user-business-logic.md) | **Product contract** — user journeys, states, rules |
| [docs/use-cases/](docs/use-cases/) | Detailed use cases (UC-001 … UC-015) |
| [docs/glossary.md](docs/glossary.md) | Domain language + business rules (BR-001 … BR-027) |
| [docs/attribution.md](docs/attribution.md) | Third-party assets (fuel icons) |
| [CHANGELOG.md](CHANGELOG.md) | Release history |
| [docs/architecture.md](docs/architecture.md) | Layers, packages, data flow, database |
| [docs/tech-stack.md](docs/tech-stack.md) | **Definitive stack** — libraries and module deps |
| [docs/adr/001-kotlin-compose-stack.md](docs/adr/001-kotlin-compose-stack.md) | Architecture decision record |
| [docs/data-sources.md](docs/data-sources.md) | ANP file formats and URL patterns |
| [docs/commit-conventions.md](docs/commit-conventions.md) | Git commit and PR standards |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute — TDD, layers, i18n, PRs |
| [docs/license.md](docs/license.md) | MIT license — free use, attribution required |
| [docs/play-store-listing.md](docs/play-store-listing.md) | Play Store listing draft (en + pt-BR) |
| [docs/privacy-policy.md](docs/privacy-policy.md) | Privacy policy — no personal data collected |
| [docs/local-build.md](docs/local-build.md) | **Local compile** — debug APK, install, prerequisites |
| [docs/release-build.md](docs/release-build.md) | Signed release APK/AAB build instructions |
| [docs/releases/v3.1.0.md](docs/releases/v3.1.0.md) | GitHub Release v3.1.0 draft notes |
| [docs/releases/v3.0.0.md](docs/releases/v3.0.0.md) | GitHub Release v3.0.0 draft notes |
| [docs/releases/v2.0.0.md](docs/releases/v2.0.0.md) | GitHub Release v2.0.0 draft notes |
| [docs/releases/v1.0.0.md](docs/releases/v1.0.0.md) | GitHub Release v1.0.0 draft notes |
| [ROADMAP.md](ROADMAP.md) | Current phased execution plan; replaces the legacy git-ignored local plan |

## Development

Before implementing a feature:

1. Read the use case in `docs/use-cases/` (do not implement undocumented flows)
2. Check business rules in `docs/glossary.md`
3. Verify stack and layer placement in `docs/architecture.md`
4. Write domain tests first (TDD, GIVEN/WHEN/THEN)
5. Add i18n keys for all user-visible strings
6. Follow [commit conventions](docs/commit-conventions.md) (English, Conventional Commits)

See **[CONTRIBUTING.md](CONTRIBUTING.md)** for the full contributor guide.

### Quick start — compile locally

```bash
./gradlew validateRepoBaseline   # Gate 0.1 — repo + .cursor rules
./gradlew test                     # unit tests (same as CI)
./gradlew :app:assembleDebug       # debug APK
./gradlew :app:installDebug        # install on device/emulator
```

Full details: **[docs/local-build.md](docs/local-build.md)**.

## Releases

**Latest:** [v3.1.0](https://github.com/AlexandreZanata/brazil-fuel-prices-app/releases/tag/v3.1.0) — [APK download](https://github.com/AlexandreZanata/brazil-fuel-prices-app/releases/download/v3.1.0/app-release.apk) · [release notes](docs/releases/v3.1.0.md). Previous: [v3.0.0](docs/releases/v3.0.0.md), [v2.0.0](docs/releases/v2.0.0.md), [v1.0.0](docs/releases/v1.0.0.md).

Validate a release locally:

```bash
./gradlew validateReleaseBuild     # signed APK/AAB (v3.1.0)
./gradlew validateReleaseTag       # version, license, disclaimers (Gate 15)
./gradlew validateGate15Release    # tag, artifacts, release notes (Gate 15)
./gradlew publishGithubRelease     # dry-run GitHub Release publish (R2.3)
./gradlew test                     # full unit test suite (all modules)
```

See [CHANGELOG.md](CHANGELOG.md) for version history.

## License

This project is released under the **[MIT License](LICENSE)** — free for any use (personal, commercial, modification, distribution). The only obligation is **attribution**: keep the copyright notice and reference the [project origin](https://github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS).

Full details, examples, and FAQ: **[docs/license.md](docs/license.md)**.

## Data attribution

Fuel price data is published by [ANP](https://www.gov.br/anp) under open government data policies.
