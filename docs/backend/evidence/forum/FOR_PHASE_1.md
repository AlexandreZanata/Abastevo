# Forum Phase 1 — Editorial foundation / first useful release

Status: IN_PROGRESS
Validation: PASS (baseline reconciliation & scope freeze)
Delivery: INTEGRATION_PENDING

## Authorized scope and baseline reconciliation

Implementation of Forum Abastevo Phase 1 (FOR-00 through FOR-09) authorized by user directive.
Phase 1 delivers the editorial foundation and MVP read experience:
- Staff publication/editing/hiding via existing admin controls.
- Public read API (categories, editorial feed, post detail) with stable cursors.
- Android domain, Room offline cache (24h/2 MiB bound), and Compose UI (Community card, feed, and post detail screens).
- B-BR-FOR-01–07/12/14/16 and BUC-FOR-01/02.

### Reconciled repository baseline

- Current GitHub `main`: `3fc77acc6d3dc85819cbc9d00cadaec056448d0a` (PR #191 merged).
- Current GitHub `dev`: `6857ec6311d5e58974044ab968fc3da0e0ea3f5c` (PR #192, Quick verification PASSED).
- Isolated delivery worktree: `/tmp/abastevo-forum-plan` on HEAD `6857ec63` (ancestor `3fc77acc`).
- Preserved worktrees: primary checkout (`/home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices`) with detached HEAD `e22824db` and uncommitted edits; other existing worktrees remain untouched per ADR-020.

### Reconciled schema & migration state

- PostgreSQL / PostGIS backend:
  - Latest migration: `backend/db/migrations/000156_registry_station_fact_lookup.sql`.
  - Next available migration allocation: `000157`.
- Android Room database (`AnpFuelDatabase`):
  - Latest schema: `data/schemas/com.anpfuel.data.local.AnpFuelDatabase/36.json` (version 36).
  - Next available Room migration allocation: `36` -> `37`.
- OpenAPI contracts:
  - Canonical specification: `contracts/openapi/v1.yaml`.
  - Validation: vacuum lint clean (zero errors).

### Architectural and economic boundaries

1. **No paid APIs, LLMs, or external search services:** Pure Go stdlib and PostgreSQL FTS for all backend search/ranking.
2. **Text-first editorial MVP:** No user media uploads, private S3 attachment storage, or image processing in Phase 1.
3. **No reactions or relevance algorithms:** Chronological/pinned editorial feed only.
4. **Strict origin and feature gating:** All Forum endpoints default-off behind runtime configuration.
5. **No impact on core fuel price/station flows:** Community feed card is additive; four-tab bottom navigation contract preserved.

## Microtask execution record

| Task | Scope | Status | Tested revision | Evidence / Notes |
|---|---|---|---|---|
| **FOR-00** | Reconcile main baseline and freeze scope | LOCAL_DONE | Current commit | Baseline `3fc77acc` + `6857ec63`; migration 000157 and Room v37 allocated; Phase 1 evidence opened; worktrees isolated. |
| **FOR-01** | Freeze behavior, privacy and contracts | LOCAL_DONE | Current commit | Normative OpenAPI v1 routes (`/v1/forum/categories`, `/v1/forum/posts`, `/v1/forum/posts/{post_id}`), schemas, 9 testdata vectors, structural & boundary tests (title 1–120, excerpt max 400, body max 4000, forbidden leaks). Vacuum lint clean (0 errors). |
| **FOR-02** | Pure editorial domain and ports | LOCAL_DONE | Current commit | Pure Go stdlib domain (`Post`, `Source`, `Category`, `PublicationState`, `Visibility`, state machine transitions, UTF-8/rune bounds validation, public projections `ToSummary`/`ToDetail`) and application layer (`Service`, `CategoryReader`, `PostReader`, `Clock`, default-off gates). Full unit suite + stdlib-only guard + `-race` PASSED. |
| **FOR-03** | Foundation SQL and migration recovery | LOCAL_DONE | Current commit | Migration 000157 (forum_sources, forum_feed_state, forum_posts, forum_changes), sqlc-generated pgx queries, Store adapter (PostReader/CategoryReader), opaque keyset cursor, monotonic feed serialization under lock, optimistic revision checks, 11 constraint negative checks, migration idempotency, and EXPLAIN partial index plan verified under `-race`. |
| **FOR-04** | Existing admin editorial publication | LOCAL_DONE | Current commit | Audited editorial controls in backend API and web panel: AdminHandler routes, AdminService, pgx Store mutations with optimistic concurrency checks and monotonic feed sequence, strict audit privacy guard (body/excerpt never in audit records), web panel editorial interface (list/filter/create/edit/publish/hide with mandatory reason), 53 Node tests (100%), Go unit tests + real Postgres integration tests under `-race`. |
| **FOR-05** | Public read API and composition | LOCAL_DONE | Current commit | Public read endpoints mounted on API router (`GET /v1/forum/categories`, `GET /v1/forum/posts`, `GET /v1/forum/posts/{post_id}`), wired in `cmd/api/main.go` with `SystemClock` and runtime feature gating. Keyset pagination with sealed HMAC cursors (15m TTL, filter hash binding, base64 payload), UUID input validation, 404 privacy enforcement for draft and hidden posts, and `Cache-Control: no-store`. Unit tests with HMAC tamper/expiry/scope vectors + real PostgreSQL integration test passed under `-race`. |
| **FOR-06** | Android domain/application contracts | LOCAL_DONE | Current commit | Pure Kotlin domain models (`ForumCategory`, `ForumPostKind`, `ForumSourceAttribution`, `ForumRegion`, `ForumPostSummary`, `ForumPostDetail`, `ForumFeedPage`, `ForumException`), `ForumSafeLinks` (HTTPS-only, user-info/port/length guards), `ForumGateway` repository port, application use cases (`GetForumCategoriesUseCase`, `GetForumFeedUseCase`, `GetForumPostDetailUseCase`, `ValidateForumSafeLinkUseCase`). Full A1 test suite (19 JVM tests passing: mapping, null provenance, stale scope/bounds, UUID validation, auth-free, HTTPS link refusal). |
| **FOR-07** | HTTP, Room cache and Hilt | NEXT | - | Transport client, Room v37 entities/DAO, 24h/2 MiB offline cache, DI wiring. |
| **FOR-08** | Community card, Forum feed/detail routes | PLANNED | - | CommunityScreen card, ForumScreen, ForumDetailScreen, navigation graph, strings. |
| **FOR-09** | MVP checkpoint and safe degraded operation | PLANNED | - | End-to-end integration proof, offline degradation, default-off verification. |

## Verification and quality gates

- `git diff --check`: PASSED (no trailing whitespace or merge conflict markers).
- Secret surface review: PASSED (`scripts/scan-secrets.sh` clean, no credentials or PII introduced).
- OpenAPI lint (`vacuum lint`): PASSED (0 errors).
- Android JVM test suite (A1): PASSED (19 tests: 12 in `:domain` + 7 in `:application`, 0 failures, 0 skipped).
- Go unit test suite with `-race`: PASSED (`./internal/modules/forum/...` adapters, http, domain & application + `./internal/platform/config/...` + `./cmd/api/...`).
- PostgreSQL integration test suite with `-race`: PASSED (`TestForumMigration_AppliedAndIdempotent`, `TestForumConstraints_NegativeChecks`, `TestForumPublication_MonotonicSequenceAndConcurrency`, `TestForumFeed_KeysetPaginationAndSnapshot`, `TestForumUpdateAndVisibility_RollbackAndFeedInvalidation`, `TestForumExplainPlan_UsesPartialIndex`, `TestStore_AdminEditorialMutationsAndAuditing`, `TestHandler_Integration_PublicReadLifecycle`).
- Frontend test suite: PASSED (`npm test` in `admin-web/`: 53 tests passing, 0 failures, `npm run build && npm run check` valid dist, 0 unsafe DOM violations).
- Static analysis: `go vet` PASSED (0 errors).
- Code formatting: `gofmt` verified.
- Repository worktree isolation: PASSED (persistent worktree on maintained dev; occupied checkouts untouched).
- Next action: FOR-07 (HTTP, Room cache and Hilt).
