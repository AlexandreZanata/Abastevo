# SEC09-A — Current-head review of all corrections

Date: 2026-10-11. Status: LOCAL_DONE. Scope: review + targeted
hardening at head `29f2725` (no product change beyond one
test-harness race fix). Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC09). Contract: all SEC00–SEC08 evidence docs; post-audit
additions (CS02–CS07A helpfulness/moderation/feed/bench, Android
consent/helpfulness/outbox/auth/location layers). Constraint: review
is evidence-backed (suites re-run at this head); prior evidence does
not cover later commits — this doc is the current-head record.

Validation: PASS (with two recorded environment owes)

## Review coverage (60 commits since audit snapshot `954a8d8`)

- Re-read the correction chain SEC01-A→SEC08-D (budgets, resolver,
  reservations, erasure, expiry, outbox, refresh, JWKS, location) +
  post-audit CS02–CS07A and Android CS08/CS11 diffs at
  this head: verdict vocabularies, error mappings, and retention
  bounds consistent; no second refresh/JWKS/location semantic drift.
- Found and fixed one real race in review: feedback `testCommentService`
  `IDGen` counter raced by 140 parallel admission goroutines
  (harness-only; production store is mutex-guarded). Fix: mutex in the
  helper closure. `-race` now clean.
- Crash-after-commit paths re-green at this head: helpfulness
  crash-replay, erasure-ledger replay, outbox lost-ACK/durable-receipt,
  idempotency-keyed replays (account/feedback/community suites).

## Validation (actual, head `29f2725`)

- `go test ./...` (unit): 71 packages ok, zero failures.
- Real-PostGIS `-race -tags=integration`: account, community,
  privacy, feedback, identity trees green — incl. new SEC07-A/SEC09-A
  proofs and the real-HTTP suites (`adapters/http` packages).
- New `TestPoolSaturationKeepsPrivacyAuthResponsive` (MaxConns=2
  pool): family/history/privacy reads + other-account admission
  complete behind a stuck per-account advisory-lock holder; same-
  account admission waits bounded (5 s ctx) then succeeds on release.
  PASS (5.4 s by design).
- `go build ./...`, `go vet`, `gofmt -l` (clean), `sqlc vet`,
  `git diff --check` clean; secret scan passed.

## Owes (environment, not code)

- `TestLocalCaddyForwardsObservedClient` (identity edge): FAILS here
  with 502 — port 18081 is squatted by another host process (verified
  listening + 502 independent of the test). Reproduced identically at
  pre-session commit `1b67367` in an isolated worktree (since
  removed): pre-existing environment conflict, not a regression.
  SEC02-C's original edge proof stands for its environment; re-run
  needs a free 18081.
- SEC08-D phone matrix + SEC05-E/CS10C soaks stay time/device-owed
  (tracked separately); this review claims nothing for them.

## Limits

- Saturation proof covers pool/advisory-lock granularity on auth +
  privacy reads; a full load-harness knee measurement is out of scope
  (SEC09-D bounds staging instead).
- No production claim.

Next microtask: SEC09-B dependency and container scans.
