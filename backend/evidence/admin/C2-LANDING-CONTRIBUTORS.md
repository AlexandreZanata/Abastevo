# C2 — Landing: `/contribuidores/` page

Date: 2026-10-27. Branch `codex/adm-c2-landing-contributors` stacked on C1
(`codex/adm-c1-app-contributors`). PR targets the C1 branch; it merges after
C1, which targets `main`.

## Delivered

- `landing/static/contribuidores/index.html`: exact page shell (header with
  the uniform GitHub button + prelaunch pill, section-local nav, breadcrumb,
  footer), static explainer (what a contributor is, GitHub repo link, app
  link, no-account note), live-list container, JSON-LD breadcrumbs, canonical
  + OG/Twitter tags, `main.js` + fingerprinted `contributors.js`.
- `landing/src/contributors.ts`: reads the API origin from
  `<meta name="abastevo-api">` (absent/empty = disabled, stays inert);
  fetches `GET {origin}/v1/contributors`; renders cards with `textContent`
  only (`name`, verified badge, since, roles, links with
  `rel="noopener noreferrer nofollow ugc"` + `target="_blank"`); links pass
  the same CTB-08 allowlist as the app; malformed items skipped; bad
  payloads and fetch failures render nothing and never throw.
- Build: `contributors.js` emitted like `search.js`;
  `--api-origin`/`ABASTEVO_API_ORIGIN` (empty default = disabled) baked into
  the meta tag; `{{API_CONNECT_EXTRA}}` substituted into a new
  `/contribuidores/*` `_headers` block (`connect-src 'self'` + origin, or
  plain `'self'` when disabled).
- `_headers`, `sitemap.xml`, `llms.txt`, footer Projeto nav (new page only;
  other pages untouched), README build option documented.

## Tests

- `contributors-content.test.mjs` (2): no-JS static content (explainer,
  repo/app links, note), API-origin meta on/off.
- `contributors-csp.test.mjs` (2): API origin in CSP only on the
  contribuidores path; disabled build keeps self-only connect-src.
- `contributors-render.test.mjs` (4): XSS payload stays text with no
  script nodes/event attributes/`javascript:` urls and only the allowlisted
  link rendered; malformed items skipped; disabled/inert behavior; compiled
  asset has no unsafe DOM calls.

## Commands and results

```sh
cd landing
npm ci && npm run typecheck && npm run build && npm run check && npm test
# 69 tests, 69 pass, 0 fail (61 pre-existing + 8 new)
```

Two pre-existing suite tests initially failed on the new page (uniform
header pill + resolvable in-page anchors); fixed by conforming the page to
the shell (full GitHub button, prelaunch pill, dropped dangling footer
link) — no test was weakened.

## Not done

- C3 (claim/profile self-service), C4 (staging check). No staging touched.
