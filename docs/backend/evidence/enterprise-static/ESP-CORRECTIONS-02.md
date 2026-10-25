# ESP second-review corrections — publication, freeze, pause and assets

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Date: 2026-10-25
Branch: `codex/enterprise-static-portal`
Validated code head: `7fd5c414` (documentation commit follows)
Base reviewed: `cc0b1c42` after `0d981cf2`, `a8f27a54`, `37bac14b`,
`5ff9d87c`, `58dbfe81`, `b96b0b1c`.

Rules: B-BR-ESP-09/10/13/14/15; BUC-ESP-02/03/06/07.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md).

## Corrected diagnosis

The first follow-up batch fixed real defects, but its report was too broad.
The second review found four additional real failures that the earlier
tests did not cover:

1. two distinct revisions could read the same predecessor and both publish;
2. a published revision accepted a late `StageBatch`, mutating its rows;
3. an operational pause could not recover the same published source bytes;
4. a rollback could discard the current release's assets with `keep=1` or a
   promotion-time budget, and the retained store had a rename window without
   a serving root.

All four are fixed below. This record does not repeat the IEEE-754 census,
the S-ESP-03 rights decision or S-ESP-04, and does not claim a new manual
staging execution.

## 1. Publication is serialized per survey window

B-BR-ESP-10/13; BUC-ESP-02/06.

- `BeginRun`, `StageBatch` and `FinishRun` now take a transaction-scoped
  advisory lock on the survey window before the revision row, including the
  first publication where no pointer row exists. `LockRevision ... FOR UPDATE`
  fences the staged set; the lock order is identity → window → revision with
  no cycle.
- `FinishRun` checks the run/revision link, persisted status, guarded update
  row counts and the pointer in one transaction. A first publication needs an
  empty pointer; a correction needs the pointer to still name the revision in
  `supersedes_revision_id`. A stale competitor retires as `superseded`.
- Duplicate finish of the same revision is idempotent only while it remains
  current. A cancelled/failed transaction leaves run `running`, revision
  `staging`, rows and pointer unchanged.
- A resumed `BeginRun` validates and returns the persisted survey window;
  a different caller window is refused. Orphan adoption stays safe.
- The unique-identity race uses `ON CONFLICT DO NOTHING`; the adapter never
  re-reads inside an aborted PostgreSQL transaction.
- Append-only migration `000131_official_stale_superseded.sql` is retained.
  Its status constraint is asserted; readers continue to select only
  `published` and never export `superseded`.

Maintained tests: `TestConcurrentFinishesFirstPublicationPublishOnce`,
`TestConcurrentCorrectionsFromSamePredecessorPublishOnce`,
`TestConcurrentSameRevisionFinishIsIdempotent`,
`TestPublishedRevisionRejectsLateStage`, `TestTerminalRevisionsRejectStage`,
`TestStageConcurrentWithFinishRespectsTerminalState`,
`TestIdenticalRestageIsNoOpAndDivergentBatchRejected`,
`TestFinishTransactionRollbackLeavesStaging`,
`TestCancelledFinishPersistsNothing`,
`TestBeginResumeUsesPersistedSurveyWindow`,
`TestFinishRejectsRevisionRunMismatch`,
`TestOfficialSupersededStatusMigrationApplied`.
The concurrency proofs use real disposable PostGIS and a database trigger
that widens the publication window; they do not require two finishes to reach
a barrier that a serialized implementation cannot reach.

## 2. Terminal revisions are frozen

B-BR-ESP-10; BUC-ESP-02.

`StageBatch` now runs in one transaction holding the window and revision
locks, checks `status=staging`, and compares an existing `source_row` before
inserting. Identical retries are no-ops; a divergent row returns
`ErrDivergentRestage` and rolls the whole batch back. Published, failed,
needs-review and superseded revisions return `ErrRevisionFrozen` and keep
their rows, raw text, counts, verdict and pointer unchanged. Tallies are
merged into the caller only after commit.

## 3. Operational pause is recoverable; revocation is not

B-BR-ESP-09/14; BUC-ESP-07.

- `Withdraw` persists `withdrawal_reason=operator-pause` for an operational
  pause and `revoked-source` for invalid content. A pause records no official
  revocation. Legacy withdrawals without a reason fail closed.
- `Promote` may clear a pause for the same verified content (the caller has
  established that the authoritative source is published again). Rollback and
  fallback alone never resurrect paused content.
- Revoked official revision/lease identities remain in `state.json` and are
  refused by any hash. A later source "published" status cannot clear them.
- Recovery is proven through `Refresh` in full and official-only modes,
  including a fresh handler (restart), a failed recovery that preserves the
  pause, a subsequent valid revision, and revoked counterproofs.

Maintained tests: `TestESPFollowupPauseRecoversSameSource` (full and
official-only), `TestESPFollowupRevokedSourceStaysRefusedAfterRepublish`,
`TestESPFollowupAutomaticWithdrawalRevokesSourceIdentity`,
`TestESPReviewPauseRecoversOnlyThroughPromote`,
`TestESPReviewRevokedSourceCannotReturnByAnotherHash`.

## 4. Retained assets follow the served release

B-BR-ESP-13/14; BUC-ESP-06/07.

- Every release that can be served as current is mandatory in the retained
  store, even when it predates `keep` or is an old rollback target. Promotion
  and rollback include both the old and new current during the pointer move;
  the budget must hold the mandatory set before the pointer changes.
- Unique asset paths are hardlinked once and counted once; conflicting bytes
  for the same manifest path fail closed. Only clean relative `assets/...`
  paths are accepted. Shared and nested assets are retained; withdrawal
  rebuilds the store without the withdrawn release, including nested paths.
- `Promote`, `Rollback`, `Refresh` and `Withdraw` prepare/verify assets
  automatically. `portalrelease retain` remains a repair command.
- `retained` is now an atomically replaced symlink to an immutable generation
  under `retained-generations/` with a completeness marker. A failed switch
  keeps the previous generation; pending/incomplete trees are ignored;
  a missing link recovers the newest complete generation; the legacy
  directory is migrated once and recoverably. The current and immediately
  previous generations stay for in-flight readers; withdrawal drops older
  generations.
- Withdrawal prepares a transition store containing the still-current and
  fallback releases, moves the pointer, records the withdrawal, then purges
  the withdrawn release. A purge failure leaves the fallback fully serveable
  and is surfaced for a repair `retain`.

Maintained tests: `TestRetentionKeepsCurrentAssetsBeyondKeep`,
`TestFirstPublicationRetainsEveryCurrentAsset`,
`TestPromotionFailsBeforeSwitchWhenRequiredAssetsDoNotFit`,
`TestRetentionHandlesNestedAssetsAndWithdrawPurgesThem`,
`TestRetentionRejectsConflictingSharedPaths`,
`TestRetainedStoreSwitchFailureKeepsPreviousGeneration`,
`TestRetainedStoreRecoversAfterMissingLink`,
`TestRetainedLegacyDirectoryMigrationIsRecoverable`,
`TestRetainedStoreCleansOldGenerationsAfterSwitch`,
`TestWithdrawKeepsCurrentAssetsUntilPointerMoves`,
`TestWithdrawPurgeFailureLeavesServingFallback`,
`TestRollbackKeepsCurrentAssetsUntilPointerMoves`,
`TestRetainedHTTPReadersSurviveTransitions`,
`TestESPFollowupRefreshRetainsCurrentAssets`.

## Verification at `7fd5c414`

Disposable local PostGIS; existing synthetic portal artifacts. Commands run
from `backend/` unless noted:

```sh
go test -race -count=1 -tags=integration ./internal/modules/official/...
go test -race -count=1 -tags=integration ./internal/modules/enterpriseexport/...
go build ./...
go vet ./internal/modules/official/... ./internal/modules/enterpriseexport/... ./cmd/portalrelease
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/official/... ./internal/modules/enterpriseexport/... ./cmd/portalrelease
sqlc vet && sqlc generate
gofmt -l internal/modules/official internal/modules/enterpriseexport cmd/portalrelease
git diff --check
```

All PASS; `sqlc generate` was byte-stable. Staticcheck's pinned v0.8.1 needs
`GOTOOLCHAIN=go1.27.1` because it predates the go1.27.2 export format.

Local isolated Caddy rehearsals (pinned `caddy:2.10.2-alpine`):

- full: `PORTAL_ARTIFACT_A=/tmp/esp-rehearsal/a-full-noindex`,
  `PORTAL_ARTIFACT_B=/tmp/esp-rehearsal/b-full-indexable`, port 18081;
- official-only: `a-official-noindex` / `b-official-indexable`, port 18082.

Both PASS promotion, A→B→A, retained-only asset after rollback, corrupt
current rollback, pause/recovery of the same artifact, second pause,
withdrawal to fallback, withdrawal to 404 and purged withdrawn asset.
The HTTP transition reader runs in-process with concurrent GETs during real
rollback/promote/retain switches.

## Scope, integration and remaining work

- No VPS, staging data, third party, push, PR, merge, wiki or publication.
  No monetary-precision, rights or public-scope approval is implied.
- The branch is based on `bf32027d`; `dev` has three later Android-only
  commits with no ESP file overlap. No blind merge was performed; the batch
  is `INTEGRATION_PENDING` against the maintained branch.
- No new manual full staging drill was run. The existing manual artifacts are
  synthetic/local and the staging serve copy must be re-cut through the
  automatic retention path when staging work is authorized.
- Remaining obligations: integration into `dev`/final PR and required remote
  checks; authorized staging A→B→A with real admitted content; monetary
  precision policy; source rights/clarification answers; public scope,
  origin/CDN/TLS/approval/window; G09 production certification.
