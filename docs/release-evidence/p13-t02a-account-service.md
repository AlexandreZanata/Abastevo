# P13-T02 — Email access-code backend (sliced: T02A / T02B / T02C)

Status: T02A LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #23. The task is oversized for one atomic change (domain +
Postgres + transport), so it ships in three letter-suffixed slices with
explicit acceptance below, per the roadmap splitting rule. One slice per
atomic commit; issue #23 stays open until the phase PR merges.

## Slice acceptance (frozen before coding)

- T02A — domain services + application use cases + SHA-256 hasher +
  mutex memory store (this commit): valid/replay/expired/attempts/
  enumeration/cooldown/quota, refresh rotation with reuse→revoke,
  access expiry, revoke-all, no-plaintext-at-rest, concurrent consume
  admits exactly one. Pure unit tests with fake clock, no DB.
- T02B — Postgres store + append-only migration `000020` + sqlc `account`
  package + real-DB integration under `-race`: concurrent consume once,
  resend quota, enumeration-identical errors, refresh reuse revokes
  family, migration from empty + checksum/ledger behavior.
- T02C — HTTP transport + route wiring + memory mail adapter + redaction
  tests + additive `/v1/accounts*` OpenAPI paths with contract vectors;
  vacuum/apicontract green. SMTP delivery stays out (local deterministic
  adapter only, per A02).

## T02A what changed

- `account/domain/account.go` — `Account`, `EmailCode`, `SessionFamily`
  rows (salted token hashes only), statuses, `Live` predicates,
  PII-minimized `AddressHash`.
- `account/domain/hash.go` — `SHA256Hasher` (constant-time compare),
  CSPRNG `GenerateCode`/`GenerateToken`/`GenerateAlias`.
- `account/domain/ports.go` — session errors (`reuse` revokes family,
  `revoked`, `expired`) + stable verdict codes.
- `account/application/accounts.go` — `Service` with `RequestCode`
  (enumeration-safe nil on unknown/cooldown/quota), `ConsumeCode`
  (find-or-create FREE account + session), `Refresh` (rotate; superseded
  token revokes family), `ValidateAccess`, `RevokeAll`. Injectable
  clock/generators; plaintext never crosses into storage.
- `account/application/memstore.go` — mutex memory `Store` honoring the
  exact atomicity contract T02B re-implements in SQL (single-step
  consume/rotate under one lock).
- Tests: 4 domain (bounds, hasher, generators, PII) + 8 service
  (valid/replay/expiry/attempts/enumeration/quota/rotation/reuse/
  access/revoke/no-plaintext/16-way race). 12/12 T01 fixture vectors
  remain the adversarial spec.

## Validation (exact commands, this host)

```sh
cd backend && gofmt -l ./... && go vet ./internal/modules/account/... \
  && go test -count=1 ./internal/modules/account/... \
  && go test -count=1 -race ./internal/modules/account/...
make quick-verify
git diff --check
```

Outcome: all suites pass incl. `-race` (16-goroutine consume admits
exactly one); `gofmt`/`go vet` clean; `quick-verify` ok; `diff --check`
clean; secrets scan clean.

## Limits (not claimed)

- No Postgres, no migrations, no HTTP, no mail delivery — T02B/T02C.
- Memory store is test/local-only; production path is Postgres (T02B).
- Suspended/deleted enforcement is P13-T04; the status field persists now.

## Rollback

Pure addition: deleting `account/application/` service files (domain ports
stay for T02B) restores prior behavior. No migration, no flag, no data.

## Next

P13-T02B Postgres store + migration + real-DB integration (#23, same
branch). Then T02C transport. Then P13-T03 provider verification (#24).
