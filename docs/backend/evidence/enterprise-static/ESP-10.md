# ESP-10 — Add bounded static search and filters

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-09 `1f7c06ce`.

Rules: B-BR-ESP-11/12/15; BUC-ESP-04.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-10--add-bounded-static-search-and-filters).
[Routes/editorial contract](../../../product/ENTERPRISE_PORTAL_ROUTES_AND_EDITORIAL.md#search-implementation-esp-10).

## Before behavior

`/consultar/` had a state directory and a note that static search would come
later. There were no public search shards, no accent/CNPJ normalization, no
bounded browser result rendering and no missing/version-mismatch fallback.

## Delivered

- `landing/src/search.ts`: accent-insensitive name normalization, numeric and
  alphanumeric CNPJ normalization, duplicate-name matches, 80-character input
  clamp, 50-result cap, city filter, immutable release validation, abortable
  single-region shard loading and DOM rendering with `textContent` only.
- `npm run build` now emits `dist/assets/search.<hash>.js` from the same
  TypeScript stack. `portalgen --search-js` copies the compiled script to
  `assets/portal.<hash>.js`, references it with `type="module" defer`, and adds
  it to the portal manifest. Without `--search-js`, the generated pages keep no
  JavaScript and the HTML directory remains the fallback.
- `/consultar/` now has the GET search form with UF/city selectors, visible
  status and results list, a `<noscript>` explanation, and a state directory.
  The form reads `data-shard-base="/dados/busca/"` and the input-content release
  slug.
- The generator writes only per-state shards plus `sem-uf` under
  `/dados/busca/{release}/{uf}.json`; it never writes a national all-stations
  locator. Shards contain UUID, display/search name, CNPJ, municipality
  code/name, UF, source status and latest fuel count/price map. The release path
  is immutable, so a mid-session publication cannot mix new data with old code.
- Missing shard, wrong release/schema, aborted request and invalid UF all fall
  back to the HTML directory; obsolete work is aborted. No query ever becomes
  HTML, a URL path or executable code.
- No API, ANP call, database call, national payload, new dependency, route or
  publication was added. Candidate pages remain `noindex` by default.

## Tests and actual results

Landing, from `landing/`:

```sh
npm run typecheck
npm test
npm run build
npm run check
```

All PASS. The full landing suite is 60/60, including the new search tests:
accent normalization (`São`→`sao`, `Épsilon`→`epsilon`), numeric and
alphanumeric CNPJ punctuation, duplicate names, abuse input, result bounds,
missing shard, release mismatch, one-region fetch, fake-DOM rendering/fallback,
no `innerHTML`/`/v1/`/`eval`, and the fingerprinted `search.<hash>.js` build
asset. The final build emitted `search.2828d19c.js`; typecheck, build and check
pass.

Backend, from `backend/`:

```sh
go build -o /tmp/portalgen ./cmd/portalgen
go test -race -count=1 ./internal/modules/enterpriseexport/...
go vet ./internal/modules/enterpriseexport/... ./cmd/portalgen
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/enterpriseexport/... ./cmd/portalgen
gofmt -l internal/modules/enterpriseexport cmd/portalgen
git diff --check
```

All PASS. The portalgen fixture now generates 16 pages and 22 files, including
`SP`, `MG` and `sem-uf` shards and no national shard; the SP shard carries
normalized names/CNPJ/status/prices. `TestSearchScriptIsEmbeddedAndReferenced`
also PASSes with the real compiled asset via
`ANPFUEL_TEST_REAL_SEARCH_JS=.../search.2828d19c.js`. The synthetic 6,000-station
size test measured:

```text
regional SP: 346,084 bytes / 23,639 gzip
national:    1,020,084 bytes / 70,029 gzip
```

The regional design was selected; the regional gzip is below the 250 KiB
budget. The real PostGIS build→portal test still PASSes with shards present.

## Limitations

- The real ANP edition remains `BLOCKED_OVER_PRECISION` (ESP-07), so the search
  shards shown here are synthetic fixtures/disposable PostGIS data. No real
  search or price page is published.
- The scale test is a synthetic 6,000-station snapshot, not the real 7,708-station
  edition; real-scale shard bytes and a real low-memory device/browser session
  remain OWED (ESP-13/16).
- The fake-DOM test proves the wiring and fallback logic, but no real browser
  automation was available in this environment.
- `/fiscalizacao/` remains gated (ESP-11); `/consultar/` remains `noindex` in
  candidate builds.
- Next: ESP-11 — safely associated official-record pages, only after the
  selected FQ source/runtime and station-link acceptance.
