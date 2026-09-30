# P14-T04 — Validity votes and percentages

Status: LOCAL_DONE on `codex/phase-14-station-feedback` (phase PR
#33, draft). Issue: #31 (slice 4 of 5; issue stays open until the
phase PR merges). No live-provider contact.

## Slice acceptance (frozen before coding)

- T04 (this commit) — changeable one-account-one-revision votes:
  `000025` (live-unique per account/comment/revision, exact
  per-revision tallies), self-vote refusal, converge/rewrite/
  idempotent-remove, revision move on comment edit, vote/remove/
  tally transport reusing `FeedbackAgreement` math, exact display
  counts. Zero votes is null; denominators match the shared
  fixtures (2/1 → 6666).

## What changed

- `db/migrations/000025_feedback_votes.sql` (append-only) +
  7 owner queries + committed regeneration.
- `feedback/domain`: vote choices, `StoredVote`, `VoteTally`
  (+ `Agreement` fold through the single math path),
  `ErrSelfVote`/`ErrVoteChoiceInvalid` + verdicts.
- `feedback/application`: `VoteStore`, `Service.Vote`/
  `RemoveVote`/`Tally`/`RebuildTally` (gate + target + self
  checks; removal converges no-op by task text, unlike rating
  delete), MemStore parity with signed deltas.
- `feedback/adapters`: transactional `PGStore` lanes;
  `http` vote/remove/tally routes (200 tallies, 400/401/403/
  404 mapping) with null-percentage JSON.
- Tallies AND rating stats moved from recompute-in-transaction
  to atomic signed deltas (see bug below); full recomputation
  stays in the Rebuild paths for reconciliation.
- `contracts/openapi/v1.yaml`: `FeedbackTally` schema + 3
  paths; `vacuum` 0/0; 2 tally vectors keep `apicontract`
  green.

## Concurrency bug found and fixed in-task

The T04 8-voter race tallied 2/0 instead of 8/0: recompute-in-
transaction loses updates under concurrency (overlapping
snapshots, last-writer-wins stale counts). The same latent race
existed in T02 rating stats (its tests never raced multiple
accounts). Both lanes now apply signed deltas in a single
`INSERT … ON CONFLICT DO UPDATE` statement, which serializes on
the row; recompute remains only for rebuilds. New multi-account
ratings race test covers the T02 lane. This fix is recorded
here, not hidden in a rewrite.

## RED → GREEN

- RED proven by neutering the self-vote guard: author votes
  proceed instead of refusing; GREEN on restore.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/feedback cmd/api; GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/feedback/... ./cmd/api/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/feedback/...
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/feedback/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
(cd backend && GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/platform/apicontract/...)
make quick-verify
git diff --check
```

Outcome: unit green (vote/change/remove/revision/gate matrix);
integration with `-race` green (golden 2/1 denominator,
change/remove/revision, 8-voter exact tally, suspended
refusal); `vacuum` 0/0/26 info; `apicontract` ok;
`quick-verify` selection=full ok; `diff --check` clean;
secrets scan clean.

## Limits (not claimed)

- No moderation/privacy (T05); no jobs — tallies stay
  synchronous by design (single-statement deltas need no
  broker; rebuilds are the replay story, documented above).
- Reads are no-store; shared-cache policy deferred.

## Rollback

Append-only migrations stay; code rollback = delete vote
slices (domain records, application, adapters, transport,
OpenAPI additions). Ratings/comments keep working.

## Next

P14-T05 moderation/privacy/exit (#32, same branch). Issue #31
stays open until the phase PR merges.
