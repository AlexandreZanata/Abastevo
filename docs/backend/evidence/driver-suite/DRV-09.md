# DRV-09 — Cost profiles, maintenance and exact ledger links

Status: LOCAL_DONE
Validation: PASS (local; not production, HTTP, cloud or full-suite acceptance)
Integration: INTEGRATION_PENDING
Parent: babf66a7. Behavior: 9241b869. Contract: [DRV-09](../../../product/DRV_09_CONTRACT.md).

T01: append-only migration 000135 after 000134 adds versioned cost profiles/items
and maintenance schedules. Profile versions are expected+1 per vehicle and old
versions remain readable; zero centavos is preserved separately from NULL.
Maintenance schedules retain kind, basis, effective period, nullable interval/
baseline/next-due values, the explicit odometer segment and reset context.
Operation/body retries converge; stale versions conflict.

T02: actual costs are linked through the transaction-bound ledger port
`RequireDriverLedgerCost`, implemented by the ledger module. The port locks the
authoritative ledger row with `FOR SHARE`, requires the same owner, vehicle and
exact revision, rejects deleted/voided/ambiguous records and returns the
authoritative centavo amount. The driver item amount must match (including both
being unknown). The driver adapter never references ledger tables and writes no
ledger record; a concurrent ledger update is blocked until the linked profile
commits. Missing port, stale revision, foreign source and duplicates fail
without a profile.

T03: cost/maintenance owner epochs and `(vehicle,version)`/`(schedule,version)`
tombstones block stale recreation. Owner erasure advances the epoch and clears
profiles, schedules, commands and tombstones. Get/list ports are owner-scoped;
soft account deletion erases these datasets and physical deletion cascades.
The registry in [DRIVER_PRIVATE_DATA](../../DRIVER_PRIVATE_DATA.md) now lists all
driver datasets, their read/erase boundaries and the fact that cloud backup
remains excluded. Movement remains account-erasure-only; per-record movement
tombstones and full-history export are explicitly not claimed here.

## Actual validation

Real owned disposable PostGIS (`abastevo-pr150-postgis-20261025`,
127.0.0.1:55439): **21 driver adapter tests under `-race` PASS**, including five
new cost/maintenance tests:

- cost version/restart, replay, stale version, stale ledger revision, tombstone,
  owner epoch and exact ledger amount preservation;
- foreign owner/vehicle, deleted/voided/ambiguous ledger sources and missing
  ledger port refusals;
- atomic ledger fencing: a concurrent ledger update waits for the `FOR SHARE`
  holder until the cost profile commits;
- maintenance zero-versus-NULL, odometer segment/reset, version replay, record
  tombstone and owner epoch;
- soft-account cleanup of cost/maintenance rows.

The driver domain suite adds 19 top-level tests (including maintenance
validation); the `-race` command reports **40 PASS** across adapter+domain.
The migration test now applies the actual **three** pending migrations (133–135)
once under concurrent migrators; the prior count assertion was updated to the
real pending set, not weakened.

```
cd backend
go test -count=1 ./...                                   # PASS
ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:55439/anpfuel?sslmode=disable \
  go test -race -count=1 -v -tags=integration ./internal/modules/driver/...  # PASS, 40 checks
go vet ./... && go vet -tags=integration ./internal/modules/driver/... ./internal/modules/ledger/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/driver/... ./internal/modules/ledger/...
sqlc vet && sqlc diff && gofmt -l . && git diff --check  # PASS
```

Logs: `/tmp/drv09-integration.log`, `/tmp/drv09-go-unit.log`.

## Deliberate limits

No HTTP route, signed admission, Premium check, cloud opt-in, backup manifest
change, mobile UI, GPS collection or deployment was added. The existing
vehicles/favorites/preferences cloud allowlist is unchanged. Full movement
history export and movement record tombstones remain DRV-11/15/27 obligations;
no complete-export claim is made. The inherited global Android coverage gate is
unchanged and not re-run here.
