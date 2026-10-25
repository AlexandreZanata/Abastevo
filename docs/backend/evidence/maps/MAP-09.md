# MAP-09 — Transactional map-overlay import and publication

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (real disposable PostGIS, upgrade, replay, concurrency, rollback)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `721582b8` plus the uncommitted
MAP-09–11 changes. Date: 2026-10-24. The tree is intentionally not pushed or
merged; no deployed environment was changed.

## Behavior frozen

- Only a complete `abastevo.map-prepared-v1` **PUBLIC_OVERLAY** is imported.
  Reference packages, private state and non-public visibility are refused.
- The envelope is checked against the exact assertion bytes: format/version,
  profile/rights hashes, safe input names, artifact bytes/SHA-256/records,
  reconciled counts, generation equality and canonical `content_sha256`.
- Any invalid assertion rejects the whole generation. No partial overlay is
  written.
- One transaction acquires the `map_generation_publish` advisory lock, marks the
  previous ACTIVE generation SUPERSEDED, inserts the new generation, assertions
  and exact price facts, then commits. A failure leaves the previous generation
  ACTIVE and all prior rows intact.
- Replaying identical content is idempotent. A different content creates a new
  generation and leaves exactly one ACTIVE row.
- Assertions are source-specific and preserve canonical station UUIDs, CNPJ,
  OSM/OCM identities, separate display/entrance points, location quality, exact
  milli-BRL product/unit prices, clocks and rights. No second canonical station
  table or distance-based merge is introduced.
- Prices are independent of native routing packages. The native package is not
  imported or published by this phase.

## Implementation

- `backend/db/migrations/000122_map_overlays.sql`: generations, assertions,
  price facts, append-only assertion revocations and generation revocations;
  GiST point index and bounded query indexes.
- `backend/internal/platform/mapcontract/prepared.go`: strict public-overlay
  envelope parser and canonical content hash.
- `backend/internal/modules/mapdata/store.go`: atomic publish, idempotent replay,
  active-generation read, bounded query and revocations.
- `backend/cmd/map-load`: one-shot local importer (no migrations, no network).
- `backend/cmd/map-revoke`: operator revocation tool.

## Verification executed

Fresh disposable PostGIS database created from the local dev container:

```sh
docker exec anpfuel-dev-db-1 psql -U anpfuel -d anpfuel \
  -c 'CREATE DATABASE anpfuel_map_proof'
cd backend
ANPFUEL_DATABASE_URL='postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel_map_proof?sslmode=disable' \
  go run ./cmd/migrate
# applied 000001..000122
ANPFUEL_DATABASE_URL='postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel_map_proof?sslmode=disable' \
  go run ./cmd/map-load --envelope /tmp/map-cli-proof/envelope.json \
  --assertions /tmp/map-cli-proof/assertions.json
# generation=6d884633-7819-4a49-81e8-4d7222db2417 status=ACTIVE
# source_revision=overlay-2026-10-24.1
```

Real PostGIS integration suite:

```sh
ANPFUEL_TEST_DATABASE_URL='postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable' \
  go test -race -count=1 -tags=integration ./internal/modules/mapdata
# ok
```

Covered cases: empty install through 122; upgrade from 121; publish and exact
price read; idempotent replay; new-generation supersession; two concurrent
writers with one ACTIVE row; canceled publish leaves zero generations;
assertion revocation preserves history; generation kill switch; malformed
overlay/hash/count/path/truncated input refused; API process start on the
imported generation.

Other checks:

```sh
go test ./...
go test ./internal/platform/mapcontract ./internal/platform/apicontract
go test -race -tags=integration ./cmd/api
go test -race -tags=integration ./internal/modules/directory/... ./internal/modules/charging/...
go vet ./internal/modules/mapdata ./internal/platform/mapcontract ./cmd/map-load ./cmd/map-revoke ./cmd/api
gofmt -l ...   # empty
git diff --check
```

The directory integration run initially failed only because
`ANPFUEL_TEST_PREPARER_PATH` was unset; rerunning with the existing pinned
`prepare_registry` binary passed. `staticcheck` could not run because the
installed binary cannot read Go 1.27 export data; this is a local tooling
limitation, not a code result.

## Limits

- One fixture overlay (five assertions) and one pilot database. No national
  import, no production deployment and no remote publication.
- No native `.mwm` package is admitted, signed or served here; that is MAP-12/13.
- OCM/ANP/community producers remain MAP-04–07; this phase consumes their
  frozen corpus, it does not claim source activation.
