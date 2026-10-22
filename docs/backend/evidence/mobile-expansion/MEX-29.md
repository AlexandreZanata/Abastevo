# MEX-29 — Public price ranking endpoint, cache and correction invalidation

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `eb168dbe` (PR #142 merge). Date: 2026-10-22. Depends MEX-28,
accepted. Rules B-BR-MEX-09/16/17; BUC-MEX-06/09.

Before behavior: expose the MEX-28 typed ANP weekly ranking through one bounded
anonymous route with a visible interpretation, source revision, coverage and
stable cursor/ETag. Every request resolves the current published revision; a
revision switch invalidates cached pages, cursors and conditional responses. No
provider fetch, ingestion, broker, search cluster, materialized projection or
new dependency is introduced. Community prices and non-STANDARD conditions stay
separate and are refused here.

## Delivered

- `GET /v1/prices/ranking`: required `source=ANP_WEEKLY`,
  `survey_week_start`, `fuel`, `unit`, `geography`, `metric`, `group_by` and
  `sort`; optional `uf`, `ibge_code`, `condition=STANDARD`, `limit` (1..100,
  default20) and `cursor`. Unknown/duplicate/empty parameters and unsupported
  source/condition/metric combinations return `400`.
- Response binds `interpretation`, the exact `source` revision, full
  `coverage`, `stations`/`areas`, `next_cursor` and `generated_at`. Exact
  rational means and `max-min` variation are preserved; `sampled=true` and
  `national_completeness_claimed=false` are explicit.
- Cursor/ETag bind every validated filter, policy version, limit and the
  resolved source revision. A correction that switches the pointer changes the
  filter hash and the stable ETag payload, so an old cursor returns `400` and an
  old `If-None-Match` returns a fresh `200`, never a stale `304`.
- Bounded in-process LRU cache: entry and total-item bounds, TTL expiry, and
  revision/filter/position in the key. The resolver still runs on every
  request; a database outage returns `503 no-store` instead of a cached page.
  `ANPFUEL_PRICE_RANKING_CACHE_ENTRIES=0` runs the exact uncached query;
  `ANPFUEL_PRICE_RANKING_ENABLED=false` disables the route.
- MEX-28 reader split into `ResolveRankingRevision` + `ReadRankingAt`, with
  keyset SQL for station pages (`amount` asc/desc, station UUID tie-break) and
  exact position filtering for area pages.
- `contracts/openapi/v1.yaml` path/schemas; `backend/README.md` and
  `backend/.env.example` operator documentation.

## Actual checks

From this worktree (2026-10-22, parent `eb168dbe`):

- Unit (`go test -race ./internal/modules/official/adapters/http`): 10 tests
  PASS. Cold request resolves1/reads1; conditional repeat resolves2/reads1 and
  answers `304` with an empty body; warm repeat resolves3/reads1 with the same
  bytes. Revision switch returns `200` with a new revision/ETag/value. Outage
  returns `503 no-store` without leaking the cached price. Cursor pages are
  bounded and filter-bound; an ASC cursor on a DESC query and an expired cursor
  return `400`. Cancellation writes nothing and touches no storage. Cache LRU,
  TTL, total-item bound and 32 concurrent accesses pass; zero entries disables.
- Real HTTP/PostGIS integration (`-tags=integration`): actual route, actual
  SQL, OpenAPI `PriceRankingResponse` validation, 8 concurrent fills, revision
  switch with old ETag→`200` and old cursor→`400`, corrected mean `5800`.
- Full affected matrix, one run, loopback disposable PostGIS:
  `go test -race -count=1 -tags=integration ./internal/modules/official/...
  ./internal/platform/config ./internal/platform/apicontract ./cmd/api`
  PASS, 11 packages.
- `go vet`, `GOTOOLCHAIN=go1.27.1 staticcheck`, `sqlc vet`, `sqlc diff`,
  `gofmt -l`, `git diff --check` and `bash scripts/scan-secrets.sh` PASS.
- `vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml
  --no-update-check` PASS: zero errors, 414 warnings and 103 informs (the
  existing documentation/example warnings; this is not a zero-warning result).

## Limits

- The cache is per process/replica, not shared; there is no materialized
  projection because the indexed read is bounded and no measured need exists.
  A revision switch leaves old entries unreachable until TTL/LRU eviction.
- Only ANP weekly STANDARD observations are ranked. Community prices and other
  conditions return `400`; no provider/source activation or deploy occurred.
- The endpoint is enabled by config default but was not deployed or certified;
  no device/UI journey (MEX-31), production measurement or release acceptance
  is claimed.
- After MEX-29, no other unit is both dependency-ready and authorized:
  MEX-20a is retained by the handoff, MEX-30 requires the blocked MEX-23, and
  EE-04/05 remain source-admission/rights-gated.

Next: obtain explicit authorization/source gates for MEX-20a, EE-04/05 or the
MEX-21–23 chain before continuing.
