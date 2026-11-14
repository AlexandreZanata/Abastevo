# EMP-IDN-013 — Atomic work-contact change and admission invalidation

Date: 2026-11-12.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Source parent: `cce39312a2aaeb28340c6a6cd85622a440a6dfb9`.
Owned branch: `codex/enterprise-idn013`; publish to maintained dev in cumulative
[PR203](https://github.com/AlexandreZanata/Abastevo/pull/203).
[Task210](https://github.com/AlexandreZanata/Abastevo/issues/210) stays open until
protected integration. B-BR-EMP-85/86, BUC-EMP-44 and EDR-055 precede behavior.
This is one existing-account backend checkpoint; F1/M1 remains open.

## Behavior and source reuse

Reuse `POST /v1/orgs/{org_id}/employees/{employee_id}/change` with the normal
company cookie, origin/CSRF, current fresh idn.employee.manage, capability/root
configuration-write policy and employee admission quota. It replaces the full
profile at its exact profile revision; omitted optional fields clear their
values. OpenAPI now describes the actual contact effects rather than incorrectly
promising that every profile edit leaves outstanding admission unchanged.

The existing domain canonicalizer and profile revision/lifecycle rules remain
unchanged. Trimmed lower-case ASCII work e-mail and trimmed international work
phone are descriptive organization data, never personal account proof. Changing
or clearing a stored canonical contact triggers the owning invitation/admission
projection in the same transaction. Equivalent canonical contacts and unrelated
metadata leave invitations pending. No parallel contact route/store is added.

Existing migrations188/189 already retain CONTACT_CHANGED, source PROFILE,
actual employee ID and profile revision, revoke PENDING invitations, end WAITING
acceptances/reservations and create pending notification facts. ACTIVE memberships
and roles, accounts, personal families and other organizations remain unchanged.
The manager reissues explicitly, creating a distinct reservation after the old
one ended. Exact original employee receipts return original profile revisions
without reapplying a contact reduction to a newer invitation. Original acceptance
receipts remain historical facts; replay cannot restore ended admission.

## Corrected failure and atomic boundary

A real PostGIS RED test blocks the profile update after the entry authority
check, observes the actual ChangeEmployeeProfile lock wait and advances across
the real15-minute freshness boundary. The parent head acknowledged the contact
change despite expired proof. The correction rechecks the owning authority
under retained account/proof/capability/root locks after projection/audit/receipt
work and before returning an acknowledgement. Receipt replay also rechecks after
its lookup. GREEN returns no provisional result and rolls back profile,
invitation/admission reductions, notices, audit and receipt together.

A second real test blocks EmployeeCommandReceipt during an exact retry, observes
the lock and freshness boundary, then verifies refusal and byte-identical source
history, including a newer pending invitation. Both tests pass in9.179s. There
is no clock bypass in production, no sleep-based assumption that a lock occurred,
and no new schema, dependency, applied-migration edit or runtime grant.

## Validation commands

From the owned repository, pinned Go1.27.4, disposable PostgreSQL18/PostGIS3.6,
ordinary non-owner/non-BYPASSRLS runtime, synthetic data only. The disposable
database URL is supplied through ANPFUEL_TEST_DATABASE_URL; Go/staticcheck caches
are writable and outside Git.

```sh
go -C backend test ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/apicontract ./internal/platform/enterprisesuite ./internal/platform/orgapi
go -C backend test ./internal/modules/membership/domain -cover
go -C backend test -race -tags=integration -timeout=10m ./internal/modules/organization/adapters -run TestContactChange -count=1
go -C backend test -race -tags=integration -timeout=10m ./internal/modules/organization/adapters -run 'TestContactChange|TestInvitationOrdinary|TestRedemptionOrdinary' -count=1
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/membership/... ./internal/modules/organization/... ./cmd/api -count=1
# Added final blocked-receipt proof test after the aggregate was started:
go -C backend test -race -tags=integration -timeout=5m ./internal/modules/organization/adapters -run 'TestContactChangeFreshProofAfter(Receipt|Projection)Wait' -count=1
go -C backend vet -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/apicontract ./internal/platform/enterprisesuite ./cmd/api
# From backend/, compatible installed analyzer toolchain:
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/apicontract ./internal/platform/enterprisesuite ./cmd/api
make quick-verify
bash scripts/check-compat.sh
python3 docs/enterprise/tools/check_plan.py
python3 scripts/tests/test-enterprise-local-sql.py
python3 scripts/tests/test-enterprise-protection.py
git diff --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

Initial source acceptance passes in18.478s: e-mail, phone, clearing, equivalent
canonical contact, metadata and no-admission edits; retained source revisions;
old-token refusal; original receipt after explicit reissue; waiting/active and
two-organization/personal independence; twelve identical retries (one mutation)
and twelve distinct operations (one winner); four deferred audit/employee-receipt/
invitation-notice/admission-notice failures with complete rollback and recovery.
The final scoped recovery passes in33.077s, including all contact tests,
foreign/non-manager/disabled write and retry refusals, actual HTTP,
contact/redeem/resend/confirm contention and populated ordinary source-migration
upgrade/recovery/RLS/retention. The complete affected aggregate ran membership,
organization and API with race detection: membership25.663s and API68.910s pass;
organization434.595s reports only one new fixture failure. Its snapshot was taken
before a legitimate audited capability-disable command, incorrectly attributing
that audit to denied employee writes. The corrected snapshot starts after that
committed audit; exact before/after source comparison and every denial assertion
remain. The entire contact/source-upgrade suite was then repeated successfully
as above, plus final integration-tag staticcheck. All other aggregate cases
passed; no production code, gate, test or assertion was removed. There is no
second aggregate run for this fixture-only correction.

New HTTP acceptance uses real accounts/cookies and the complete composed router.
It proves cookie/origin/CSRF, missing/null/unknown/revision intent, foreign
employee denial, deferred failure without token revocation, schema conformance,
actual CONTACT_CHANGED provenance, old secret rejection after a prior preview,
original retries preserving a later invitation, DRIVER refusal, surviving
personal proof and two current organization memberships, Retry-After and reads
remaining available after write quota. Fixture-only API/path/source-envelope and
expected capability revision errors were corrected before final acceptance;
there is no production route added to satisfy a mistaken test. An uncommitted
resend plan is not treated as a published invitation in the race assertion.

Full local Quick verification passes in265s, including backend units, build/vet,
format/staticcheck, sqlc vet/drift, OpenAPI, deployment harnesses and secret
checks. Govulncheck reports no vulnerabilities. OpenAPI has zero errors, with834
warnings and486 informs visible. Membership domain coverage is91.2%; scoped
integration-tag vet/staticcheck and the final added proof test pass. Compatibility,
plan348 tasks/0 problems and both enterprise harnesses pass. Production behavior
did not change after Quick; final test-only changes pass scoped verification.
Main protection was reverified: strict required Quick verification, administrator
enforcement enabled; no protection was changed or bypassed.
Tested backend tree: `2613623f72e0db99cd161a2638053da8a63e2bfe`.
Tested contract Git blob: `380a1abc5de141693c35a7af1304102d54b9f3a7`. Required remote Quick
verification awaits the published head.

## Retained obligations and recovery

Disable writes or retry an aborted transaction while preserving accepted contact
revisions, revocations, ended periods, receipts and audit. Never reactivate an old
token or restore active/personal credentials as rollback. No migration is needed;
existing populated source migration tests remain part of affected regression.
Managed activation/code producers are uninstalled behind GOV-013/IDN-006..008;
they must consume this contact source before activation. This checkpoint does
not fabricate a code inventory or certify that future integration. Notification
delivery, native/device acceptance, providers/runtime grants, legal/staging and
production gates remain open. No main merge, wiki, deployment or emulator/native
acceptance is claimed.
