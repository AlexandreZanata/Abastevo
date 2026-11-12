# F1 versioned settings — EMP-ORG-008

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Date: 2026-11-12
Source parent: ORG-007 `3dff15d7`, owned enterprise execution worktree,
`codex/enterprise-f0-f1`, targeting existing PR202 (`dev` → `main`).
F1 remains open. This task is not phase publication, integration, current-head/
base CI evidence, notification delivery or production acceptance.

Tested backend tree: `f3261807d723330748ae6ce33276da533c39971d`.
Tested contract Git blob: `ec266c823882afe89e743e08f5e48e8f776226a7`.
The enclosing task commit pins these runtime inputs; closure and handoff docs
do not change the tested backend tree or OpenAPI blob.

## Contract and decisions

B-BR-EMP-67/68 and BUC-EMP-34 preceded behavior. EDR-043 explicitly adopts
only the proposed settings-read row as `org-permissions-v3`: OWNER, ADMIN,
FLEET_MANAGER, SUPERVISOR, FINANCE, APPROVER and AUDITOR have ORG coverage.
The frozen v1/v2 vectors remain historical; every other v2 cell remains
unchanged. DRIVER, operations and station/data roles gain no settings access.
Management remains OWNER/ADMIN under current fresh authority and capability.

EDR-044 records immutable typed values, effective server intake, scoped routing,
actual lazy initialization and prospective default-zone ownership. It avoids
an arbitrary JSON policy bag and unscoped legacy inventory under forced RLS.
No new dependency, service, login, definer or RLS exception is introduced.

Defaults require review for trips/fuelings/expenses, one approval level and
KM_PER_L display. An empty review pair means administrator/owner fallback;
creator membership initializes platform/privacy/billing contacts. Ownership
transfer validity is fixed at seven days. Closure period defaults to thirty
days, bounded 7–90. Source policy holders retain OFFL intake window 15 days
(0–60) and clock tolerance 300 seconds (nonnegative int64), AUTH delegation
maximum 90 days (1–180), IDN invitation validity seven days (1–30) and activation
validity 48 hours (1–168). These are validated holders, not implementations of
the consuming owners' future commands. Real retention/purpose/legal admission
is not invented or enabled by a settings change.

## Persistence and effective intake

Migration `000178_organization_settings.sql` adds five forced-RLS tables:
immutable versions, current heads, immutable designation projections, original
command acknowledgements and immutable pending notification facts. Composite
keys/FKs retain scoped membership and revision references. Strict JSON shape,
fixed keys, exact numeric bounds, canonical UUID/empty slots and static SQL
refusals prevent unvalidated values and private constraint details.

Actual new onboarding creates safe defaults in its creation transaction,
using the authentic creator/operation. Older authentic roots initialize lazily
in an authorized transaction, with the actual bootstrap instant and system
source; they do not acquire an invented past author/time. Metadata-only anchors
without a genuine onboarding receipt cannot silently bootstrap. The additive
migration performs no global private-root backfill or prior-row rewrite.

Changes serialize through the current account/proof → capability → root →
membership/roles lock order. The application requires the exact request-bound
AUTH decision; the store rechecks current authority and proof after waits.
CAS, typed values, immutable author/reason, audit, head, root default and
original acknowledgement commit together. Identical retries still require
current authority and retain their original revision/root revision/server time
after later changes. Changed intent or stale revisions mutate nothing.
A late storage/commit failure returns unavailable without provisional data.

The explicit owning-workflow `CaptureSettingsTx` port requires an ordinary,
correctly scoped transaction and captures server receipt/current settings
under the same root lock held through its business mutation. It accepts no
device time or client-selected revision. Historical `SettingsRevisionTx` reads
an explicit retained revision and does not grant authority. A regressed server
clock fails closed; equal server instants are ordered by revision. Synthetic
owning-record tests exercise persisted revision/receipt/base integers without
pretending that future fuel/expense/trip intake is already implemented.

Time zone publication owns `organizations.time_zone`; it advances the root
revision only when that field changes. Existing units keep their explicit
captured zones. New/edited omitted-zone unit commands consume the new root
default. A direct unversioned zone update is refused once a settings head
exists; prior metadata-only foundations retain their existing behavior.
Display presentation converts only an already eligible owner-computed exact
ratio: KM_PER_L or L_PER_100KM; L_PER_H and unknown results retain their basis.
It neither calculates eligibility nor rewrites stored integers/arithmetic.

## Designations and reductions

Four primary/substitute pairs reference current ACTIVE scoped membership,
active employee/account and no pending account-erasure exit. They route work;
no role, permission, account access or delegation is created. The consumer
still checks authority on its actual record. Membership suspension/end clears
only that member's slots in a new system revision, preserving other members
and recording the real source member/revision. Reactivation restores none.
A durable `PENDING_NOTIFICATION_CORE` fact is retained for NOTIF-001; no
unavailable notification provider is declared delivered.

The existing account-erasure definer gets only the new projection privileges
needed by its membership-table owner. A two-organization ordinary non-login/
NOBYPASSRLS rehearsal consumes actual designations and restores the caller's
scope. That test reproduced a deferred-check failure after the definer restored
its GUC. The deferred publication check now temporarily installs the actual
inserted event's already-validated organization, checks the published head as
an invoker under forced RLS, then restores the prior GUC. No unscoped policy,
security definer or privilege bypass was added. Failure rolls back all source
membership/role changes, settings and pending notice, including on a SECURITY-
suspended root. The pending-notice guard requires an actual reduction version.

## Private entry points

Four company-cookie POST routes are composed by the actual default-off router:

- `/v1/orgs/{org_id}/settings/read`
- `/v1/orgs/{org_id}/settings/revision/read`
- `/v1/orgs/{org_id}/settings/history`
- `/v1/orgs/{org_id}/settings/change`

Reads require current `org.settings.read`; changes require fresh
`org.settings.manage` plus the organizations capability. Changes share the
existing `org.setup` synthetic 200/hour admission on every axis with onboarding
activation, rather than registering the same permission in two quota classes.
This inherited construction allowance is not a measured production limit.
Dedicated PANEL cookie, exact HTTPS Origin/Host, CSRF, no-store/no ETag,
strict complete typed input and opaque bound history cursors remain mandatory.
Clock tolerance is a canonical decimal string up to int64 max, avoiding client
floating-point precision loss. Private field/range error codes are frozen;
submitted values, SQL text and other organization's references are not exposed.

OpenAPI contains the actual guard descriptors, strict schemas and synthetic
examples for requests, acknowledgements, retained versions and pages. Actual
router/schema/security/selector drift is rejected by the contract tests.

## Validation record

Go 1.27.4, sqlc 1.31.1, staticcheck 0.8.1 with Go 1.27.1, vacuum 0.30.6,
and the owned disposable PG18/PostGIS 3.6 fixture were used. No persistent or
production database was targeted.

- Domain and codec RED → GREEN: typed safe defaults, immutable historical
  snapshots, source/author/cause/scope validation, stale and overflowing revision,
  regressed/equal clocks, exact int64 strings, explicit false/zero/empty values,
  unknown/missing/null/case/duplicate fields and exact display ratios.
- Real PostGIS/race: twelve competing CAS changes, twelve identical retries,
  serialized owning receipt versus settings, twelve designation/offboarding
  races, foreign/unavailable references, immutable publication/default-zone
  guards and rollback/recovery of audit/head/receipt/root/projections.
- Actual company HTTP: all seven explicit read roles, genuine nominated DRIVER
  denial, current cookie/Origin/CSRF/source-family revocation, disabled write
  capability with continued reads, historical/keyset/cursor isolation, allowed
  range details and late unavailable without provisional acknowledgements.
  Existing units retain zones; newly created omitted-zone units use the new
  default. Owning synthetic records retain base integers and their captured
  revision, even when a later receipt has an earlier device declaration.
- Ordinary populated additive upgrade reaches an injected failure, rolls back,
  recovers, preserves prior rows and accepts the previous image schema check.
  Forced-RLS ordinary ownership and actual-time/bootstrap source are checked.
  New onboarding still initializes atomically. A late bootstrap read/list failure
  reproduced a misleading 404, now corrected to unavailable with no provisional
  settings; recovery initializes once.
- Source reductions preserve other slots, never restore on reactivation and
  retain pending notice/cause. Ordinary two-organization erasure and GUC
  restoration pass. Injected late notice failure rolls back source membership,
  role endings and settings, including a SECURITY-suspended root.
- Full affected organization, membership, orgaccess, orgaudit, orgcapability,
  orgtime, orgvalue, companysession, orgapi, enterprisesuite, OpenAPI and httpapi
  race suites pass. Full OpenAPI suite: 262.6 seconds. After the bootstrap fix,
  the complete affected organization suite passes again with composed coverage:
  domain 90.76%, application 75.16%, adapters 77.09%, HTTP 78.68%.
- The actual full production-router/default-off contract test passes. Scoped
  vet/staticcheck/build, sqlc vet/diff pass. Canonical vacuum has zero errors,
  789 warnings and 366 informational findings; those non-error results are not
  represented as a warning-free contract.
- Plan checker: 348 tasks, 545 rules, 167 use cases, 145 screens; zero structural
  problems, three checker tests pass. Structural validity is not acceptance.
  Strict changed-file gofmt, diff, added local links and staged secret checks
  accompany the atomic task commit.

Representative commands, with the explicit disposable PostGIS DSN in the
integration environment:

```sh
GOCACHE=/tmp/enterprise-go-build go -C backend test -race -tags=integration \
  ./internal/modules/organization/... ./internal/modules/membership/... \
  ./internal/modules/orgaccess/... ./internal/modules/orgaudit/... \
  ./internal/modules/orgcapability/... ./internal/modules/orgtime/... \
  ./internal/modules/orgvalue/... ./internal/modules/companysession/... \
  ./internal/platform/orgapi ./internal/platform/enterprisesuite \
  ./internal/platform/apicontract ./internal/platform/httpapi
GOCACHE=/tmp/enterprise-go-build go -C backend test -race -tags=integration -p 2 \
  -coverpkg=./internal/modules/organization/... \
  -coverprofile=/tmp/enterprise-org008-composed.out \
  ./internal/modules/organization/... -count=1
GOCACHE=/tmp/enterprise-go-build go -C backend test ./cmd/api \
  -run '^TestFullProductionRouterEnterpriseContractAndDefaultOff$' -count=1
TZ=America/Cuiaba python3 docs/enterprise/tools/check_plan.py --write
python3 -m unittest discover -s docs/enterprise/tools -p 'test_*.py'
```

The final organization rerun is justified by the reproduced bootstrap failure
and its correction; unchanged aggregate release checks are not repeated.

## Next and retained gates

ORG-009 suspension/reactivation is the next eligible organization task,
consuming the actual ORG-003 checkpoint. The maintainer ended autonomous
progression and requested finishing only ORG-008 plus a review handoff; no next
task is started. Resume dependency order only upon a new request. F1/M1 closure, phase publication and final current-head/base CI remain
open. Actual AUTH/IDN/device providers, reviewed ordinary runtime grants,
single-family revocation choice, vault/staging/backup/native/Kotlin evidence,
GOV-014/PLAT-012 and actual receipt/purpose/legal approval remain separate.
NOTIF-001 owns pending-fact consumption and real delivery. No Android UI or
native behavior changes in this backend task; emulator use remains authorized
when an owning Android task needs it. No merge, release or deployment occurs.

## Final PR202 review corrections (2026-11-12)

The two focus points held: the deferred check after account erasure runs
under each affected organization's own scope (three-organization probe,
mid-way failure fully rolled back) and old-settings bootstrap is serialized
and returns no provisional data on late failure (30 concurrent calls after a
populated upgrade, one bootstrap version per organization). Correction: a
command re-proved availability for every designated slot, so one designee
whose account was later suspended or had a pending deletion blocked every
unrelated change with a field-less 422. Migration 000180 re-proves only
changed slots. Whether suspension or a pending deletion should clear
designations remains an owner decision. Runtime grant inventory: the role
that deletes accounts needs SELECT on `org_settings_heads`, because the
deferred publication trigger runs as the session role. [Review record](f1-pr202-review.md#final-review-head-45230e28).
