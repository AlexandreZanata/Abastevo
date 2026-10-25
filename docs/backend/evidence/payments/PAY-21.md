# PAY-21 — provider sandbox proof

Status: PENDING (harness ready, proof blocked on credentials)
Validation: PASS (fault fixtures, harness gates and full local suites)
Date: 2026-10-25
Depends on: PAY-20
Branch: `codex/payments-stripe-asaas`
Parent: `45e90604` (PAY-20).

Objective: prove real Stripe test-mode and Asaas sandbox journeys. Per the
phase contract, missing credentials keep this phase pending: no sandbox
credentials were supplied, so no provider API was touched and no sandbox
row is closed. What lands here is the gated acceptance harness plus the
sandbox-relevant fault behavior pinned against local doubles.

## B-BR/BUC addressed before implementation

- B-BR-PAY-21: retryable provider faults stay ambiguous (bounded retry,
  never fake success); declines and binding violations are terminal;
  revoked keys diagnose as unavailable.
- B-BR-PAY-24: local proof, sandbox proof and deployment stay distinct;
  fixtures never count as provider integration.
- BUC-PAY-06/08/10: the harness journeys mirror intake, recovery and
  dispute handling against real test endpoints once credentials exist.

## Implementation

- `backend/internal/modules/payments/adapters/pay21_fault_test.go`
  (default suite): 429/503/deadline stay ambiguous on both providers;
  card decline is terminal-invalid; dispute-won closes; Asaas revoked
  keys stay unavailable; overdue never settles; chargeback reversal
  closes (MED path); cancel-authorization call shape and 404 mapping.
- `backend/internal/modules/payments/adapters/sandbox_test.go`
  (`//go:build sandbox`): real test-mode/sandbox journeys — Stripe
  customer/checkout/recovery and subscription/portal, Asaas
  customer/payment/QR/retrieve — running only with explicit
  `ANPFUEL_SANDBOX_*` credentials, skipping otherwise, refusing live
  keys hard, logging counts only. Environment is hardcoded to test.
- `backend/internal/modules/payments/adapters/sandbox_guard_test.go`
  (default suite): pins the harness build tag, test-only environment,
  documented credential gates and zero identifying log output.

## Validation performed

Executed from `backend/` unless noted, on disposable local PostGIS where
marked (`ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable`):

```sh
go test -count=1 -run 'TestPAY21|TestSandboxHarness' ./internal/modules/payments/adapters/
go test -tags=sandbox -count=1 -run 'TestSandbox' ./internal/modules/payments/adapters/
ANPFUEL_SANDBOX_STRIPE_SECRET='<live-looking test value>' ... go test -tags=sandbox -count=1 -run 'TestSandboxStripeCustomerCheckoutAndRecovery' ./internal/modules/payments/adapters/
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/... ./internal/modules/subscription/...
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./cmd/api/
go test -count=1 ./...
go vet ./internal/modules/payments/... ./internal/platform/config/ ./cmd/api/ ./cmd/worker/
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/... ./cmd/api/ ./cmd/worker/
govulncheck ./internal/modules/payments/...
sqlc vet && sqlc diff
go mod verify
./scripts/check-security.sh --static-only
bash scripts/scan-secrets.sh
gofmt -l <changed trees>
git diff --check
```

Results:

- Fault fixtures PASS (9/9): retry windows, decline terminality,
  dispute-won, revocation, overdue, reversal closure, cancel shape.
- Harness without credentials: all journeys SKIP, exit 0. With a
  live-looking key: hard failure before any network use. Guard PASS.
- Full race payments/subscription suites PASS; full `cmd/api`
  integration suite PASS; full backend unit suite PASS (114 packages,
  zero failures).
- vet, staticcheck, govulncheck (no vulnerabilities), sqlc vet/diff,
  module verification, security static review, secret scan, gofmt and
  diff checks PASS.
- Zero provider calls were made; no secret, merchant, price or resource
  identifier entered the repository, logs or fixtures.

## Unsupported external proofs (need real sandbox)

Only real test accounts can prove: paid-invoice → callback → grant on
each channel; real webhook delivery (edge forwarding); 3DS/action-
required/decline behavior; Asaas Pix payment simulation and MED reversal
timing; fee/net amount reconciliation; eligibility transitions; portal
session branding; subscription renewal timing; live key revocation.
No live-money substitution is accepted for any of these.

## Running the harness (operators)

Supply test credentials in the calling shell only (never commit, never
log), then run:

```sh
go test -tags=sandbox -count=1 -run 'TestSandbox' ./internal/modules/payments/adapters/
```

Record merchant/environment/resource identifiers in the private handoff,
and pass/fail counts in the follow-up evidence. Closing sandbox matrix
rows requires this run, not the fixtures above.

Next action: supply credentials and run the harness; on green, close the
`sandbox` matrix rows and proceed to PAY-22 after authorization.
