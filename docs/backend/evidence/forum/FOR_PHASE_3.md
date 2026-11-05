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
| **FOR-15** | Signed owner and command admission | OPEN | — | Wire session/proof/binding and transaction runner; stable logical intent hash excluding refresh credentials; private owner projection. |
| **FOR-16** | Privacy, notices and moderation readiness | OPEN | — | Forum export and batched erasure integration; legal terms and operator responsibility. |
| **FOR-17** | Discussion HTTP endpoints | OPEN | — | Public comment/reply pages and signed owner endpoints; safe receipts and rate error handling. |
| **FOR-18** | Reports, review queue and action integration | OPEN | — | Report receipts, typed moderation targets, and admin queue/action integration. |
| **FOR-19** | Android discussion and social checkpoint | OPEN | — | Android discussion UI, owner controls, report dialog, and social switch readiness. |

## Verification and quality gates

- `git diff --check`: PASSED.
- Secret surface review: PASSED (`scripts/scan-secrets.sh` clean).
- OpenAPI lint (`vacuum lint`): PASSED (0 errors).
- Go unit and integration test suite with `-race` and `-tags=integration`: PASSED (`./internal/modules/forum/...` including `TestComments_*`, `TestFOR13_*`, `TestForumAdmin_*`, `TestForumJobs_*`, `TestForumSourceFetch_*`, and `TestClassifyCategory_*`).
- Migration runner tests: PASSED (`./internal/platform/migrate/...`).
- SQL schema verification: `sqlc generate && sqlc vet` PASSED (0 errors).
- Code formatting: `gofmt` verified.
- Repository worktree isolation: PASSED (persistent worktree on maintained dev; occupied checkouts untouched).
- Next action: FOR-15 (Signed owner and command admission).
