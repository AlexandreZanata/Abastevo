# DRV-11 — Revisioned reports and private HTTP

Status: LOCAL_DONE
Validation: PASS (local; no real provider, mobile or production acceptance)
Integration: INTEGRATION_PENDING
Parent: 568ae322. Behavior: 470e1d26. Contract: [DRV-11](../../../product/DRV_11_CONTRACT.md).

T01: the report service projects DAY/WEEK/MONTH/CUSTOM windows in an explicit
IANA timezone, filters owner/vehicle/platform, includes only fully contained
earnings/cost items, keeps actual and modeled costs separate, selects one
distance basis/window, and emits exact centavos/metres/rational strings. It
refuses more than 1000 source rows instead of truncating and computes a SHA256
source revision from earnings revisions, adjustments, cost profile and distance
content. A late earning edit changes that revision; an expected old revision is
rejected as stale. Work time is explicitly unavailable rather than zero.

T02: `POST /v1/driver/earnings` is an owner-signed free retained page with
1..100 items and an owner-bound HMAC cursor. `POST /v1/driver/reports/build`
requires signed owner plus `professional.report`; `POST /v1/driver/reports/get`
is a free retained read that returns 409 when the source revision changed.
All routes are no-store without ETags; errors follow the same policy. OpenAPI
adds the routes/schemas and golden vectors preserve exact units.

T03: migration 000137 adds bounded report snapshots with source revision,
increasing version, private JSON payload and account soft-delete erasure. The
worker has a dedicated bounded cleanup handler (1000 snapshots/day, 400-day
retention) registered only when `ANPFUEL_DRIVER_ENABLED` is true. The API mounts
the driver handler behind the same default-off switch; disabled routes return
404 without storage access and existing ledger/app routes are untouched.

## Actual validation

Real disposable PostGIS (`abastevo-pr150-postgis-20261025`): **51 top-level
PASS under `-race`** across driver adapter/PostGIS, HTTP, worker-job, application
and domain packages. New tests prove day/week/month window calculation, exact
recorded/modeled results (45000 earned, 20000 actual cost, 25000 recorded;
10000 modeled cost, 15000 modeled result; 500000 m; 50/30 R$/km), snapshot
restart, stale reads after edit, capability lapse with retained get, owner-bound
cursor, no-store, feature-disabled 404, and aging cleanup.

```
cd backend
go test -count=1 ./...                                   # PASS
ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:55439/anpfuel?sslmode=disable \
  go test -race -count=1 -v -tags=integration ./internal/modules/driver/...  # PASS
go build ./... && go vet ./... && go vet -tags=integration ./internal/modules/driver/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/driver/... ./internal/platform/config/...
sqlc vet && sqlc diff && gofmt -l . && git diff --check
cd .. && vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
```

Logs: `/tmp/drv11-final-integration.log`, `/tmp/drv11-final-unit.log`.
OpenAPI lint exits 0 with existing warnings; apicontract golden tests PASS.

## Deliberate limits

No mobile/device, real identity-key composition, real payment/provider proof,
staging deploy or production certification. The report does not project work
time or maintenance goals yet (DRV-18/23/25). The feature flag defaults off.
The inherited global Android coverage gate is unchanged and not re-run here.
