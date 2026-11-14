# Abastevo Empresas web shell

EMP-IDN-016 extends EMP-PLAT-008 with the C1 employee and invitation panel.
EMP-ORG-014 adds persisted onboarding, activation and versioned settings.
This is a separate,
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
is supplied. Dist contains only 25 allowlisted static files. Credentials,
source maps, TypeScript and package metadata are excluded. No `_headers`
file ships: Caddy ignored it and served it as a public file. The artifact
check reads the served company policy from the staging web Caddyfile.

TypeScript 7.0.2 is the existing admin build pin (Apache-2.0, development
only). The package lock graph is identical after replacing package
identity; no version or dependency edge is added. This reuse provides the
needed compiler without a UI framework, bundler or browser-test dependency.
The compiler runs only in the build/test process. Package audit could not
complete locally: the configured mirror has no security-advisory endpoint,
and the official npm endpoint is blocked by the workstation's network.
This is recorded as unavailable, not a successful vulnerability scan.

## Authentication and routing

Entries are /empresas/, /empresas/entrar/, /empresas/criar/,
/empresas/organizacoes/{canonical_org_uuid}/ and its /configuracao/ child.
A path never grants access.
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
Visibility return rechecks authority without a loading state: the current
view stays until the server answers, an unchanged answer keeps focus, the
pending choice and the live region, and any refusal still clears it. Local
expiry timers do not send heartbeats or refresh authentication.
FRESH_AUTH_REQUIRED from the company plane asks for a new sign-in instead
of reporting an invalid key. A failed server logout clears local
state and reports that server confirmation failed. The logout request uses
keepalive and no page signal, so navigation or pagehide cannot cancel it.
Choosing a company in the selector changes nothing until "Abrir empresa" is
used, because arrow keys on a closed select fire change on every step.

## Membership integration

MembershipSource in src/controller.ts is the current-authority port. The shipped
main.ts now uses CurrentMemberships and the actual POST
/v1/org-access/contexts/list and /contexts/read routes. Every page is bound to
the signed-in account; canonical organization/membership references, pagination
progress and selected membership are checked. Empty eligible pages advance by
the server's last scanned reference. Failed or mixed-account responses never
become a partial organization list. Reset, logout and current refusals clear
the in-memory directory.

The selected context response supplies current UI permission/grant hints and
the configured invitation validity. These hints do not authorize a command.
Every actual employee, invitation and acceptance endpoint repeats its source
permission, fresh-proof, capability, target and revision checks. The UI lists
all supported C1 role/scope templates with plain descriptions, disabling any
combination absent from the current server hint; OWNER is not treated as a
hierarchy that can grant every other role. Managed credentials have no controls.

Employees can be filtered by the server's actual lifecycle states, opened and
created. Current access state is separate descriptive source metadata: an
ACTIVE employee record does not imply ACTIVE access. Likely duplicate contacts
are shown alongside the draft and require explicit review before overriding.
The panel never creates an account while creating an employee record. The basic C1 form leaves the optional home unit unset. Existing unit links are
shown as linked, without inventing a unit name or exposing an editable UUID.
Named unit/team selection and import remain with their scoped structure/import
UI; no unit/team names or relationships are inferred.

Invitation history contains state, grants, issuer membership, expiry and failed
attempts, without credentials. Issue/resend show the response's one-time code
and fragment-only link in a temporary delivery section. Copy needs a user
clipboard gesture; printing uses the current document's restricted print view.
Closing, refreshing, switching, session loss or logout clears delivery. A lost
write acknowledgement retries the immutable operation and body: metadata-only
replays cannot restore or derive delivery. Resend acknowledgement immediately
shows new delivery and removes the old pending entry while refreshing its
SUPERSEDED source metadata. A failed history refresh preserves already
acknowledged delivery and does not replay that write. Expired/revoked tokens
use a new issue; only usable pending invitations offer resend/revoke.

Accepted invitations expose the account alias and manager-confirmation state
through the actual acceptance read route. Confirm/reject require explicit
review and the server's exact revision. A conflict clears the pending decision
and requires a new read; it never automatically approves a later revision.

Every navigation/switch clears the prior scope immediately, aborts prior work
and advances a generation guard. Late responses cannot repopulate it. Business
requests share the memory-only CSRF transport, no-store/redirect policy and
bounded JSON handling. A business-plane 401/fresh refusal resets the outer
shell immediately; a permission refusal clears its private records. Successful
business requests move the idle deadline and the shell timer rechecks that
current deadline before expiring an active panel. No heartbeat extends proof.

## Caddy and delivery

The existing staging web Caddyfile serves the separate empresas/ directory
in the signed landing bundle. The Kubernetes ConfigMap mirrors that exact
file. Company HTML/errors are no-store without ETag, with strict CSP,
no-referrer and noindex, and carry no Last-Modified validator. Conditional
HTML requests cannot return a stale 304 shell. Only content-fingerprinted
assets are immutable; the unhashed font licence texts revalidate hourly. Public/admin
routes and the API's independent default-off switch retain their owners.

Quick verification selects company typecheck/build/check/tests and the
actual Caddy check (tests/caddy.py) for business-web, its landing asset
inputs, the staging web Caddyfile and its ConfigMap. CI requires Docker for
it; a local run without Docker prints that it was not run. The staging workflow packages
the built shell, but this source task does not activate APIs or deploy.
Rollback uses the preceding signed static bundle; account data and schema
are preserved.

## Browser and proxy acceptance

After building, from the repository root:

```sh
# Requires an explicit disposable loopback ANPFUEL_TEST_DATABASE_URL,
# Python Playwright/Chromium, psycopg, Go and openssl; caddy.py needs Docker and PyYAML.
GOCACHE=/tmp/enterprise-go-build python3 business-web/tests/browser.py
python3 business-web/tests/caddy.py
```

The browser script starts its own loopback HTTPS proxy and actual Go API,
clears inherited ANPFUEL configuration, applies migrations only to the
explicit disposable database, disables external discovery and creates a
synthetic accounts. Migration setup uses the explicitly supplied disposable
owner connection; the API uses a generated NOSUPERUSER/NOBYPASSRLS/NOINHERIT
login with explicit per-table C1 grants and no table ownership. An exact
operator capability fixture is seeded only for the organization created by
this test; business commands still run through the actual HTTP/source gates.
The runtime role and its grants are removed after the API stops.
Local certificates/credentials and process logs are
temporary and never committed. It verifies real company issuance/logout,
cookie reuse refusal, reload/restored-page guards, real keyboard selection,
scope isolation and desktop/mobile layouts. Separate shell-race tests inject authority only in that local test browser.
The employee/invitation flow uses the actual account/context/employee APIs,
resend/revoke, privileged WAITING acceptance, manager confirmation/rejection,
keyboard actions, responsive layout and memory cleanup. The clock regression
checks that continued business activity preserves the session past its original
idle deadline; it does not claim fresh authentication was renewed.

The Caddy test uses the reviewed digest-pinned Caddy 2.10.2 image of the
staging/production edge, starts and removes only its uniquely named
loopback container, verifies ConfigMap equality (PyYAML) and exercises
actual responses. The browser script stays local: it needs Python
Playwright/Chromium, Go and a disposable PostGIS, which CI does not
provision. Neither script targets staging, production or an existing user
browser.
[Executed evidence](../docs/enterprise/evidence/f1-plat008-business-web-shell.md).


## Onboarding and settings

The isolated setup view confirms the current company session before using the
organization ID as a selector for the protected settings read. It does not
invent an operational context from a creation acknowledgement. The source read
returns current profile/state/checklist, settings-specific hints and distinct
root/settings revisions in one scoped transaction. Historical settings are
shown separately; opening history also refreshes a complete current snapshot.
Name/kinds are editable in the creation draft and read-only once persisted;
timezone changes use the existing versioned settings command.

Creation, activation and settings retries capture immutable operation IDs and
bodies. A received write acknowledgement followed by a failed current read has
a bound read-only retry, rather than a new write. Unknown write outcomes retain
the original intent. Own-account CREATED-receipt discovery can resume a saved
setup after a lost response/reload, filtering every candidate through current
scoped read authority. No private browser persistence is introduced. The operator
still controls default-off capability; the view truthfully distinguishes saved,
pending, suspended and active states. Only source-pinned synthetic terms/data
are enabled; this does not certify real-data/legal or production readiness.

All current settings and history are exposed, including the confirmation policy,
exact int64 clock-skew string and named primary/substitute memberships. The
bounded picker uses current source eligibility and saves recheck it. Read-only
members have no write controls. Stale revision conflicts require explicit
reload/new decision; source revalidation preserves dirty values while marking
newer revisions stale. Quiet tab return preserves focus on unchanged proof and
clears private state on authority loss, including late-response races.

The pre-existing settings digest omitted invitation-confirmation policy.
EMP-ORG-014 corrects it with a complete v2 digest. Immutable legacy receipts
replay only when their original settings revision proves the entire same intent;
flipped confirmation flags are refused without rewriting history.

The HTTPS browser harness creates and removes only its own fresh database and
ordinary NOSUPERUSER/NOBYPASSRLS runtime role on an explicitly supplied disposable
loopback PostGIS control connection. It exercises actual APIs and source data;
operator capability changes are explicit synthetic fixture actions. Caddy tests
verify the exact restricted deep links/security headers against both policy
representations, leaving unknown suffixes as real404 responses. Task evidence:
[EMP-ORG-014](../docs/enterprise/evidence/f2-org014-onboarding-settings-panel.md).
