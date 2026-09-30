# P13-T03C — Link/unlink transport + OpenAPI + sandbox evidence

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #24 (slice 3 of 3 — P13-T03 COMPLETE, pending phase
merge; slice plan in
[p13-t03a-oidc-verifier](p13-t03a-oidc-verifier.md)). No live
Google/Apple contact anywhere (deferred to P13-T05/G13 device
acceptance, recorded, never mocked green).

## What changed

- `account/adapters/http/http.go` — session-authenticated provider
  transport: `Service` gains `Link/Unlink/ListProviders`;
  `Handler.Audience` carries the frozen server-side audience
  (`anpfuel-backend` default; client flags untrusted);
  `POST /v1/accounts/providers/{link,unlink,list}` derive the caller
  account via `ValidateAccess` (no client account IDs), bind with
  `LinkProvider`, refuse last-method/not-linked atomically.
  Responses are `no-store` with opaque subjects only; `fail()` maps
  `oidc-*` → 401 (unavailable → 503), cross/email-only → 403,
  last-method → 409, not-linked → 404, unknown account → 401.
- `account/adapters/http/providers_test.go` (7 suites, stub-issuer
  sandbox): link/unlink/list happy flow both providers, cross-account
  403 without subject echo, replay 401 + mismatch 401, malformed 400
  matrix, forged session 401, secret non-echo, full `fail()` status
  map (incl. 409/404).
- `contracts/openapi/v1.yaml` — additive paths `link`/`unlink`/
  `list` reusing `ProviderLink` (request bodies inline; session proof
  in JSON body, never URL). `vacuum` 0 errors / 0 warnings
  (added `example: []` to the list array after isolating 2 new
  `oas3-missing-example` warnings to that path); `apicontract` green
  on existing vectors.
- `cmd/api/main.go` — production verifier wiring: `HTTPKeys`
  (`ProductionJWKS`) behind the frozen 1h `CachedKeys` plus
  `PGNonces`, `Issuers` google/apple, 120s skew, server-side
  audience; stub issuers stay test-only.

## RED → GREEN

- RED proven by dropping the empty-`id_token` transport guard:
  `TestProviderTransportRejectsMalformed/empty-token` fails with
  `got 401 (account.oidc-wrong-audience)` instead of 400; GREEN on
  restore. T03B cross-account RED stays recorded in its slice.

## Validation (exact commands, this host)

```sh
cd backend && go build ./...
gofmt -l internal/modules/account cmd/api
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/account/... ./cmd/api/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/account/...
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/account/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
(cd backend && GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/platform/apicontract/...)
make quick-verify
git diff --check
```

Outcome: unit 6/6 account packages green (http 8/8 incl. 7 new
provider suites); integration with `-race` green (adapters incl. 5/5
T03B link suites + email suites, http, application, domain);
`build` ok; `vacuum` 0 errors/0 warnings/12 info; `apicontract` ok;
`quick-verify` selection=full ok 9s; `diff --check` clean; secrets
scan clean.

## Limits (not claimed)

- Sandbox evidence runs against stub-minted tokens only; live
  Google/Apple JWKS fetch is wired but uncontacted — device/sandbox
  runs with real providers belong to P13-T05/G13 acceptance.
- Suspension/erasure enforcement is P13-T04; transport maps the new
  errors but suspended/deleted gating stays pending.
- List uses POST with body session proof (consistent with revoke);
  header-based auth stays future work.

## Rollback

Transport rollback = revert `http.go` Service/Handler/routes plus
`providers_test.go`, OpenAPI paths and `main.go` verifier wiring;
T03A/B persistence stays inert without routes. No migration change
in this slice.

## Next

P13-T04 recovery revocation and privacy (#25, same branch).
P13-T03 issue #24 stays open until the phase PR merges.
