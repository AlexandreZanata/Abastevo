# PAY-09 — durable Asaas webhook inbox

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-02, PAY-04, PAY-05
Branch: `codex/payments-stripe-asaas`
Parent: `bb200890` (PAY-08).

Objective: authenticate Asaas callbacks with the dedicated webhook token, persist
bounded event metadata durably before HTTP 200, and handle duplicates, conflicts,
unknown/additive events and delivery retries without trusting a source IP or
inventing a body HMAC. Runtime wiring remains PAY-19; financial processing remains
PAY-10.

## Implementation

- `backend/internal/modules/payments/adapters/http/asaas_webhook.go` exposes
  `POST /v1/payments/asaas/webhook` for later runtime wiring. No live router is
  changed in this phase.
- Authentication uses exactly one `asaas-access-token` header compared in constant
  time against the current token and an optional previous token for rotation.
  The API key must be present separately and must not equal either webhook token;
  otherwise the handler is unavailable. A source IP, Stripe signature or invented
  body HMAC is never consulted.
- `RequireTLS` rejects non-TLS requests unless a trusted edge supplies
  `X-Forwarded-Proto: https`; runtime deployment owns trusted-proxy verification.
  Content type is JSON, body is capped at 1 MiB and JSON parsing is bounded by the
  same input.
- Event ID/type and `account.id` are required and the account must equal the
  configured merchant. The handler extracts the known `payment`, `authorization`
  or `paymentInstruction` resource ID; authentic unknown/additive events fall back
  to the event itself and remain accepted.
- SHA-256 of the exact raw body and minimal event metadata are sent through the
  PAY-04 inbox port. The inbox row and transactional `job_queue` outbox commit
  before HTTP 200. DB failure returns 503, duplicate same content is idempotent
  200, duplicate-ID alteration is 409 and malformed input is 400/401/403/413.
- The callback has no provider client, financial effect, settlement/grant path or
  synchronous processing. Asaas at-least-once delivery is absorbed by event-ID
  deduplication; a 503 keeps the provider retrying. Asaas retains pending events
  for up to 14 days and can pause after 15 consecutive failures, so PAY-10/18 must
  reconcile backlog/reopened delivery; this handler cannot declare success on DB
  failure.

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

- Handler tests PASS:
  - valid payment event and unknown eligibility event are durably accepted;
  - current/previous token rotation works, retired token fails, API-key reuse is
    unavailable, multiple token headers fail;
  - missing/wrong token, wrong content type, invalid JSON, wrong account, missing
    event ID/type are refused without inbox calls;
  - TLS requirement and oversized body are enforced;
  - duplicate same-content is 200, altered duplicate is 409;
  - DB outage returns 503 and a later retry succeeds, proving no fake success;
  - a wrong-token request from an arbitrary source IP is still rejected.
- Full backend unit suite PASS. Race integration, vet, staticcheck, govulncheck,
  sqlc, security static review, secret scan, gofmt and diff checks PASS.
- No real Asaas token/account/webhook/provider call, settlement/grant, sale or
  deployment was created. Tests use synthetic tokens/events and local dispatch.

## Remaining obligations

- PAY-10 validates settlements and projects access from inbox events; PAY-18
  owns adversarial/backlog operations; PAY-19 wires the route into runtime.
- PAY-21 must prove the actual Asaas dashboard webhook token/rotation and delivery
  behavior. Token rotation requires coordinated provider/app changes.
- Reopened/paused delivery and 14-day retention require reconciliation jobs; the
  handler only guarantees durable local acceptance or an explicit failure.
- All sale switches remain false.

Next action: PAY-10 only after maintainer authorization.
