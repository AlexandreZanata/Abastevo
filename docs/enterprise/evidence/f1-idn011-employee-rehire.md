# EMP-IDN-011 — Employee rehire without restored access

Date: 2026-11-12.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Source parent: `b721691a1ac39a02fd164b0f6edbf168563ce297`.
Owned branch: `codex/enterprise-idn011`; publish to maintained dev in cumulative
[PR203](https://github.com/AlexandreZanata/Abastevo/pull/203).
[Task208](https://github.com/AlexandreZanata/Abastevo/issues/208) stays open
until actual protected integration. B-BR-EMP-82, BUC-EMP-42 and EDR-053 precede
implementation. This request completes one backend task only, not F1/M1.

`POST /v1/orgs/{org_id}/employees/{employee_id}/rehire` requires the normal
company cookie, origin/CSRF, schema1, operation UUID, exact identity revision
and recorded reason. Current fresh OWNER/ADMIN idn.membership.manage is a
configuration write under the existing organization capability and membership
write quota. The application requires its real request-bound AUTH decision;
the transaction retains account/proof/capability/root locks and checks authority
again after waits and before acknowledgement creation. Private target/receipt
lookup follows actual source authorization. Foreign/unknown targets disclose
no employee facts; a current DRIVER without management permission is refused.

The pure plan accepts only an unmerged TERMINATED employee with no INVITED,
ACTIVE or SUSPENDED period. SQL checks the same state, revision, merged pointer
and absence of a live period. Only employee state/revision changes to ACTIVE.
Profile and identity, original ended memberships/account bindings, role periods,
tombstones and consumed source reductions remain unchanged. There is no
membership, invitation, account, role or device provisioned by this command.
The response explicitly says SEPARATE_INVITATION_REQUIRED. A fresh invocation
of the existing invitation workflow creates a distinct INVITED, unbound period
on the same employee; the old period and all its grants remain ended. Rehire
does not recover a personal account or activate managed provisioning.

Migration192 adds one FORCE-RLS immutable rehire receipt table and an invoker
source guard. It binds actual employee state/revision, no live period, audit
actor/target/action/reason code/hash and a server timestamp. It backfills no
employment event and changes no earlier migration or existing state function.
Exact caller/intent-bound retries return the original ACTIVE acknowledgement,
reason, revision and time even after a later termination/rehire. Current proof,
permissions, root and capability still apply to retries. Audit/receipt or deferred
commit failure rolls back the employee change and returns no provisional result.
No notification obligation or speculative worker is introduced by this task.

Validation (repository root; explicit disposable PostGIS URL supplied outside
Git, writable Go/staticcheck caches):

```sh
go -C backend test ./internal/modules/membership/domain -run '^TestRehire' -count=1
go -C backend test -race -tags=integration ./internal/modules/organization/adapters -run '^TestRehire' -count=1
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/membership/... ./internal/modules/organization/adapters ./cmd/api -run 'Test(Rehire|Offboarding|MembershipAccess|Membership|Employee|Invitation|Redemption|FullProductionRouterEnterpriseContractAndDefaultOff)' -count=1
go -C backend test ./internal/modules/membership/domain -cover -count=1
go -C backend vet -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/enterprisesuite ./internal/platform/orgtest ./internal/platform/apicontract ./cmd/api
# From backend/, compatible analysis toolchain:
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/enterprisesuite ./internal/platform/orgtest ./internal/platform/apicontract ./cmd/api
# Pinned Go1.27.4 directory prepended to PATH:
make quick-verify
bash scripts/check-compat.sh
python3 docs/enterprise/tools/check_plan.py
python3 scripts/tests/test-enterprise-local-sql.py
python3 scripts/tests/test-enterprise-protection.py
git diff --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

Domain RED preceded GREEN. Real PostGIS covers actual offboarding/termination,
a populated profile, byte-identical ended history, a new invitation with a new
period, original receipts after later employee transitions, twelve-way same-op
and distinct CAS contention, foreign/missing/merged targets, stale revision,
disabled-capability replay, observed capability/root waits ageing proof, late
audit/receipt rollback/recovery and ordinary populated191→192 upgrade failure,
idempotent recovery, previous-image compatibility, owner FORCE RLS and immutable
retention. The overlap negative first proves ordinary SQL refuses termination
while live access exists, then injects inconsistent legacy data only through the
disposable administrator with the original guard restored; rehire fails closed.
That fixture is not a claim that ordinary runtime can create the bad state.

Actual composed HTTP validates response schemas, missing/null/unknown intent,
exact reasons, current non-manager denial, cookie/CSRF/origin, foreign targets,
late rollback, original retries, Retry-After, personal session survival and
two-organization independence. The former membership remains inaccessible
after employee reopening. The normal production router stays default-off;
contract drift checks reject a bare route or missing employee selector.
Membership domain coverage:91.8%. The complete new PostGIS/HTTP suite passed
in19.686s before adding the final explicit non-manager case, which is included
in the affected regression below. Full local Quick passed in159s, including all
backend units, build/vet/format/staticcheck, sqlc vet/drift, OpenAPI, deployment
harnesses, secret checks and govulncheck (no vulnerabilities found). OpenAPI
reports zero errors with warnings visible. Compatibility, plan348 tasks/0
problems, both enterprise harnesses and integration-tag vet/staticcheck pass.
The affected race regression passed, including organization in116.785s and the
actual production router. The final non-manager HTTP case is included there;
no code behavior changed after the general run, and no gate/assertion was
weakened or skipped.

Recovery retains employee state and all accepted history: retry an aborted
migration, or disable the new command. Never restore ended grants/account
bindings, remove receipts or run a destructive down migration. Managed access
retains GOV-013 and its owning implementation. Native/device, delegation,
operational work, late intake, external providers/runtime grants, legal receipts,
staging and production obligations remain open. No merge, wiki, deployment,
emulator/native acceptance or full F1/M1 completion is claimed.

Main protection was reverified before publication: strict required
`Quick verification`, with administrator enforcement enabled. No protection was
changed or bypassed.

Tested backend tree: `6407d5836ddd646b8f128528a1e2031dbd41d530`.
Tested contract Git blob: `64ecca76b6afb67b7a0b5ffcc28209de7899536d`.
Required remote Quick verification awaits the publication head.
