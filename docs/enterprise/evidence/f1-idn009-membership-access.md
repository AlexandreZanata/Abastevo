# EMP-IDN-009 — Membership suspension and reactivation

Date: 2026-11-12.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Source parent: `621b676787782602e955119298e87bfa65633899`.
Owned branch: `codex/enterprise-idn009`; publication: maintained `dev`,
[PR203](https://github.com/AlexandreZanata/Abastevo/pull/203).
[Task206](https://github.com/AlexandreZanata/Abastevo/issues/206) remains open
until protected integration. This request closes one backend task only.

B-BR-EMP-78/79, BUC-EMP-40 and EDR-051 precede implementation. IDN-006..008
still require GOV-013; this unblocked task does not provision managed accounts.
The literal OWNER/ADMIN `idn.membership.manage` matrix governs all three fresh
company-cookie routes:

- `POST /v1/orgs/{org_id}/memberships/{membership_id}/read`
- `POST /v1/orgs/{org_id}/memberships/{membership_id}/suspend`
- `POST /v1/orgs/{org_id}/memberships/{membership_id}/reactivate`

Commands require schema version, operation UUID, current membership revision
and an exact nonblank reason of at most 280 Unicode characters, excluding
control/format characters. Sorted account locks precede retained source proof,
capability and root locks. Authority is current after waits and immediately
before acknowledgement creation. Private lookup and immutable receipt replay
follow actual authorization. Self-suspension fails, and the real database
last-owner/future-coverage policy remains authoritative under contention.
Read and reduction remain available under the existing disabled-capability and
archived/suspended-root semantics; reactivation is a configuration write.

Suspension records MANAGER, preserves original role periods and denies the
target's next organization request. Reactivation requires SUSPENDED, an active
employee/account and at least one original assignment valid at server time.
Expired, ended and future assignments are not extended or recreated. Ordinary
AUTH candidates continue to require ACTIVE membership; the new explicit
reactivation-period port supplies data only. An ended membership is never
resurrected. Existing reductions consume issuer invitations, waiting admission,
team links, settings designations and ownership proposals; reactivation restores
none of them. Personal families and other organizations remain independent.

Migration190 adds scoped FORCE-RLS immutable commands and source-backed pending
notices. Guards bind the exact resulting state/revision, audit actor, action,
target and request hash. Server timestamps, original reason and prior suspension
cause survive authenticated retries after later state changes. No account,
session or tombstone selector appears in the response. Notification transport
is not claimed: notices remain PENDING_NOTIFICATION_CORE. A deferred SQL,
audit, receipt or notice failure rolls back all effects and returns no result;
late database failures report unavailable rather than a false missing target.
Migration189 and every earlier published migration remain byte-identical.

Tests include twelve-way identical retries, competing revisions, concurrent
last-owner suspension, revoked/current/fresh proof, observed capability/root
waits, foreign and missing targets, disabled capability, finite expired and
future assignments, inactive employee, no-role reactivation, consumed source
projections, original acknowledgements and deferred rollback/recovery. The
ordinary populated189→190 rehearsal injects migration failure, verifies no
partial schema/ledger/history change, recovers idempotently and confirms
previous-image compatibility, FORCE RLS and immutable retention. Actual HTTP
uses genuine account families and company cookies, validates response schemas,
strict/null/unknown intent, personal session survival, cross-organization
independence, immediate denial, quota Retry-After and failure recovery.
Contract drift checks reject bare routes and missing membership selectors.

Validation commands (repository root; disposable database URL supplied outside
Git, writable Go/staticcheck caches):

```sh
go -C backend test -race -tags=integration ./internal/modules/organization/adapters -run '^TestMembershipAccess(Through|Deferred)' -count=1
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/membership/... ./internal/modules/orgaccess/... ./internal/modules/organization/adapters -run 'Test(MembershipAccess|Membership|Employee|Assignment|Role|Ownership|Invitation|Acceptance|CompanyAuthority|Fresh)' -count=1
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/membership/... ./internal/modules/orgaccess/... ./cmd/api -count=1
go -C backend test -race -tags=integration -timeout=20m ./internal/platform/apicontract -run 'TestMembershipAccess|Test.*Production.*Router|Test.*Default.*Off' -count=1
go -C backend test ./internal/modules/membership/domain -cover -count=1
go -C backend vet -tags=integration ./internal/modules/membership/... ./internal/modules/orgaccess/... ./internal/modules/organization/... ./internal/platform/enterprisesuite ./internal/platform/orgtest ./internal/platform/apicontract ./cmd/api
# From backend/, using the compatible analysis toolchain:
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/membership/... ./internal/modules/orgaccess/... ./internal/modules/organization/... ./internal/platform/enterprisesuite ./internal/platform/orgtest ./internal/platform/apicontract ./cmd/api
# Pinned Go1.27.4 toolchain directory prepended to PATH:
make quick-verify
bash scripts/check-compat.sh
python3 docs/enterprise/tools/check_plan.py
python3 scripts/tests/test-enterprise-local-sql.py
python3 scripts/tests/test-enterprise-protection.py
git diff --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

The prior621b6767 Quick verification failed solely on formatting in
`ownership_transfer_test.go`; this delivery applies the pinned Go1.27.4
formatter without changing assertions. Initial integration fixtures needed the
real capability revision and existing producer-table grants. A late HTTP fault
then exposed the outage/missing-target error mapping; the mapper was corrected
and the actual HTTP/deferred-failure cases passed. No assertion or gate was
weakened. Domain RED preceded GREEN; membership domain coverage is 93.6%.
The full local Quick verification passed in270s, including all backend unit
tests, build/vet, format, staticcheck, sqlc vet/drift, OpenAPI, deploy-agent and
manifest harnesses, secret scans and govulncheck (no vulnerabilities found).
Affected integration-tag vet/staticcheck, compatibility and both enterprise
guard harnesses also pass; plan verification reports348 tasks and0 problems.
The complete membership/orgaccess suites and actual production API router pass
with race detection; the scoped organization regression passes in197.863s and
the new contract guard suite in93.263s. Existing OpenAPI warnings/informs remain
visible; zero lint errors. Local Quick ran on the changed working tree above
the recorded parent, not a claim about that parent's failed remote check.
Main protection was reverified before publication: strict required
`Quick verification`, with administrator enforcement enabled; publication is
authorized to dev only. No protection was changed or bypassed.
Final staged whitespace review removed one trailing blank SQL line; sqlc diff
still reports no generation drift. The artifact pins below include that cleanup.

Recovery retains all prior rows and reductions: retry an aborted migration,
or disable the new entry/capability while preserving commands, notices, audit
and membership history. Never reactivate by restoring consumed source tokens
or by running a destructive down migration.

F1/M1 remains open. Native locking/acceptance, device revocation, offboarding,
offline late-operation review, managed credentials, external AUTH/IDN providers,
runtime grants, notification delivery, legal receipts, staging and production
acceptance remain separate obligations. No merge, wiki or deployment is claimed.

Tested backend tree: `461a88dd458968b3da374e036b6b7bd9b085f1d6`.
Tested contract Git blob: `b8d6848fe0ac0c4249598b34d2c1c38a9ad9ec2b`.
Remote Quick verification awaits the publication head; local success does not
imply remote or production acceptance.
