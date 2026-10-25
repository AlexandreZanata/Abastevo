# PAY-19 — verified payment runtime composition

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-25
Depends on: PAY-05, PAY-10, PAY-11, PAY-13, PAY-14, PAY-15, PAY-16, PAY-17, PAY-18
Branch: `codex/payments-stripe-asaas`
Parent: `3d529a8c` (PAY-18).

Objective: compose the real API and worker payment runtime from validated
configuration with no new sellable surface: config-gated provider clients,
webhook intake, single-owner inbox drain, settlement/lifecycle/dispute/cycle
processing, billing exits, refunds and switches — every unconfigured or
unapproved path failing closed, sales impossible, Play untouched.

## B-BR/BUC addressed before implementation

- B-BR-PAY-01/02/05: server catalog (empty production lineage), credential
  gating per channel, redacted preflight; no credential presence alone
  enables anything.
- B-BR-PAY-09/11/12: durable inbox before acknowledgement; exactly one
  service owns each event; unknown events retained minimally with no effect.
- B-BR-PAY-13/22: bounded wake-up drain per channel, no long DB locks around
  provider calls, financial work on its own serial worker slot isolated from
  safety/map workloads.
- B-BR-PAY-16/19: refunds need fresh operator proof with dual approval by
  default; billing retention survives exits; restores cannot replay.
- B-BR-PAY-24: local runtime, sandbox proof and deployment stay distinct
  gates; fixtures never bypass grants.
- BUC-PAY-06/07/08/10: event intake, cancel/export/delete, refund/dispute
  and operate/recover routes demonstrated on the composed stack.

## Implementation

- `backend/internal/modules/payments/adapters/jobs/router.go` (new): inbox
  `Router` draining bounded pending rows per configured channel to exactly
  one service (settlement owns paid events, lifecycle subscription/checkout/
  failure events, dispute chargebacks, cycles Pix Automático mandates;
  unknown retained minimally). Unconfigured owners and unavailable/ambiguous
  dependencies preserve PENDING; conflicts stay marked. `Handler`
  (`payment-inbox`, v1) validates the strict wake-up envelope.
- `backend/internal/modules/payments/adapters/verify_mux.go` (new):
  `SettlementMux` fans verification to the configured provider only.
- `backend/cmd/api/payments.go` (new): `composePayments` builds the whole
  API runtime from `config.PaymentsConfig` — providers only when
  maintenance-ready, empty `pay-catalog-v1` catalog (production prices
  unresolved per PAY-01), sales flags through, nil proofs, empty switch
  merchant and portal without configuration ID so those commands fail
  closed. `registerPaymentRoutes` mounts webhook intake only; startup logs
  the redacted preflight.
- `backend/cmd/worker/payments.go` (new): `composePaymentInbox` builds the
  same store/verifier stack plus router/handler for the worker.
- `cmd/api/main.go`: uses the composed billing exit (account-deletion hook
  unchanged), mounts webhooks, logs redacted preflight.
- `cmd/worker/main.go` + `dispatchers.go`: `payment-inbox` registered on
  its own fixed serial slot, excluded from the general dispatcher — safety
  and map loops keep their isolation.
- Typed-nil discipline: providers are assigned into service interfaces only
  when configured, so nil guards fail closed instead of panicking.

Deliberate residuals: owner/operator proofs (no account step-up surface),
per-target switch merchant routing, portal configuration ID, and the
maintenance kill-switch have no production implementation yet — those
commands stay unavailable and land with deployment packaging (PAY-20).
Provider verification against live APIs stays PAY-21 sandbox proof.

## Validation performed

Executed from `backend/` unless noted, on disposable local PostGIS
(`ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable`;
each suite creates and drops its own databases):

```sh
go test -count=1 ./internal/modules/payments/adapters/jobs/ ./internal/modules/payments/adapters/
go test -count=1 -run 'TestPAY19' ./cmd/api/ ./cmd/worker/
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/adapters/jobs/
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 -run 'TestPAY19' ./cmd/api/
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/... ./internal/modules/subscription/...
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./cmd/api/
go test -count=1 ./...
go vet ./internal/modules/payments/... ./internal/modules/subscription/... ./internal/platform/jobs/... ./cmd/api/ ./cmd/worker/
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/... ./internal/modules/subscription/... ./internal/platform/jobs/... ./cmd/api/ ./cmd/worker/
govulncheck ./internal/modules/payments/... ./internal/modules/subscription/... ./cmd/api/ ./cmd/worker/
sqlc vet && sqlc diff
go mod verify
./scripts/check-security.sh --static-only
bash scripts/scan-secrets.sh
gofmt -l <changed trees>
git diff --check
```

Results:

- Router unit suites PASS: every known Stripe/Asaas type reaches its
  single owner, unknown events retained with no processor call, unmarked
  ignores converge, outage preserves PENDING, envelopes strictly validated.
- Router PostGIS suites PASS (`-race`): real inbox rows reconciled,
  restart converges with no second effect, outage then recovery processes
  once, conflicts retained without re-entering the drain.
- API composition suites PASS: unconfigured runtime fails closed on every
  command, webhook routes mounted with no other payment surface, no Play/
  cookie/redirect reference in the runtime path.
- API journey suites PASS (`-race`): authenticated Asaas callback → PENDING
  → composed drain → PROCESSED with zero grants; restart no-op; Stripe
  without configuration refused; outage preserves PENDING with zero grants;
  composed settlement → ACTIVE grant → operator refund → REVOKED → owner
  cancel DONE with owner-scoped history; flags without credentials enable
  nothing while free reads keep working.
- Worker suites PASS: isolated slot topology (4 safety + payment-inbox,
  general excludes all), empty-config wake-up is a no-op.
- Full race payments/subscription suites PASS; full `cmd/api` integration
  suite PASS; full backend unit suite PASS (114 packages, zero failures).
  `cmd/worker` quality-native integration cases require the pinned Rust
  preparer binaries (`ANPFUEL_TEST_QUALITY_PREPARER_PATH`, built per
  backend README) and were not executed here; worker unit/PAY-19 suites
  PASS. No payments behavior is involved in that environmental gate.
- vet, staticcheck, govulncheck (no vulnerabilities), sqlc vet/diff,
  module verification, security static review, secret scan, gofmt and diff
  checks PASS.
- No provider account, charge, mandate, refund, key or deployment was
  used. All sale channels remain disabled; production catalog is empty.

## Remaining obligations

- PAY-20 packages deployment/configuration: owner/operator proofs, switch
  merchant routing, portal configuration ID and maintenance kill-switch.
- PAY-21 proves actual provider sandbox journeys (remaining `sandbox`
  matrix rows, fee balances, MED names, retry windows, live revocation).
- PAY-22/23 own final protected integration and scoped VPS test deployment.

Next action: PAY-20 only after maintainer authorization.
