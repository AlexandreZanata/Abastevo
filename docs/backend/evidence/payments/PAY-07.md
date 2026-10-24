# PAY-07 — conventional Asaas Pix orders

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-04, PAY-05
Branch: `codex/payments-stripe-asaas`
Parent: `6e52f9bd` (PAY-06).

Objective: create/reuse an owner-bound Asaas customer and conventional Pix charge
with server price, exact BRL amount, expiry/correlation and bounded QR response;
serialize creation intents; recover inconclusive creates by owner/external
reference without generating another charge; expose status/QR only to the owner.
No auto-debit consent, settlement/grant or live sale is added.

## Implementation

- `backend/internal/modules/payments/application/pix_ports.go` and `pix.go` add
  the conventional-Pix use case. Request accepts only operation ID, offer ID and
  first-customer payer name/tax ID/optional email; client amount/currency/
  customer/order/metadata are refused.
- Order ID is deterministic from `(account, operation)`, catalog/price/BRL and
  capabilities come from the server, and existing orders are reused across
  retries without re-quoting.
- Asaas customer mapping is owner/provider/environment/merchant bound. Payer
  name/CPF-CNPJ are used only when no mapping exists and are never stored in the
  payment tables, logs or evidence. Existing mapping wins and needs no payer data.
- Creation claims a durable PAY-04 operation, commits `DISPATCHED` before the
  Asaas call, and persists success/unknown outcome. Stable correlation is the
  order ID (`externalReference`); Asaas has no idempotency header, so recovery
  searches by `externalReference` before any retry. Multiple matches conflict;
  incomplete searches remain `ErrAmbiguous`; only proven absence may create.
- Replay with a persisted remote ID retrieves the same payment; after an
  ambiguous/expired lease, `FindPayment` validates owner customer, amount,
  external reference and status before reuse. No second charge is generated.
- `Status` is owner-only (foreign/unknown order returns `ErrNotFound`) and returns
  bounded QR only while the class is `pending`. `settled`, `expired`, `refunded`
  and `disputed` return status without a new charge or QR. Duplicate remote
  matches conflict. No access is granted in this phase; settlement projection is
  PAY-10.
- `backend/internal/modules/payments/adapters/asaas.go` is a thin stdlib
  `net/http` client (no vendor SDK): sandbox/production base URLs,
  `access_token` + stable `User-Agent`, exact BRL decimal from centavos,
  `billingType=PIX`, due date, `externalReference`, bounded JSON responses,
  customer/payment lookup, QR retrieval, status mapping and safe error mapping.
  It does not create mandates/subscriptions, accept card data or log tax IDs.
- Payload/QR sizes are bounded (4 KiB copy-paste, 64 KiB encoded image). QR
  expiration is returned when provider-parsable. No schema, dependency, HTTP
  route, API/worker wiring or sale activation was added.

## Validation performed

```sh
go test -race -tags=integration -count=1 ./internal/modules/payments/...
go test -count=1 ./...
go vet ./internal/modules/payments/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/payments/...
govulncheck ./internal/modules/payments/...
sqlc vet && sqlc diff
./scripts/check-security.sh --static-only
bash scripts/scan-secrets.sh
gofmt -l backend/internal/modules/payments
git diff --check
```

Results:

- Application tests PASS: server-priced Pix/QR; every client tamper field denied;
  payer required only for first customer; concurrent starts converge on one
  payment; lost response → `UNKNOWN` → recovery without a second charge;
  incomplete search stays ambiguous; wrong customer/amount/order binding refused;
  owner status/QR and foreign/unknown reads denied; expired/settled status returns
  without new charge; oversized/foreign QR rejected.
- Asaas adapter tests PASS with `httptest` (no real account): customer search/
  create, auth/user-agent, exact PIX/amount/due-date/external-reference shape, no
  card fields, found/no-match/incomplete/multiple payment lookup, QR parsing/bounds
  and status/amount mapping.
- Real PostGIS store integration PASS: one customer/order/operation/attempt, QR
  status, zero settlements/grants; lost response recovery after lease expiry with
  one provider create and one search.
- Full backend unit suite PASS. Race integration, vet, staticcheck, govulncheck,
  sqlc, security static review, secret scan, gofmt and diff checks PASS.
- No real Asaas account, key, customer, CPF/CNPJ, charge, QR, webhook or deployment
  was used. Tests use synthetic `00000000000` and local HTTP servers.

## Remaining obligations

- PAY-08/09/10 add signed/authenticated webhook inboxes, settlement verification
  and grant projection; PAY-11/12/13 cover recurring/Pix Automático lifecycle.
- PAY-19 wires authenticated HTTP/status/QR routes and runtime; PAY-21 proves the
  actual Asaas sandbox merchant/Pix journey. All sale switches remain false.
- Late receipt, duplicate settlement and refund/dispute access effects are
  completed at PAY-10/16; this phase only prevents second charges and exposes
  owner status.
- Error-to-HTTP mapping and rate limits are later runtime work.

Next action: PAY-08 only after maintainer authorization.
