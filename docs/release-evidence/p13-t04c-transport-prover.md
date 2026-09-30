# P13-T04C — Binding transport + device-key prover

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #25 (slice 3 of P13-T04; T04D cross-module erasure
closes the task). No live-provider contact; stub issuers/provers stay
test-only.

## Slice acceptance (frozen before coding)

- T04C (this commit) — transport + production prover:
  session-authenticated `POST /v1/accounts/bindings/{bind,unbind,
  list}` with key proof in body, `ContributorBinding` schema +
  vectors, fail mapping (cross→403, not-found→404, invalid→400,
  unavailable→503, denied→401), production P-256 device-key prover
  over identity-owned keys via one narrow closure (no cross-module
  table imports), `main.go` wiring. Proof binds the ceremony
  account, so cross-account proofs deny on top of the stolen-ID
  ownership refusal.
- T04D (next) — cross-module erasure on delete plus old-key/session
  revoke races.

## What changed

- `account/domain`: `KeyProver` widened with `accountID`
  (phase-local refinement, unmerged); `ErrKeyProofDenied`
  (`key-proof-denied`) + verdict (uniform denial, no oracle).
- `identity/adapters/keysource.go`: `Registrar.FindDeviceKey`
  over the owned `FindKeyByFingerprint` query (no new queries);
  revocation surfaces instead of hiding.
- `account/adapters/keyprover`: frozen proof
  `<fp>.<exp>.<b64sig>` over
  `anpfuel-bind.v1\n<account>\n<contributor>\n<exp>`, 300s window,
  stdlib P-256, uniform denial. Real-crypto suites: happy, wrong
  key, cross-account proof, contributor mismatch, tamper, expiry
  edges, unknown/revoked/malformed, nil source.
- `account/adapters/http`: Service gains bind/unbind/list;
  handlers forward session + proof (account server-derived);
  `no-store` opaque responses.
- `contracts/openapi/v1.yaml`: `ContributorBinding` schema + 3
  paths reusing `Forbidden/NotFound/Gone`; `vacuum` 0/0; 2 new
  vectors keep `apicontract` green.
- `cmd/api/main.go`: `keyprover.Prover` wired onto
  `identityRegistrar.FindDeviceKey` closure + `Service.KeyProver`.

## RED → GREEN

- RED proven by accepting signatures without `ecdsa.Verify`: the
  tampered-signature case passes instead of denying; GREEN on
  restore. Stolen-ID RED stays recorded in T04B.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/account internal/modules/identity cmd/api
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/account/... ./internal/modules/identity/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/account/... ./internal/modules/identity/...
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/account/... ./internal/modules/identity/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
(cd backend && GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/platform/apicontract/...)
make quick-verify
git diff --check
```

Outcome: unit green (keyprover 4/4, http 13/13 incl. 5 binding
suites, identity unit green); integration with `-race` green
(account incl. T04B lanes, identity incl. 2 keysource suites);
`vacuum` 0/0/14 info; `apicontract` ok; `quick-verify`
selection=full ok 15s; `diff --check` clean; secrets scan clean.

## Limits (not claimed)

- Cross-module social erasure on delete and old-key/session revoke
  races are T04D; binding stores the proven fingerprint so rotation
  interplay has a stable base.
- Live providers stay P13-T05/G13; sandbox runs use stubs.

## Rollback

No migration change in this slice. Code rollback = revert prover,
transport, OpenAPI additions, vectors and wiring; T04A/B behavior
keeps working service-level.

## Next

P13-T04D erasure orchestration (#25, same branch). Issue #25 stays
open until the phase PR merges.
