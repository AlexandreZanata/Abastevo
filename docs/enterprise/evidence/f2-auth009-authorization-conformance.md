# EMP-AUTH-009 — Current-source authorization conformance

Date: 2026-11-13.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

[Task218](https://github.com/AlexandreZanata/Abastevo/issues/218) remains open
until protected integration. Source checkpoint `1cc9a7b3` on owned codex/enterprise-f2. This is a required
prerequisite of the full F2 in [execution](f2-execution.md), not phase closure.
B-BR-EMP-100, BUC-EMP-51 and EDR-063 precede behavior. Reuse the independent
v4 matrix and existing complete-router authority gate; exercise applicable
ORG/SELF request authority through ordinary real PostGIS. Do not modify policy
to fit tests or claim absent feature routes/scopes as passed.

## Opening applicability audit

The independent v4 fixture has 85 permission keys × 13 fixed roles = 1,105
static descriptor cells: 215 ORG/assigned, 54 SELF and 836 denied. The actual
composition has 69 normal guarded routes over 12 permissions and 15 explicit
account/session/bootstrap/self/device entries. The twelve routed permissions
have no SELF descriptor yet. Three station roles cannot be persisted by the
F1 source (domain and migration171); their 36 routed descriptor cells remain
unavailable-scope obligations. The remaining 120 current-source cells span
10 persistable roles (41 positive, 79 denied); 73 permission keys have no route.

The full-router descriptor/AuthorityCompletion gate is structural evidence.
It proves complete current-authority wiring is declared, not the semantics of
an arbitrary callback. The actual composed HTTP/source tests provide the
behavioral proof. No synthetic route is substituted for a business route.

The current pilot capability policy retains authorized Read/ReduceAccess/
RegularizePlan/RestoreOrganization effects when a scoped capability is off;
global disable still denies all. Suspended/archived organization exceptions
are checked by the actual source Effect policy. The stale generic validation
rows were reconciled with this approved behavior instead of changing policy.

The hand-written validation examples remain explicit obligations until their
actual UNIT/trip, fueling/review/payment, tracking/access-log or fleet routes
exist. ADMIN assigning OWNER is already a real current role-command regression.
Common-kernel own-record/review fixtures are not feature-route acceptance.

## Implemented acceptance

The independent JSON inventory enumerates all 85 fixed permissions, its actual
routes/representative or a named future obligation, all15 special boundaries
and three unavailable station-role obligations. The full production router
walk verifies exact69 normal routes and15 special entries, reuses the canonical
OpenAPI/current-authority gate, and refuses unknown/new/omitted/unguarded routes.
Seven route mutations and11 inventory mutations prove those failures, including
an extra route using an already-known permission and missing authority completion.
Targeted cmd/api tests pass0.139s; affected vet/staticcheck/format pass.

The actual enterprisesuite HTTP composition uses ordinary Runtime PostGIS for
both account/company proof and private business/authority providers. Account
creation is privileged synthetic fixture setup only. The suite asserts login,
NOSUPERUSER and NOBYPASSRLS properties explicitly. It dispatches each of the120
applicable matrix cells through the real chosen route, not a synthetic handler.
The independent v4 fixture supplies41 positive and79 denied expectations.

Final race run passes9.04s (package10.069s), with1,689 named request assertions:
120 in-scope cells;120 each missing/expired proof, invalid origin/CSRF, foreign
organization, suspended membership, ended/expired assignment, stale sensitive
authentication, capability off, suspended organization and ended membership;
108 narrower SELF scopes (OWNER/SELF is not a persistable source state,
so its twelve impossible request variants are explicitly inapplicable); one actual ADMIN→OWNER assign refusal; ten retained
company/account-proof checks after organization offboarding; and ten terminal
parent-family revocations using the same cookie on the next actual request.
Ten additional nonparticipant reads return the opaque not-found envelope.
Every refusal checks exact safe envelope keys/message/approved permission
details, no-store/no ETag and unchanged private source/business/receipt state.
SELF assignment narrowing is tested against actual own proposal participation;
this does not invent an `S` descriptor or accept future trip/fueling scopes.

The existing descriptor matrix was rerun: all1,105 cells pass0.012s. Existing
selected real source regressions also pass: proof after observed root/capability
wait5.77s; ADMIN union/owner protection1.02s; concurrent retry/CAS1.19s; deferred
commit failure0.92s; actual Android/company proof1.03s. These retained tests prove
their scoped behavior; they are not a claim of native device acceptance or all
future business routes. Full phase Quick remains reserved for batch closure.

## Reproduced failures and limits

Initial loopback access required normal sandbox escalation. Fixtures initially
used the existing administrative account store for dispatch and tried to insert
an already-expired period. The matrix now uses a Runtime account-store clone
and a genuine short finite period expiring by server clock; production temporal
constraints were preserved. The ADMIN→OWNER command returns the registered
permission detail, so the expected public envelope was corrected rather than
changing source policy. The final affected matrix rerun passes; no known failure
is deferred. Other selected regressions passed and were not rerun after this
expectation-only edit.

Independent review PASS on both inventory and final HTTP source. Plan checker
348 tasks/0 problems and its3 tests, scoped vet/staticcheck/pinned gofmt, diff
and secret reviews pass. No production Go policy, database migration, dependency,
Android or API schema changed. No phone test is necessary for this test-only task.

Frozen SHA256:

- authorization-routes-v1.json: `dbccbe7a671f04562ef0682601f9ffdfc200e91824cafa774d16135711741752`.
- authorization_inventory_test.go: `d903f91a38e464c62f4208524142ab542e9b018b6c2b70dc43ba430d863f10fb`.
- authorization_matrix_http_integration_test.go: `de6d02592c32de8aa7c1caf22135a5ac2c1c418ff4432ec82f0534a55ac2e156`.

Commands from backend: `GOCACHE=/tmp/enterprise-go-build go test ./cmd/api
-run '^TestAuthorizationInventory' -count=1 -v`; descriptor `go test
./internal/modules/orgaccess/domain -run '^TestCatalogMatrix$' -count=1 -v`;
actual request matrix `go test -tags=integration -race
./internal/modules/organization/adapters -run '^TestAuthorizationMatrixActualCompanyHTTP$'
-count=1 -v`, with an explicitly owned disposable ANPFUEL_TEST_DATABASE_URL.
Local diagnostic logs are /tmp/enterprise-auth009-http-final.log and
/tmp/enterprise-auth009-integration-final.log; no private test data is tracked.

The owned emp-f2-20261113 disposable PostGIS project is retained for following
F2 SQL tasks; remove only this project at phase closure. This checkpoint does
not close F2/M1/M2/C1 or future scope/feature/privacy/runtime/legal gates.
Next bounded dependency: EMP-ORG-014 company onboarding and settings panel.
