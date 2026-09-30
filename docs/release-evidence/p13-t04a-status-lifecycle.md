# P13-T04A — Account suspension/deletion + status enforcement

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #25 (slice 1 of P13-T04; slice plan below — P13-T04 is
oversized for one commit, so it ships letter-suffixed per ROADMAP
rules). No live-provider contact; no contributor-key binding yet.

## Slice acceptance (frozen before coding)

- T04A (this commit) — status lifecycle + enforcement: `Suspend`
  (status + revoke all families atomically), `Reactivate`
  (suspended→active; sessions stay revoked; deleted refuses),
  `Delete` (status + revoke + drop address/provider bindings;
  audit row stays). Status-first ordering on consume (pre-check
  without code burn + post-session race guard), refresh (pre +
  post-rotate guard with revoke), `ValidateAccess` (covers all
  session transport), link (pre-check before nonce burn + store
  re-check), unlink (store check). Deleted addresses re-sign as NEW
  accounts. Transport: `POST /v1/accounts/deletion` self-delete +
  403/410 mapping. Suspend has no public route (operator `Service`
  API).
- T04B (next) — contributor-key binding with fresh reauth, stolen-ID
  cases, old-key/session revoke races, cross-module erasure
  (community/evidence/trust) and recovery audit.

## What changed

- `account/domain`: `ErrAccountSuspended` (`account-suspended`),
  `ErrAccountDeleted` (`account-deleted`) + verdicts (`ports.go`,
  `stdlib_test.go`); `Account.Active()` (`account.go`).
- `account/application`: `Store` gains `Get/Suspend/Reactivate/
  DeleteAccount`; `status.go` use cases + `accountUsable`
  (unknown status fails closed as suspended); `RequestCode` skips
  issuance for dead addresses with identical nil answers (no
  oracle); `ConsumeCode`/`Refresh` race guards revoke-then-refuse;
  `LinkProvider` pre-checks before nonce burn.
- `account/adapters`: `MemStore` + `PGStore` (`SetAccountStatus`,
  `DeleteAccountAddresses/Providers` via new sqlc queries, stanza
  unchanged — same `000021` schema); suspend/delete are single
  transactions (status + revoke + drops).
- `account/adapters/http`: `POST /v1/accounts/deletion`
  (session-derived self-delete only; suspended callers use the
  operator path), `fail()` 403 suspended / 410 deleted.
- `contracts/openapi/v1.yaml`: additive `deletion` path + `Gone`
  response ref + 403/410 refs on consume/refresh/link/unlink/list;
  `vacuum` 0 errors/0 warnings (DELETE-with-body and verb-in-path
  variants refused during work: `no-request-body`,
  `no-http-verbs-in-path`; POST noun path is the repo convention).
- `contracts/testdata/account`: `account-suspended.json`,
  `account-deleted.json` frozen vectors.

## RED → GREEN

- RED proven by dropping the consume pre-check: the suspended
  consume burns the code and the reactivated re-consume fails with
  `code already consumed` instead of succeeding; GREEN on restore.
- Status-first ordering proven by post-suspend refresh answering
  `suspended` (not `revoked`) in unit + PG suites.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/account; GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/account/... ./cmd/api/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/account/...
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/account/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
(cd backend && GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/platform/apicontract/...)
make quick-verify
git diff --check
```

Outcome: unit 6/6 green (5 new status suites + 8 HTTP incl. 4
deletion suites); integration with `-race` green (3 new PG status
suites incl. 16-way post-delete refresh all `deleted`); `build`
ok; `sqlc` clean + stable; `vacuum` 0/0/13 info; `apicontract` ok;
`quick-verify` selection=full ok 10s; `diff --check` clean; secrets
scan clean.

## Limits (not claimed)

- Contributor-key binding, fresh-reauth proofs, stolen-ID races and
  cross-module social erasure are T04B.
- Suspended self-delete via transport refuses 403 (operator
  `Service.DeleteAccount` covers it); unsuspend is operator-only.
- Nonce ledger has no purge; live providers stay P13-T05/G13.

## Rollback

No migration change in this slice (status columns since `000020`).
Code rollback = revert status use cases, enforcement guards,
transport route, OpenAPI additions and vectors; stores keep inert
methods.

## Next

P13-T04B contributor binding + cross-module erasure (#25, same
branch). Issue #25 stays open until the phase PR merges.
