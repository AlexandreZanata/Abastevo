# P13-T04B — Contributor binding + recovery audit

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #25 (slice 2 of P13-T04; T04C cross-module erasure +
transport follows). No live-provider contact; no identity imports —
ports only, stubs in tests.

## Slice acceptance (frozen before coding)

- T04B (this commit) — binding core: migration `000022`
  (`account_contributor_bindings` with `UNIQUE(contributor_id)` +
  `account_binding_audit` append-only), dual-proof `Bind`
  (live session + key proof, account server-derived), stolen-ID
  refusal, same-account converge, `Unbind` + not-found, audit trail,
  delete drops bindings. Freshness holds by construction (access
  ≤900s). No transport/OpenAPI (T04C).
- T04C (next) — key-proof production adapter, bind/unbind transport,
  cross-module erasure on delete, old-key/session revoke races.

## What changed

- `db/migrations/000022_account_bindings.sql` (append-only) +
  `sqlc.yaml` stanza + 9 owner queries + committed regeneration.
- `account/domain`: `ContributorBinding`, `BindingAudit`,
  bind/unbind actions; `ErrBindingCrossAccount`
  (`binding-cross-account-refused`), `ErrBindingNotFound`,
  `ErrBindingInvalid`, `ErrKeyUnavailable` + verdicts; `KeyProver`
  port (opaque proof in, proven fingerprint out).
- `account/application`: `Store` gains bind/unbind/list/owner/audit;
  `binding.go` ceremony (session → status pre-check → key proof →
  atomic bind+audit); `Service.KeyProver` (nil fails closed).
- `account/adapters`: `MemStore` parity + `PGStore` transactional
  lanes (owner check then insert/update, unique-race maps to
  cross-account); `DeleteAccount` (both) drops bindings.
- `contracts/testdata/account/account-binding-cross-account.json`
  frozen stolen-ID vector.

## RED → GREEN

- RED proven by dropping the `byContributor` stolen-ID guard in
  `MemStore.BindContributor`: the stolen bind succeeds and persists
  a second owner row instead of refusing; GREEN on restore.
- Status-first ordering reused from T04A (suspended/deleted binds
  refuse before key verification, without burning proofs).

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/account; GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/account/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/account/...
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/account/...
make quick-verify
git diff --check
```

Outcome: unit 6/6 green (6 new binding suites); integration with
`-race` green (4 new PG suites incl. 16-way same-contributor race
splitting winners/refusals with one surviving owner);
`quick-verify` selection=full ok 16s; `diff --check` clean; secrets
scan clean.

## Limits (not claimed)

- Production key-proof adapter, bind/unbind transport + OpenAPI,
  cross-module social erasure and old-key revoke races are T04C.
- Audit retention purge follows the scheduler inventory (P07-T05
  pattern); no TTL delete in this slice.
- Live providers stay P13-T05/G13.

## Rollback

Append-only migration stays; code rollback = revert binding use
cases, store methods, queries and vectors. No transport to unwind.

## Next

P13-T04C binding transport + erasure wiring (#25, same branch).
Issue #25 stays open until the phase PR merges.
