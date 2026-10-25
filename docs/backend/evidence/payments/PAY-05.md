# PAY-05 — provider configuration and channel switches

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-02, PAY-04
Branch: `codex/payments-stripe-asaas`
Parent: `b931a1df` (PAY-04).

Objective: bind provider environment/merchant/catalog/key/return-origin settings,
expose independent new-sales and maintenance switches, and provide a redacted
read-only preflight. Missing or mixed configuration must disable only the unsafe
sale channel, never the whole process or unrelated channels. No provider call,
resource, credential or live sale is created.

## Configuration

`backend/internal/platform/config/payments.go` adds `PaymentsConfig` to the
validated process `Config`:

- Global: `ANPFUEL_PAYMENTS_CATALOG_VERSION` (default `pay-catalog-v1`) and
  `ANPFUEL_PAYMENTS_RETURN_ORIGIN` (HTTPS origin; loopback HTTP only in
  development).
- Stripe: `ANPFUEL_STRIPE_MODE` (`test`/`live`), secret key, webhook secret,
  merchant ID, Price ID, `ANPFUEL_STRIPE_CARD_NEW_SALES_ENABLED`, and
  `ANPFUEL_STRIPE_MAINTENANCE_ENABLED`.
- Asaas: `ANPFUEL_ASAAS_MODE` (`test`/`production`), API key, webhook token,
  merchant ID, independent conventional-Pix and Pix-Automático new-sales flags,
  and `ANPFUEL_ASAAS_MAINTENANCE_ENABLED`.
- New sales default **OFF**; maintenance defaults **ON**. Enabling a sale requires
  the provider credentials, matching platform/provider mode, merchant mapping,
  webhook verification material, catalog version, and (for card) return origin.
- Maintenance readiness is independent of new-sales flags and of price/catalog/
  return-origin values. Disabling new sales cannot erase accepted access, webhook
  intake, cancellation or reconciliation.
- Stripe secret accepts restricted (`rk_`) or secret (`sk_`) test/live keys by
  constructing prefixes at runtime; no literal live key marker is committed.
  Stripe merchant/Price/webhook prefixes and Asaas token rules are validated
  without echoing values. Asaas webhook token must not equal the API key.
- Mixed modes (e.g. live Stripe in staging, test Asaas in production) fail closed
  for that provider only. Malformed optional values are recorded as redacted
  issues and fall back to safe flags rather than crashing unrelated configuration.
- `PaymentsConfig.Preflight()` returns a read-only, JSON-safe readiness view:
  per-channel requested/ready/maintenance state, mode, configured booleans,
  reason and variable-name issues. No secret, merchant ID or token is included.
- `logValue()` logs only modes and booleans; `Config.LogValue()` remains redacted.
- Key rotation is an environment replacement + restart path. Tests load rotated
  keys and prove readiness remains valid while preflight/log output is unchanged
  and does not contain old or new secret material.
- `infra/env.staging.example` now documents test-mode placeholders with every
  sale switch false and maintenance true; no real value is committed. Production
  uses the same names with `live`/`production` and is not authorized here.

Startup performs no provider request and creates no customer, price, checkout,
webhook, QR, mandate or subscription.

## Validation performed

```sh
go test -race -count=1 ./internal/platform/config
go test -count=1 ./...
go vet ./internal/platform/config
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/platform/config
./scripts/check-security.sh --static-only
./scripts/check-infra-config.sh staging
bash scripts/scan-secrets.sh
gofmt -l backend/internal/platform/config
git diff --check
```

Results:

- Config/race tests PASS:
  - default sales off; independent maintenance default on;
  - full test-mode staging config ready for all three channels;
  - sales off keeps maintenance ready;
  - staging live Stripe refuses only Stripe while test Asaas stays ready;
    production test Asaas refuses only Asaas while live Stripe stays ready;
  - missing/malformed secret or boolean fails closed without crashing;
  - missing return origin disables card only; invalid/non-HTTPS origins refused
    outside development;
  - missing catalog disables sales but not maintenance;
  - Asaas webhook token equal to API key refused;
  - disabling maintenance disables sales for that provider only;
  - preflight JSON and `Config.LogValue()` contain no secrets;
  - key rotation reloads new material, stays ready and leaves preflight identical.
- Full backend unit suite PASS (`go test ./...`).
- `go vet`, staticcheck, security static review, staging infra configuration and
  secret scan PASS.
- No provider account, network call, runtime route, deployment or sale was
  created. The preflight is a redacted Go configuration surface; authenticated
  operator/API exposure belongs to later runtime work.

## Remaining obligations

- Real merchant/account IDs, keys, webhook endpoints, Price ID, return origin and
  sandbox proof remain account-dependent and are intentionally absent.
- PAY-06/07/08/09/10/11/12/13/14/16/19 implement adapters, webhook intake,
  projection, lifecycle, reconciliation and runtime wiring.
- PAY-21 must prove actual provider journeys; PAY-23 owns scoped VPS test
  acceptance. All sale switches remain false until those gates.
- Production template/live activation is not authorized by this phase.

Next action: PAY-06 only after maintainer authorization.
