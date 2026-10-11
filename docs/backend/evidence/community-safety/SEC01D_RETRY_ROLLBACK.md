# SEC01-D — Retry, rollback and stable 429s

Date: 2026-10-11. Status: LOCAL_DONE. Scope: mail-failure rollback,
cooldown/duplicate convergence, exhausted-budget stability, hourly
rotation, legitimate second client, Retry-After + frontend states —
no SMTP, no SEC02+ scope.
Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC01, AUD-01). Contract: B-BR-A01/A02/A04/A05 + B-BR-F03/F04;
BUC-A01/A04 + BUC-F. Constraint: free use stays free inside
ceilings; aggregate mail suppression stays nil (no oracle); no
PG-state leak; hashed accounting only.

Validation: PASS

## Changes

- `db/queries/account/accounts.sql` (+ regenerated sqlc):
  `DeleteCodeByID` compensation delete.
- `account/application`: `Store.DeleteCode`; `RequestCode` rolls
  back the admitted row when `SendCode` fails (zero rows/mail,
  budget intact, recovered retry succeeds).
- `account/adapters` (Mem + PG): `DeleteCode` (unknown IDs
  converge no-op).
- `feedback/application` + `feedback/adapters/pgstore.go`:
  `TryInsertComment` replays an identical live retry (same ID,
  same account) converged without consuming budget; any other ID
  reuse fails closed (`ErrCommentNotFound`, no oracle). PG checks
  existence before insert so replays never abort the tx;
  concurrent same-ID races fail closed.
- `account/adapters/http/http.go`: budget 429s carry stable codes
  plus `Retry-After` (`admission-exhausted` 3600 upper bound as the
  hourly window slides; `session-budget-exhausted` 60 polite hint —
  budget frees on revoke/expiry/user action, hint guarantees no
  slot); refusals stay no-store with no identity echo.
- `feedback/adapters/http/http.go`: report `QuotaDeniedError`
  carries its exact `Retry-After`; comment `quota-exceeded` maps
  to 429 + `Retry-After: 60` (same hint semantics).
- Tests: unit rollback (fail → zero rows, recover → 1/1),
  cooldown duplicate (1 row/1 mail), hourly rotation (exhausted
  nil + zero growth, post-window fresh succeeds),
  duplicate-username stability, duplicate comment-ID replay +
  foreign-rebind refusal; HTTP 429 code/header/no-leak tests;
  real-PostGIS rollback, duplicate/rotation (account) and
  duplicate replay (feedback).

## Validation (actual)

- `go test ./internal/modules/account/... ./internal/modules/feedback/...`
  green (incl. 4 new rollback/rotation + 1 duplicate-ID + 2 HTTP
  metadata tests).
- `-race -tags=integration` green on disposable local PostGIS:
  `account/adapters` (incl. 2 new SEC01-D), `feedback/adapters`
  (incl. 1 new SEC01-D); pre-existing SEC01-C parallel/purge
  suites re-green.
- `go build ./...`, `go vet`, `gofmt -l` clean, `sqlc vet`
  clean, `git diff --check` clean, `scan-secrets.sh` passed
  (synthetic addresses only).
- RED pre-GREEN: new rollback test failed before DeleteCode
  compensation (row retained); PG duplicate replay failed before
  the check-before-insert reorder (aborted-tx unique violation);
  feedback quota 429 lacked headers before this task.

## Limits / owed

- Session/comment `Retry-After: 60` is a polite hint, not a slot
  guarantee (budget frees on revoke/delete/expiry); aggregate
  3600 is an upper bound (slots free sooner as the window
  slides). Documented here and in code; frontend should pair the
  countdown with manage-sessions/delete guidance.
- Client idempotency keys (lost-response double submit with two
  server IDs) remain two rows by design; same-ID replay covers
  store-level retries only.
- Multi-replica strict serializability beyond advisory xact locks
  not proven here. No VPS/phone/shared-edge change; no
  production readiness claim.

Next microtask: SEC02-A proxy-fairness regressions.
