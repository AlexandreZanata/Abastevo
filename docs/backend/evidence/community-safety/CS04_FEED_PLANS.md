# CS04 — Bounded feeds and query plans

Date: 2026-10-11. Status: LOCAL_DONE. Scope: read-path pins and
measured plans only — no route/schema/migration change, no VPS/APK.
Head: maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md).
Rules: B-BR-CS04.

## Pins (new tests, all green)

- `read/feedplans_integration_test.go` over a 1500-row synthetic load:
  cheapest feed plan uses indexes with no seq scan; tally PK lookup
  hits `community_helpfulness_tallies_pkey` (single index search);
  rebuild aggregate uses `community_helpfulness_target_idx`;
  all 1500 identical-price rows paginate in strict station_id order
  across every page (tiebreak holds on 2nd+ pages); all four orders
  serve the seeded load.
- `application/feed_matrix_test.go`: full 8 products × 4 orders
  validate (exact frozen vocabulary, incl. `GASOLINE_ADDITIVED` and
  `LPG_P13` — early draft guessed two wrong codes, corrected).
- `read_test.go`: marshalled `FeedItem` carries no helpfulness,
  overlay, voter, contributor, cursor, key or device fields.

## Measured costs (disposable local PostGIS, 1500-row load)

- Feed cheapest: `Index Only Scan (directory_stations_city_covering_idx)`
  + `Index Scan (community_current_prices_station_fuel_idx)`, zero seq
  scans; planning 0.7 ms, execution 4.1 ms.
- Tally PK lookup: `Index Scan using pkey`, 1 index search.
- Cursor security (pre-existing, cited): round-trip, filter-change,
  expiry, tamper, garbage and wrong-secret refusals; route cap 50 vs
  shared 100 already decided in B-BR-CS05 context.

## Recorded positions

- No RLS policies exist in the schema; reads rely on query
  predicates + sealed filter-bound cursors. RLS/role enforcement
  stays with P08 provisioning; read-only role evidence is owed there,
  not invented here.
- The 35-day ledger horizon is privacy retention (existing retention
  tests), not feed expiry (24 h, already covered); no new time test.
- Feed projections are untouched by helpfulness: tallies travel the
  tally endpoint, overlays the signed read path, never feed items.

## Checks

`go build`, `go vet` (incl. integration tags), `gofmt -l`,
`git diff --check` clean; `scan-secrets.sh` passed; full unit module
green; 4/4 new integration tests green.

Next microtask: CS05 — moderation queue.
