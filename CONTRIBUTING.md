# Contributing to ANP Fuel Prices

Thank you for contributing. This project is an international open-source Android app; all **source code, commits, and documentation** are in **English**. User-visible strings are localized via Android string resources (`en` default, `pt-BR`).

## Before you start

Read these documents in order:

| Document | Why |
|----------|-----|
| [docs/user-business-logic.md](docs/user-business-logic.md) | Product contract — user journeys and business rules |
| [docs/architecture.md](docs/architecture.md) | Layer boundaries and module dependencies |
| [docs/tech-stack.md](docs/tech-stack.md) | Accepted libraries and versions (do not substitute) |
| [docs/glossary.md](docs/glossary.md) | Domain language and business rules (BR-001 … BR-015) |
| [docs/use-cases/](docs/use-cases/) | UC-001 … UC-014 — **undocumented flows must not be implemented** |
| [docs/commit-conventions.md](docs/commit-conventions.md) | Commit and PR format |

Agent engineering rules live in [`.cursor/rules/`](.cursor/rules/) and mirror the same constraints.

## Development setup

**Requirements:** JDK 17, Android SDK (see `libs.versions.toml` for `compileSdk` / `minSdk`).

```bash
git clone https://github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS.git
cd TABELA-ANP-COMBUSTIVEIS
./gradlew validateRepoBaseline   # Gate 0.1
./gradlew test
./gradlew :app:assembleDebug     # compile debug APK
```

See **[docs/local-build.md](docs/local-build.md)** for prerequisites (`local.properties`, JDK 17, SDK 35), install on device, and troubleshooting.

Do not commit `local.properties`, keystores, `.env`, or downloaded ANP files under `data/downloads/`.

Validate repository baseline (Gate 0.1):

```bash
./gradlew validateRepoBaseline
```

## Architecture rules

The project uses Clean Architecture across four Gradle modules:

| Module | Responsibility |
|--------|----------------|
| `:domain` | Entities, value objects, business rules, ports — **pure Kotlin, no Android** |
| `:application` | Use cases only — depends on `:domain` |
| `:data` | Room, OkHttp, parser, WorkManager — implements domain ports |
| `:app` | Compose UI and ViewModels — calls use cases, never DAOs directly |

Violations (e.g. UI importing repository implementations, domain importing Room) will be rejected. See [docs/architecture.md](docs/architecture.md) for package layout and data flow.

## Test-driven development (TDD)

Domain changes follow **TDD**:

1. Write a failing unit test in `:domain` (GIVEN / WHEN / THEN).
2. Implement the minimum code to pass.
3. Target **90%+ instruction coverage** on `:domain` (enforced by JaCoCo).

Test pyramid:

| Layer | Share | Tooling |
|-------|-------|---------|
| Unit | ~75% | JUnit 5, MockK — especially `:domain` |
| Integration | ~20% | Use cases + Room/network with test doubles |
| E2E | ~5% | Compose UI tests, instrumented tests |

Run tests locally:

```bash
./gradlew test                    # all JVM unit tests
./gradlew :domain:test            # domain only
./gradlew :domain:jacocoTestReport # coverage report
./gradlew securityCheck           # secret scan + security unit tests
```

CI runs `./gradlew test` on every push and pull request to `main` (see [.github/workflows/ci.yml](.github/workflows/ci.yml)).

## Implementing a feature

Checklist for every change:

1. **Use case exists** in `docs/use-cases/` before implementing a user flow.
2. **Domain terms** match [docs/glossary.md](docs/glossary.md) exactly.
3. **Business rules** are tested in `:domain` before outer layers.
4. **i18n keys** added for every user-visible string (`values/strings.xml`, `values-pt-rBR/strings.xml`).
5. **Layer placement** verified — no framework imports in `:domain` or `:application`.
6. **One logical change per commit** with Conventional Commits (see below).

Parser and sync work should use fixtures in `data/samples/` and follow [docs/data-sources.md](docs/data-sources.md).

## Internationalization

- Identifiers and comments: **English**.
- UI text: **i18n keys only** — never hardcode strings in Compose.
- Key naming: `feature_component_description` (snake_case).
- Format dates, numbers, and currency with locale-aware APIs.

## Commits and pull requests

Follow [docs/commit-conventions.md](docs/commit-conventions.md):

```
<type>(<scope>): <subject>

[optional body with Refs: UC-00X / BR-00X]
```

**Types:** `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `chore`, `style`, `build`, `ci`  
**Scopes:** `domain`, `application`, `data`, `ui`, `i18n`, `docs`, `sync`, `parser`, …

### Pull request template

```markdown
## Summary
- Bullet points of changes

## Use cases
- UC-00X (if applicable)

## Test plan
- [ ] Domain unit tests pass
- [ ] Relevant integration / UI tests pass
- [ ] Manual verification steps (if UI)
```

Branch naming: `<type>/<short-description>` (e.g. `feat/municipality-search`).

## Reporting issues

When filing a bug or feature request, include:

- Android version and device (if UI/runtime)
- Steps to reproduce
- Expected vs actual behavior
- Relevant use case (UC-00X) or business rule (BR-00X) if known

## License and attribution

By contributing, you agree that your contributions are licensed under the same [MIT License](LICENSE) as the project. When reusing or redistributing, **attribute the original source** — see [docs/license.md](docs/license.md).

Fuel price data is published by [ANP](https://www.gov.br/anp); the app is not affiliated with ANP.
