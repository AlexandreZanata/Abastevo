# Agent guide

Abastevo is an open-source fuel-price community for Brazil: an Android app,
a Go backend, a static public landing/portal and two internal web apps. This
file is the entry point for coding agents. Read it fully, then only the docs
the task needs ([docs/README.md](docs/README.md) is the index).

## Repository map

| Path | What lives there |
|------|------------------|
| `app/` | Android UI: Jetpack Compose screens, ViewModels, navigation, resources (`com.anpfuel.app`). |
| `application/` | Android use cases; depends on `domain` only. |
| `domain/` | Pure Kotlin entities, value objects, rules and ports. No Android/framework imports. |
| `data/` | Android infrastructure: Room, OkHttp, workers, OCR, repositories implementing domain ports. |
| `maps/engine/` | Native map engine wrapper (CoMaps-derived SDK, pinned by SHA-256). |
| `backend/` | Go modular monolith: `cmd/` binaries, `internal/modules/<module>/{domain,application,adapters}`, `internal/platform/`, `db/migrations`, `db/queries` (sqlc). |
| `contracts/` | OpenAPI and shared cross-language vectors (Go/Kotlin) under `testdata/`. |
| `landing/` | Static public site and SEO portal (TypeScript build, pt-BR content in `content/`). |
| `admin-web/`, `business-web/` | Internal admin console and company web app (TypeScript). |
| `tools/station-prep/` | Rust preparers for official station/quality/EV source files. |
| `infra/` | Compose for local dev/validation, k8s staging manifests, Caddy, deploy agent. |
| `scripts/` | CI gates, release helpers, asset generators and their tests. |
| `release/` | Android release channel, signing certificate and bundled third-party notices. |
| `iosApp/` | Archived iOS app. Do not work on it unless explicitly asked. |
| `docs/` | Architecture, product rules, operations runbooks, decisions. |

## Architecture rules

- Backend modules own their domain, application services and adapters. The
  domain package is pure; cross-module access goes through explicit ports.
- SQL lives in `backend/db/queries` and is generated with sqlc. Always
  parametrized. Run `sqlc generate` after editing queries.
- Migrations in `backend/db/migrations` are append-only: never edit an applied
  migration; add a new numbered one with a recovery test when it is risky.
- Public reads may be anonymous. Signed writes and private owner reads require
  proof of possession. The organization boundary
  ([decision 025](docs/decisions/025-private-abastevo-empresas-organization-boundary.md))
  applies only to private Abastevo Empresas data; public, community and personal
  products stay tenant-free.
- Official data (ANP, registry, quality) and community data stay distinct.
  Payment never buys trust and never gates free registration or participation.
- Android follows `domain` ← `application` ← `data`/`app`. ViewModels only
  orchestrate UI state; business logic stays in `domain`/`application`.
  The package root is `com.anpfuel`. Content cards and surfaces use
  `AnpContainerDefaults` ([UI containers](docs/android/ui-containers.md)); do
  not duplicate outlines or outline structural layouts. Actions are never
  bare text: a filled `Button` plus same-size `OutlinedButton`s
  ([design system: buttons](docs/android/design-system.md#buttons-never-text-only-actions-owner-rule-2026-11-28)).
- Stack: Go (net/http, chi, pgx, sqlc, slog), PostgreSQL + PostGIS, private S3
  media, Caddy, k3s; Kotlin, Compose, Material 3, Room, Hilt, WorkManager.
  Adding a dependency needs a stated need, license check and security review.

## Commands

| Area | Command |
|------|---------|
| Android unit tests | `./gradlew test` |
| Android debug build | `./gradlew :app:assembleDebug` (install with `:app:installDebug`) |
| Backend fast gate (no DB) | `bash scripts/check-backend-fast.sh` |
| Backend unit tests | `cd backend && go test ./...` |
| Backend integration | `cd backend && ANPFUEL_TEST_DATABASE_URL=postgres://…@127.0.0.1:…/db?sslmode=disable go test -race -tags=integration ./...` (disposable PostGIS only) |
| Go formatting | the go.mod toolchain gofmt: `"$(go env GOROOT)/bin/gofmt" -l .` |
| Web apps | `cd landing` (or `admin-web`, `business-web`) `&& npm ci && npm run typecheck && npm test && npm run build` |
| Rust preparers | `make check-rust` |
| Change-aware gate | `make quick-verify` (what the required `Quick verification` check runs) |
| Secrets | `bash scripts/scan-secrets.sh` |

`make help` lists the remaining infra, deploy, backup and portal targets.

## How to work

1. Start from `dev`, kept equal to `main` plus the change in flight. One
   bounded change at a time; use a separate git worktree if another effort
   occupies the checkout, and never overwrite or stash someone else's work.
2. Read the code and tests you will touch before editing. No unrelated
   refactors, invented rules or speculative services.
3. Behavior changes: write the rule down first (product doc or test name),
   then RED → GREEN → REFACTOR. Domain logic is test-first.
4. Auth, money, privacy, SQL, migrations and jobs need negative, failure and
   concurrency tests against real PostGIS. Never skip, weaken or defer a known
   failing test.
5. Before committing run the targeted tests, the affected compile/format/static
   checks, `git diff --check` and the secret scan.
6. Deliver through a pull request `dev → main`. Merge only when every required
   check is green (`Quick verification`, `backend`, `CI`) and the head has not
   changed. No force push, `--no-verify`, admin bypass or destructive reset;
   fix published work with new commits. `scripts/git-flow.sh finish` and
   `scripts/validate-merge-checks.py` implement the guarded merge. A PR head
   costs about 15 minutes of CI, so batch related commits into one PR instead
   of stacking tiny ones.
7. Keep docs true in the same change: update the doc that owns the rule or
   runbook. Future work goes to [ROADMAP.md](ROADMAP.md); never add progress
   logs, task ledgers or evidence dumps to the repository.
8. Android screens captured for the public tutorials are listed in
   [docs/assets/app-tutorial/manifest.json](docs/assets/app-tutorial/manifest.json).
   When you change one of their sources or strings, run
   `python3 scripts/app_tutorial/tutorial_contract.py stale` and follow the
   [capture contract](docs/assets/app-tutorial/CAPTURE_CONTRACT.md): recapture
   on the emulator under the frozen conditions and update the site page or
   label that shows it.

## Deployment

A merge to `main` runs CI; when every main workflow passes, `release-staging`
publishes a signed release manifest and the deploy agent on the VPS applies it
(schema gate, backup, migrations, rollout, smoke). Android releases are built
and published by `android-release`. Details: [docs/operations/](docs/operations/).

## Conventions

- English for code, comments, docs, commits and PRs. User-facing strings go
  through resources/i18n (`en` default, `pt-BR` and other locales). The public
  site is pt-BR first. Other locales ship only as reviewed translations under
  [decision 026](docs/decisions/026-multilingual-public-site.md) and its
  [multilingual contract](docs/product/landing/organic-growth-i18n-plan.md);
  [decision 027](docs/decisions/027-ai-reviewed-landing-translations.md) lets
  an AI review, disclosed to readers, sign the `en` and `es` landing.
- Conventional Commits: `type(scope): imperative subject` (≤ 72 chars), one
  logical change per commit, body explains why.
- Never commit secrets, keystores, `local.properties`, personal data, precise
  contributor GPS, production photos or OCR training data.
- Preserve module names, `com.anpfuel` and the original MIT notices in
  `LICENSES/`.
