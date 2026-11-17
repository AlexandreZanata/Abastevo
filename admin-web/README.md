# Admin web

Static owner/staff panel for the backend admin API (`/v1/admin/*`): login,
accounts, grants and staff roles, contributors, station lookup, forum and
audit. It is served at `/admin/` from the signed landing bundle and calls only
same-origin relative `/v1/admin/...` paths. It is not the company app
(`business-web/`).

## Layout

| Path | Contents |
|------|----------|
| `src/api.ts` | Admin API client (DOM-free, unit-tested). |
| `src/validators.ts` | Form validation (DOM-free, unit-tested). |
| `src/app.ts` | Page wiring and rendering. |
| `static/app/index.html` | Single shell served for every admin route. |
| `static/assets/admin.css` | Admin-only rules using the landing tokens. |
| `static/404.html`, `robots.txt`, `_headers` | Noindex fallback, disallow-all, CSP/cache rules. |
| `scripts/build.mjs`, `scripts/check.mjs` | Build (tsc, fingerprinting, landing styles/fonts/brand copy) and dist verification. |
| `tests/` | Node test runner suites. `dist/` is the gitignored output. |

## Commands

```sh
npm ci
npm run typecheck
npm run build
npm run check
npm test
```

`make quick-verify` runs these when `admin-web/` changes; `release-staging`
copies `dist/` into the landing bundle's `admin/` directory. The API mounts the
admin routes only when `ANPFUEL_ADMIN_KEY_SHA256` and `ANPFUEL_ADMIN_ORIGIN`
are set (optional network fence: `ANPFUEL_ADMIN_ALLOWED_CIDRS`); the raw key
comes from `ops admin keygen` and never enters Git.

## Rules

- Plain TypeScript `7.0.2` (strict), no framework and no runtime dependency.
- Every root-absolute reference starts with `/admin/`; build and check fail
  otherwise.
- Render data with `textContent`/`createElement` only.
- The CSRF token lives in a module variable, never in web storage. In-app
  navigation uses `history.pushState`; every full load starts at login. Any
  401 on an authenticated call returns to `/admin/login/`.
- Every mutation sends `X-Admin-CSRF` and an `Idempotency-Key`.
- The admin key is never displayed, logged or stored; the input is a cleared
  `type=password` field with autocomplete off.

See [security and privacy](../docs/security/security-privacy.md) and the
[operations runbooks](../docs/operations/).
