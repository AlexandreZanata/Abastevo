# PAY-20 — secure payment operations packaging

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-25
Depends on: PAY-19
Branch: `codex/payments-stripe-asaas`
Parent: `4ef85fe3` (PAY-19).

Objective: close the PAY-19 deployment residuals with a complete
configuration contract and safe defaults — owner proofs from live account
sessions, per-source switch merchants, portal configuration admission and
a maintenance kill-switch — plus the staging example and the operations
runbook. No migration, no new route, no live activation.

## B-BR/BUC addressed before implementation

- B-BR-PAY-01/02/05: portal configuration joins the validated credential
  set (optional, `bpc_*`); secrets alone still enable nothing.
- B-BR-PAY-11/12/15: switch records bind the old source's merchant so the
  card fence cannot confirm foreign renewals; owner cancel/export verify
  one live session per action.
- B-BR-PAY-21: bounded admission and kill-switch semantics are pinned by
  test; thresholds and budgets unchanged.
- B-BR-PAY-24: local packaging, sandbox proof and deployment stay
  distinct; the runbook names every remaining external gate instead of
  claiming readiness.
- BUC-PAY-05/07/10: renewal/change fences, owner cancel/export and
  operate/recover procedures are wired and documented.

## Implementation

- `backend/internal/platform/config/payments.go`: new
  `ANPFUEL_STRIPE_PORTAL_CONFIGURATION_ID` (optional, `bpc_*`
  validated), `PortalConfigured` preflight signal; sales and maintenance
  switches untouched.
- `backend/internal/modules/payments/application/switch_ports.go` +
  `switch.go`: `SwitchConfig` carries `CardMerchantID` and
  `PixAutoMerchantID`; admission requires the new channel's merchant and
  readiness, and the record stamps the old source's merchant (only the
  card fence consumes it; mandate fences use their own configuration).
- `backend/cmd/api/payments.go`: `accountOwnerProof` verifies one
  action-time `familyID:accessToken` session per call — nothing cached,
  transport outages retryable, every explicit denial unauthorized, proof
  never logged or stored. `composePayments` takes the validator (nil
  keeps owner commands unavailable), blanks webhook credentials while a
  channel's maintenance is off, and wires the portal configuration ID.
- `backend/cmd/api/main.go`: attaches the account-service validator to
  the composed billing proof; deletion hook unchanged.
- `infra/env.staging.example`: portal key documented (empty by default).
  Compose/Caddy need no change: webhooks already ride `/v1/*`, env-driven.
- `docs/operator/payments.md` (new): setup beyond keys, read-only
  preflight procedure, rotation with overlap, channel-disable, queue
  recovery, backup/restore/rollback, bounded tests, monitoring signals,
  notification and fiscal boundaries.

Deliberate residuals: operator proofs have no identity surface (refunds,
holds and replays stay unavailable in production until the approval
matrix lands); provider verification, fee balances and MED semantics
stay PAY-21 sandbox proof; catalog admission stays a commercial gate, so
checkout still refuses every offer.

## Validation performed

Executed from `backend/` unless noted, on disposable local PostGIS
(`ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable`;
each suite creates and drops its own databases):

```sh
go test -count=1 ./internal/platform/config/
go test -count=1 -run 'TestPAY20' ./cmd/api/ ./cmd/worker/
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 -run 'TestSourceSwitch|TestPAY20|TestPAY19' ./internal/modules/payments/application/ ./cmd/api/
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/... ./internal/modules/subscription/...
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./cmd/api/
go test -count=1 ./...
go vet ./internal/modules/payments/... ./internal/modules/subscription/... ./internal/platform/... ./cmd/api/ ./cmd/worker/
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/... ./internal/platform/config/ ./cmd/api/ ./cmd/worker/
govulncheck ./internal/modules/payments/... ./internal/platform/config/ ./cmd/api/ ./cmd/worker/
sqlc vet && sqlc diff
go mod verify
./scripts/check-security.sh --static-only
bash scripts/scan-secrets.sh
gofmt -l <changed trees>
git diff --check
```

Results:

- Config suites PASS: portal optional/validated/independent; rotation
  stays redacted; mixed modes refuse per channel.
- Switch suites PASS: old-source merchant stamped per source, foreign
  merchant refused before any provider write, missing per-channel
  merchant fails closed at admission; all pre-existing switch tests
  green unmodified in behavior.
- Composition suites PASS: live-session proofs per action (malformed,
  foreign, revoked and outage cases pinned), maintenance-off darkens the
  channel (503 intake, nil provider, pending preserved, free reads
  intact), dual merchants and portal ID wired, mixed modes give no
  provider, flags alone enable nothing.
- Journey suites PASS (`-race`): kill-switch preserves money and reads;
  proof wiring enables owner cancel with owner-scoped history.
- Full race payments/subscription suites PASS on re-run (one first-run
  failure in the pre-existing Pix 6-racer unit timing test, untouched by
  this phase; green alone and on re-run). Full `cmd/api` integration
  suite PASS; full backend unit suite PASS (114 packages, zero failures).
  Worker quality-native cases need the pinned Rust preparers
  (environmental, unrelated).
- vet, staticcheck, govulncheck (no vulnerabilities), sqlc vet/diff (no
  migration drift), module verification, security static review, secret
  scan, gofmt and diff checks PASS.
- No provider account, charge, mandate, refund, key or deployment was
  used. Startup with payments unconfigured boots identically; no
  app/map/site behavior changed.

## Remaining obligations

- PAY-21 proves actual provider sandbox journeys (remaining `sandbox`
  matrix rows, fee balances, MED names, retry windows, live revocation,
  eligibility and portal sessions).
- PAY-22/23 own final protected integration and scoped VPS test
  deployment. Operator proofs land with the approval matrix there.
- Catalog admission (real prices/offers) is a commercial gate; checkout
  refuses every offer until then by design.

Next action: PAY-21 only after maintainer authorization.
