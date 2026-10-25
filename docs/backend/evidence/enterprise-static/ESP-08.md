# ESP-08 — Freeze portal routes, fields and editorial templates

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-07 `0bba900a`.

Rules: B-BR-ESP-03/06–09/12/16; BUC-ESP-04/05.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-08--freeze-portal-routes-fields-and-editorial-templates).
[Routes and editorial contract](../../../product/ENTERPRISE_PORTAL_ROUTES_AND_EDITORIAL.md).

## Before behavior

The plan named URL families but had no frozen canonical station route, field
allowlist, empty/stale states, attribution copy, source-gated official-record
page or prototype. There was no evidence that essential navigation, prices and
dates work without JavaScript.

## Delivered

[Enterprise portal routes and editorial contract](../../../product/ENTERPRISE_PORTAL_ROUTES_AND_EDITORIAL.md)
freezes:

- canonical station identity at `/postos/{uuid}/`; no slug/CNPJ/name redirect
  can move a page to another operator; unknown UUIDs are no-data/404;
- `/consultar/`, `/precos/` (national/state/municipality), `/combustiveis/{fuel}/`,
  `/fiscalizacao/` (gated), `/curiosidades/` (later), `/metodologia/` and the
  existing `/404.html`;
- public field allowlists for lists, detail, summaries and official records;
- editorial rules: sample/period wording, no “cheapest in Brazil/now”, no
  zero-price gap, official-vs-derived labels, source/date/attribution, no risk
  score, no guilt inference and no private fields;
- list/detail/summary/empty/stale/gated/error templates, navigation,
  accessibility, no-JS behavior, provisional byte budgets and source gates;
- rollback: the prototype is isolated from `landing/dist`.

A labelled synthetic prototype was added under `landing/prototype/`:

- `consultar/` — GET search form, static result cards, empty municipality and
  stale-data notices, no JavaScript;
- `postos/.../` — active and suspended examples, exact prices, dated history
  with an explicit missing week, source/checksum and canonical UUID;
- `precos/` — national/state summaries, sample/observation counts, derived
  label and insufficient-sample municipality;
- `metodologia/` — sources, clocks, sample/ranking rules, derived-vs-official,
  corrections, privacy, rights and the ESP-07 precision blocker;
- `fiscalizacao/` — honest gated page with no fabricated records.

The prototype is not part of `landing/scripts/build.mjs`, `dist/` or the
production sitemap. It uses a self-contained prototype CSS with focus,
responsive and print rules.

## Tests and actual results

From `landing/` after `npm ci` (existing locked TypeScript devDependency):

```sh
node --test tests/portal-prototype.test.mjs
npm test
npm run typecheck
npm run build
npm run check
```

All PASS: the prototype suite is 9/9; the full landing suite is 54/54; typecheck,
build and artifact check pass (`styles.4db58265.css`, `main.37b29769.js`,
prelaunch state). The prototype test asserts static pages, synthetic labels,
single h1/main/skip link, no script/API/`/v1/`/CPF, search form, empty/stale
states, canonical UUID, exact period/prices, missing-week wording, suspended
status semantics, derived summary label, insufficient-sample state, gated
fiscalization without a table, methodology coverage, local relative links and
CSS focus/responsive/print rules.

`git diff --check` is clean. No backend test was required by this phase.

## Limitations

- The prototype is synthetic and `noindex`; it does not generate production
  pages, search shards, sitemap entries or downloads. ESP-09/10 implement those.
- The real price publication remains blocked by ESP-07 (`over-precision`) and
  ESP-01 rights; `/fiscalizacao/` remains gated by FQ-06/07 runtime and safe
  station links.
- Byte budgets are provisional until ESP-10 measurement.
- Station coordinates are not rendered in V1; no map/geocoding is implied.
- Next: ESP-09 — generate catalog, price and bounded-history pages from the
  frozen export/analytics, preserving these templates and states.
