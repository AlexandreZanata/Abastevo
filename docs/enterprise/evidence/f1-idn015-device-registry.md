# EMP-IDN-015 — Scoped device enrolment and terminal revocation

Date: 2026-11-13.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Construction source: `f1ad43ac884c15f52dee3bef395bab942b24a25b`.
Owned branch: `codex/enterprise-idn015`; publish to maintained dev in cumulative
[PR203](https://github.com/AlexandreZanata/Abastevo/pull/203).
[Task211](https://github.com/AlexandreZanata/Abastevo/issues/211) stays open until
actual protected integration. B-BR-EMP-87..89, BUC-EMP-45 and EDR-056 precede
behavior. This completes one existing-account backend task, not F1/M1.

## Prerequisite CI repair

The prior published Quick check succeeded. Backend run43304322306 failed because
the full organization and contract packages reached Go's implicit10-minute
package deadline, and an ordinary audit CASCADE fixture lacked TRUNCATE on new
FK descendants (`org_role_commands`). Commit `57bec09c` repairs these tests
before dependent behavior. The full CI command keeps every package, race/count
and integration flags, with an explicit20-minute per-package bound already used
by local acceptance. It does not skip cases or extend proof/production deadlines.

The audit fixture now discovers only the real transitive FK closure, grants
TRUNCATE to its disposable role for those tables and temporarily suspends their
TRUNCATE guards. The actual audit guard stays enabled; exact55000 and message
assertions remain, and every saved trigger state is restored. Parent fixture
RED is reproduced in0.775s. Full ordinary PostGIS/race orgaudit passes5.158s;
the timeout-site proof test passes6.751s and contract drift test65.562s.
Workflow guard, scoped static/format/secret checks pass. No protection or
production grant was changed.

## Actual proof and scope

Two signed POSTs use the existing personal family proof, registered P256 key,
real identity verifier/nonces and current contributor/account binding:

- `/v1/orgs/{org_id}/devices/enroll` is the one enrolment bootstrap exception.
- `/v1/orgs/{org_id}/devices/check` requires the current exact enrolment on
  every request; there is no cached device authority or alternate cookie path.

Existing CompanyOnly routes remain unchanged. Narrow signed/self descriptors
are allowlisted to fixed paths by the actual production-router/OpenAPI gate.
Client key/account/device IDs and proof flags are never accepted as authority.
The source rechecks account status/deletion, current family/hash/expiry, bound
current unrevoked key, active membership/employee, root and capability under
account→family/key/binding→capability→root locks. Ownership follows the current
contributor binding, not an initial fingerprint that would break valid personal
key rotation. Final proof/enrolment checks happen after source waits.

Default3 current enrolments per membership is controlled only by deployment
`ANPFUEL_ENTERPRISE_DEVICE_LIMIT`, validated1–5 and passed through real API
composition. Count and insertion serialize. A revoked period never reopens.
LOST_OR_COMPROMISED bars that key throughout the organization, including
bootstrap and original enrolment retries. ACCESS_ENDED is terminal in its old
membership; a distinct freshly invited ACTIVE membership may enrol the still
valid personal key as a new period. No role or old access is automatically
restored. Managed accounts remain explicitly refused by this runtime.

Fresh cookie self list/revoke needs the actual account/membership binding, even
for ended history, without an invented organization permission. Manager list
and CAS revoke require current fresh idn.credential.manage through normal AUTH.
Reduction remains available for suspended/archived roots and disabled capability.
Lists/page cursors are scoped to one selected membership and disclose only
label, enrolment/revocation state, revision and timestamps. No personal family,
account session, fingerprint, key/contributor inventory or other organization
is exposed. Last-sync stays null without a real sync producer.

## Persistence, ending and recovery

Append-only migration194 adds FORCE-RLS enrolment periods and immutable original
command receipts. Source/audit guards prove actual current key/owner/membership,
slot count, actor/action/hash and exact result JSON. A deferred guard requires
the actual command at commit; missing receipt and null/forged result roll back
device/audit/receipt together. Exact retries retain original results and reasons
under current authority. No applied migration is edited.

Membership ending atomically revokes remaining ENROLLED rows ACCESS_ENDED,
retaining the actual membership revision. Earlier lost classifications stay
intact. Suspension denies use while retaining the period. Personal families,
keys, cloud devices and other memberships remain unchanged. The classes supply
no normal authority or fabricated late-intake service.

The populated ordinary193→194 rehearsal injects failure after all DDL and proves
complete rollback, previous data/ledger equality, recovery/idempotence and no
invented enrolments. A separate ordinary non-superuser/non-BYPASSRLS migrator
temporarily owns `org_memberships` for trigger creation; cleanup restores
ownership. Runtime is distinct and non-owner. Missing new grants returns42501;
there is no production grant. Populated owner FORCE RLS, original receipt and
personal-family preservation, immutable retention, source ending and two direct
SQL commit/source failures pass in3.513s.

## Validation

Pinned Go1.27.4 and writable Go/staticcheck caches; disposable PostgreSQL18 /
PostGIS3.6 under owned project `emp-idn015-20261113-ci`, tmpfs, no persistent
volume or external service. Database URL supplied only via
ANPFUEL_TEST_DATABASE_URL. All accounts/keys/labels are synthetic.

```sh
go -C backend test ./internal/modules/membership/domain ./internal/modules/membership/application ./internal/modules/membership/adapters/http ./internal/platform/config ./internal/platform/orgapi
go -C backend test ./internal/modules/membership/domain -cover
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/organization/adapters -run TestOrganizationDevice -count=1
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/membership/... ./internal/modules/organization/... ./internal/modules/orgaudit/... ./internal/modules/orgaccess/... ./internal/modules/companysession/... ./internal/platform/orgapi ./cmd/api -count=1
go -C backend vet -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/modules/orgaudit/... ./internal/modules/orgaccess/... ./internal/platform/orgapi ./internal/platform/apicontract ./internal/platform/enterprisesuite ./internal/platform/config ./cmd/api
# From backend/, compatible installed analyzer toolchain:
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/membership/... ./internal/modules/organization/... ./internal/modules/orgaudit/... ./internal/modules/orgaccess/... ./internal/platform/orgapi ./internal/platform/apicontract ./internal/platform/enterprisesuite ./internal/platform/config ./cmd/api
make quick-verify
bash scripts/check-compat.sh
python3 docs/enterprise/tools/check_plan.py
python3 scripts/tests/test-enterprise-local-sql.py
python3 scripts/tests/test-enterprise-protection.py
git diff --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

Label-domain RED initially fails on the absent validator; GREEN rejects invalid
UTF-8/control/format bytes, noncanonical whitespace and overlong labels. Private
actor/record/command/result/page formatting and serialization redact credentials.
Domain/application, vet/staticcheck pass; domain coverage91.3%.

Real ordinary source tests prove independent enrolments, default3/configured1
slots, twelve distinct final-slot contenders and twelve identical retries,
changed intent, paging, current proof/key/account binding, foreign/non-manager
denials, terminal revocation and actual next-request denial. Membership ending,
enrolment/revoke/check races, stale CAS outcomes, late audit/receipt rollback and
proof expiry after observed root and projection waits preserve exact source
history. A distinct fresh invitation/new membership preserves the old
ACCESS_ENDED period while allowing a new one; LOST remains barred.

Actual six-route HTTP acceptance passes3.938s using real signatures/nonces and
ordinary-runtime verification. It covers forged/partial proof/body, cookie and
origin separation, nonce replay, schema/no-store/privacy, actual self/manager
binding, exact retry, deferred rollback, DEVICE_REVOKED403 on the next check
and on bootstrap, independent organization/personal survival and quota429 with
Retry-After. Fresh manager reduction with actual disabled capability and
ACTIVE/SUSPENDED/ARCHIVED roots passes3.517s. Production router/device drift
checks pass (API1.057s; contract1.648s). Initial combined acceptance passes26.781s.

Final review additionally reproduces RED for a device-only five-minute expiry
ceiling that narrowed the existing identity-v1 clock envelope. The source now
preserves the verifier's allowed future-created/lifetime envelope while checking
actual database expiry after waits. Final device/protocol/source/upgrade and
static acceptance passes in76.389s, including all device/schema cases. An unused preliminary
membership result flagged by staticcheck is discarded explicitly; the binding
check itself is retained. A failed general run is repeated after that repair;
no gate, assertion or test is removed.

Full local Quick passes in246s: build, all backend units, vet/format/staticcheck,
sqlc vet/drift, OpenAPI and deployment harnesses, secret scans and govulncheck
(no vulnerabilities found). OpenAPI has zero errors, with841 warnings and509
informs visible. Compatibility, plan348 tasks/0 problems and both enterprise
harnesses pass. Integration-tag vet/staticcheck and final domain/application/
HTTP/config/orgapi units pass. No production behavior changes after this pass.

The affected PostGIS/race matrix passes membership23.479s, audit7.842s,
orgaccess57.448s, companysession24.547s, shared gateway4.196s and API62.565s.
Organization577.842s reports only two legacy ordinary erasure-owner fixtures
missing SELECT/UPDATE on the new device table. Their exact grants are updated
without broadening receipt access, changing ownership/BYPASSRLS or removing
assertions; both cases then pass in3.881s. All other aggregate cases pass.
There is no duplicate aggregate run for this fixture-only recovery.

A final populated two-organization device/account erasure test passes in2.083s.
It uses a distinct ordinary non-login/non-BYPASSRLS enterprise definer and an
ordinary caller, checks actual membership tombstone/revision causes, preserved
original enrolment receipts and an unrelated personal family, and restores the
caller's scope. Auto-review rejected the initial blanket fixture grant; the
accepted test instead enumerates only the required table/operation privileges
from the installed personal erasure triggers. No blanket grant is added by
this task. Its metadata, field and privilege assertions remain intact. Final
test-only fixture additions receive scoped vet/static/format/secret verification.

Main protection reverified strict required Quick verification with administrator
enforcement; no bypass or main development push.
Tested backend tree: `783ee2c2250d6c924426c6648d49d9e706ffd7b3`.
Tested contract Git blob: `e03535c45986af9f5b898921f30970ebfc3b0ad0`.
Required remote verification awaits the new published head.

## Remaining boundaries

Disable new routes or retry an aborted migration while retaining accepted periods,
receipts, source causes and revocations. No destructive down migration or personal
credential revocation is supported. Managed activation/recovery stays behind
GOV-013/IDN-006..008. Native enrolment/wipe acceptance, actual sync producer,
late-intake window, production grants/providers, legal/staging and production
certification remain separate. No main merge, wiki, deployment, emulator/native
acceptance or full F1/M1 completion is claimed.
