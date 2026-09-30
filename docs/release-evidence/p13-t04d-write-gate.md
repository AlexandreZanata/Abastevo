# P13-T04D — Social-write gate + suspend-revoke races

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #25 (slice 4 of P13-T04 — P13-T04 COMPLETE, pending
phase merge). No live-provider contact.

## Slice acceptance (frozen before coding)

- T04D (this commit) — enforcement of suspension/erasure on social
  writes: `ErrAccountBlocked` + nil-safe `CheckAccount` on
  community `Ports`/`VotePorts`, enforced in Submit/Confirm/Dispute
  before quota burn; HTTP 403 `community.account-blocked`;
  account-side `BindingBlocked` helper (unbound keeps the anonymous
  baseline; bound follows owner status; deleted reads as unbound
  since bindings drop at deletion); `main.go` wiring for both port
  sets; suspend-storm race proof (concurrent session use vs
  suspend stays closed, post-commit everything refuses, every
  family revoked). Key purge of ex-contributors stays with the
  privacy erasure flow (P07), which retires keys; the audit ledger
  retains contributor IDs for that operator path.

## What changed

- `community/application`: `ErrAccountBlocked`; `CheckAccount`
  ports on `Ports` + `VotePorts` (nil = pre-account baseline);
  gate before quota in all three write use cases.
- `community/adapters/http`: 403 `community.account-blocked`
  mapping (+ table row).
- `account/application/binding.go`: `BindingBlocked` helper over
  the `Store` port (no new queries).
- `cmd/api/main.go`: shared `accountStore` handle feeds the
  account service plus the `checkAccountGate` closure (blocked →
  `ErrAccountBlocked`, store errors propagate to 500) wired into
  both community port sets.
- Tests: community `gate_test.go` (block before quota, error
  passthrough, nil baseline, votes block); account `BindingBlocked`
  unit matrix + PG matrix; `pggate` suspend storm
  (4 independent logins × validate/refresh racers + suspender,
  stable across 3 runs).

## RED → GREEN

- RED proven by dropping the `Submit` gate call: a blocked submit
  proceeds into execution instead of refusing with
  `ErrAccountBlocked` (test fails); GREEN on restore.
- Storm-test bug found and fixed in-task: shared-session racers
  confused post-rotation stale-token denial (`code-unknown`,
  correct behavior) with gate failure. Independent logins per
  racer pair plus kind-aware classification fixed it; the
  investigation is recorded here, not hidden.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/account internal/modules/community cmd/api
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/account/... ./internal/modules/community/... ./cmd/api/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/account/... ./internal/modules/community/...
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/account/... ./internal/modules/community/...
GOTOOLCHAIN=go1.27.1 go test -count=3 -run TestPGSuspendStormClosesSessions -tags=integration ./internal/modules/account/adapters/
make quick-verify
git diff --check
```

Outcome: unit green (community gate 2/2 + votes mapping row,
account gate matrix); integration with `-race` green (account +
community incl. storm 3/3); `vacuum` 0/0/14 info (no spec change);
`quick-verify` selection=full ok 14s; `diff --check` clean;
secrets scan clean.

## Limits (not claimed)

- Data purge of ex-contributor social rows stays with the privacy
  erasure flow (existing per-contributor scopes + ledger replay);
  this slice blocks writes, which is the T04 acceptance.
- Validation-time recheck needs token→contributor reverse lookup;
  submit-time TOCTOU vs suspend matches the quota pattern and the
  storm proves the session layer closes it.
- Live providers stay P13-T05/G13.

## Rollback

No migration change in this slice. Code rollback = revert gate
ports, helper, wiring and tests; account/community behavior
returns to T04C.

## Next

P13-T05 shared and native login integration (#26, same branch;
needs G12 device evidence per roadmap). Issue #25 stays open
until the phase PR merges.
