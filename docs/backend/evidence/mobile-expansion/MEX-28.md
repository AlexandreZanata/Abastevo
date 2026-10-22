# MEX-28 — Typed price exploration and aggregation backend

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `d64eb98b` (PR #141 merge). Date: 2026-10-22. Depends MEX-02/03,
both accepted. Rules B-BR-MEX-08/09/16; BUC-MEX-06.

Before behavior: expose a server-side typed ANP weekly price aggregation with
exact metric definitions, stable top/bottom ordering, full-population coverage
and no phone-only ranking. Keep ANP observations and community prices separate.
No free-form SQL/expression, provider fetch, materialized projection or HTTP
route is introduced here; the public ranking endpoint/cache/cursor is MEX-29.

## Delivered

- `official/application/ranking.go`: validates the frozen MEX-02 query subset
  (ANP_WEEKLY; STANDARD only; product/unit pairing; COUNTRY/STATE/MUNICIPALITY
  geography; OBSERVATION_PRICE by STATION; AREA_MEAN by MUNICIPALITY/STATE with
  ELIGIBLE_SOURCE_OBSERVATIONS; limit 1..100). Community source, other
  conditions/metrics, malformed geography and unsupported combinations are
  refused, never substituted.
- Exact builder: one latest observation per station (`collected_on DESC, id`),
  exact rational mean (`sum/count`, reduced), exact `max-min` variation, stable
  ties (station UUID; state then IBGE code), state roll-up from municipality
  rows, and coverage for the full selected population (raw source observations,
  stations with price, source stations, municipalities, states, unlocated).
  `Sampled=true`; national completeness is never claimed.
- `official/adapters/read/ranking.go` and `db/queries/official/ranking.sql`:
  resolve the current published revision for the requested survey week, then
  read only that revision. No aggregate/projection write: a corrected revision
  is visible as soon as the current pointer switches. A pointer to a
  non-published revision returns `ErrRankingUnavailable`.
- Migration `000110_official_ranking_index.sql`: additive covering index
  `official_station_prices_ranking_idx
  (revision_id, fuel_product, unit, station_id, collected_on DESC, id)
  INCLUDE (amount_milli_brl)`; existing rows/revisions/pointers unchanged.
- No HTTP route or OpenAPI change (MEX-29 owns the public wire/cursor/ETag).

## Actual checks

From this worktree (2026-10-22, parent `d64eb98b`):

- `go test -race -count=1 ./internal/modules/official/application` PASS:
  6 ranking tests plus 10 refusal subtests. Hand-derived municipality means
  (`5999`, `6001`, `6100`), exact fractional `11999/2`, state roll-up
  `17999/3`, tie-break `3509502` before `3550308`, descending tie stability,
  station ordering by amount/UUID, bounded pages and zero-price coverage.
- `go test -race -count=1 -tags=integration
  ./internal/modules/official/adapters/read` PASS (2 MEX-28 tests):
  real PostGIS fixture with 5 stations/2 states/3 municipalities, one
  unlocated station and an older duplicate observation. Verified latest-per-
  station (`5999`, not `5900`), municipality/state means, `COUNTRY`/`STATE`
  scopes, raw observation vs station coverage, missing product
  (`DIESEL_S10`) as zero priced stations over 5 source stations, and an
  unlocated observation excluded from area means but counted in coverage.
- Correction/expiry: publishing a second revision for the same week switched
  the pointer; the ranking changed to `5800` without any
  `official_summary_prices` row (`count=0`); marking the current revision
  non-published produced `ErrRankingUnavailable`.
- Plan: index present; with a staging revision of 20,000 rows across 1,000
  stations plus `VACUUM (ANALYZE)`, the representative aggregate plan uses
  `official_station_prices_ranking_idx` and has no sequential scan of
  `official_station_prices`.
- Full affected matrix PASS: `go test -race -count=1 -tags=integration
  ./internal/modules/official/... ./internal/platform/apicontract` (9
  packages, including existing importer/http/jobs/source/read suites).
- `go vet`, `GOTOOLCHAIN=go1.27.1 staticcheck`, `sqlc vet`, `sqlc diff`,
  `gofmt -l`, `git diff --check` and `bash scripts/scan-secrets.sh` PASS.
  Synthetic CNPJs/rows only; no source file, credential or real PII committed.

## Limits

- Only the ANP weekly source and STANDARD condition are implemented. Community
  eligible prices and other conditions remain later/owned units and are
  refused rather than guessed.
- No HTTP endpoint, cursor, ETag or cache; MEX-29 owns those and the public
  OpenAPI. No materialized `official_summary_prices` projection was added
  because the indexed read is bounded and no measured need exists yet.
- National source coverage is a sample; `Sampled=true` and
  `NationalCompletenessClaimed=false` are explicit. Unlocated observations are
  excluded from area means and reported in coverage.
- Device/native UI and community integration remain outside this unit.

Next: MEX-29 public ranking endpoint/cache/correction invalidation, reusing
this port and the indexed query.
