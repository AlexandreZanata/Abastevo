# P14-T02 — Rating transactions and aggregates

Status: LOCAL_DONE on `codex/phase-14-station-feedback` (phase PR
#33, draft). Issue: #29 (slice 2 of 5; issue stays open until the
phase PR merges). No transport, no paths, no jobs in this slice.

## Slice acceptance (frozen before coding)

- T02 (this commit) — one current rating per account/station/fuel:
  `000023` (live row + partial unique index + rebuildable stats),
  converge-on-equal-stars / revision-bump-on-change / tombstone
  delete, per-write exact stats with full-scan rebuild, required
  account-gate + station-exists ports (fail closed, no anonymous
  baseline). Suspended sessions, wrong targets and unknown keys
  refuse; public identity never leaks (account IDs stay
  server-side; no transport yet to leak through).

## What changed

- `db/migrations/000023_feedback.sql` (append-only) +
  `sqlc.yaml` feedback stanza + 7 owner queries + committed
  regeneration (`feedback_ratings` with 1–5 CHECK + live-unique,
  `feedback_rating_stats` rebuildable).
- `feedback/domain`: `StoredRating` (+ `Live`), `RatingStats`
  (+ integer `MeanMilli`), `ErrGateRequired`/`ErrStatsMissing`/
  `ErrRatingNotFound` + verdicts, `Clock` port.
- `feedback/application`: `Store`, required `AccountGate` +
  `StationExists` (nil fails closed), `Service.Rate`/
  `DeleteRating`/`RebuildStats`, mutex `MemStore` with the exact
  Postgres atomicity contract.
- `feedback/adapters`: transactional `PGStore` (lock → converge/
  update/insert → recompute stats → commit; lost unique race
  re-reads the winner, never forks).
- Deletion policy frozen: tombstone keeps history for audit and
  leaves aggregates; account deletion preserves rating facts
  (privacy erasure unlinks identity per P07 flow); suspended
  accounts keep past ratings but cannot write.

## RED → GREEN

- RED proven by dropping the MemStore converge branch: duplicate
  ratings recreate instead of converging; GREEN on restore.
- `int16`/`int32` sqlc mismatch (SMALLINT) found and fixed at
  first `go vet`.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/feedback; GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/feedback/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/feedback/...
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/feedback/...
make quick-verify
git diff --check
```

Outcome: unit green (converge/edit/delete/rebuild/gate matrix);
integration with `-race` green (16-way same-key converge with
exactly one creator, edit/delete/rebuild exact, suspended +
wrong-target refusals); `quick-verify` selection=full ok;
`diff --check` clean; secrets scan clean.

## Limits (not claimed)

- No HTTP transport or OpenAPI paths (T03); no vote projections
  or jobs (T04); no moderation/privacy (T05).
- Station existence is an injected port (faked in tests); main
  wiring lands with transport.

## Rollback

Append-only migration stays; code rollback = delete the feedback
application/adapters additions (domain T01 stays). No transport
to unwind.

## Next

P14-T03 comments/replies + transport (#30, same branch). Issue
#29 stays open until the phase PR merges.
