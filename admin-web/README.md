# abastevo — Admin Web Panel

Static owner panel for the **abastevo** administrative API (Front B).

## Stack

- Plain static HTML5 + TypeScript **7.0.2** (`strict: true`), no framework,
  no runtime dependency. Node built-in test runner only.
- Visual standard identical to the landing: same CSS tokens, Manrope +
  Source Sans 3 local fonts. `scripts/build.mjs` copies
  `landing/static/assets/styles.css` and the hash-verified fonts into
  `dist/` at build time; admin-only rules live in
  `static/assets/admin.css` and may only use existing tokens.
- All data is rendered with `textContent`/`createElement`. The CSRF token
  lives in a module variable only (never `localStorage`/`sessionStorage`).
  Because a full page load would wipe it, both admin routes serve the same
  shell and in-app transitions use `history.pushState` without reloading;
  every full load starts at the login view. Every mutation sends
  `X-Admin-CSRF` and `Idempotency-Key`. Any 401 on an authenticated call
  returns to `/admin/login/`; a 401 from the login call itself (wrong key)
  stays on the form and shows the invalid-key message.
  The raw admin key is never displayed or logged; the login input is
  `type=password`, `autocomplete=off`, cleared after submit.

## Layout

```text
admin-web/
  static/
    app/index.html          # Single shell served at every /admin/ route
    404.html                # Plain noindex fallback
    robots.txt              # Disallow everything (noindex defense in depth)
    _headers                # Admin CSP, no-store HTML, immutable assets
    assets/admin.css        # Admin-only rules (existing tokens only)
  src/
    api.ts                  # Admin API client (DOM-free, unit-tested)
    validators.ts           # Form validation (DOM-free, unit-tested)
    app.ts                  # Page wiring (DOM)
  scripts/
    build.mjs               # tsc + fingerprinting + asset copy
    check.mjs               # dist integrity, headers, budgets
  tests/                    # Node built-in test runner
  dist/                     # Deployable artifact (gitignored)
```

## Commands

```bash
npm ci
npm run typecheck
npm run build
npm run check
npm test
```

`dist/` is served from the API origin at `/admin/*` (Front B3) with the
`/admin` prefix stripped before the file lookup (Caddy `handle_path`). Every
page and stylesheet therefore references `/admin/assets/...` (including the
font URLs re-rooted from the landing stylesheet) while the files live at
`dist/assets/`; `build` and `check` fail on any reference outside `/admin/`.
The panel calls relative `/v1/admin/...` paths with
`credentials: 'same-origin'`.
