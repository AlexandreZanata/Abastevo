# PAY-08 — durable Stripe webhook inbox

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-02, PAY-04, PAY-05
Branch: `codex/payments-stripe-asaas`
Parent: `0af7918e` (PAY-07).

Objective: authenticate bounded exact Stripe callbacks, bind the expected
merchant/mode/API version, persist a minimal durable inbox before acknowledging,
and handle duplicate/conflicting/unknown events without remote calls in the
callback path. Runtime wiring remains PAY-19; event processing remains PAY-10.

## Implementation

- `backend/internal/modules/payments/adapters/http/stripe_webhook.go` exposes
  `POST /v1/payments/stripe/webhook` for later authenticated runtime wiring.
  No route is added to the live router in this phase.
- The handler requires `Content-Type: application/json`, a 1 MiB body cap and a
  bounded `Stripe-Signature` header. It verifies the exact raw bytes with the
  maintained SDK `webhook.ConstructEventWithOptions`, default 300-second
  tolerance and pinned API version `2026-09-30.endive`; no manual HMAC is used.
  Multiple `v1` signatures from rotated endpoint secrets are accepted, while an
  old-only/invalid/stale signature is rejected.
- Signature/config/binding failures return 400; missing secret/merchant or
  environment/mode mismatch returns 503. `livemode`, empty/no Connect account and
  event ID/type are checked. No response echoes event data.
- A SHA-256 of the exact raw body and minimal `{provider, environment, merchant,
  event_id, event_type, resource_type, resource_id}` metadata are sent through
  the PAY-04 durable inbox port. `RecordInbox` inserts the event and transactional
  `job_queue` outbox in one transaction; the handler returns HTTP 200 only after
  that commit succeeds. DB failure returns 503, never a fake success.
- Duplicate event ID with the same payload is idempotently acknowledged; the same
  ID with different content returns 409. Unknown/additive event types are stored
  for later safe processing. The handler has no provider client, no settlement/
  grant path and no network call.
- `InboxEvent`/`InboxResult` moved to the application port; the existing Store
  aliases preserve PAY-04 callers. No schema, dependency or runtime wiring change.

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
  - valid/unknown events are durably accepted and hash/event/resource metadata
    match the exact raw body;
  - duplicate same-content is idempotent; duplicate-ID alteration is 409;
  - forged, mutated-raw-body, stale, missing and oversized signatures are 400;
  - rotated header with new+old signatures passes; old-only fails;
  - wrong livemode, wrong/missing API version and Connect account are 400;
  - missing secret/merchant or environment/mode mismatch is 503;
  - wrong content type, oversized body and oversized signature are bounded/refused;
  - DB failure returns 503 and no test sees a 2xx.
- Full backend unit suite PASS. Race integration, vet, staticcheck, govulncheck,
  sqlc, security static review, secret scan, gofmt and diff checks PASS.
- No real Stripe secret, webhook endpoint, provider call, settlement/grant, sale
  or deployment was created. Tests use synthetic signed payloads/local httptest.

## Remaining obligations

- PAY-09 adds the distinct Asaas token-authenticated inbox; PAY-10 verifies
  settlements and projects access; unknown-event processing remains asynchronous.
- PAY-19 wires the handler into the authenticated runtime and config; PAY-21
  proves the actual Stripe dashboard webhook/endpoint secret and rotation.
- Backlog/reconciliation, duplicate processing effects and provider retry policy
  are covered by PAY-10/18/21. All sale switches remain false.

Next action: PAY-09 only after maintainer authorization.
