# PR202 review of EMP-PLAT-005 to EMP-PLAT-008

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Delivery state INTEGRATION_PENDING; F1/M1 remain open.
Reviewed PR202 (`dev → main`) at head `b739b2c6` over base `ef96cd13`:
`5b692fb5` (EMP-PLAT-005), `277fd470` (EMP-PLAT-006), `56549a5e`
(EMP-PLAT-007) and `b739b2c6` (EMP-PLAT-008), plus their interaction with
the earlier reviewed range through `68c1d1fa`. The fixes follow as new
commits on the same branch; published history is unchanged.

Method: the backend capability and company-session code was read directly.
Two independent read-only reviews covered private originals and the company
web shell/Caddy. Every finding below was confirmed in the source before it
was accepted, and every fix has a test that failed before it.

## Findings

High:

1. **Account key could reach the URL.** `business-web/static/index.html`
   (login form). The form had no method and the key input was named, so a
   native submission (module blocked, not yet loaded, JS disabled) sent
   `GET /empresas/?account-key=<key>` into history and sync. Fixed: unnamed
   field, `method="post"`, submit disabled until the module mounts, artifact
   check refusing either regression, Chromium check with the module blocked.
2. **Panel sign-in exhausts the account session budget.**
   `business-web/src/api.ts` (`signIn`) with
   `backend/internal/modules/account/adapters/pgstore.go` (`TryCreateFamily`).
   - Each sign-in calls key login, which creates a session family. Nothing
     revokes it: the panel drops the refresh token, and the only revoke
     endpoint revokes every session.
   - Ten live families in 30 days block every new login for that account,
     including Android, with 429.
   - It happened even with the company plane off, which is the default.
   - Mitigated: sign-in first sends a credential-free read and continues
     only on 401/403. With the plane off, the key is never sent and no family
     is created.
   - Open: with the plane on, each sign-in still holds a family for 30 days.
     Single-family revocation (AUTH/IDN) blocks activation.

Medium:

3. **Keyboard switching changed context.** `business-web/src/app.ts`
   (selector `change`). Arrow keys on a closed select fire `change`, so each
   step opened another company (WCAG 3.2.2). Fixed: an explicit
   "Abrir empresa" button opens the chosen company.
4. **Logout could be cancelled.** `business-web/src/controller.ts`
   (`signOut`). The logout request carried the page signal, so popstate,
   pagehide or reset aborted it and hid the failure. Fixed: no page signal,
   and `keepalive` lets it finish after pagehide.
5. **Sign-in failed on older browsers.** `business-web/src/api.ts`.
   `AbortSignal.any` is missing before Safari 17.4, Chrome 116 and
   Firefox 124; the TypeError became "Não foi possível entrar". Fixed: a
   fallback combines the page and timeout signals.
6. **Substituted originals looked like outages.**
   `backend/internal/modules/orgmedia/application/service.go` (`Upload`,
   `Open`). Storage integrity failures became `ErrUnavailable`. Put never
   replaces an object, so retries could never succeed and nothing
   distinguished the case. Fixed: `ErrIntegrity` is kept for both paths.
7. **One transfer pool for every organization.** Same file,
   `NewService`/`enter`. Four slots per process, shared by all
   organizations and both directions, reject immediately. One member holding
   slow uploads blocks everyone until the 15-second read deadline. Open: there
   is no route yet; the upload route task must add per-member limits.

Low:

8. `backend/internal/modules/evidence/adapters/storage/server_put.go`:
   `PresignServerPUT` signed any namespace, which contradicted the package
   rule that finals and quarantine never use this path. Fixed: only `org/`.
9. The orgmedia unit fixture used one constant ID, so grant/read audit
   linkage was untested. Fixed: distinct IDs. Each read is now audited with
   its grant ID and its own request ID.
10. After a refused sign-in, focus was lost because the field was disabled
    while busy, and the refusal was announced twice. Fixed.
11. Open, for the owning document/route tasks:
    - Audit records an access attempt even when the final recheck then
      refuses delivery, and the grant ID of a read is only in its digest.
    - Each upload and each read fully decodes the JPEG. A single-scan
      progressive four-component image can take about 200 MB.
    - Uploads are not audited, and a post-PUT recheck failure leaves an
      object with no record.
    - Token issue time rejects issuers whose clock is ahead.
12. Open, for the panel:
    - Returning to the tab reloads and moves focus.
    - The overview link is hidden at 850 px and below.
    - `FRESH_AUTH_REQUIRED` shows the invalid-key message.
    - `check.mjs` asserts `_headers`, which Caddy ignores, and the Caddy and
      Chromium scripts are not part of CI.
    - Unhashed font licence files are cached as immutable, and the no-store
      HTML still sends Last-Modified.

Confirmed without change:

- Capability flags: default-off checks, `FOR SHARE` write locks against
  revision-checked updates, forced RLS and audited replay.
- Company session:
  - Exact Origin/Host, CSRF and `__Host-` cookie attributes.
  - Snapshot-bound issuance under account and family locks.
  - Rotation, admission, separation from admin, and private no-store errors.
- Freshness follows BR-AUTH-010: an access token issued less than 15 minutes
  ago, so an account refresh renews it.
- Migrations 000166 and 000167 are additive and keep floor `000132`. 000167
  builds a unique index on `account_session_families` without
  `CONCURRENTLY`, a brief write lock at deploy.
- Caddy:
  - Company pages get a strict CSP, no-store and no ETag.
  - Conditional requests cannot produce a 304.
  - Public and admin policies are unchanged.
  - The ConfigMap is byte-identical to the Caddyfile.
- No migration number collides with another branch.

## Validation (current execution, 2026-11-11)

Linux amd64, Go 1.27.4, Node 20+ with the existing TypeScript 7.0.2 lock.
Disposable Compose project `enterprise-review202-20261111`: PostGIS 18/3.6
and MinIO from `infra/compose.validation.yml`, tmpfs, loopback ports only.

- `go test -race -tags=integration` on head `b739b2c6`:
  - Passed: orgcapability, orgmedia, companysession, orgaudit, database,
    migrate, kernel, orgapi, apicontract, httpserver, httpapi, config,
    account, admin, privacy, evidence storage and `cmd/api`.
  - `cmd/worker`: five tests need the pinned Rust quality/sanctions CLIs,
    which are not installed here (environment). Its unit tests passed.
- After the fixes:
  - The orgmedia and storage unit tests, with `-race`, pass with the new
    regressions; they failed before the fixes.
  - The real MinIO substitution test now also checks the service result.
- Panel:
  - `tsc --noEmit`, build, artifact check and 13 Node tests passed. The new
    tests failed against the previous sources.
  - Chromium against the actual API and disposable PostGIS passed. It covers
    the blocked module, refused-login focus, keyboard opening, A/B
    isolation and the restored page.
  - The Caddy 2.10.2 check passed.
- Online npm audit is still unavailable. The configured mirror
  (registry.npmmirror.com) has no audit endpoint, and from this network
  registry.npmjs.org fails TLS verification ("unable to get local issuer
  certificate"); verification was not bypassed. The lock is unchanged:
  TypeScript 7.0.2 and its platform packages, dev-only, no runtime
  dependency. Run `npm audit` on a network that reaches the official
  registry.

## Remaining before activation or VPS use

- Single-family account session revocation for panel sign-in.
- Real AUTH membership and authority providers.
- A non-bypass runtime login for `WithOrgTx`; staging uses a superuser.
- Organization routes, with the route gate run on the composed router.
- Per-member transfer limits.
- The staging MinIO bucket's one-day lifecycle rule would delete private
  originals that share the bucket.
- Backup/restore roles need BYPASSRLS or superuser for forced-RLS tables.
