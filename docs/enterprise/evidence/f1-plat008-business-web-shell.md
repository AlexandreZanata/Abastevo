# F1 EMP-PLAT-008 business web shell

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Delivery state INTEGRATION_PENDING; F1/M1 remain open.
The owner authorized the next enterprise task and GitHub publication.
The next bounded task is EMP-PLAT-008 above published \`56549a5e\`.
The clean isolated \`codex/enterprise-f0-f1\` worktree matches PR202's dev
head; main \`ef96cd13\` is an ancestor. Occupied local dev and all other
worktrees are preserved. Delivery reuses
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202), dev to main.

## Delivered boundary

B-BR-EMP-19, B-BR-EMP-20 and BUC-EMP-11 were written before behavior.
The separate framework-free TypeScript business-web build provides
/empresas/, /empresas/entrar/ and canonical organization deep-link routing.
It reuses landing design tokens, brand assets and self-hosted OFL fonts.
The build checks font provenance, preserves font licenses and fingerprints
modules after rewriting their imports, including transitive dependencies.

The account-key form uses existing account login and exchanges fresh proof
for EMP-PLAT-007's company cookie. It holds company CSRF/account/expiry in
private memory only. It keeps no refresh token, account proof or private
cache. No admin endpoint, admin CSRF, unsafe DOM sink, console logging,
service worker or client persistence is used.

Private POSTs use fixed same-origin endpoints and credentials, no-store,
no redirects, no referrer and ten-second transport cancellation. Successful
JSON is bounded at 64 KiB and must carry no-store. Errors expose fixed copy.
Expiry, changed account, session refusal, full reload or restored page
clears private state. Visibility return rechecks current authority; the
local expiry timer sends no heartbeat. Logout clears scope immediately and
reports server failure without claiming confirmed revocation.

The explicit MembershipSource port separates current memberships from
account authentication. A list snapshot must match the current account,
contain canonical nonduplicate scopes and valid membership references.
Selecting even a listed organization requires a current authorize result
for that exact account/organization/membership before rendering. Switching
or logging out immediately clears prior content and aborts work. Generation
guards prevent late responses from restoring an earlier organization.

The production composition deliberately has an unavailable membership
provider until AUTH/IDN implements its server-backed adapter. It sends no
guessed organization endpoint request and renders no demo organizations.
Real login works with the company API enabled, then shows unavailable
membership integration. Synthetic providers exist only in unit tests and
the explicitly local browser fixture. This source acceptance covers the
shell/port, not actual organizational membership delivery.

The native labeled selector works by keyboard. Loading, choose, empty,
ready, denied, unavailable and signing-out states are explicit. Live status,
labels, focus, semantic headings and responsive layouts support accessible
entry/switching. Only current confirmed scope is rendered.

The staging web Caddyfile and its byte-identical decoded Kubernetes
ConfigMap serve the new directory in the signed landing bundle. Known
organization paths receive only the shell; arbitrary paths/assets return
real 404s. HTML/errors are no-store, no ETag, strict CSP, no-referrer and
noindex. Conditional HTML requests cannot produce stale 304 shells.
Fingerprinted static assets cache independently. Existing public/admin
security policies and API routing ownership are preserved.

Quick verification classifies business-web changes and runs its bounded
typecheck/build/artifact/test checks; landing input changes also select
them. The signed staging bundle workflow packages the company build.
There is no deployment, live flag change, new backend service, migration,
runtime dependency or business feature route in this task.

Tested source blobs: model \`50c762d9031a211db513ceb9d156dfac45fc02cf\`,
API \`5116b8564ed969d7e1c29a32934518bc2e39dba0\`,
controller \`a369ed6f902812cc1aefb5bc642d8ee28996167c\`,
app \`9cdd0c86eda68f1a5651009d42bdd8afabe7d0a6\`,
build \`2e6f83d1022d59e83292ee83bfdc7f77a347456b\`,
artifact checker \`edd27e19dc4a5d07c1e00f723be21a4b95f589a8\`,
Caddy \`d1a3b6345cd825e0f2571f68d2cd9f9e781348ac\`.
Implementation, tests and evidence are committed atomically.

## Executed validation

Linux amd64, Node 26.3.1, npm 12.0.2, TypeScript 7.0.2, Go 1.27.4.
The unique tmpfs Compose project enterprise-plat008-20261111 supplied
pinned PostGIS 18/3.6 on ephemeral loopback port 32818. All SQL writes used
disposable databases and synthetic accounts. Browser/API/proxy/Caddy
processes were created locally and stopped by their harnesses.

- Initial tests failed before the route/model source existed. Green route
  and membership cases reject foreign namespaces, malformed/nil IDs, query
  credentials, mixed accounts and duplicate organization references.
- \`npm ci --ignore-scripts --no-audit --no-fund\`, typecheck, build,
  artifact check and all ten Node tests passed. Deterministic rebuild
  preserved fingerprints and removed stale output. Injected hash drift,
  source leakage and unmounted resource references were refused.
- Session tests proved exact account-to-company proof exchange, CSRF
  transport, no-store, private-field nonserialization, account substitution
  and CSRF/session refusal, exact idle expiry, invalid/expired/malformed
  response rejection, oversized JSON, late login cancellation and truthful
  failed logout.
- Controller tests exercised current per-selection authorization, empty
  versus unavailable integration, foreign membership/account refusal,
  immediate content clearing, A-to-B switching, ignored late A response,
  source abort, revocation and logout while a load was pending.
- \`ANPFUEL_TEST_DATABASE_URL=<disposable-loopback-dsn> GOCACHE=/tmp/enterprise-go-build python3 business-web/tests/browser.py\`
  passed against the actual API, HTTPS loopback and real PostGIS. It
  created a synthetic account, submitted the real key form, issued the
  dedicated secure/HttpOnly/Strict cookie, read session metadata, logged
  out and refused cookie reuse. Reload did not recover private UI/CSRF.
  No credential appeared in DOM text or client storage.
- The same actual Chromium run injected synthetic membership ports only
  in its isolated fixture. Native keyboard open/choose/confirm switched A
  then B; main content contained no A data afterward. Foreign authority
  hid scope, authoritative empty state rendered correctly, restored pages
  cleared state, focus and a single visible h1 were verified. Desktop
  1440x1000 and mobile 390x844 had no horizontal overflow.
- Real-browser RED caught the missing border-box reset in the standalone
  responsive shell. The reset fixed it without changing shared landing CSS.
  Local screenshots were inspected for login/workspace in both sizes:
  /tmp/enterprise-plat008-{login,workspace}-{desktop,mobile}.png.
  Only synthetic company labels were used; screenshots are not committed.
- \`python3 business-web/tests/caddy.py\` passed against local Caddy 2.10.2
  image \`sha256:c3d7ee5d2b11f9dc54f947f68a734c84e9c9666c92c88a7f30b9cba5da182adb\`.
  It verifies the decoded ConfigMap equality and actual 308/200/404,
  conditional/no-store/noETag HTML, immutable hashed assets, strict company
  security headers, unchanged public/admin policies and no static API proxy.
  RED found global deferred header precedence overriding company policy.
  Separate company/ordinary policy matchers fixed it.
- Existing companysession domain/application/HTTP/PostGIS adapters passed
  \`go test -race -tags=integration ./internal/modules/companysession/... -count=1\`
  with the explicit disposable DSN. No Go production source changed.
- Four quick-classification/selection tests, three CI-cadence regressions,
  four Kubernetes manifest tests, release workflow YAML parsing and shell
  syntax checks passed. Enterprise verifier/regressions, local links,
  whitespace and changed/new-file secret review passed.

## Dependency review and external limitation

The only dependency is the existing admin build's development-only
TypeScript 7.0.2, Apache-2.0. Normalized lock graphs are identical; no version
or edge was added. No install script was executed for the local install.
The build does not ship the compiler or dependencies and uses only local
assets with strict CSP. This records need, license and execution surface
review for the reused compiler rather than adding a new toolchain.

Online npm advisory audit is UNAVAILABLE: the configured mirror returned
404 NOT_IMPLEMENTED; the official npm bulk advisory endpoint returned the
workstation network's blocking 403. No known advisory was suppressed and
no clean vulnerability-scan result is claimed. Online audit should run on
an allowed network before final release acceptance. This external signal
does not invalidate the executed source/build/browser checks.

## Remaining ownership and delivery

AUTH/IDN owns real membership listing and current organization authority;
business modules own private screens and operations. Real-data/legal,
ordinary runtime-role, remote checks/review, rollout and production gates
remain owed. This scaffold does not activate company APIs, station portal,
Android enterprise mode, real receipts or any deployment.

F1/M1 remain open. Next bounded task EMP-PLAT-009 upon authorization.
Publication reuses PR202, preserving its earlier review and atomic commits.
Required remote checks/review must validate the new head before protected
integration. This evidence claims no merge, CI result or VPS deployment.

## PR202 review corrections (2026-11-11)

The [PR202 review](f1-pr202-review.md) found and fixed defects behind some
statements above:

- The form could submit natively with a named key field, so the key reached
  the URL when the module was missing. It is now unnamed with a disabled
  submit button until mount, and the artifact check refuses regressions.
- Every sign-in leaves an account session family that is not revoked and
  counts against the ten-family budget for 30 days. Sign-in now stops before
  sending the key when the company plane is off. With the plane on, the
  family remains; single-family revocation is an activation blocker.
- Arrow keys on the closed selector switched companies. Switching now needs
  "Abrir empresa".
- Navigation or pagehide could cancel the server logout. Logout now uses
  keepalive without a page signal.
- `AbortSignal.any` broke sign-in on older browsers; a fallback combines the
  signals.
- A refused sign-in now returns focus to the key field, and the refusal is
  announced once.
