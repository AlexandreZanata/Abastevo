# P14-T05B — Account-footprint export/erase + G14 exit

Status: LOCAL_DONE on `codex/phase-14-station-feedback` (phase PR
#33, draft). Issue: #32 (slice 2 of P14-T05; issue stays open
until the phase PR merges). No live-provider contact.

## Slice acceptance (frozen before coding)

- T05B (this commit) — account-footprint export/erase for station/
  fuel feedback: `ExportAccount` deterministic `feedback-export-v1`
  JSON (only the requested account's ratings/comments/votes, sorted,
  empty sections `[]`); `EraseAccount` tombstones live ratings/
  comments/votes by account and rebuilds every touched rating key
  and vote tally from live rows (history stays for audit,
  aggregates ignore it); convergent replay (second run zero;
  resurrected rows re-tombstone = restore replay via the same
  command, no separate ledger); `000028` account indexes (append-
  only); `ops feedback export|erase` + runbook; G14 exit battery
  (maintained stats/tallies equal full rebuilds, public reads
  exclude erased, hidden-leak audit, 8-way erase race).
  Policy frozen: suspension/deletion blocks writes; past rows stay
  until this explicit erasure runs. No public HTTP export/erase
  path (restricted operator tool only).

## What changed

- `000028_feedback_account_idx.sql` (additive indexes on
  account_id for ratings/comments/votes) + `sqlc.yaml` stanza +
  `db/queries/feedback/erase.sql` (ListRatings/Comments/VotesByAccount).
- `feedback/application`: `Store`/`CommentStore`/`VoteStore`
  `List*ByAccount` ports; `footprint.go` (`ExportAccount`,
  `ExportAccountBytes`, `EraseAccount` with concurrent-erase
  convergence on `ErrCommentNotFound`).
- `feedback/adapters`: `pgstore_erase.go` (PG lists) +
  `MemStore` lists (sorted, deterministic).
- `cmd/ops feedback export|erase` (pure parse + audited run,
  usage line) + `docs/operator/feedback-moderation.md` T05B section.
- Tests: `footprint_test.go` (redaction, tombstone+rebuild,
  replay zero, blank/gate refusals); `pgerase_integration_test.go`
  (real-DB export/erase + G14 exit + restore-resurrect replay +
  8-way concurrent erase).

## RED → GREEN

- RED proven by construction: pre-erase rows read live (export
  holds 1/1/1, votes tally 1/0); dropping the `EraseAccount` call
  leaves `ViewComment c1` readable and the `must vanish` assertion
  fails; GREEN on restore.
- Real race found and fixed in-task: 8-way concurrent `EraseAccount`
  failed with `feedback: comment not found` (list-then-delete lost
  race between erasers). Fixed by treating `ErrCommentNotFound` in
  `EraseAccount` as converged (ratings/votes lanes already returned
  false/nil); GREEN on rerun, total tombstones exactly 2 across
  8 racers.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/feedback cmd/ops
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/feedback/... ./cmd/ops/
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/feedback/... ./cmd/ops/
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/feedback/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
make quick-verify
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: unit green (export redaction + erase tombstone/rebuild/
replay); integration with `-race` green (real-DB export 1/1/1,
erase 1/1/1 + 1 key + 1 tally, stats 1/3 = rebuild, c2 tally 0/0
= rebuild/null agreement, erased leak audit, replay zero,
restore-resurrect re-tombstone, 8-way erase total 2); `vacuum`
0/0/27 info (no spec change); `quick-verify` selection=full ok
12s; `diff --check` clean; secrets scan clean.

## Limits (not claimed)

- Export/erase run via the restricted operator tool; no public
  HTTP owner archive path (privacy contributor export flow
  untouched — feedback uses account IDs, a separate identity
  space documented in the runbook).
- Feedback erasure has no separate deletion-ledger table:
  tombstoned history + deterministic export serve audit; restore
  replay is convergent re-erase, proven by resurrect test.
- Live providers stay device evidence (deferred).

## Rollback

Migrations stay append-only (`000028` indexes drop cleanly);
code rollback = revert footprint/erase/ops additions and docs.
Moderation/visibility behavior for old targets is untouched.

## Next

G14 exit review + phase closure (#32, same branch; issues #28–#32
stay open until PR #33 merges).
