# EMP-IDN-012 — Reviewed employee merge and historical resolution

Date: 2026-11-12.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Source parent: `054eacf05c6700ebf0816331c8deda2cfb41d86a`.
Owned branch: `codex/enterprise-idn012`; publish to maintained dev in cumulative
[PR203](https://github.com/AlexandreZanata/Abastevo/pull/203).
[Task209](https://github.com/AlexandreZanata/Abastevo/issues/209) stays open
until protected integration. B-BR-EMP-83/84, BUC-EMP-43 and EDR-054 precede
behavior. This checkpoint completes one backend task, not F1/M1.

## Behavior and decisions

Three company POST routes under `/v1/orgs/{org_id}/employees/{employee_id}`
provide `/merge/preview`, `/merge/confirm` and `/resolution/read`. Current fresh
organization-scoped OWNER/ADMIN `idn.employee.manage` is required even for
private preview, resolution and receipt replay. The normal company cookie,
origin/CSRF and request-bound AUTH lease apply. Confirmation is a configuration
write under existing capability/root policy and employee-write admission limits.
The transaction retains actual account/proof/capability/root locks and rechecks
source authority after waits and before acknowledging the mutation. Foreign or
unknown targets disclose no private facts. The composed router stays default-off.

Preview returns explicit source/survivor records, current live-period counts,
actual source team links and a digest binding identities, profile revisions and
team plans. Confirm requires schema1, operation UUID, both exact identity
revisions, that digest, an explicit reviewed flag and a recorded reason. A profile
or team change invalidates review. Existing duplicate matches stay warnings;
they neither select a survivor nor merge automatically.

EDR-054 records the recommended decision: retain any INVITED, ACTIVE or SUSPENDED
membership holder as the chosen survivor. A source with live access or two live
records conflicts. The command never swaps the chosen orientation, offboards
someone, rewrites an immutable membership employee ID or merges personal
accounts. This preserves independent memberships in other organizations.

The source becomes TERMINATED with revision+1 and an immutable survivor pointer,
including when it was already TERMINATED. Both IDs/profiles, ended membership
and account bindings, role periods and original personal families remain.
Team effects use the owning organization module and existing link guards:
source periods end REMOVED under the actual merge audit operation; new survivor
periods are created only when no current survivor link exists. Old team-link IDs
and employee IDs are retained. Transferring current teams requires an ACTIVE
survivor and admissible actual team/unit. No profile fields are silently copied.
At most500 current team links may transfer; excess is refused, never truncated.

Original actor/intent-bound receipts retain the chosen survivor, versions, reason,
effects and timestamp after subsequent merges. Replays still require current
authority. Resolution follows scoped pointers for at most64 hops (65 IDs),
returns the original record and current canonical record, and fails closed on a
cycle, missing intermediate or excessive depth. It never rewrites old pointers.

## Persistence and recovery

Append-only migration193 adds an immutable FORCE-RLS merge receipt table, a
scoped pointer index and a forward replacement of the existing identity guard.
The guard retains original transitions and additionally proves source eligibility,
an unmerged same-organization survivor, absence of live/source team links,
root write state and bounded ancestor depth. Receipt guards prove the actual
source/survivor revisions, audit actor/intent and each real team effect. Audit,
team periods, source pointer and receipt commit together; deferred or projection
failure returns no provisional result. No earlier migration is edited.

The populated192→193 rehearsal uses a separate ordinary non-superuser,
non-BYPASSRLS migration owner for the existing employee table/function, which
are needed for CREATE INDEX and CREATE OR REPLACE. Runtime remains a distinct
non-owner role. Injected failure after all new DDL restores the old function
definition, ledger, index/table absence and populated history. Recovery and
idempotent rerun pass. Previous-image reads, forced owner RLS, immutable
receipt retention and missing runtime grants are checked.

## Validation

Commands ran from the owned repository with its pinned Go1.27.4 toolchain,
writable Go/staticcheck caches and disposable synthetic PostGIS URL supplied
through `ANPFUEL_TEST_DATABASE_URL` (no production data):

```sh
go -C backend test ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/apicontract ./internal/platform/enterprisesuite
go -C backend test ./internal/modules/membership/domain -cover
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/organization/adapters -run TestEmployeeMerge -count=1
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/membership/... ./internal/modules/organization/... ./cmd/api -count=1
# The API build hit local disk exhaustion in the aggregate command above.
# Membership and organization completed successfully; only the missing API
# package was repeated in full after clearing this execution's Go cache.
go -C backend test -race -tags=integration -timeout=20m ./cmd/api -count=1
# Added distinct-operation CAS case, after the aggregate run:
go -C backend test -race -tags=integration -timeout=20m ./cmd/api ./internal/modules/organization/adapters -run 'TestEmployeeMergeTwelveWayRetriesAndOppositeRace|TestFullProductionRouterEnterpriseContractAndDefaultOff|TestCompanyProductionCompositionIsOptInAndContractChecked' -count=1
go -C backend vet -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/enterprisesuite ./internal/platform/orgtest ./internal/platform/apicontract ./cmd/api
# From backend/, with the compatible installed analyzer toolchain:
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/platform/enterprisesuite ./internal/platform/orgtest ./internal/platform/apicontract ./cmd/api
make quick-verify
bash scripts/check-compat.sh
python3 docs/enterprise/tools/check_plan.py
python3 scripts/tests/test-enterprise-local-sql.py
python3 scripts/tests/test-enterprise-protection.py
git diff --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

Pure-domain RED initially failed on the absent merge symbols; GREEN covers
state/revision/scope/live-period/team limits and overflow. Membership domain
coverage is91.2%. Real PostGIS tests prove new and deduplicated team periods,
unchanged profiles/accounts/access history, previously terminated sources,
original receipts after later merges, two-organization independence and
duplicate warnings without automatic merge. Twelve identical retries give one
mutation and original receipts; twelve distinct operations give one winner and
eleven conflicts. Opposite merges cannot form a cycle. Profile/team review
changes, live-source/two-live orientation, stale revisions, foreign targets,
disabled replay, capability/root waits ageing proof beyond15min and deferred
audit/team/receipt failures all fail without partial mutation.

Sixty-four actual sequential merges resolve all65 IDs; the next merge is refused.
A privileged synthetic legacy cycle fixture restores the ordinary guard before
resolution and returns no partial result; ordinary runtime does not create it.
Actual HTTP checks fresh non-manager denial, cookie/origin/CSRF, strict missing,
null and unknown intent, reviewed=false, changed-reason operation reuse,
foreign/live conflicts, schema conformance, late rollback, exact original
receipts, historical resolution and Retry-After. Personal sessions and both
organization memberships remain usable. Full affected membership/organization
race suites pass (organization333.274s). After local disk-space recovery, the
complete API race/integration suite passes in56.165s; added distinct-CAS and
composition checks also pass. No assertions, tests or gates were removed.

Full local Quick verification passes in161s, including backend units,
build/vet/format/staticcheck, sqlc vet/drift, OpenAPI, deployment harnesses,
secret checks and govulncheck (no vulnerabilities found). OpenAPI has zero
errors, with834 warnings and486 informs visible. Integration-tag vet/staticcheck,
compatibility, plan348 tasks/0 problems and both enterprise harnesses pass.
No production behavior changed after that general run; final additions are
adversarial tests and documentation, with targeted checks afterward.

Main protection was reverified: strict required `Quick verification`, with
administrator enforcement enabled. No protection was changed or bypassed.
Tested backend tree: `93f595c7f684de83163432c1f11d7237ffb33a64`.
Tested contract Git blob: `43a33000eab604f6c2bd04507f9c2e4015721cb5`.
Required remote Quick verification awaits the publication head.

## Remaining boundaries

Recovery retries an aborted migration or disables the new command while retaining
all accepted identity, audit and team history. No destructive down migration,
unmerge, account rebinding or grant restoration is supported. Managed/GOV-013,
native/device/operational/late-intake, providers/runtime grants, legal receipt,
staging and production obligations remain open. No merge, wiki, deployment,
emulator/native acceptance or full F1/M1 completion is claimed.
