# P14-T05A — Reports, visibility and operator path

Status: LOCAL_DONE on `codex/phase-14-station-feedback` (phase PR
#33, draft). Issue: #32 (slice 1 of P14-T05; issue stays open
until the phase PR merges). No live-provider contact.

## Slice acceptance (frozen before coding)

- T05A (this commit) — moderation entry for feedback: COMMENT
  target in the moderation allowlist (invalidate/review/resolve/
  dismiss; block stays contributor-only), `000026` target widening
  + `000027` visibility lane, report endpoint opening real cases
  with quota and flag-once, operator hide/show bound to open
  cases, hidden rows excluded from every public read, runbook.
  Reporter deletion cannot cascade (cases store no reporter).
- T05B (next) — account-footprint export/erase + restore replay
  + G14 exit battery.

## What changed

- `moderation/domain`: `TargetComment` + validation +
  `AllowedForTarget` (invalidate allowed, block refused) +
  P2 default; existing suites extended in place.
- `000026` (moderation CHECK widening, additive) +
  `000027` (visibility visible/flagged/hidden + index);
  sqlc stanzas extended per owner (moderation/feedback).
- `feedback`: `SetVisibility`/`FlagComment` ports (Mem parity,
  PG lanes), `ReportComment` (reason bounds, gate, quota,
  open, flag), `SetVisibility` use case (operator, tombstone
  wins), report route (202) + 400/403/404/429 mapping, OpenAPI
  path, main closures (moderation Open, report quota).
- `cmd/ops feedback hide|show` (case-bound, OPEN/IN_REVIEW
  only) + pure parse tests + usage line.
- `docs/operator/feedback-moderation.md` runbook.

## RED → GREEN

- RED proven by dropping the flag call after case open: the
  report succeeds but visibility stays visible; GREEN on restore.
- `sqlc` rejected a combined cross-owner migration (moderation
  stanza cannot see feedback tables): split 000026/000027 by
  owner before any test ran green.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/feedback internal/modules/moderation cmd/api cmd/ops
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/feedback/... ./internal/modules/moderation/... ./cmd/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/feedback/... ./internal/modules/moderation/... ./cmd/ops/
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/feedback/... ./internal/modules/moderation/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
make quick-verify
git diff --check
```

Outcome: unit green (target matrix, report/quota/visibility,
ops parse); integration with `-race` green (real COMMENT case
rows, flag→hide→unhide walk, hidden-leak audit, reporter
deletion independence); `vacuum` 0/0/27 info; `quick-verify`
selection=full ok; `diff --check` clean; secrets scan clean.

## Limits (not claimed)

- Account-footprint export/erase + restore replay are T05B.
- Reads are no-store; appeal route is a new case (runbook).
- Live providers stay device evidence (deferred).

## Rollback

Migrations stay append-only; code rollback = revert target
const, visibility paths, report/ops additions and docs.
Moderation behavior for old targets is untouched.

## Next

P14-T05B export/erase/restore + G14 exit (#32, same branch).
Issue #32 stays open until the phase PR merges.
