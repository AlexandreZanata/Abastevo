# MAP-07 — Reviewed charging/station identity and access points

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (real disposable PostGIS, concurrency, rollback and upgrade)
Integration: INTEGRATION_PENDING

Tested source: `codex/map-ev-implementation` after MAP-06. Date: 2026-10-24.

## Behavior rules and use cases

- A charging place and a Directory station are linked only by an explicit
  reviewed decision (`REVIEWED`, `UNRESOLVED`, `REJECTED`). Proximity, equal
  coordinates or a similar name never create a reviewed link automatically.
- The review records method (`CNPJ`, `ADDRESS_NAME`, `MANUAL`), evidence,
  reviewer reference and time. An existing active decision is superseded in
  the same transaction; history stays append-only. A failed/new invalid review
  rolls back and leaves the previous active decision intact.
- Exactly one active decision exists per place. Concurrent reviews serialize on
  the place row and leave one active link.
- Display point, entrance and road snap are separate rows per station. Each
  kind has one active point; updating an entrance does not alter display or road
  snap. Coordinates are bounded; invalid kinds/qualities/stations refuse.
- Linking never merges station rows, never changes a price generation and never
  deletes source assertions. Unresolved/rejected places remain visibly unlinked.
- CNPJ remains text in the existing Directory identifier chain; this task does
  not rewrite CNPJ, UUID or source IDs.

## Implementation

- `backend/db/migrations/000121_charging_identity_links.sql`:
  append-only `charging_place_station_links` and `directory_access_points`,
  partial unique active indexes and deferred self-referencing supersede FKs.
- `backend/internal/modules/charging/adapters/linking/store.go`: reviewed link
  insert/supersede/active lookup and access-point record/list, parameterized SQL,
  per-place/per-station locks and bounded transactions.
- No public endpoint, Android UI or automatic matcher was added. A reviewer or
  operator command must supply the explicit decision/evidence.

## Verification performed

Environment: Go `1.27.3`; disposable loopback PostGIS
`ANPFUEL_TEST_DATABASE_URL`; no persistent/staging database.

```sh
GOTOOLCHAIN=go1.27.3 go test -tags=integration -count=1 \
  ./internal/modules/charging/adapters/linking
# ok: review/supersede/rollback, no-proximity-link, 8-way concurrency one active,
# access points per kind, invalid inputs, upgrade from pre-121 schema and replay

GOTOOLCHAIN=go1.27.3 go test -tags=integration -count=1 ./internal/platform/migrate/...
# ok: empty install, checksum drift, concurrency, feed upgrade, replay

GOTOOLCHAIN=go1.27.3 go vet ./internal/modules/charging/adapters/linking
GOTOOLCHAIN=go1.27.3 go build ./internal/modules/charging/... ./cmd/api
gofmt -l internal/modules/charging/adapters/linking   # empty
git diff --check                                      # empty
```

The upgrade test applies every migration except `000121`, seeds an existing
station/charging batch/place, applies the new migration (`000121` only), verifies
the existing place survives and re-applies the chain with zero new migrations.
The concurrency test performs eight simultaneous reviews and proves eight
history rows with exactly one active link.

## Limits

- No reviewed-linking API/UI, operator command or automatic CNPJ/address
  suggestion exists yet. The primitive is storage/composition only; a later
  task may add a review surface over the same rules.
- No real OCM/Directory review batch was linked. The disposable test uses
  synthetic station/place rows; no production identity was merged.
- No map marker, price projection, profile merge or access-point display exists
  yet (MAP-09/10/18/19).
- No national coverage, production deployment, device or release claim.

Next: MAP-08 native map-generation toolchain.
