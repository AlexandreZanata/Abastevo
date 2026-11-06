# Forum Phase 2 — Authorized news candidates

Status: LOCAL_DONE
Validation: PASS (full test suite, rights enforcement, news review, deduplication, reject workflow, rollback)
Delivery: INTEGRATION_PENDING

## Authorized scope and baseline reconciliation

Implementation of Forum Abastevo Phase 2 (FOR-10 through FOR-13) authorized by user directive.
Phase 2 addresses candidate news ingestion, parser safety, and rights governance:
- Revalidation of candidate sources, terms of service, and copyright compliance.
- Formal rights decisions per Brazilian Copyright Law (Lei 9.610/98).
- Freezing representative permitted fixtures for the stdlib RSS/Atom parse contract.
- Hardening against XML entity expansion (XXE) and SSRF.
- Operational disposition: `NEWS_SOURCE_BLOCKED` fallback until formal bilateral commercial licensing.

### Reconciled repository baseline

- Current GitHub `main`: `3fc77acc6d3dc85819cbc9d00cadaec056448d0a` (PR #191 merged).
- Current GitHub `dev`: `cc394a82` (PR #192, Phase 1 + Phase 2 in progress).
- Isolated delivery worktree: `.worktrees/abastevo-forum-plan` on HEAD `cc394a82`.
- Preserved worktrees: primary checkout (`/home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices`) with detached HEAD `e22824db` and uncommitted edits; other existing worktrees remain untouched per ADR-020.

## Source register and rights determinations (FOR-10)

Under Brazilian Copyright Law (Lei 9.610/98), automated redistributive publication in a commercial application requires an explicit license. Government portals citing CC BY-ND 3.0 permit redistribution only with attribution and prohibit derivative works (e.g., machine summaries/paraphrasing); critically, they provide no machine RSS/Atom feeds. External media portals restrict commercial syndication.

| Candidate | URL / Format | Legal & Commercial Analysis | Rights Status | Operational Disposition |
|---|---|---|---|---|
| **Abastevo Editorial** | `https://abastevo.com.br` / First-party | Owned by Alexandre Zanata / Abastevo; original editorial content. | `APPROVED` | **ACTIVE_EDITORIAL** (Published via admin) |
| **Agência Brasil / EBC** | `https://agenciabrasil.ebc.com.br` / RSS 2.0 | Reproduction policy requires explicit commercial licensing. Representative XML fixture frozen for parse contract. | `DENIED` | **NEWS_SOURCE_BLOCKED** (No automated ingest) |
| **ANP Notícias** | `https://www.gov.br/anp/pt-br` / HTML | CC BY-ND 3.0; HTML only, no machine RSS feed provided. Web scraper prohibited. | `DENIED` | **EDITORIAL_MANUAL_ONLY** (Links permitted) |
| **Senatran** | `https://www.gov.br/transportes/pt-br/assuntos/transito` / HTML | CC BY-ND 3.0; HTML only, no machine RSS feed provided. | `DENIED` | **EDITORIAL_MANUAL_ONLY** |
| **Ministério dos Transportes** | `https://www.gov.br/transportes/pt-br` / HTML | CC BY-ND 3.0; HTML only, no machine RSS feed provided. | `DENIED` | **EDITORIAL_MANUAL_ONLY** |
| **DNIT** | `https://www.gov.br/dnit/pt-br` / HTML | CC BY-ND 3.0; HTML only, no machine RSS feed provided. | `DENIED` | **EDITORIAL_MANUAL_ONLY** |
| **Motor1 Brasil** | `https://motor1.uol.com.br` / RSS 2.0 | Commercial media portal; terms of service prohibit app redistribution without paid contract. | `DENIED` | **BLOCKED** |
| **Detran-MT** | `https://www.detran.mt.gov.br` / Unverified | Official site timed out during research; unverified availability. | `DENIED` | **BLOCKED** |

### Parse contract vectors frozen

Representative synthetic test fixtures created in `backend/testdata/forum-sources/`:
1. `source_register.json`: Machine-readable register defining candidate metadata, URLs, and rights dispositions.
2. `rss2_sample_valid.xml`: Valid RSS 2.0 fixture conforming to RFC-822 timestamps, `<guid>`, `<link>`, `<description>`.
3. `atom_sample_valid.xml`: Valid Atom 1.0 fixture conforming to RFC-3339 timestamps, `<id>`, `<summary>`, `<link>`.
4. `malformed_entity_expansion.xml`: Negative security vector verifying `<!DOCTYPE` / XXE refusal.
5. `rss2_missing_dates.xml`: Negative vector verifying handling of missing timestamps.

## Microtask execution record

| Task | Scope | Status | Tested revision | Evidence / Notes |
|---|---|---|---|---|
| **FOR-10** | Approve one real source or record block | LOCAL_DONE | `80b0fd71` | Source register frozen in `source_register.json`; rights evaluated for 8 candidates; operational status `NEWS_SOURCE_BLOCKED` recorded with editorial fallback; parse contract vectors frozen for RSS/Atom; validation suite in `source_register_test.go` PASSED. |
| **FOR-11** | Safe feed parser and fetch adapter | LOCAL_DONE | `253b15ec` | Bounded RSS 2.0 and Atom 1.0 parser with strict XXE refusal, max depth 32, max body 2 MiB, max 100 items, title/excerpt HTML sanitization; safe fetcher with origin allowlist, HTTPS-only, IPPolicy refusing private/loopback/cloud-metadata addresses, DNS rebinding TOCTOU prevention (pinned IP dialer), bounded body reader, redirect limiter, conditional GET (304). Complete unit and security suite PASSED. |
| **FOR-12** | Scheduled collection and deduplication | LOCAL_DONE | `cc394a82` | Append-only migration 000158 (forum_source_runs, forum_source_checkpoints); ForumSourceFetchHandler and SourceFetchRunner; deterministic category classifier; conditional GET (304); transactional deduplication by GUID and canonical URL hash; blocked/revoked rights refusal without egress; coexistence test with privacy safety job. Full unit and PostgreSQL integration test suite PASSED. |
| **FOR-13** | Editorial review and news checkpoint | LOCAL_DONE | Current commit | Admin candidate review queue, rights view with checkpoint metrics, and candidate rejection workflow; `AdminService.ListSources` and `AdminService.RejectPost` with audit event `admin.forum.post.reject` and change tracking; HTTP endpoints `GET /v1/admin/forum/sources` and `POST /v1/admin/forum/posts/{id}/reject`; strict rights validation in `PublishPost` (checks `RightsApproved`, `PublicationEnabled`, and `ReviewExpiresAt`); deduplication confirmed so rejected candidates never reappear on subsequent ingestion runs; `admin-web` extended with rejection action, rejected status filter, and source metrics list; acceptance suite `TestFOR13_NewsCheckpoint_FullIntegration` PASSED against real PostgreSQL. Phase 2 completed. |

## Verification and quality gates

- `git diff --check`: PASSED.
- Secret surface review: PASSED (`scripts/scan-secrets.sh` clean).
- OpenAPI lint (`vacuum lint`): PASSED (0 errors).
- Go unit and integration test suite with `-race` and `-tags=integration`: PASSED (`./internal/modules/forum/...` including `TestFOR13_NewsCheckpoint_FullIntegration`, `TestForumAdmin_*`, `TestForumJobs_*`, `TestForumSourceFetch_*`, and `TestClassifyCategory_*`).
- Android JVM test suite: PASSED (`:domain:test`, `:application:test`, `:data:testDebugUnitTest`, `:app:testDebugUnitTest`).
- Frontend test suite: PASSED (`npm test` in `admin-web/`: 53 tests passing; `npm run build && npm run check` valid dist).
- Static analysis: `go vet` and `sqlc vet` PASSED (0 errors).
- Code formatting: `gofmt` verified.
- Repository worktree isolation: PASSED (persistent worktree on maintained dev; occupied checkouts untouched).
- Next action: Phase 3 — Safe discussion on published posts (FOR-14 Comment/reply domain and persistence).
