# Abastevo Empresas web shell

EMP-PLAT-008: LOCAL_DONE / INTEGRATION_PENDING. This is a separate,
framework-free static TypeScript build under /empresas/, using the approved
[company surface](../docs/enterprise/evidence/f0-m0-decisions.md#company-web-surface)
and [EMP-PLAT-007 session](../docs/enterprise/evidence/f1-plat007-company-web-session.md).
It is not an admin panel or a station portal.

## Local checks

From this directory, with Node >=20:

```sh
npm ci
npm run typecheck
npm run build
npm run check
npm test
```

The build extracts existing landing tokens and self-hosted font declarations,
checks font provenance, preserves OFL notices and fingerprints the complete
ES-module graph after rewriting imports. Fonts and brand assets are read-only
inputs from landing/static/assets. No runtime dependency, remote font,
inline script, service worker, persisted account proof or client data cache
is supplied. Dist contains only 20 allowlisted static files. Credentials,
source maps, TypeScript and package metadata are excluded.

TypeScript 7.0.2 is the existing admin build pin (Apache-2.0, development
only). The package lock graph is identical after replacing package
identity; no version or dependency edge is added. This reuse provides the
needed compiler without a UI framework, bundler or browser-test dependency.
The compiler runs only in the build/test process. Package audit could not
complete locally: the configured mirror has no security-advisory endpoint,
and the official npm endpoint is blocked by the workstation's network.
This is recorded as unavailable, not a successful vulnerability scan.

## Authentication and routing

Entries are /empresas/, /empresas/entrar/ and
/empresas/organizacoes/{canonical_org_uuid}/. A path never grants access.
Unknown routes/assets are real Caddy 404s. The shell accepts no query
credentials. Full loads and restored pages require login because the
company CSRF value is held only in memory.

The login form accepts the existing account key. The key field has no form
name and the submit button stays disabled until the module mounts, so a
native submission can never carry the key. Before sending the key, the
client posts an empty credential-free read: 401/403 means the company plane
is on, anything else stops sign-in, so a disabled plane never receives the key
or causes a new account session family. It then POSTs to
/v1/accounts/keys/login and exchanges the fresh account family/access proof
at /v1/org-access/session/login. It keeps only the company account, CSRF
and expiry metadata in private memory. Account refresh tokens are not used.
It never reads or writes the HttpOnly cookie. Read/logout use only the
company paths and X-CSRF, with same-origin credentials, no-store,
redirect refusal and a ten-second transport deadline. Successful response
JSON is bounded at 64 KiB and must carry no-store. Errors display fixed copy.

The guard verifies current company session state before membership work,
rejects changed accounts and clears private UI on expiry/reload/logout.
Visibility return rechecks authority; local expiry timers do not send
heartbeats or refresh authentication. A failed server logout clears local
state and reports that server confirmation failed. The logout request uses
keepalive and no page signal, so navigation or pagehide cannot cancel it.
Choosing a company in the selector changes nothing until "Abrir empresa" is
used, because arrow keys on a closed select fire change on every step.

## Membership integration

MembershipSource in src/controller.ts is an explicit current-authority port,
not an implemented membership API. Its list snapshot must match the
authenticated account, contain canonical nonduplicate organization/member
references and bounded display names. authorize must confirm that same
account, organization and membership at each selection.

Production main.ts composes unavailableMemberships until the owning AUTH/IDN
tasks provide the real server-backed adapter. It shows an unavailable state
after a successful real login. It makes no guessed organization request,
seeds no organization and never treats missing integration as an empty list.
The picker, empty/loading/denied/ready states and race isolation are tested
through injected synthetic membership ports. There is no production demo
mode or browser injection hook.

Every navigation/switch clears the prior scope immediately, aborts prior
work and advances a generation guard. Late responses cannot repopulate it.
Even a listed organization is not rendered until current authorize succeeds.
Future module requests still need server-side permission checks.

## Caddy and delivery

The existing staging web Caddyfile serves the separate empresas/ directory
in the signed landing bundle. The Kubernetes ConfigMap mirrors that exact
file. Company HTML/errors are no-store without ETag, with strict CSP,
no-referrer and noindex. Conditional HTML requests cannot return a stale
304 shell. Only fingerprinted static assets are immutable. Public/admin
routes and the API's independent default-off switch retain their owners.

Quick verification selects company typecheck/build/check/tests for both
business-web and its landing asset inputs. The staging workflow packages
the built shell, but this source task does not activate APIs or deploy.
Rollback uses the preceding signed static bundle; account data and schema
are preserved.

## Browser and proxy acceptance

After building, from the repository root:

```sh
# Requires an explicit disposable loopback ANPFUEL_TEST_DATABASE_URL,
# Python Playwright/Chromium, Go, openssl and Docker with local Caddy 2.10.2.
GOCACHE=/tmp/enterprise-go-build python3 business-web/tests/browser.py
python3 business-web/tests/caddy.py
```

The browser script starts its own loopback HTTPS proxy and actual Go API,
clears inherited ANPFUEL configuration, applies migrations only to the
explicit disposable database, disables external discovery and creates a
synthetic account. Local certificates/credentials and process logs are
temporary and never committed. It verifies real company issuance/logout,
cookie reuse refusal, reload/restored-page guards, real keyboard selection,
scope isolation and desktop/mobile layouts. Organization-ready UI tests
inject synthetic authority only in that local test browser.

The Caddy test starts and removes only its uniquely named loopback
container, verifies ConfigMap equality and exercises actual responses.
Neither script targets staging, production or an existing user browser.
[Executed evidence](../docs/enterprise/evidence/f1-plat008-business-web-shell.md).
