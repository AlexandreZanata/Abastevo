# P13-T03B — Provider links + nonces persistence + real-DB integration

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #24 (slice 2 of 3; slice plan in
[p13-t03a-oidc-verifier](p13-t03a-oidc-verifier.md)). No endpoints,
no OpenAPI paths yet (T03C).

## What changed

- `db/migrations/000021_provider_links.sql` (append-only):
  `account_provider_links` (PK `(account_id, provider)`, UNIQUE
  `(provider, subject)`, provider CHECK google/apple, non-empty
  issuer/subject) + `account_oidc_nonces` (PK nonce). Subjects only;
  email is display/relay, never a merge key.
- `db/queries/account/accounts.sql` + `sqlc.yaml` stanza (adds
  `000021`) + committed regeneration (`sqlc vet`/`generate` clean,
  stable): `GetAccountByID`, `FindProviderOwner`, `Get/List`,
  `Insert/Update/DeleteProviderLink`, `CountAddressesByAccount`,
  `InsertNonce` (`ON CONFLICT DO NOTHING RETURNING`).
- `account/domain`: `ProviderLink` + `ValidProvider` (`account.go`);
  `ErrLastLoginMethod` (`link-last-method-refused`),
  `ErrProviderNotLinked` (`link-provider-not-linked`),
  `ErrAccountNotFound` (`account-unknown`) + verdict mapping
  (`ports.go`, `stdlib_test.go` extended).
- `account/application`: `Store` extended (link/unlink/list/owner/
  address-count/nonce); `links.go` `LinkProvider` (verified token is
  the only proof; email never consulted), `UnlinkProvider`
  (atomic last-method guard), `ListProviders`; `MemStore` parity
  (mutex single-step, cross-account refusal, same-account converge/
  rotation, bare-account support via `accountsByID`).
- `account/adapters`: `PGStore` link/unlink/list/owner/count/nonce
  (transactional `FOR UPDATE`-free serial lanes: owner check, then
  insert/update; unique-race maps to cross-account; unlink counts
  providers + addresses atomically); `PGNonces` DB ledger
  implementing the OIDC `NonceStore` (PK admits once, errors fail
  closed).
- Tests: `application/links_test.go` (7 suites: both-provider happy,
  cross-account, nonce replay + mismatch, last-method with
  email-remaining vs bare-account refusal, email-only never merges,
  rotation refusal, ledger single-use); `adapters/
  pglinks_integration_test.go` (`integration` tag, fresh DB per test,
  `000020`+`000021` asserted): happy both providers, cross-account,
  bare-account last-method, nonce replay across two verifiers sharing
  one DB ledger, rotation against DB nonces.

## RED → GREEN

- RED proven by dropping the `bySubject` cross-account guard in
  `MemStore.LinkProvider`: `TestLinkCrossAccountRefused` fails with
  `got <nil>` + `verdict "ok"` and persists a second binding; GREEN
  on restore (unit pass).
- No transport changes: `adapters/http` untouched; OpenAPI paths stay
  T03C.

## Validation (exact commands, this host)

```sh
cd backend && sqlc vet && sqlc generate
gofmt -l internal/modules/account db/queries/account
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/account/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/account/...
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/account/...
make quick-verify
git diff --check
```

Outcome: unit 6/6 packages green; integration (real disposable
PostGIS, fresh DB per test) green under `-race` — adapters 5/5 link
suites + 6/6 email suites, application/domain/oidc/http green;
`sqlc` clean + stable; `gofmt`/`vet` clean; `quick-verify`
selection=full ok 10s; `diff --check` clean; secrets scan clean.

## Limits (not claimed)

- No link/unlink HTTP transport or OpenAPI paths (T03C); no live
  Google/Apple contact anywhere — sandbox evidence lands in T03C
  against stubs, live-provider runs stay outstanding to P13-T05/G13
  device acceptance.
- Suspension/erasure enforcement is P13-T04; links persist now.
- Nonce ledger has no TTL purge yet (retention follows T04 inventory).

## Rollback

Append-only migration stays (never rewrite applied history); code
rollback = delete `links.go`, `pgnonces.go`, link tests, provider
methods + `000021` stanza rows (DB tables stay inert until T03C).

## Next

P13-T03C link/unlink transport + OpenAPI + sandbox evidence (#24,
same branch).
