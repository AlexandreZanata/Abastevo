# B2 — Admin web panel: contributors, roles, audit

Date: 2026-10-27. Branch `codex/adm-b2-admin-web-contributors` from B1
(`codex/adm-b1-admin-web`). PR stacked on #170, which merges only after
#164. Built against the A3/A4 admin API contract with fake `fetch` in
tests; no backend code copied or changed in this branch.

## Delivered

- `contributors/` view: GitHub snapshot (repo, fetched-at, stale badge,
  refresh, per-login link/claim state), claims queue (state filter,
  cursor paging, per-claim reason + override checkbox, approve/reject with
  confirm), direct link form (account, login, reason, override), profile
  hide/unhide form (account, reason).
- Roles section on the account detail: assign form (role select, optional
  end date, reason, confirm) plus the assignments created in the session
  with inline revoke. The contract has no list-roles-by-account endpoint,
  so pre-existing assignments are found through the audit view; the panel
  never invents endpoints.
- `audit/` view: action/target filters, list, cursor paging.
- Header nav (Contas, Contribuidores, Auditoria) intercepted with
  `pushState`: full loads would wipe the in-memory CSRF token.
- API client: snapshot/claims/approve/reject/link/hide/unhide/roles/audit
  endpoints, `Retry-After` kept, error mapping extended with
  `contributor.*`/`staff.*` codes.
- Defensive normalization: admin approve/reject/hide responses use the
  backend's capitalized field shapes while list views use lowercase; the
  client accepts both and emits the documented lowercase view.
- Validators: GitHub login rules, role enum, claim-state filter.
- Build emits the shell at 4 routes; check asserts all pages, refs and the
  new view ids; 21 tests green.

## Local integration (A4 backend, disposable PostGIS, synthetic data only)

Backend from `codex/adm-a4-staff-roles` with the `contributorService.Runner`
fix, fresh disposable database (two synthetic accounts, one seeded pending
claim, one seeded snapshot row, one seeded profile), memory-only test key.
Driven through the real panel in headless Chrome:

```text
login 200 → whoami 200 → snapshot 200 → claims 200 → approve 200 →
link 201 → hide 200 → unhide 200 → search 200 → detail 200 →
roles 201 → revoke 200 → audit 200 → logout 204
```

Screenshots (synthetic data only): `b2-01-contributors.png` …
`b2-04-audit.png` in this directory. Reused cookies after logout return
401 (covered in B1).

## Backend issue found by this proof (fixed on the A4 branch, not here)

Contributor admin mutations (approve/reject/link/hide) returned 503:
`cmd/api/main.go` never set `contributorService.Runner`, leaving the
shared admin audit/idempotency helper unreachable. Fixed in A4 commit
`bd7cc4fd` ("fix(api): wire contributor admin mutations through the shared
audit helper"); this B2 run proves approve/link/hide/assign end to end.
The A3/A4 integration suites cover the service layer with injected
runners; the missing piece was composition-only.

## Commands and results

```sh
cd admin-web
npm ci && npm run typecheck && npm run build && npm run check && npm test
# 21 tests, 21 pass, 0 fail
```

## Not done

- B3 (Caddy serving, staging validation, staging screenshots).
- A5b (same staging script on real staging, with B3).
- Pre-existing role assignments have no list endpoint in the contract, so
  the detail page shows session-scoped assignments plus audit history.

## Review fixes (2026-10-27)

| Fix | Commit | Tests added (RED first, then GREEN) |
|---|---|---|
| Merged the B1 review fixes (`/admin/assets` mount, login 401 error, Brazil end-of-day grant date). `scripts/check.mjs` keeps B2's contributors/audit page inventory and uses B1's mount resolver instead of B2's `basePath` option; the mount checks cover every B2 page and nav link, and role end dates use the fixed `toEndOfDayIso`. | `0dccf49b` (merge) | B1 tests run on B2: mount/resolve test over all dist HTML/CSS, `app-login.test.mjs`, ENT-04 validator tests. |
| `resetRoleEvents()` was never called: account A's assignment row (and its "Revogar papel" button) stayed visible on account B. The list now resets when a different account's detail renders and when the session ends; an assignment answered after switching accounts is not rendered on the new account. | `f38d2650` | `app-roles.test.mjs`: switching accounts clears A's events and revoke form; reloading the same account keeps them; late assignment response not shown on B; logout clears them. |
| Paging: "Carregar mais" stayed enabled in flight (double click appended a page twice), and audit "more" re-read unapplied filter inputs with a cursor sealed to the previous filter (`400 admin.invalid`). One request per list: "more" disabled while loading, duplicate "more" ignored, a first-page load supersedes an in-flight one and drops its stale page; audit keeps the filter its cursor was issued for. | `defc6ea4` | `app-paging.test.mjs`: claims and audit double click append once with the button disabled; audit "more" keeps the issued filter; applying a filter re-seals the cursor; a filter applied while "more" is in flight drops the stale page. |
| Tests only: capitalized Go-default response normalization was covered for reject, not for approve or profile hide/unhide. | `e8341918` | `api-client.test.mjs`: approve with a capitalized and a lowercase claim under `claim`; profile hide and unhide with capitalized and lowercase shapes (mutation-checked: dropping a capitalized key fails them). |

App-level tests drive the compiled `app.js` through the dependency-free DOM
double `admin-web/tests/support/fake-dom.mjs` (ids taken from the shipped
shell).

```sh
cd admin-web
npm ci && npm run typecheck && npm run build && npm run check && npm test
# typecheck OK, build OK, check PASSED; 48 tests, 48 pass, 0 fail
```
