# Forum Phase 4 — User topics and bounded discovery

Status: LOCAL_DONE
Validation: PASS (pure domain, ownership SQL, scoped CAS edit and tombstone remove, rate limiting, live topic budget, author operation uniqueness, B-BR-FOR-05–11 invariants)
Delivery: LOCAL_DONE / INTEGRATION_PENDING

## Authorized scope and baseline reconciliation

Implementation of Forum Abastevo Phase 4 (FOR-20 through FOR-24) authorized by user directive.
Phase 4 enables user-authored topics with review by default, owner CAS controls, and bounded discovery:
- TOPIC post kind with author ownership fields, strict geographic/fuel/station subject-matter validation.
- Idempotent topic creation via unique `(author_account_id, author_operation_id)` constraint and body SHA-256 validation.
- Per-account topic quotas per B-BR-FOR-11: 3/hour, 10/day, and 20 live active topics per account across Forum.
- Premoderation by default: authored topics start as `PENDING_REVIEW` and never appear in public feeds before operator approval.
- Invariants: no external `source_id` on user topics (anti-source spoofing), no paid privilege, author presence mandatory.
- Owner controls: CAS edit returning to `PENDING_REVIEW`, tombstone removal wiping body/alias and setting `REMOVED` + `HIDDEN`.
- Terminal removal: removed topics cannot be edited or removed again.
- Audited change tracking in `forum_changes` for CREATE, EDIT, and REMOVE actions.

### Reconciled repository baseline

- Current GitHub `main`: `99f21830` (PR #196 merged).
- Current GitHub `dev`: `b753a077` (PR #192, Phase 1, Phase 2, Phase 3 completed).
- Isolated delivery worktree: `.worktrees/abastevo-forum-plan` on HEAD `b753a077`.
- Preserved checkouts: primary checkout (`/home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices`) with detached HEAD `e22824db` and uncommitted edits; other existing worktrees remain untouched per ADR-020.

## Microtask execution record

| Task | Scope | Status | Tested revision | Evidence / Notes |
|---|---|---|---|---|
| **FOR-20** | Topic writes and ownership SQL | LOCAL_DONE | f9dd7c46 | Pure domain validation in `domain/topics.go` and `domain/topics_test.go` (1..120 runes title, 1..4000 runes body, plain UTF-8, no NUL bytes, 7 categories, 27 Brazilian UFs, 7-digit IBGE code matching UF prefix, 7 allowed fuel products, station UUID validation); application `TopicService` and `TopicStore` interfaces in `ports.go` and `topic_service.go`; SQL queries in `db/queries/forum/topics.sql` and `topics.sql.go` generated via `sqlc`; adapter store in `topics_store.go` implementing `CreateTopic` (with transactional rate limiting, live count check `<= 20`, idempotency replay and insert race 23505 handling), `EditTopic` (CAS revision check, state return to `PENDING_REVIEW`), `RemoveTopic` (tombstone clearing body/alias, `REMOVED` + `HIDDEN`), and `GetTopicByID`; audit change logging in `forum_changes`; full test suites in `topics_test.go`, `topic_service_test.go`, and real PostgreSQL integration suite in `topics_integration_test.go` (10 tests: `TestTopic_CreateAndIdempotency`, `TestTopic_SimultaneousCreateRace`, `TestTopic_LiveQuotaEnforcement`, `TestTopic_RateQuota`, `TestTopic_CASEditLifecycle`, `TestTopic_TombstoneRemoval`, `TestTopic_ForeignOwnerDefense_IDOR`, `TestTopic_GuardAccountStatus`, `TestTopic_KindSpoofAndConstraintDefenses`, `TestTopic_AuditedForumChanges`) PASSED with `-race` and `-tags=integration`. |
| **FOR-21** | Topic API and OpenAPI fixtures | LOCAL_DONE | e4f4cda7 | Signed topic HTTP commands exposed: `POST /v1/forum/topics`, `POST /v1/forum/topics/{topic_id}/edit`, `POST /v1/forum/topics/{topic_id}/remove`; extended `POST /v1/forum/owner/read` supporting `topic_id`; OpenAPI specification v1.yaml updated with schemas `ForumCreateTopicRequest`, `ForumEditTopicRequest`, `ForumRemoveTopicRequest`, `ForumTopicReceipt`, `ForumOwnerTopicResponse`, and `ForumOwnerReadRequest` topic_id; 7 golden vectors in `contracts/testdata/api/`; wire handler and DTOs in `DiscussionHandler` and `cmd/api/main.go`; premoderation isolation verified (unapproved topic 404 in public feed and detail); IDOR defense verified (foreign owner read/edit/remove returns 404); unit test suite in `for21_topics_test.go` and real PostgreSQL integration suite in `for21_topics_integration_test.go` (6 gates: signed create with premoderation default, replay vs conflict defense, owner read and IDOR defense, CAS edit and foreign defense, tombstone removal terminal state, quota enforcement 429) PASSED with `-race` and `-tags=integration`. |
| **FOR-22** | Topic privacy and report integration | LOCAL_DONE | Current commit | Extended FOR-16 account export (`ExportAccount`/`ExportAccountBytes`), LGPD erasure (`EraseAccountBatch`), and FOR-18 moderation actions to authored topics: `ForumAccountFootprint` extended with deterministic `Topics []ForumTopicExport` and `TotalTopics`; `EraseAccountBatch` scrubs authored topics (`ScrubTopicForErasure` clearing title to '[Tópico removido]', body/excerpt to empty, public alias to NULL, setting `publication_state = 'REMOVED'` and `visibility = 'HIDDEN'`) recording `SYSTEM` `erasure-scrub` in `forum_changes`; child comment visibility suppression verified (`ListCommentsByPost` returns 0 comments for removed/erased topics); no inherited permissions verified (child comments retain their distinct authors, author B's comment is not wiped when topic author A is erased, author B can still export and owner-read their comment, permission boundaries prevent foreign edits/removals); operator restore refusal after removal verified (`SetPostVisibility` with `visibility = 'VISIBLE'` on removed topics updates 0 rows, returning `ErrPostNotFound`); metadata-only audit verified (no post title, excerpt, body, alias, or PII in `forum_changes`); worker restart and batched continuation verified across multiple iterations; idempotent deletion replay verified; unit tests in `for22_export_test.go` and real PostgreSQL integration suite in `for22_privacy_integration_test.go` (6 gates) PASSED with `-race` and `-tags=integration`. |

## Verification and quality gates

- `git diff --check`: PASSED (0 errors).
- Secret surface review: PASSED (`scripts/scan-secrets.sh` clean).
- OpenAPI lint: PASSED (`vacuum lint` 0 errors).
- OpenAPI contract tests: PASSED (`internal/platform/apicontract/...` all green).
- Go unit and integration test suite with `-race` and `-tags=integration`: PASSED (`./internal/modules/forum/...` all tests green).
- SQL schema verification: `sqlc vet && sqlc generate` PASSED (0 errors).
- Code formatting: `gofmt` verified.
- Repository worktree isolation: PASSED (persistent worktree on maintained dev; occupied checkouts untouched).
- Next action: FOR-23 (Android topic forms).
