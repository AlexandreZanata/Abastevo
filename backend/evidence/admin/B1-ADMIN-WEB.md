# B1 — Admin web panel (login, accounts, grants)

Date: 2026-10-27. Branch `codex/adm-b1-admin-web` from `origin/main`
`d4dcf716`. PR targets `main` and merges only after #164. Front A API used
from the contract and the open A1/A2 PRs (no backend code copied).

## B0 recon

Read from `origin/main`: `landing/README.md`, `landing/scripts/build.mjs`,
`landing/scripts/check.mjs`, `landing/static/assets/styles.css` (tokens,
Manrope + Source Sans 3 local fonts, 48/64rem breakpoints, focus rings),
`landing/tests/headers-and-redirects.test.mjs` and
`landing/tests/local-fonts.test.mjs`, plus `landing/tsconfig.json`,
`landing/static/_headers` and `landing/package.json` (TypeScript 7.0.2
strict, Node test runner, no runtime dependency).

## Delivered

New top-level package `admin-web/` mirroring the landing layout:

- `src/api.ts` — admin API client (relative `/v1/admin/...` paths,
  `credentials: 'same-origin'`, CSRF kept in a module variable,
  `X-Admin-CSRF` + `Idempotency-Key` (`crypto.randomUUID()`) on every
  mutation, 401 callback, `Retry-After` parsing, pt-BR error mapping for
  `admin.*`/`entitlement.*` codes). DOM-free and unit-tested.
- `src/validators.ts` — reason 10–500 code points, tier enum, optional end
  date (real calendar day, not past, within 5 years; server enforces the
  exact 1h..5y window), search minimum, UUID. DOM-free and unit-tested.
- `src/app.ts` — page wiring with `textContent`/`createElement` only.
- `static/app/index.html` — single shell served at `/admin/login/` and
  `/admin/accounts/` (login view, search, detail, grants, revoke, logout).
- `static/assets/admin.css` — admin-only rules using existing tokens only
  (enforced by test).
- `static/_headers` — admin CSP (`frame-ancestors 'none'`), `X-Robots-Tag`
  noindex, `Referrer-Policy: no-referrer`, `no-store` HTML, immutable
  assets. `robots.txt` disallows everything.
- `scripts/build.mjs` — tsc compile, per-file fingerprinting with import
  specifier rewriting, landing stylesheet + hash-verified fonts copied at
  build time (landing files only read, never edited).
- `scripts/check.mjs` — dist gate (pages, fingerprinted refs, noindex,
  headers, budgets, no leaks).
- 8 test files: build, unsafe-DOM scan, headers, API client (fake fetch),
  validators, admin-css tokens, check script, login password discipline.

## Design notes

- Single shell + `history.pushState` (no full reload between login and
  accounts): the CSRF token lives in a module variable, so any full page
  load would wipe it. Every full load therefore starts at the login view;
  this is the honest consequence of the no-storage rule, documented in
  `admin-web/README.md`.
- Logout sends the fixed reason `admin logout via web panel`: the API
  requires a 10–500 char reason on every mutation, including logout.
- `fetch` is always invoked through an arrow wrapper: passing the bare DOM
  `fetch` as a callback throws illegal-invocation in browsers. Found by the
  local integration run; covered by a regression test.
- No `console.*`, no `localStorage`/`sessionStorage`, no `innerHTML` family
  anywhere in `src/` (scan test fails the suite otherwise).

## Local integration (A4 backend, disposable PostGIS, synthetic data only)

Backend built from `codex/adm-a4-staff-roles` (`f5004a77`, contains A1–A3),
fresh disposable database with all migrations, one synthetic account, one
locally generated test admin key (memory only, never the staging key).
Panel served from the same origin through a test-only static+proxy harness
(replicates the B3 same-origin shape).

Request transcript driven by the real panel in headless Chrome (statuses):

```text
POST /v1/admin/session                          200 login
GET  /v1/admin/session                          200 whoami
GET  /v1/admin/accounts?q=synthetic             200 search
GET  /v1/admin/accounts/{id}                    200 detail
POST /v1/admin/accounts/{id}/grants             201 grant PRO
GET  /v1/admin/accounts/{id}                    200 detail reload
POST /v1/admin/grants/{id}/revoke               200 revoke
GET  /v1/admin/accounts/{id}                    200 detail reload
DELETE /v1/admin/session                        204 logout
```

Screenshots (synthetic data only): `b1-01-login.png` … `b1-06-logged-out.png`
in this directory. Reuse of the session cookie after logout returns 401
(verified in a follow-up run).

Two harness-only artifacts were found and isolated (not panel/backend bugs):
`fetch` illegal-invocation (fixed in `src/api.ts` + regression test) and a
DELETE-body forwarding limitation of the throwaway Node proxy (direct
`DELETE /v1/admin/session` against the API returns 204).

## Commands and results

```sh
cd admin-web
npm ci && npm run typecheck && npm run build && npm run check && npm test
# 17 tests, 17 pass, 0 fail
```

## Not done

- B2 (contributors, claims, roles, audit pages).
- B3 (Caddy serving, staging validation, screenshots on staging).
- The panel builds against the A1/A2 contract; contributor/role/audit
  endpoints arrive in B2.

## Review fixes (2026-10-27)

| Fix | Commit | Tests added (RED first, then GREEN) |
|---|---|---|
| Built HTML and the copied landing stylesheet referenced root `/assets/...`; under Caddy `handle_path /admin/*` they resolved to the API origin root and 404. Source HTML now uses `/admin/assets/...`, the build re-roots landing font `url()`s to `/admin/assets/fonts/...`, and build + check fail on any reference outside `/admin/` or one that does not resolve once the prefix is stripped. | `113e105b` | `build.test.mjs`: every asset reference is mounted under /admin and resolves after the prefix is stripped; font test now requires `/admin/assets/fonts`. `check.test.mjs`: rejects a root-absolute `/assets/` reference. |
| Wrong key (`401 admin.denied` from `POST /v1/admin/session`) showed nothing: the global 401 redirect fired and the login form skipped 401. The anonymous login call no longer triggers the session-expiry redirect; the form shows the mapped message for every failure. | `7c9595e3` | `app-login.test.mjs` (compiled `app.js` on a dependency-free DOM double, `tests/support/fake-dom.mjs`): wrong key shows the error without a redirect loop; correct key after a wrong one opens accounts; 401 on an authenticated call still returns to login. `api-client.test.mjs`: login 401 does not fire the unauthorized handler. |
| `toEndOfDayIso` sent `T00:00:00Z` (start of the UTC day): today was rejected by ENT-04's 1h minimum and the date read one day earlier in Brazil; the client upper bound used 5 calendar years instead of 5 × 365 days. Now `YYYY-MM-DDT23:59:59-03:00`, Brazilian "today" and default, and `endInstantError` checks `now + 1h ≤ end_at ≤ now + 1825 days` (inclusive, like the server). | `11fcc3b7` | `validators.test.mjs`: Brazil end of day; today late evening (< 1h) rejected, exactly 1h accepted; tomorrow accepted; instant bounds (+1h, +1825 d accepted; +1825 d 1 s, +1826 d rejected); last selectable day; leap-year range. |

Consequence of the server rule: because a picked day ends at 23:59:59 in
Brazil, the last selectable day is today + 1824 days (its end must still be
within now + 1825 days); today + 1825 days is rejected by the client exactly
as the server would reject it.

```sh
cd admin-web
npm ci && npm run typecheck && npm run build && npm run check && npm test
# typecheck OK, build OK, check PASSED; 29 tests, 29 pass, 0 fail
```
