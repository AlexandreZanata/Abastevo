# PAY-18 — adversarial privacy and performance acceptance

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-25
Depends on: PAY-15, PAY-16, PAY-17
Branch: `codex/payments-stripe-asaas`
Parent: `24a0c0dc` (PAY-17).

Objective: close the locally provable adversarial matrix rows for privacy
and bounded performance with synthetic fixtures only: no webhook refusal or
durable intake echoes card data/secrets/QR/payment URLs/tokens; rotated
operator proof fails closed; restore and stale replays create no financial
effect; provider outage stays pending without duplicating writes; callback
intake meets frozen ceilings and hot queries use indexes.

## B-BR/BUC addressed before implementation

- B-BR-PAY-11/12: webhook intake authenticates, bounds and durably records
  before acknowledgement; storage failure never returns fake success.
- B-BR-PAY-16: restricted refunds require fresh operator proof with
  reason/audit; stale proof cannot request, replay or reconcile.
- B-BR-PAY-19/20: failed/uncertain work preserves paid-through state;
  retention keeps minimal hashes/IDs, never raw payment material.
- B-BR-PAY-21: bounded admission, flood convergence, retry budgets and
  concurrency controls are pinned by test; ceilings are frozen in code.
- B-BR-PAY-24: local tests, sandbox proof and deployment remain distinct
  gates; this phase claims local acceptance only.
- BUC-PAY-06/08/10: event intake, refund/dispute fences and operate/recover
  behavior are exercised end to end on disposable PostGIS.

## Implementation

Tests only; no production behavior change was required. Two new files:

- `backend/internal/modules/payments/adapters/http/pay18_privacy_test.go`:
  Stripe and Asaas webhooks receive card/secret-laden adversarial bodies on
  every refusal path (forged/wrong credential, malformed JSON, oversized
  body, DB outage) plus one valid tainted intake. Every response body is
  asserted free of PAN/CVV/secret/QR-URL/token markers with `no-store`
  caching; the durable inbox event is asserted hash-only (sha256 of the raw
  body, bounded IDs, no raw material retained).
- `backend/internal/modules/payments/application/pay18_acceptance_integration_test.go`
  (real disposable PostGIS, `-race`):
  - `TestPAY18RotatedOperatorProofRefusesStaleRefund` (AUTH-05): a rotating
    proof accepts the first refund, then rotation makes the old proof fail
    closed on fresh requests, completed-intent replays and reconciliation,
    with exactly one provider write and one audit row.
  - `TestPAY18RestoreReplayCreatesNoFinancialEffect` (REC-05): after a
    partial refund, re-driving the original settlement event is a durable
    no-op (one ACTIVE grant, access preserved), replaying the completed
    refund intent converges without a second provider write, and a stale
    state transition is refused as a conflict with the terminal record
    unchanged.
  - `TestPAY18ProviderOutageStaysPendingWithoutDuplicating` (REC-02): an
    ambiguous provider outage leaves the refund PENDING with access
    preserved; recovery reconciles the one durable intent exactly once
    (one provider write, access then revoked).
  - `TestPAY18BoundedCallbackIntakeAndHotQueryPlans`: frozen ceilings
    (200 inserts total < 120s, mean < 1s, p95 < 2s; 64-way duplicate flood
    < 60s with exactly one durable effect and zero grants) plus EXPLAIN
    pins that inbox-PK, grants-owner, orders-owner and pending-inbox
    queries all use indexes with no sequential scan.

RED observed and resolved inside the phase: replaying a completed *full*
refund fails closed on amount bounds before the replay path, so the restore
test uses the valid partial-refund replay path; a stale refund transition
is refused as `ErrConflict` (row guard), not `ErrStale`, and the test pins
that exact refusal with the terminal record unchanged.

## Validation performed

Executed from `backend/` unless noted, using a disposable local PostGIS
(`ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable`;
each suite creates and drops its own databases):

```sh
go test -count=1 -run 'TestPAY18' ./internal/modules/payments/adapters/http/
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 -run 'TestPAY18' ./internal/modules/payments/application/
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/... ./internal/modules/subscription/...
go test -count=1 ./...
go vet ./internal/modules/payments/... ./internal/modules/subscription/... ./internal/platform/jobs/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/... ./internal/modules/subscription/... ./internal/platform/jobs/...
govulncheck ./internal/modules/payments/... ./internal/modules/subscription/...
sqlc vet && sqlc diff
go mod verify
./scripts/check-security.sh --static-only
bash scripts/scan-secrets.sh
gofmt -l <new files>
git diff --check
```

Results:

- New privacy tests PASS (2/2, 9 subcases): all refusal bodies clean,
  `no-store` on every response, intake hash-only.
- New integration tests PASS (4/4, `-race`): rotation refusal, restore
  no-op convergence, outage pending-then-once recovery, bounded intake.
- Measured intake on disposable PostGIS: n=200 total=174.76ms
  mean=873.80µs p95=1.12ms — far inside the frozen ceilings above.
- Full race payments/subscription suites PASS; full backend unit suite
  PASS on re-run (112 packages ok). First full run hit one unrelated
  pre-existing flake, `TestWrongKeyAndAccountMismatch` (account
  keyprover, untouched by this phase): its tamper case replaces the
  trailing base64 pair with `AA`, which is a no-op when the signature
  already ends there, so a valid proof passes and the test fails. It
  passes in isolation and on re-run; no payments behavior is involved.
- vet, staticcheck, govulncheck (no vulnerabilities), sqlc vet/diff,
  module verification, security static review, secret scan, gofmt and
  diff checks PASS.
- No provider account, charge, mandate, refund, key or deployment was
  used. All sale channels remain disabled.

## Remaining obligations

- PAY-19 wires actual API/worker composition and regressions with
  MAP/ESP/PRM (BND-04/BND-05/BND-07 stay open until then).
- PAY-21 proves actual provider sandbox journeys (remaining OPEN rows
  with `sandbox` proof: AUTH/WHK/STL/LIF/REC settlement, lifecycle and
  recovery timing, fee balances, MED names, retry windows).
- The keyprover tamper-case flake above is owned outside payments; it
  was observed, not introduced, here.
- Actual approved enterprise price/term/deliverable, fiscal workflow and
  deployment remain unresolved and unbuilt by design.

Next action: PAY-19 only after maintainer authorization.
