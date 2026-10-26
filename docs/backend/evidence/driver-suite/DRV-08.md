# DRV-08 — Durable shifts, recovery gaps and distance histories

Status: LOCAL_DONE
Validation: PASS (local; not production, HTTP, cloud or full-suite acceptance)
Integration: INTEGRATION_PENDING
Parent: 84dfd74d. Behavior: 4026df2a. Contract: [DRV-08](../../../product/DRV_08_CONTRACT.md).

T01: append-only migration 000134 after 000133 adds `driver_shifts`, recovery
gaps and command receipts. The pure Go shift state machine mirrors DRV-04:
READY/RUNNING/PAUSED/CLOSED/RECOVERY_REQUIRED, caller-supplied monotonic/wall
policies, UTC evidence, explicit gaps and reviewed work. Every accepted command
is version-fenced, receipted and pruned to the last 50 per shift. The store
locks the owner, requires the live cloudsync vehicle/account port and commits
state, gap and receipt atomically. No coordinate/trace/route field is admitted.

T02: `driver_distance_observations` stores only an aggregate UTC window, origin,
vehicle segment, basis, optional metres, complete/gap coverage, quality
kind/version/reason and—for ODOMETER—the chosen start/end metres. Odometer
distance must equal the endpoint difference and reversed/cross-segment values
are refused. The same owner/vehicle/basis cannot overlap; different bases may
share a window for reconciliation and are never summed. Identical
operation/body retries converge; changed bodies and overlapping records
conflict. No raw GPS trace, latitude/longitude, passenger or note field exists.

T03: real owned disposable PostGIS (`abastevo-pr150-postgis-20261025`,
127.0.0.1:55439, PostGIS 3.6/PostgreSQL 18) ran **16 adapter integration tests
under `-race`**, including five new movement tests:

- shift start/pause/resume/end, restart, lost-response replay, closed history
  and owner/scoped-vehicle isolation;
- boot-change gap persistence, restart, explicit review and duplicate-review
  refusal;
- concurrent two-device starts (one winner), identical concurrent start
  convergence, stale pause race, and final-owner-proof denial with complete
  rollback;
- odometer distance restart, replay, changed-body conflict, overlapping-window
  refusal, adjacent window, different-basis reconciliation, foreign
  owner/vehicle denial, bounded list and soft-account erasure;
- empty pre-134 schema, actual upgrade to 134, active-shift index and both
  account-erasure triggers.

The migration failure/recovery test now applies exactly **two** pending
migrations (133 and 134) once under concurrent migrators; the prior assertion of
one version was updated to the actual pending count, not weakened. The existing
earnings migrations and tables were not rewritten.

## Verification

```
cd backend
go test -count=1 ./...                                   # PASS
ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:55439/anpfuel?sslmode=disable \
  go test -race -count=1 -v -tags=integration ./internal/modules/driver/...  # PASS, 33 checks
go vet ./... && go vet -tags=integration ./internal/modules/driver/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/driver/...
sqlc vet && sqlc diff && gofmt -l . && git diff --check  # PASS
```

Logs: `/tmp/drv08-final-unit.log`, `/tmp/drv08-final-integration.log`,
`/tmp/drv08-integration-final.log`. The 33 PASS count is 16 adapter/PostGIS
tests plus 17 driver domain tests; it is not 33 HTTP/UI/device journeys.

## Deliberate limits

No HTTP route, signed admission, Premium check, cloud opt-in, GPS collector,
location permission, mobile UI, backup/restore, export, tombstone/epoch or raw
trace was added. Account soft deletion erases the new rows and physical
deletion cascades; stale-restore/epoch fencing remains DRV-09/15. The inherited
global Android coverage gate is unchanged and not claimed here. No deployment,
provider activation, PR update, merge or wiki publication occurred.
