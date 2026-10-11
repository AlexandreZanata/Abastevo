# SEC01-C — Atomic admission + budgets

Date: 2026-10-11. Status: LOCAL_DONE. Scope: per-address + aggregate
mail, aggregate key-usernames, per-account sessions, per-account
comments, invalid-first validation, retention purges — no SMTP,
no SEC01-D matrix.
Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC01, AUD-01). Contract: B-BR-A01/A02/A04/A05 (free registration,
code policy, sessions, free social) + B-BR-F03/F04 (280-scalar
comments/replies); BUC-A01/A04 + BUC-F social writes. Constraint:
free registration/social stay free inside ceilings; no oracle leaks
status; hashed accounting only.

Validation: PASS

## Changes

- `account/domain/policy.go`: frozen `MaxAggregateCodesPerHour=20`,
  `MaxKeyAccountsPerHour=100`, `MaxLiveSessionsPerAccount=10`,
  `CodePurgeGraceSeconds=24h`, `SessionPurgeGraceSeconds=7d`.
- `account/domain/account.go`: `ValidEmail` (cheap shape: <=254,
  one @, no spaces); `account/domain/ports.go`:
  `ErrAggregateExhausted`, `ErrSessionBudgetExhausted` + verdict
  codes (`admission-exhausted`, `session-budget-exhausted`).
- `account/application`: `Store` gains `CountAllCodesSince`,
  `CountKeyAccountsSince`, `CountLiveFamiliesByAccount`,
  `PurgeExpiredCodes/Families`, `TryIssueCode`,
  `TryCreateKeyAccount`, `TryCreateFamily` (single lock/tx).
  `RequestCode` validates email before any IO then admits via
  `TryIssueCode` (suspended/per-addr/aggregate/cooldown atomically,
  refusals nil with no rows/mail); `CreateKeyAccount` admits via
  `TryCreateKeyAccount` (aggregate → ErrAggregateExhausted);
  sessions admit via `TryCreateFamily` (live cap →
  ErrSessionBudgetExhausted).
- `account/adapters/pgstore.go` + `db/queries/account/accounts.sql`
  (sqlc regenerated, vet clean): counts, purges, `Try*` with
  `pg_advisory_xact_lock` (global 810001/810002, per-account
  `families:` hash) + single-tx check+insert.
- `account/adapters/http/http.go`: `validEmail` delegates to
  domain (single predicate); new 429 mappings for both budget
  errors.
- `feedback/domain/feedback.go`: frozen
  `MaxLiveCommentsPerAccount=100` (edits converge, deletes free).
- `feedback/application`: `CommentStore` gains
  `CountLiveCommentsByAccount` + `TryInsertComment` (single
  lock/tx, over-budget → ErrQuotaExceeded); `submit` validates
  target/text before authorize/station/parent IO.
- `feedback/adapters/pgstore.go` + `db/queries/feedback/comments.sql`
  (sqlc regenerated): live count + `TryInsertComment` under
  per-account advisory lock; `feedback/adapters/http/http.go` maps
  plain `quota-exceeded` to 429.
- Tests: 6 SEC01-A skips inverted to enforced GREEN (session test
  rewritten to cap+second-account); new unit accounting/purge +
  parallel-ceiling tests (account + feedback); new real-PostGIS
  `TestSEC01C*` (aggregate parallel exact, session cap +
  second-account, comment budget + parallel exact, purge exact).

## Validation (actual)

- `go test ./internal/modules/account/... ./internal/modules/feedback/...`
  green (incl. 6/6 regressions + 5 new unit).
- `-race -tags=integration` green: `account/adapters`
  (incl. 2 new SEC01-C), `feedback/adapters` (incl. 2 new SEC01-C)
  on disposable local PostGIS (shared dev DB host, per-test DBs).
- `go build ./...`, `go vet` (account/feedback), `gofmt -l`
  clean, `sqlc vet` clean, `git diff --check` clean,
  `scan-secrets.sh` passed (synthetic addresses only).
- RED pre-GREEN: SEC01-A evidence proved all 6 FAIL unskipped;
  session placeholder could never pass as written (always Fatal);
  this task rewrote it to cap+legit-second and turned all 6 GREEN
  with parallel/purge proofs.

## Limits / owed

- Full SEC01-D matrix still owed: quota rollback, duplicate
  intent/retry, exhausted-global retry metadata, rotating
  subjects, multi-window expiry, HTTP 429 Retry-After wiring for
  account budgets (feedback 429 mapped; account 429 coded, header
  owed), real-SQL concurrency beyond the bounded parallel proofs
  here.
- Multi-replica strict serializability beyond advisory xact locks
  (e.g. partitioned PG, failover) not proven here.
- SMTP needs explicit need/license/security record — not added.
- No VPS/phone/shared-edge change; no production readiness claim.

Next microtask: SEC01-D real-SQL flood/rollback/retry proofs.
