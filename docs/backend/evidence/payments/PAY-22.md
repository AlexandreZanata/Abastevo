# PAY-22 — batch integration to protected main

Status: INTEGRATION_PENDING (PR open, CI running on corrected head)
Validation: PASS (local candidate gates)
Date: 2026-10-25
Branch: `codex/payments-stripe-asaas` integrated into `dev`
PR: `AlexandreZanata/Abastevo#150` (dev → main, ready)
Heads: dev `6e5e8518` reviewed; fixes `dbff7917`, `c9ea4aa4`, `8313283b`,
`d149e092` merged after; local `dev` branch (occupied elsewhere, stale
at `5c9f5204`) untouched — all integration work runs in the isolated
`pay22-dev` worktree and a separate `pay22-full` clone owns the helper.

Objective: coordinate latest dev/main, merge owned payment changes
normally, and deliver through one cumulative protected dev → main PR
with a guarded finish. No force, no rebase of published history, no
admin bypass, no direct-main development.

## B-BR/BUC addressed before implementation

- B-BR-PAY-24: local proof and remote activation stay distinct; this
  record claims integration progress, not a merge.
- BUC-PAY-10: the candidate preserves operate/recover behavior across
  the dev union (maps, Android Premium, payments).

## Concurrent Pix failure: cause and correction

Repro on the reviewed head (`backend/`,
`go test -race -count=50 -run '^TestPixConcurrentStartsCreateOnePayment$'`):
23 failures in 50 with `payments: conflict`.

Cause: the `memoryStore.CreateOrder` double accepted a repeated order ID
when only account/offer/channel/amount matched and returned success,
while the durable `Store.CreateOrder` re-reads the row and requires the
identical booking **including `QuotedAt`/`ExpiresAt` timestamps**. Two
racers computing different `now` values (nil clock) therefore diverged:
the loser kept its locally built order with a different `QuotedAt`, the
`pixPayload` (which hashes `QuotedAt`) differed, and `ClaimAttempt`
refused the second claim as a conflict instead of reporting busy. The
card payload carries no timestamps, which is why only Pix flaked. PAY-20
recorded this as a timing flake; the record stands and this is the
corrected diagnosis.

Correction (`dbff7917`, double only — production `Start` already
refetches the winner on conflict): the double mirrors durable
semantics, conflicting on any divergent field including timestamps and
capabilities. Added deterministic regression (double contract test),
barrier-plus-controlled-clock convergence tests for Pix and card (a
rendezvous on the build-then-store boundary forces the dispute; a
millisecond-step clock keeps racers distinct above the payload's
microsecond truncation), and a PostGIS convergence proof (one order,
one operation, one provider write, busy losers, never conflict).
Original repro re-run: 50/50 green. Tamper, replay and lost-response
tests unchanged and green.

## PAY-21 harness corrections (no external acceptance claimed)

- Stripe 401/403 now maps to unauthorized (`c9ea4aa4`), so the harness
  taxonomy (expected absence / authentication / unavailability / other)
  is real instead of declarative.
- Harness reworked (`8313283b`): no probe subscriptions or customers —
  checkout/portal journeys create their own account, customer, order and
  session; cancel runs against an operator-supplied test subscription
  (new `ANPFUEL_SANDBOX_STRIPE_SUBSCRIPTION`, else a precise skip);
  per-run UUIDs separate executions while operation keys stay fixed
  within an operation plus recovery; live responses assert
  account/customer/order/price/environment binding; call-count log
  lines removed.
- Credential gating extracted to testable pure code with unit tests;
  live refusal and absence skips re-verified; scanner-safe (no live-key
  literals). PAY-21 stays PENDING/PROOF_OWED: no credentials supplied,
  zero provider calls made.

## Worker slot fallout and pre-existing set

The required `payment-inbox` slot broke four partial dispatcher maps in
quality worker tests; fixed with one-line additions each (`d149e092`).
Remaining `cmd/worker` integration failures are byte-identical on
pristine dev `721582b8` (verified side by side): Rust preparer fixture
rejections and a private-source requirement owned by other fronts — no
payments behavior involved, not fixed here, not hidden.

## Batch description corrections

- Android contributes behavior and UI (cached Premium skeleton,
  container standardization across screens) plus unit tests — not
  test-only. Premium suites re-run on the candidate: 20 tests, 0
  failures, module compiled by the test task.
- MAP content is inherited as-is with its stated limits (native
  manifests/digests, OSM/ANP preparation, charging identities, comaps
  spike/diagnostics, partial gates). No later MAP/ESP branch is
  imported to complete this batch.
- Inherited batch note (not fixed here): the expansion entities ship
  Room `@Database` version 25 while versioned schema exports stop at
  18 — exports 19–25 were never committed. Compiling `:app` regenerates
  `25.json` (five added tables) as untracked build output; it is left
  out of this integration for the owning front to reconcile with its
  migration chain.
- Worker regressions affected by this batch (dispatcher topology,
  payment-inbox drain, safety-slot isolation) run green; the full
  worker integration set was executed with pinned Rust preparers and
  disposable PostGIS, leaving only the pre-existing set above.
- `make quick-verify` is the CI gate, not a substitute for the
  per-area proofs listed here.

## Implementation

- Verified `origin/dev` (`721582b8`) is fully contained in the payments
  branch: fast-forward merge, no conflicts, no foreign code. Pushed
  `721582b8..6e5e8518` fast-forward only (plus fix merges to follow).
- Local CI findings fixed in new commits on the front (secret scanner
  flagged a live-key literal in the sandbox guard and a live-looking
  value in PAY-21 evidence; both redacted without weakening refusal).
- Opened PR #150 (ready, not draft) with real identifiers, included
  phases, validation results and explicitly open obligations. No
  `Closes`: no eligible issue meets completion criteria; PAY-21 stays
  pending by contract.

## Validation performed

Candidate gates run on the exact integration head in
`/tmp/opencode/pay22-dev` (`backend/` unless noted; disposable local
PostGIS `ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable`):

```sh
make quick-verify
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/... ./internal/modules/subscription/...
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./cmd/api/
go test -race -count=50 -run '^TestPixConcurrentStartsCreateOnePayment$' ./internal/modules/payments/application/
go test -race -count=1 -run 'ConvergeWithDistinctClocks|TestMemoryStoreCreateOrderRequiresIdenticalRow' ./internal/modules/payments/application/
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 -run 'TestPixConcurrentStartsConvergeRealStore' ./internal/modules/payments/application/
go test -tags=sandbox -count=1 -run 'TestSandbox' ./internal/modules/payments/adapters/
export ANPFUEL_TEST_*_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/* \
  ANPFUEL_TEST_DATABASE_URL=<disposable> && go test -race -tags=integration -count=1 ./cmd/worker/
./gradlew :app:testDebugUnitTest --tests "com.anpfuel.app.ui.premium.*"
go test -count=1 ./...
go vet <affected trees> && staticcheck <affected trees> && govulncheck <affected trees>
sqlc vet && sqlc diff && go mod verify
./scripts/check-security.sh --static-only && bash scripts/scan-secrets.sh
gofmt -l <changed trees> && git diff --check
```

Results:

- `make quick-verify`: ok on the reviewed head; re-run reserved for
  the helper after remote checks approve the corrected head.
- Race payments/subscription and `cmd/api` suites PASS on the reviewed
  head; affected suites re-run on the corrected head (see evidence
  commits).
- Pix repro 50/50 green after the fix; rendezvous convergence suites
  fail deterministically on the old double and pass on the new one;
  PostGIS convergence proof PASS.
- Sandbox harness: all journeys skip without credentials (exit 0);
  live-looking key refused before any network use; gating unit and
  no-leak guard PASS.
- Worker integration with pinned preparers: topology-affected tests
  PASS; remainder byte-identical to pristine dev (pre-existing).
- Android Premium suites on the candidate: 20 tests, 0 failures.
- Full backend unit suite PASS; vet/staticcheck/govulncheck/sqlc/mod
  verify/security/secret-scan/gofmt/diff-check clean.

## Remaining obligations

- Push the corrected head, update PR #150 (scope, corrected description,
  new validation), await required Quick verification plus every other
  enabled signal; fix failures in new commits; changed head/base
  invalidates this proof.
- Guarded finish (`git-flow.sh finish --base origin/main --pr 150
  --required "Quick verification"`) from the separate dev clone only on
  green CI; verify ancestry; retain dev; eligible issue closure through
  the merged PR; wiki mirror per merged batch (failure stays
  WIKI_PENDING).
- PAY-21 sandbox proof (credentials), PAY-23 VPS deployment, operator
  proofs matrix — separately gated, none claimed here. No PAY-23, VPS
  or commercial activation by inference.

Next action: push corrections, refresh PR #150, complete the guarded
merge on green CI.
