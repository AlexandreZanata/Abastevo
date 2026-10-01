# P17-T01 — Shared feedback feature use cases

Status: LOCAL_DONE on `codex/phase-17-social-iphone-parity` (first task
of the phase; no PR yet at task start — draft PR follows this commit).
Issue: #55. No backend change, no migration, no existing screen touched.

## B-BR / BUC (frozen before coding)

- B-BR-F01: feedback targets an existing station + catalog fuel; every
  rating, comment, reply, vote and report requires an active account
  session. Payment never gates participation, vote weight or trust —
  no payment port exists on any path.
- B-BR-F02: rating is an integer 1–5, one current rating per
  account/target; edits converge idempotently; count/sum aggregates
  are rebuildable and travel with every write outcome.
- B-BR-F03: comments/replies are plain text, nonempty, at most 280
  Unicode scalar values after trim + CRLF normalization
  (`PortableText`, shared Go/Kotlin/Swift vectors). Only the author
  edits their text.
- B-BR-F04: replies attach to a live top-level comment of the same
  station/fuel; depth is one level (MVP freeze).
- B-BR-F05: validity votes are VALID/INVALID, one live vote per
  account/comment/revision, author excluded; change/removal is
  idempotent; revision-scoped tallies refresh on every write.
- B-BR-F06: agreement = floor(10000·valid/total) basis points, null
  with zero votes; valid/invalid/total counts travel with tallies.
- B-BR-F07: moderation states visible/flagged/hidden; reports never
  grant moderation power; public reads serve opaque aliases only.
- B-BR-F08: revoked sessions block new writes; feedback never
  overwrites official ANP records.
- BUC-F01…F05: rate/edit/remove; comment/reply/edit/delete;
  vote/change/remove; report; paged source-separated reads with
  projection reconciliation.

## What changed

- `domain/.../repository/FeedbackPorts.kt` — gateway, first-page
  cache and outbox ports; `RatingStatsSnapshot`, `VoteTallySnapshot`,
  `FeedbackCommentView`, `FeedbackPage`, write receipts,
  `PendingFeedbackOp`, `FeedbackRejectKind` (mirrors backend
  `VerdictCode`), `FeedbackException`.
- `application/.../port/FeedbackFlagProvider.kt` — feature flag.
- `application/.../usecase/feedback/SubmitFeedbackUseCase.kt` —
  rate/deleteRating/comment/reply/edit/deleteComment/vote/
  removeVote/report. Disabled → `Disabled`; blank account →
  `LoginRequired` (no IO, anonymous proof alone never authorizes);
  local violations throw `DomainException` fail-fast; server
  refusals → `Rejected(kind)` for optimistic rollback; transport →
  `Queued(op)` bounded outbox, never fake success.
- `application/.../usecase/feedback/GetFeedbackPageUseCase.kt` —
  anonymous reads; fresh pages cached; transport falls back to
  `StaleCache` (explicit) else `Unavailable`; limits clamp 1…100
  (default 20); corrupt cursors fail closed as unavailable.
- Tests: `SubmitFeedbackUseCaseTest` (8) + `GetFeedbackPageUseCaseTest`
  (5) with hand fakes, no mocks.

## RED → GREEN

- RED: new tests failed compilation (`Unresolved reference
  FeedbackGateway/FeedbackFlagProvider/…`) — production code absent.
- GREEN: after ports + use cases, `./gradlew :domain:test
  :application:test` green; result XMLs confirm
  `SubmitFeedbackUseCaseTest` 8/8 and `GetFeedbackPageUseCaseTest`
  5/5, 0 failures, 0 skipped. `assertThrows` on suspend calls uses
  the repo-precedent `runBlocking` wrapper
  (`SubmitCommunityVoteUseCaseTest`).

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test --no-daemon
./gradlew :data:testDebugUnitTest :app:testDebugUnitTest --no-daemon
./gradlew :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: all BUILD SUCCESSFUL; `check-mobile --static-only` ok;
`diff --check`/secrets clean. Reply/delete/removeVote paths reuse
the same guard helpers exercised directly by the rate/comment/edit/
vote/report tests; full device-matrix proof stays with P17-T04/G17,
never claimed here. Rollback: feedback flag OFF preserves existing
free/offline behavior and compatible API.
