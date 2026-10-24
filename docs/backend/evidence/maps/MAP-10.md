# MAP-10 — Bounded spatial discovery and exact price projections

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (real PostGIS + HTTP process + stale-generation cursor)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `721582b8` plus the uncommitted
MAP-09–11 changes. Date: 2026-10-24.

## Behavior frozen

- `GET /v1/map/assertions` is anonymous and read-only.
- Required `min_lat`, `min_lon`, `max_lat`, `max_lon`, `source` (`ANP` or
  `COMMUNITY`); optional exact `product`, `limit` (1..100, default 100) and
  `cursor`.
- Invalid bounds, inverted bbox, `NaN`/`Inf` coordinates and viewports larger
  than 25 square degrees are refused before storage. A viewport is never
  expanded into a national query.
- Only pin-eligible assertions with an admitted point are returned. Unknown and
  city-centroid records are not fabricated into pins.
- Prices are projected only for the selected source and optional product.
  `ANP` never returns community prices and `COMMUNITY` never returns ANP
  prices. `capabilities.price_known` is projected with the price array, so a
  hidden community price is not reported as a known ANP price. Charging/
  reference records remain visible in either mode with empty price arrays;
  unknown is not zero/free.
- Each item carries source, kind, canonical identities, quality, separate
  entrance coordinates, exact amount/product/unit/date/period, clocks, rights
  and capabilities.
- The cursor is HMAC-sealed and bound to generation, bbox, source, product and
  limit. A new active generation invalidates old cursors. Response caching is
  `no-store`.
- One SQL statement returns the bounded page and aggregated price facts; no
  per-pin HTTP/database fan-out.

## Verification executed

PostGIS integration (`TestHandlerProjectsBoundedAssertionsAndRejectsStaleCursor`)
published the fixture, requested a 0.4 x 1.0 degree viewport with
`source=ANP&product=GASOLINE_REGULAR&limit=1`, and verified:

- HTTP 200 with the active generation id;
- one item, assertion `11111111-…`, exact `amount_milli_brl=5890`, unit `L`,
  period `2026-10-18..2026-10-24`;
- a next cursor; after publishing a new generation the old cursor returns 400.

Unit handler checks reject missing/invalid bbox, `NaN`/`Inf`, bad source, bad
product, inverted bounds and a 25-degree-plus viewport before touching storage.
The PostGIS integration also asserts that a community combined site in ANP mode
has empty prices and `price_known=false`.

Actual API process on the imported proof database:

```sh
curl -sS 'http://127.0.0.1:18080/v1/map/assertions?min_lat=-16&min_lon=-48.1&max_lat=-15&max_lon=-47.7&source=ANP&product=GASOLINE_REGULAR&limit=1'
# HTTP/1.1 200 OK
# Cache-Control: no-store
# generation 6d884633-… status ACTIVE
# item 11111111-… price 5890 L period 2026-10-18..2026-10-24
```

OpenAPI: `contracts/openapi/v1.yaml` adds `listMapAssertions` with bounded
parameters and response shape; `go test ./internal/platform/apicontract` passes.

## Limits

- Viewport discovery only; corridor/route-corridor queries and server-side
  aggregation at national zoom remain future work. The 25-square-degree cap is
  the current explicit bound.
- Antimeridian-crossing viewports are rejected rather than split; no client
  support is claimed.
- Unknown/city-centroid records are list-only and are not returned by this
  spatial endpoint; the accessible list is MAP-20.
- No Android consumer, staging, load test or national coverage claim.
