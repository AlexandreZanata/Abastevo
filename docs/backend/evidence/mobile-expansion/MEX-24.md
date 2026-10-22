# MEX-24 — Station occurrence filter query and projection

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `bd19f0c7` (maintained dev, PR #145 head). Date: 2026-10-22.
Depends MEX-21/22/23, accepted. Rules B-BR-MEX-11/13–17; BUC-MEX-08.
Risk: critical join/attribution (public SQL over linked stations;
live fencing, no fuzzy binding, bounded traversal).

Before behavior: the leased enforcement revision gains a station
occurrence page: distinct safely associated stations with bound record
counts, filtered by exact station state, occurrence segment and
document-day range. Only exact current bindings qualify; ambiguous,
unmatched and retired links never surface. A station retired in the
Directory at read time is excluded live, never served stale. Stations
without price observations are included: this page is about
occurrences, never a completeness claim about prices. Coverage reports
distinct bound stations versus distinct bound records. Cursors bind
revision and filters; a correction invalidates stale cursors. Queries
use selective predicates before limit with an index proof; limits and
cancellation hold; HTTP bodies validate against OpenAPI with no
identifiers. Recovery: remove the occurrence filter capability, not
canonical stations.

## Delivered

- `backend/db/migrations/000114_quality_enforcement_company_station_idx.sql`:
  append-only bound-link index (revision, kind, station) for the
  station join; no fuzzy or name matching indexed.
- `backend/db/queries/quality/enforcement_company_associations.sql`
  (+ generated Go): `EnforcementStationPage` (distinct bound stations
  with record counts, state/segment/day facets, station keyset) and
  `EnforcementStationCoverage` (stations vs records).
- `backend/internal/modules/quality/adapters/read/read.go`:
  `EnforcementStationFilter`, `EnforcementStation`,
  `EnforcementStationCoverage`, `EnforcementStationsAtLease` and
  `EnforcementStationCoverageAtLease` under the leased revision.
- `backend/internal/modules/quality/adapters/http/http.go`:
  `/v1/quality/enforcement/stations` (facets validated, cursor-bound)
  and `/v1/quality/enforcement/stations/coverage` with ETags; facet
  params allowlisted.
- `contracts/openapi/v1.yaml`: station paths, facet params and the
  station/coverage schemas; vacuum zero errors.
- Prerequisite fix (MEX-21 staged boundary, same open PR): the
  association stager takes the source-record census explicitly because
  decisions cover the admitted subset, not the source-global
  remainder; `EnforcementSourceInput` accepts optional pre-planned
  company associations staged atomically with publication (sanctions
  pattern), preserving nil behavior.
- `enforcement_stations_integration_test.go`: bound station paged
  with count, ambiguous link excluded, state/unknown/malformed
  facets, coverage separation, live retirement exclusion, OpenAPI
  validation and identity-leak assertions.
- `enforcement_facet_plan_integration_test.go` (read): station page
  uses an index with 20,000 unrelated rows after ANALYZE.

## Actual checks

From this worktree (2026-10-22, parent `bd19f0c7`):

- `go test -count=1 -tags=integration -run
  TestEnforcementStationsFacetsCoverageAndOpenAPI
  ./internal/modules/quality/adapters/http/` → PASS: 1 bound station
  (1 record) paged, ambiguous decision excluded, state/segment facets
  exact, unknown state pages empty, four malformed shapes return 400,
  coverage 1 station / 1 record, retired station vanishes live, page
  and coverage validate, no CNPJ in any body.
- `go test -count=1 -tags=integration -run
  TestEnforcementFilteredPageUsesIndex → TestEnforcementStation*`
  ./internal/modules/quality/adapters/read/` → PASS: EXPLAIN shows
  index access, no sequential scan, with 20,000 unrelated rows.
- `go test -race -count=1 -tags=integration
  ./internal/modules/quality/adapters/http/` → PASS (25.7 s),
  including pre-existing enforcement suites with no regression.
- `go test -count=1 ./internal/modules/quality/...` → PASS;
  `go vet ./internal/modules/quality/...` PASS;
  `GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/quality/...`
  PASS; `sqlc vet` and `sqlc diff` PASS (migrations 000113/000114,
  queries, deterministic generate).
- `vacuum lint -r contracts/openapi/vacuum-rules.yaml
  contracts/openapi/v1.yaml --no-update-check` → zero errors
  (424 warnings / 104 informs, pre-existing documentation notes).
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- No worker, publisher-logic, Directory-write or Android changes; no
  source activation, no PII in Git/logs/fixtures.

## Limits

- The page covers the enforcement stream only; other occurrence types
  are refused as unavailable, not silently merged. Status and scope
  are fixed by policy (bound, current-company-only), not facets.
- Region facets use station location (Directory state); record
  geography stays on the record pages. Municipality filtering is
  deferred: station codes and source names are different vocabularies.
- No projection table was added: direct indexed queries satisfy the
  budgets, so no rebuildable projection is justified yet.
- A filtered global page never claims completeness about prices,
  stations without occurrences, or other streams.
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: MEX-25 (Android occurrence types) needs MEX-23/24 accepted APIs;
it consumes this tested station boundary once authorized.
