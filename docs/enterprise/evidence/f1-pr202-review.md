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

- **Required integration check failed on the delivered head.** The backend
  `integration` job (real PostGIS, race) for `b739b2c6` failed. The orgmedia
  S3 tests in `backend/internal/modules/orgmedia/adapters/storage_integration_test.go`
  require `ANPFUEL_TEST_STORAGE_*`, which the job did not provide. Fixed
  without skipping them: `.github/workflows/backend.yml` adds the reviewed
  MinIO image from `compose.validation.yml` as a service, with its default
  bucket. The same image and entrypoint were run locally: orgmedia, and
  `cmd/api` on its real-S3 path, passed against it.
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

## Second review (head 1f99b018)

Date 2026-11-11, same PR202 (`dev → main`), head `1f99b018` over base
`ef96cd13`. It covered the whole cumulative diff, with emphasis on the
commits after `1034b410`: EMP-PLAT-009/010/011/013/014/015, EMP-PRIV-003,
EMP-ORG-001 and the home map translations. Three independent read-only
reviews (events/telemetry/quotas, Android vault/foundation, exact
values/time/organization) were confirmed in the source before acceptance.
The fixes follow as new commits; published history is unchanged.

Findings and resolution:

- **High, fixed.** The required Android `test` check failed on the head
  (`SecurityConfigurationTest` read only the first backup exclusion; the
  vault exclusions were inserted before it). The PRIV-003 record never ran
  `:app:testDebugUnitTest`. The test now checks every exclusion in any order
  and the actual `no_backup/enterprise-sealed-v1/` path.
- **Medium, fixed.**
  - `orgevent.SafeError` turned registered refusals (CONFLICT, ...) into a
    retryable 503; registered codes now pass bare.
  - Personal REVOKED/REUSE refreshes wrote a permanent enterprise denial
    marker for accounts without any vault; now nothing is written.
  - Organizations accepted zones operation time refused (GMT, EST, Japan,
    Factory, host-only localtime/posixrules); both use `kernel.LoadTimeZone`.
  - Unrecoverable vaults (invalidated or wrong key, corrupted file, policy
    change) had no recovery and unlock-required was a generic outage; both
    are typed, and an unrecoverable vault restarts after acknowledgement.
  - After a reboot, offline expiry never discarded the key (BR-PRIV-018);
    wall time past the validity now erases it.
- **Low, fixed.** Retry-After from the first exhausted axis only; the quota
  clock comment; ambiguous append-only TRUNCATE assertions; the zero Go
  instant as a server receipt; organization redaction through `%d` and
  nested JSON; Kotlin accepting SystemV/ zones; unbounded organization names;
  rotation fencing handles; the default session fence; a sticky cleanup
  flag; logout success shown after a failed durable clear; an allocation
  failure crashing the vault; nondeterministic process/lifecycle tests; the
  PLAT-011 record counting undiscovered tests.
- **First-review items now fixed.** Per-member/organization transfer limits
  and one full JPEG decode per process; tab-return focus; the mobile
  overview link; the `FRESH_AUTH_REQUIRED` message; the dead `_headers` file
  and its check; immutable licence texts; `Last-Modified` on no-store pages;
  the Caddy check in Quick verification.
- **Accepted and documented.** Snapshot rollback under the same key by
  someone who can write app files (root or a debuggable build); access tokens
  issued in the verifier's future stay refused, so replicas need synchronized
  clocks; DST gap/overlap policy waits for the first local-boundary builder;
  an unknown discount skips reconciliation, so EMP-FUEL must map "no
  discount" to zero.

Dependency audit: `npm audit` against registry.npmjs.org now passes TLS but
receives HTTP 403 from a network filter page. The GitHub Advisory Database,
which backs npm audit, reports zero advisories for each of the 21 locked
packages (`typescript@7.0.2` and its 20 platform packages).

Still open, with owners: single-family account session revocation for panel
sign-in (AUTH/IDN: a company session requires its source family to stay
live), real AUTH/IDN providers, a non-bypass runtime login with grants, the
route gate on the composed router with the first business route, a
scheduled vault expiry sweep and erasure on an authoritative device-revoked
contact, native device acceptance, upload audit/orphan cleanup
(EMP-DOC-014), staging bucket lifecycle versus private originals,
backup/restore roles for forced RLS, EMP-PLAT-012 (EMP-GOV-014), aggregate
Kotlin domain coverage below 90%, and the legal receipt gates. F1/M1 remain
open; the state stays INTEGRATION_PENDING.

## EMP-ORG-002 review reconciliation (head 7af7d2e5)

2026-11-11. The owner requested implementation above Claude's second-review
corrections and publication to the same PR202. Actual history contains the
13 correction/documentation commits from a56b25ce through 7af7d2e5; these
fixes are already published, not newly claimed by ORG-002. The supplied
remaining-items list matches the open gates above. None is falsely closed
by organization persistence.

ORG-002 preserves the corrected shared zone profile, bounded names and
redacted domain encodings, mapping storage fields explicitly. Its new root
FKs are initially deferred so the existing audit reservation can precede
root creation; a real ordinary-role audited transaction proves this order,
replay and rollback. Audit, capability, event, kernel/time, database and
migrator regressions pass against disposable PostGIS under race detection.
See [task acceptance](f1-org002-organization-persistence.md).

Still owed: AUTH/IDN's single-family decision/current providers, production
non-bypass login/grants, the complete-router gate with the first business
route, server partition inventory and scheduled vault sweep/device-revoked
contact cleanup, deferred native acceptance, EMP-DOC-014 upload audit,
staging lifecycle/backup-role proof, PLAT-012/GOV-014, aggregate Kotlin
coverage and legal receipt acceptance. F1/M1 remain open and the cumulative
PR remains INTEGRATION_PENDING. This task performs no deployment or merge.

## Final review (head 45230e28)

Date 2026-11-12, PR202 (`dev → main`) at head `45230e28` over base
`ef96cd13`, after the remote checks for that head had passed (Quick
verification, fast, test, integration). It covered the whole PR with
emphasis on the batch after `1c7fd66b`: EMP-ORG-002..008, EMP-IDN-001..003
and EMP-AUTH-001..004. Four independent reviews (identity/membership/
erasure, authorization/composition, ORG-002..005, ORG-006..008) reproduced
each finding with probe tests on disposable PostGIS before acceptance. No
critical or high defect was found. Fixes are new commits; history is kept.

ORG-008 focus: the deferred check after account erasure runs under each
affected organization's own scope, and a mid-way failure rolls everything
back; old-settings bootstrap is serialized, idempotent under 30 concurrent
calls after a populated upgrade, and returns no provisional data on late
commit failure.

Fixed:
- **Medium.**
  - Company reads took exclusive account and root locks and serialized per
    organization and account; legal, structure and employee reads now share
    them (settings reads stay exclusive because they can bootstrap).
  - One designee made unavailable later (suspended account, pending
    deletion) blocked every unrelated settings change (000180).
  - Ending a never-accepted invitation closed the employee's team periods
    (000179).
- **Low.**
  - All-zero CNPJ accepted (domain and 000181; root `00000000` stays valid).
  - Zero-width and decomposed variants bypassed unit, cost-centre and team
    name/code uniqueness.
  - Account binding allowed at an INVITED transition (000182).
  - Retroactive and already-ended role periods accepted (000183).
  - Last-owner/overlap guards safe only at READ COMMITTED; `WithOrgTx` now
    pins it and `ValidateOrgTx` refuses other levels.
  - Organization creation validated the body before authentication and had
    no write admission.
  - Fresh proof missing for exports, closure and grant acceptance keys.
  - The route gate could not tell an AUTH-bound route from a raw one.
  - Unexpected personal deletion failures returned 401; suspension and
    deletion commit returned raw owner-fence errors.
  - An erasure scope assertion that could not fail.

Not fixed here, with owners:
- **Activation blocker:** the organization creator is the sole OWNER and
  cannot delete the personal account until ownership transfer or the
  BR-ORG-028 abandoned-setup closure exists. Do not enable the plane for
  real accounts before that.
- The pending-setup ceiling is still enforced only at the OWNER insert; an
  early check needs a runtime-callable definer.
- Keyed fingerprints for CPF and employee contact replay digests need a
  dedicated versioned key with stored key IDs (privacy/retention work).
- Whether suspension or a pending deletion should clear settings
  designations is an owner decision.
- Runtime grant inventory for the ordinary role, including SELECT on
  `org_settings_heads` for the account-deletion role and the same owner for
  the definer-checked tables; future manager commands must keep the
  account-before-root lock order.
- The earlier gates remain: AUTH/IDN providers, single-family revocation,
  vault expiry sweep and device-revoked erasure, DOC-014, staging bucket and
  backup roles, PLAT-012/GOV-014, aggregate Kotlin coverage, legal receipt
  gates.

Native Android: the four enterprise instrumented tests (real AndroidKeyStore
vault sealing, revocation, managed-unlock key and Room partition isolation)
passed on an API 35 x86_64 emulator with a temporary device PIN. This is an
emulator with a software keystore, not StrongBox or a physical device, so
native acceptance stays open.
