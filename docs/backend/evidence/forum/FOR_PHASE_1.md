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
| **FOR-01** | Freeze behavior, privacy and contracts | NEXT | - | OpenAPI v1 schemas, negative vectors, apicontract tests for editorial foundation. |
| **FOR-02** | Pure editorial domain and ports | PLANNED | - | Post, category, rights state machine, clock/ID injection, zero DB dependencies. |
| **FOR-03** | Foundation SQL and migration recovery | PLANNED | - | Migration 000157, sqlc queries, pgx adapter, concurrency/rollback tests. |
| **FOR-04** | Existing admin editorial publication | PLANNED | - | Draft, edit, publish, hide actions, audit trail, CSRF/RBAC integration. |
| **FOR-05** | Public read API and composition | PLANNED | - | GET /v1/forum/categories, /v1/forum/posts, /v1/forum/posts/{id}, HMAC cursor. |
| **FOR-06** | Android domain/application contracts | PLANNED | - | Kotlin models, use cases, error contracts, pure JVM unit tests. |
| **FOR-07** | HTTP, Room cache and Hilt | PLANNED | - | Transport client, Room v37 entities/DAO, 24h/2 MiB offline cache, DI wiring. |
| **FOR-08** | Community card, Forum feed/detail routes | PLANNED | - | CommunityScreen card, ForumScreen, ForumDetailScreen, navigation graph, strings. |
| **FOR-09** | MVP checkpoint and safe degraded operation | PLANNED | - | End-to-end integration proof, offline degradation, default-off verification. |

## Verification and quality gates

- `git diff --check`: PASSED (no trailing whitespace or merge conflict markers).
- Secret surface review: PASSED (no credentials, tokens, or PII introduced).
- Repository worktree isolation: PASSED (occupied dev and detached checkouts untouched).
- Next action: FOR-01 (freeze v1 behavior, editorial wire contract and test vectors).
