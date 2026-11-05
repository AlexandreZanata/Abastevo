# Forum Phase 3 — Safe discussion on published posts

Status: IN_PROGRESS
Validation: PASS (pure domain, append-only migration 000159, scoped sequences, CAS/tombstone, quota check, idempotency replay, parent suppression, privacy invariants)
Delivery: INTEGRATION_PENDING

## Authorized scope and baseline reconciliation

Implementation of Forum Abastevo Phase 3 (FOR-14 through FOR-19) authorized by user directive.
Phase 3 addresses community discussion on published posts with strict safety and privacy controls:
- Scoped monotonic sequence per post with CAS edit and tombstone removal.
- Account-operation create uniqueness and body hash idempotency.
- Depth enforcement: flat top-level comments plus one reply level only (depth 0 or 1); no reply to replies.
- Sub-tree suppression: hidden or removed parent comments suppress their reply sub-tree from public queries.
- Premoderation by default: comments start as `PENDING_REVIEW` and never leak to public endpoints before operator approval.
- Rate quotas (20/hour, 100/day) and durable live comment budgets (100 live comments per account across Forum).
- Audited change tracking in `forum_changes` for CREATE, EDIT, and REMOVE actions.

### Reconciled repository baseline

- Current GitHub `main`: `3fc77acc6d3dc85819cbc9d00cadaec056448d0a` (PR #191 merged).
- Current GitHub `dev`: `2d469779` (PR #192, Phase 1 + Phase 2 completed).
- Isolated delivery worktree: `.worktrees/abastevo-forum-plan` on HEAD `2d469779`.
- Preserved worktrees: primary checkout (`/home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices`) with detached HEAD `e22824db` and uncommitted edits; other existing worktrees remain untouched per ADR-020.

## Microtask execution record

| Task | Scope | Status | Tested revision | Evidence / Notes |
|---|---|---|---|---|
| **FOR-14** | Comment/reply domain and persistence | LOCAL_DONE | Current commit | Append-only migration 000159 (`forum_comments`, scoped `next_comment_sequence` on `forum_posts`, expanded `forum_changes` target_type); pure domain models and validation in `models.go` (1..280 Unicode scalar values, trimmed, UTF-8, no NUL); application `CommentService` and `CommentStore` interfaces; transactional store in `comments_store.go` with strict lock ordering (quota check -> post lock -> parent lock -> live budget -> insert -> change audit); CAS edit with revision validation and state return to `PENDING_REVIEW`; tombstone removal wiping body text and setting `REMOVED` + `HIDDEN`; idempotency replay via `(author_account_id, create_operation_id)` with body hash comparison; parent suppression in replies; unit and real PostgreSQL integration test suites (`comments_test.go`, `comment_service_test.go`, `comments_integration_test.go`) PASSED with `-race` and `-tags=integration`. |
| **FOR-15** | Signed owner and command admission | LOCAL_DONE | Current commit | Wired session family credentials, signed owner contributor/key binding, and transaction-time guard (`Guard`) to comment create, edit, remove, and private owner read operations; stable logical intent hashing (`CanonicalCreateIntent`, `CanonicalEditIntent`, `CanonicalRemoveIntent`, `CanonicalOwnerReadIntent`, `HashLogicalIntent`) strictly excluding rotating session credentials (`family_id`, `access_token`) and request nonces; private owner projection (`CommentReceiptDTO`, `OwnerCommentResponseDTO`) returned only to authentic authors; foreign-owner attempts return indistinguishable 404 (IDOR prevention per B-BR-FOR-04); mandatory `Cache-Control: no-store` on all command responses and owner reads; OpenAPI specification extended with 5 discussion paths and 9 schemas; 9 golden contract vectors added in `contracts/testdata/api/`; contract tests in `forum_discussion_test.go`; admission unit test suite in `for15_admission_test.go`; real PostgreSQL integration test suite in `for15_admission_integration_test.go` verifying all 11 gates (create success, session refresh replay, changed body 409 conflict, cross-key replay, commit-time suspension fail-closed, commit-time revoked session fail-closed, edit/remove/read IDOR 404, private receipt vs public feed isolation, 7-day attempt expiry) with `-race` and `-tags=integration`. |
| **FOR-16** | Privacy, notices and moderation readiness | LOCAL_DONE | Current commit | Forum export and batched account erasure integrated with existing recovery; `application.ExportAccount` and `ExportAccountBytes` in `export.go` providing owner data portability (`forum-export-v1`) with deterministic sorting and defense-in-depth isolation; `ListCommentsByAccount` and `EraseAccountBatch` in `comments_store.go` scrubbing body text, resetting SHA-256, tombstoning to `REMOVED` + `HIDDEN`, and logging `forum_changes`; `cmd/worker/main.go` wired to `accountapp.ErasureRecovery` enforcing that erasure never completes until all modules succeed (`ErrErasurePending` on remaining rows); `docs/backend/FORUM_POLICIES.md` defines reviewed terms, legal-notice contact (`juridico@abastevo.com.br`, `privacidade@abastevo.com.br`), premoderation disclosure, author/reporter appeal workflow, and Marco Civil (art. 19) / LGPD operator coverage; unit test suite in `for16_export_test.go` and PostgreSQL integration test suite in `for16_erasure_integration_test.go` verifying all 6 gates with `-race` and `-tags=integration`. |
| **FOR-17** | Discussion HTTP endpoints | LOCAL_DONE | Current commit | Public comment/reply pages (`GET /v1/forum/posts/{post_id}/comments`, `GET /v1/forum/comments/{comment_id}/replies`) and signed owner endpoints (`POST /v1/forum/posts/{post_id}/comments`, `POST /v1/forum/comments/{comment_id}/edit`, `POST /v1/forum/comments/{comment_id}/remove`, `POST /v1/forum/owner/read`) wired in `DiscussionHandler` and mounted in `cmd/api/main.go`; keyset pagination with `limit + 1` and `CountPublishedRepliesForParents` index query; runtime feature gates supporting rollback mode (`ReadsEnabled: true, WritesEnabled: false`); mandatory `Cache-Control: no-store` on all command, public read, and error responses; 16 KiB request payload size ceiling; 429 rate/budget limits with `Retry-After`; unit test suite in `for17_endpoints_test.go` and PostgreSQL integration test suite in `for17_discussion_integration_test.go` verifying all 8 gates with `-race` and `-tags=integration`. |
| **FOR-18** | Reports, review queue and action integration | LOCAL_DONE | Current commit | Append-only migration 000160 adding `FORUM_POST` and `FORUM_COMMENT` to moderation target checks and creating `forum_reports` table with `UNIQUE(account_id, target_type, target_id)`; domain integration adding report reasons (`SPAM`, `OFFENSIVE`, `MISINFORMATION`, `COMMERCIAL`, `LEGAL_NOTICE`, `OTHER`), `ReportReceipt`, and verdict codes in `forum/domain`; moderation integration updating `TargetForumPost` / `TargetForumComment` in `moderation/domain` and dispatching `InvalidateForumTarget` to `ActPorts` for operator `INVALIDATE` actions; transactional reports store in `reports_store.go` enforcing 10 reports/day quota per account (`ErrReportQuotaExceeded` -> 429 with `Retry-After: 86400`), self-report prevention (`ErrSelfReportForbidden` -> 400), target existence/publication guard (`ErrReportTargetMissing` -> 404), and convergent case association with `moderation_cases`; operator invalidation setting visibility to `HIDDEN` and appending an audited `forum_changes` record; author removal precedence invariant (removed comments remain `REMOVED` + `HIDDEN` and cannot be resurrected to `VISIBLE`); parent suppression in replies query (hidden parent returns 0 replies and rejects new child comments with 404); endpoints `POST /v1/forum/posts/{post_id}/report` and `POST /v1/forum/comments/{comment_id}/report` in `DiscussionHandler` and OpenAPI spec; golden vectors in `contracts/testdata/api/`; unit tests in `for18_reports_test.go` and real PostgreSQL integration suite in `for18_reports_integration_test.go` verifying all 10 gates with `-race` and `-tags=integration`. |
| **FOR-19** | Android discussion and social checkpoint | OPEN | — | Android discussion UI, owner controls, report dialog, and social switch readiness. |

## Verification and quality gates

- `git diff --check`: PASSED.
- Secret surface review: PASSED (`scripts/scan-secrets.sh` clean).
- OpenAPI lint (`vacuum lint`): PASSED (0 errors).
- Go unit and integration test suite with `-race` and `-tags=integration`: PASSED (`./internal/modules/forum/...` including `TestFOR18_*`, `TestDiscussionEndpoints_*`, `TestFOR17_*`, `TestForumErasure_*`, `TestExportAccount_*`, `TestDiscussionAdmission_*`, `TestComments_*`, `TestFOR13_*`, `TestForumAdmin_*`, `TestForumJobs_*`, `TestForumSourceFetch_*`, and `TestClassifyCategory_*`).
- Moderation module tests: PASSED (`./internal/modules/moderation/...`).
- API production integration tests: PASSED (`./cmd/api/...`).
- Migration runner tests: PASSED (`./internal/platform/migrate/...`).
- SQL schema verification: `sqlc generate && sqlc vet` PASSED (0 errors).
- Code formatting: `gofmt` verified.
- Repository worktree isolation: PASSED (persistent worktree on maintained dev; occupied checkouts untouched).
- Next action: FOR-19 (Android discussion and social checkpoint).
