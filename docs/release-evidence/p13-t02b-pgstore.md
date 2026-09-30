# P13-T02B — Postgres store + migration + real-DB integration

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #23 (slice 2 of 3; slice plan in
[p13-t02a-account-service](p13-t02a-account-service.md)). No endpoints,
no mail delivery yet (T02C).

## What changed

- `db/migrations/000020_accounts.sql` (append-only): `accounts`,
  `account_addresses`, `account_email_codes`, `account_session_families`
  with status CHECK, alias uniqueness and address/issued/account indexes.
  Hashes and salts only; codes/tokens never persist.
- `db/queries/account/accounts.sql` + `sqlc.yaml` stanza (owner-scoped
  schema list) + committed generated package (`sqlc vet`/`generate`
  clean, re-generate stable, no drift in other packages).
- `account/adapters/pgstore.go` — `PGStore` implementing the application
  `Store`: generated queries for CRUD, one transaction with `FOR UPDATE`
  row locks for consume/rotate. Consume matches by hash across the
  address rows (newest-first, stable id tiebreak); rotation is
  compare-and-swap on the verified refresh hash plus revoked check.
- `Store` interface + both stores hardened (T02A follow-up inside this
  slice): `TryConsume` now takes the plaintext code + hasher and matches
  across rows; `Rotate` takes the expected hash (CAS); `CreateAccount`
  reports `ErrAddressLinked` so parallel signups log into the winner.
  `domain.EqualHash` shared by both paths; memstore updated identically.
- Integration `pgstore_integration_test.go` (`integration` tag, fresh
  database per test, migration `000020` asserted): round trip,
  16-way concurrent consume (exactly one), expiry/attempts/enumeration/
  quota, refresh reuse→revoke + 30-day expiry, replay-superseded, and
  parallel signup linking one account.

## Bug found and fixed in-task

Same-second issuance made `ORDER BY issued_at DESC` nondeterministic:
the service salted with the wrong row and Postgres picked either sibling.
Fixed by hash-match-first with deterministic fallback in both stores
(plus two regression tests); the memstore parity gap closed the same way.

## Validation (exact commands, this host)

```sh
docker compose -f infra/compose.dev.yml up -d db
cd backend && sqlc vet && sqlc generate
gofmt -l ./... && go vet ./internal/modules/account/...
go test -count=1 ./internal/modules/account/...
go test -count=1 -race -tags=integration ./internal/modules/account/...
make quick-verify
git diff --check
```

Outcome: unit suites pass; integration (real disposable PostGIS,
fresh DB per test) passes under `-race` — adapters 6/6, application
and domain green; `sqlc` clean + stable; `gofmt`/`vet` clean;
`quick-verify` ok; `diff --check` clean; secrets scan clean.

## Limits (not claimed)

- HTTP transport, route wiring, mail delivery and OpenAPI paths are
  T02C. SMTP stays out (local deterministic adapter only, per A02).
- Concurrent same-address signup resolves to one account via
  `ErrAddressLinked` retry-as-login; cross-family races fail closed.

## Rollback

Append-only migration stays (never rewrite applied history); disabling
is a forward `DROP`-free no-op since nothing else reads these tables.
Code rollback = delete the account module + stanza + queries.

## Next

P13-T02C transport + mail + redaction + paths (#23, same branch). Then
P13-T03 provider verification (#24).
