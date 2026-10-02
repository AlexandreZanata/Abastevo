# P22-T04 — Abuse lifecycle and phase acceptance

Status: LOCAL_DONE on `codex/phase-22-social-moderation`. Issue: #89. Entry P22-T03 satisfied (moderation journey LOCAL_DONE). Binds B-BR-F02…F08 and BUC-C03/C05 at station/fuel level. No migration, no new endpoint, no new permission, no iOS work.

## What this task adds

- Astral-plane comment boundary (`TestCommentAstralBoundary`): 280 emoji (1120 bytes, 560 UTF-16 units) pass as 280 scalars, 281 refuse, lone-CR folds like CRLF. Pins scalar counting against byte/UTF-16 length across Go/Kotlin/Swift (Kotlin counts via `codePointCount`, covered in `FeedbackDisplayTest`).
- Hot-path metering (`hotpath_bench_test.go`, application layer, in-memory store, all gates open, synthetic accounts): `SubmitComment ~531 ns/op`, `Vote ~670 ns/op`, `Rate ~533 ns/op` (10k iterations, i7-13620H). Relative application-layer costs only — never capacity or SLO claims; real-PostGIS behavior stays with the integration suites below.

## Abuse/lifecycle matrix — where each case is proven

- Unicode/280/emoji/CRLF/invalid-UTF-8: `feedback/domain` `TestCommentBounds` + new astral test; cross-platform fixtures `TestFeedbackAttackFixtures`; client 280 guard `FeedbackDisplayTest`.
- Valid/invalid voting: `TestVoteHappyAndTallyExact`, HTTP vote flow, `FeedbackHttpClientTest` choice mapping, ViewModel `SELF_VOTE`.
- Vote change / replay converge / absent-removal no-op: `TestVoteSelfChangeRemove` (+PG `TestPGVoteChangeRemoveRevision`).
- Author vote denial: self-vote 403 end to end (service → HTTP `feedback.self-vote` → client kind → ViewModel label).
- Concurrency: `TestPGConcurrentVotesConverge`, `TestPGConcurrentSameParentReplies`, moderation `TestConcurrentOpenConverges`, erase `TestPGConcurrentDoubleEraseConverges` (real PostGIS, `-race`).
- Hide/leak: `TestPGReportFlagsAndHides` (hidden view 404s, lists exclude, audit keeps); reporter-deletion survival `TestPGReporterDeletionKeepsCase`.
- Revision freeze: `TestVoteRevisionMoveOnEdit` (edit opens a new denominator, old votes stay audit).
- Suspension/erasure: `TestPGVoteSuspendedRefuses`, footprint export/erase + `TestPGExportEraseAndG14Exit`, binding matrix `TestBindingBlockedMatrix`.
- Report quota: `TestReportQuotaDenies` (429 + `Retry-After`); ViewModel `QUOTA`.
- Ratings abuse parity: out-of-range 400, unknown-delete 404, idempotent converge, revision bump (HTTP + client + ViewModel tests, P22-T02).

## Phase gate — every owning task outcome demonstrated

- T01 account journey: `docs/mobile/p22-t01-account-journey.md` (`AuthViewModelTest` 13/0-fail).
- T02 ratings/comments/votes: `docs/mobile/p22-t02-ratings-comments-votes.md` (backend routes + OpenAPI + golden vectors + Android client/ViewModel/display).
- T03 moderation/support: `docs/mobile/p22-t03-moderation-support.md` + `docs/product/COMMUNITY_MODERATION.md` (policy + frozen no-mute contract + Comunidade card).
- T04 (this task): matrix above + metering. Full backend unit `go test ./...` 0-fail; `-race -tags=integration` on feedback/moderation/account/identity/`cmd/ops` vs real PostGIS 0-fail; `apicontract` golden PASS; `vacuum lint` PASS (27 pre-existing informs); `check-compat.sh` PASS; `:domain:test` 408 + `:application:test` 246 + `:data:testDebugUnitTest` 203 (1 pre-existing live-network skip) + `:app:testDebugUnitTest` 145, 0-fail; `:app:assembleDebug` PASS; `git diff --check` clean; secret scan PASS.

## Explicitly out of scope (not waived)

- Commercial Android acceptance stays G24-owned; historical multiplatform acceptance stays G18-owned; real-production certification stays P09/G09-owned.
- Low-end encode capacity is frozen only with a real-device plan: P24-owned (per P21-T02). Real provider proof: P24-owned.
- Station-identity mapping for discussion/report screen wiring, reporter case-status/author-history reads (future backend extension + privacy review), in-app help channel (P23-T02).
