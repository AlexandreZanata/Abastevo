# Business web (Abastevo Empresas)

Static company web app served at `/empresas/` from the signed landing bundle.
An existing Abastevo account signs in, picks one of its organizations and works
in it: onboarding and settings, employees and invitations, the mission planning
board, and the checklist/incident panel. Every read and command goes to the
backend's company plane (`/v1/org-access/*`, `/v1/orgs/{org_id}/*`), which
decides all authority. It is not the admin panel (`admin-web/`) or a station
portal.

## Layout

| Path | Contents |
|------|----------|
| `src/main.ts`, `src/app.ts` | Entry point and shell (routing, session guard, idle expiry). |
| `src/api.ts` | Transport: same-origin, memory-only CSRF, no-store, redirect refusal, 10 s deadline, bounded JSON. |
| `src/controller.ts`, `src/company.ts`, `src/model.ts` | Membership source port, current memberships and role catalog, identity-carrying values. |
| `src/employees.ts` + `panel.ts`, `src/setup.ts` + `setuppanel.ts`, `src/missions.ts` + `missionpanel.ts`, `src/checks.ts` + `checkpanel.ts` | Feature parsers/controllers (employees and invitations, onboarding and settings, missions, checklists and incidents) and their DOM panels. |
| `static/` | `index.html` shell, `404.html`, `robots.txt`, assets. |
| `scripts/build.mjs`, `scripts/check.mjs` | Build (landing tokens/fonts, fingerprinted ES-module graph, allowlisted dist) and dist verification. |
| `tests/*.test.mjs` | Node suites: parsers and controllers against in-memory synthetic servers. |
| `tests/*.py` | Local-only browser and edge checks (below). |

Routes: `/empresas/`, `/empresas/entrar/`, `/empresas/criar/`,
`/empresas/organizacoes/{org_uuid}/` and its `configuracao/` child; anything
else is a real 404.

## Commands

```sh
npm ci
npm run typecheck
npm run build
npm run check
npm test
```

`make quick-verify` runs these, plus `python3 business-web/tests/caddy.py`, when
`business-web/`, its landing asset inputs or the staging web Caddyfile/ConfigMap
change. `release-staging` copies `dist/` into the landing bundle's `empresas/`.

Local-only checks (never target staging, production or a user's browser):

```sh
python3 business-web/tests/caddy.py           # pinned Caddy image; needs Docker and PyYAML
python3 business-web/tests/browser.py         # real Go API + HTTPS proxy; Playwright/Chromium,
                                              # psycopg, Go, openssl, disposable loopback
                                              # ANPFUEL_TEST_DATABASE_URL
(cd business-web && npm run build && python3 tests/missionboard.py)   # synthetic routes
(cd business-web && npm run build && python3 tests/checkpanel.py)     # synthetic routes
```

`browser.py` migrates only the explicit disposable database, runs the API as a
generated NOSUPERUSER/NOBYPASSRLS login with explicit grants, and removes it
afterwards.

## Rules

- TypeScript `7.0.2` only (dev dependency); no framework, bundler, runtime
  dependency, inline script, service worker, remote font or `_headers` file.
  Security headers and cache rules live in `infra/caddy/staging-web.Caddyfile`.
- The URL never grants access. UI permission hints from `contexts/read` only
  shape controls; the server rechecks every command.
- No credential in the URL, web storage or a client cache. The CSRF value and
  company session metadata live only in memory; full loads and restored pages
  require a new sign-in. The client never reads the HttpOnly cookie and never
  uses account refresh tokens.
- Sign-in probes the company plane with a credential-free request first, so a
  disabled plane never receives the account key.
- Every navigation or organization switch clears the previous scope and
  discards late responses. Logout, expiry and authority loss clear private
  state; a 401 or fresh-proof refusal resets the shell.
- Commands carry one immutable operation ID and the revision the user saw. An
  unknown outcome is retried only as the same operation; `CONFLICT` re-reads,
  never auto-retries.
- Shown state is the latest server read; a pending command never changes it.
- Private document content is fetched only through the audited content path
  and handed to the browser as a download, never kept.

See [enterprise](../docs/enterprise/README.md) and
[security and privacy](../docs/security/security-privacy.md).
