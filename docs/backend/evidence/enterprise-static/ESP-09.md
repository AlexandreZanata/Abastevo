# ESP-09 — Generate station, price and history pages

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-08 `aae6c993`.

Rules: B-BR-ESP-03/06–09/12/16; BUC-ESP-04/05.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-09--generate-station-price-and-history-pages).
[Routes and editorial contract](../../../product/ENTERPRISE_PORTAL_ROUTES_AND_EDITORIAL.md).

## Before behavior

The frozen templates and the export/analytics existed separately. There was no
generator that turned a pinned export candidate into static station, price and
history pages, no portal manifest and no proof that exact prices, periods,
missing weeks and derived summaries survive rendering.

## Delivered

- `backend/internal/modules/enterpriseexport/portalgen`: deterministic static
  generator from one candidate directory (`manifest.json`, `stations.jsonl`,
  `revisions.jsonl`, `prices.jsonl`, `analytics-v1.json`). It verifies the
  export schema, the analytics metrics version and that
  `input_content_sha256 == export content_sha256`; it never calls the database,
  API or a source.
- Generated pages: `/consultar/` (state directory and national summary),
  `/precos/`, `/precos/uf/{uf}/`, `/precos/uf/{uf}/municipio/{ibge}/`,
  `/postos/{uuid}/`, `/metodologia/`, gated `/fiscalizacao/`, `/404.html`,
  `/assets/portal.css`, `robots.txt` and `portal-manifest.json`.
- Station pages use canonical UUIDs, full CNPJ, sourced status, allowlisted
  address, latest observations and one history row per revision with explicit
  missing weeks. State/municipality pages use the frozen analytics summaries
  and rankings; municipality pages below five priced stations show
  `Amostra insuficiente` without a ranking claim.
- Every page carries the survey period, Abastevo publication, source/checksum,
  “não é preço ao vivo”, sample/count context and ANP attribution. No page
  claims “cheapest in Brazil/now”, zero-price gaps, risk scores or private data.
- Pages are plain semantic HTML with an embedded responsive/print stylesheet and
  no JavaScript. Candidate builds default to `noindex,nofollow` and
  `Disallow: /`; `--indexable` exists for a later authorized publication.
- `backend/cmd/portalgen` exposes the operator command:
  `--snapshot`, `--out`, `--max-pages`, `--max-bytes`, `--indexable`.
- Output budgets: default 50,000 pages / 2 GiB; an exceeded budget fails and
  removes the temporary output. The output directory must not already exist;
  writes are private (0700 directories, 0600 files) and atomic via a temporary
  sibling directory.
- `portal-manifest.json` binds the input content hash, metrics version, page
  count, sorted per-file SHA-256/bytes and a content hash for ESP-13.

No source, database, schedule, route, release pointer or public deployment was
enabled. The real source remains blocked by ESP-07 (`BLOCKED_OVER_PRECISION`)
and ESP-01 rights; the generator was exercised on synthetic fixtures and the
disposable PostGIS acceptance build.

## Tests and actual results

From `backend/`:

```sh
go build -o /tmp/portalgen ./cmd/portalgen
go test -race -count=1 ./internal/modules/enterpriseexport/...
go test -v -count=1 ./internal/modules/enterpriseexport/portalgen
go vet ./internal/modules/enterpriseexport/... ./cmd/portalgen
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/enterpriseexport/... ./cmd/portalgen
gofmt -l internal/modules/enterpriseexport cmd/portalgen
git diff --check
```

All PASS. The portalgen suite has four named tests and five rejection
subtests: a 7-station/2-revision fixture generates 16 pages and 19 files with
exact national/state/municipal prices, periods, history gaps, suspended status,
unlocated station, gated fiscalization, methodology, robots and portal manifest;
a second run is byte-identical; every absolute generated link resolves; stale
analytics, page budget, existing output, census divergence and unsafe station
IDs are rejected without residual output.

Real PostGIS integration:

```sh
go test -count=1 -v -tags=integration \
  ./internal/modules/enterpriseexport/adapters/jobs \
  -run TestBuildCoalescesCorrectionsAndReconciles
```

PASS. After the duplicate/replay/correction checks, the test generates a portal
from the real candidate and finds the station page with the latest `R$ 6,199`
and historical `R$ 5,999`, proving export → analytics → manifest → HTML for the
disposable database.

## Limitations

- No real ANP edition has a published revision (ESP-07 over-precision), so the
  generated pages shown here are synthetic fixtures / disposable PostGIS data.
  Real price pages remain blocked until rights and monetary precision are
  decided.
- `/consultar/` currently exposes the state directory, not the JS-free static
  search shards; ESP-10 adds name/CNPJ search and regional shards.
- `/fiscalizacao/` is a gated placeholder; ESP-11 owns admitted official records.
- The embedded portal CSS is the frozen prototype stylesheet; final landing
  integration, cache headers, sitemap and release pointer belong to ESP-13/14.
- Scale was not measured on the 7,708-station real snapshot; ESP-13/16 own the
  artifact-size and capacity measurements.
- No page is published or indexable by default.
- Next: ESP-10 — JS-free static search by name/CNPJ with bounded regional
  shards and no full national payload to every visitor.
