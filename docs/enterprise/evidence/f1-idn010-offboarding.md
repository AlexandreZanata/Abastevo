# EMP-IDN-010 — Scoped offboarding and personal self-leave

Date: 2026-11-12.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Source parent: `63b9365ad719da468c222266c51c86d26154819a`.
Owned branch: `codex/enterprise-idn010`; publish to maintained dev in cumulative
[PR203](https://github.com/AlexandreZanata/Abastevo/pull/203).
[Task207](https://github.com/AlexandreZanata/Abastevo/issues/207) stays open
until actual protected integration. B-BR-EMP-80/81, BUC-EMP-41 and EDR-052
precede implementation. This is the existing-account ORG_F1 backend checkpoint;
F1/M1 and the uninstalled producers below remain open.

Four actual POST routes use dedicated company-cookie, origin/CSRF and fresh
source proof:

- `/v1/orgs/{org_id}/memberships/{membership_id}/offboarding/read`
- `/v1/orgs/{org_id}/memberships/{membership_id}/offboarding/complete`
- `/v1/orgs/{org_id}/memberships/{membership_id}/leave/read`
- `/v1/orgs/{org_id}/memberships/{membership_id}/leave`

Manager operations retain the literal fresh OWNER/ADMIN membership permission
and request-bound AUTH lease. Self-leave uses a fixed narrow personal-entry
descriptor, never an invented role permission or a general AUTH exception. It
works for the selected account's suspended or no-role membership and for exact
ended retries while current account/proof and original binding still hold.
The source proves binding before exposing private root facts. Other accounts,
unknown targets and unready foreign roots receive not-found. Cookie/origin/CSRF
failures remain the exact existing boundary codes, and revoked source families
deny even an otherwise valid receipt. Managed provenance fails closed.

UUID-sorted account locks precede source proof and root locks; proof is checked
again after waits and before the acknowledgement. Server inventory contains
actual current team links and settings designations only. A digest binds source
keys/revisions and employee/membership revisions; every item requires an
explicit pending-manager flag. More than500 items fails closed without a
truncated review. Existing optional designations need no fabricated successor;
the settings/team APIs still own subsequent assignment decisions. Concurrent
source changes invalidate review. Retained flags remain readable after the
source reductions and are not a successful work closure or reassignment.

One transaction ends the membership with OFFBOARDED or LEFT, ends original
roles through the installed policy, applies the existing invitation/admission,
team, settings and ownership-proposal reducers, and records actual audit,
immutable acknowledgement, source flags and pending notice. Managers explicitly
retain or terminate the employee at its reviewed revision; self-leave cannot
terminate it. A prior MANAGER/EMPLOYEE_INACTIVE suspension cause survives.
Genuine SQL owner/future coverage prevents the last owner from being removed,
including concurrent offboarding. Personal families/keys/private data and other
organizations remain independent. A target's next ordinary request is denied;
the existing ended-membership resolution returns404 without private details.

Migration191 adds FORCE-RLS immutable commands/notices, one ending command per
membership period, server timestamps and exact audit/state/source guards.
Settings flags bind the actual immediately following membership-reduction
revision; team flags bind the exact membership-ended revision. No earlier
migration or source row is rewritten. Deferred audit/receipt/notice failures
roll back membership, roles, employee and projections and return no result.
Original actor/intent-bound receipts retain the reviewed reason, cause, employee
choice, flags and time rather than reconstructing a later state.

ORG_F1 has no managed provisioning, corporate device registry, delegation or
operational-work producer. Responses explicitly identify MANAGED_ACCESS,
DEVICE_ENROLMENTS, DELEGATIONS, OPERATIONAL_WORK and LATE_INTAKE as pending
sources; no live device/sync/trip inventory or successful revocation is invented.
Those producers must integrate their actual transactional reductions, required
successors and late-intake review before activation. Native erasure/lock,
notification delivery and managed-account disabling are not certified here.
Notices remain PENDING_NOTIFICATION_CORE; handover flags remain PENDING_MANAGER.

Validation (repository root; disposable PostGIS URL supplied outside Git,
writable Go/staticcheck caches):

```sh
go -C backend test ./internal/modules/membership/domain -run '^TestOffboarding' -count=1
go -C backend test -race -tags=integration ./internal/modules/organization/adapters -run '^TestOffboarding' -count=1
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/membership/... ./internal/modules/orgaccess/... ./internal/modules/organization/adapters ./cmd/api -run 'Test(Offboarding|MembershipAccess|Membership|Invitation|Redemption|Ownership|Role|FullProductionRouterEnterpriseContractAndDefaultOff)' -count=1
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/membership/adapters -count=1
go -C backend test ./internal/modules/membership/domain -cover -count=1
go -C backend vet -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/enterprisesuite ./internal/platform/apicontract ./internal/platform/orgapi ./internal/platform/orgtest ./cmd/api
go -C backend test ./internal/modules/membership/... ./internal/modules/organization/adapters ./internal/platform/enterprisesuite -count=1
# From backend/, compatible analysis toolchain:
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/enterprisesuite ./internal/platform/apicontract ./internal/platform/orgapi ./internal/platform/orgtest ./cmd/api
# Pinned Go1.27.4 directory prepended to PATH:
make quick-verify
bash scripts/check-compat.sh
python3 docs/enterprise/tools/check_plan.py
python3 scripts/tests/test-enterprise-local-sql.py
python3 scripts/tests/test-enterprise-protection.py
git diff --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

Domain RED preceded GREEN. Tests cover exact review/choice, missing or stale
flags, actual settings/team source history and retained flag reads, twelve-way
same-operation and distinct-revision contention, last-owner races, suspended and
no-role self-leave, foreign/unready-root secrecy, original retries, revoked proof,
observed root-wait ageing, deferred rollback/recovery and ordinary populated
190→191 failure/recovery, compatibility, RLS and retention. Actual composed HTTP
validates strict/null/unknown intent, schemas, cookie/CSRF/origin, two-organization
independence, immediate denial, surviving personal session and ended retry.
The actual production router and the contract guard retain default-off behavior
and reject bare/missing-selector routes. Membership domain coverage:91.8%.

The initial general run caught a missing inline handover example field. Actual
HTTP also exposed using an object-only decoder for the stored flags array;
the SQL read now returns a typed object, retaining strict decoding. Boundary
expectations were corrected to the existing ended404 and personal-CSRF403
contracts, with explicit denial/schema checks retained. No gate or assertion was
removed. Full local Quick then passed in166s, including all backend units,
build/vet/format/staticcheck, sqlc vet/drift, OpenAPI, deployment harnesses,
secret checks and govulncheck (no vulnerabilities found). Plan:348 tasks,
0 problems; compatibility and both enterprise harnesses pass. OpenAPI has zero
errors; warnings remain visible. Affected race regression passed, including
organization in161.813s. Final source binding hardening subsequently passed the
complete offboarding PostGIS/HTTP suite in18.515s and affected unit/vet/static
checks; unrelated aggregate tests were not repeated for that narrow fix.

Recovery preserves accepted reductions and history: retry an aborted migration,
or disable these entries while retaining membership/roles, employee choice,
source flags, audit and notices. Never run a destructive down migration or
restore ended grants/tokens to retry a command. External providers/runtime grants,
legal receipt, staging and production acceptance remain separate. No merge,
wiki, deployment, emulator/native acceptance or F1/M1 completion is claimed.

Main protection reverified before publication: strict required `Quick verification`
and administrator enforcement enabled. No protection was changed or bypassed.

Tested backend tree: `312293cf96e1729328ea6d9e6379548f1b6f526a`.
Tested contract Git blob: `3aadb41b3a0bf9fcf7c873cc1db3320dc05fa0ea`.
Required remote Quick verification awaits the publication head.
