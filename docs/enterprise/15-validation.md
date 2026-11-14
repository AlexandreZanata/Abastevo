# Validation plan

Status: **PLANNED**, 2026-11-10. This file says how the enterprise suite is proven: which kind of test proves which kind of rule, which risks are covered by which scenario, and what a milestone must show before it is called done. It defines the scenario identifiers `VAL-nn`. Nothing here has been run; a scenario counts only when its gate task in [backlog/ep20-release](backlog/ep20-release.md) records the tested revision and the result.

## 1. Approach

Validation is layered. A rule is proven at the lowest level that can prove it, and a scenario at a higher level never replaces the test below it.

| Level | Question answered | Profiles ([00-conventions](00-conventions.md) §6) | Where it runs |
|---|---|---|---|
| 1. Rule | Is the rule computed correctly from its inputs? | `V-DOM`, `V-MONEY` | Go domain packages (standard library only) and Kotlin `:domain`, both against the same golden vectors |
| 2. Persistence and concurrency | Does the database keep the rule under real transactions, replays and races? | `V-SQL`, `V-IDEM` | Real PostGIS with the integration build tag, on a disposable database |
| 3. Boundary | Can a caller reach what they must not? Does the wire match the contract? Does anything private leak into logs? | `V-ISO`, `V-AUTHZ`, `V-API`, `V-PRIV` | The two-organization fixture (§3) and the generated authorization suite (§5) |
| 4. Client | Does the app keep working offline and render every screen state? Does the web do the same? | `V-SYNC`, `V-APP`, `V-WEB` | JVM tests for `:domain`, `:application`, `:data`, `:app`; an emulator for instrumented tests; the Node test runner for the web |
| 5. Background work | Is every job safe to run twice, to lose its lease and to fail? | `V-JOB` | Integration tests with the worker role |
| 6. Scenario | Does a complete flow work across client, API, worker and panel? | `V-E2E` | The `VAL-nn` scenarios in §4, run by the gate tasks |
| 7. Capacity | Do the sizing hypotheses hold? | `V-PERF` | The benchmark in §6 (EMP-REL-010) |
| 8. Decision | Is a decision recorded and is the plan still consistent? | `V-DOC` | The plan check in §9 |

Practice reused from the repository, unchanged:

- **Targeted tests per task.** Every task runs its own meaningful tests, the affected compilation, formatting and static checks, `git diff --check` and a secret-surface review when it is done (AGENTS rule 5). Critical authorization, money, privacy, SQL, migration and job changes get their negative, failure and concurrency tests in the same task. A gate adds no behaviour and never stands in for a missing task test.
- **Real PostGIS for SQL.** Schema, queries, row-level security, exclusion constraints and races are proven only by integration tests against real PostGIS (`go test -race -count=1 -tags=integration ./...` after `go run ./cmd/migrate`, as documented in [backend/README.md](../../backend/README.md)), on a disposable database, never a persistent one.
- **No compile-only evidence.** A build, or `go test -run '^$'`, shows that code compiles. It is never recorded as behaviour evidence.
- **Instrumented Android tests on an emulator only.** The command is the existing `./gradlew :data:connectedDebugAndroidTest :app:connectedDebugAndroidTest` ([TEST_STRATEGY](../backend/TEST_STRATEGY.md)). No task installs or runs anything on the owner's device. What only a real device can show is listed as owed evidence in §7.
- **Synthetic data and a controlled clock.** Every test uses the fixture of §3 or its own synthetic data, an injected clock, and fake adapters for providers. No test calls a third-party service.
- **Local gates.** A milestone gate is a local functional checkpoint ([14-roadmap](14-roadmap.md) §1). It records `LOCAL_DONE`, the tested revision, the commands, the results and what was not run. It certifies nothing about production.

## 2. Risks and how each is proven

Each risk is prevented by shared rules, proven by scenarios and implemented by tasks. Module rules that refine the shared ones are in the module files and are not repeated here.

| Risk | Shared rules that prevent it | Scenarios | Implementing tasks |
|---|---|---|---|
| Leak between organizations | BR-TEN-001, BR-TEN-003, BR-TEN-005, BR-TEN-006, BR-TEN-007, BR-PRIV-001, BR-PRIV-002, BR-PRIV-003, BR-PRIV-004, BR-AUTH-013 | VAL-01, VAL-02, VAL-34, VAL-36 | EMP-PLAT-001, EMP-PLAT-002, EMP-PLAT-004, EMP-PLAT-006, EMP-PLAT-013, EMP-AUTH-003, EMP-AUTH-009, EMP-AGREE-009, EMP-DATA-007 |
| Access after revocation | BR-AUTH-014, BR-AUTH-006, BR-TIME-003, BR-OFFL-009, BR-OFFL-011, BR-OFFL-012, BR-PRIV-018 | VAL-03, VAL-04, VAL-22 | EMP-AUTH-003, EMP-AUTH-005, EMP-IDN-008, EMP-IDN-009, EMP-IDN-010, EMP-IDN-015, EMP-ORG-009, EMP-OFFL-009, EMP-OFFL-011 |
| Context switch with a pending queue | BR-TEN-004, BR-OFFL-003, BR-OFFL-013, BR-AUTH-012, BR-PRIV-016, BR-PRIV-017 | VAL-05 | EMP-PLAT-011, EMP-PRIV-003, EMP-PUB-001, EMP-OFFL-003, EMP-OFFL-010, EMP-IDN-018 |
| Duplicate resend | BR-OFFL-001, BR-OFFL-002, BR-OFFL-004, BR-OFFL-005 | VAL-06, VAL-23 | EMP-OFFL-001, EMP-OFFL-002, EMP-OFFL-003, EMP-OFFL-013, EMP-FUEL-006 |
| Concurrency on a vehicle | BR-OFFL-006, BR-OFFL-007, and the one-active-trip constraint of [06-domain-model](06-domain-model.md) §5 | VAL-07 | EMP-TRIP-002, EMP-TRIP-007, EMP-OFFL-008, EMP-OFFL-014, EMP-FLEET-006 |
| Decimal calculation | BR-MONEY-001, BR-MONEY-003, BR-MONEY-004, BR-MONEY-005, BR-MONEY-007, BR-MONEY-008, BR-MONEY-010, BR-MONEY-011 | VAL-08, VAL-09 | EMP-PLAT-009, EMP-FUEL-001, EMP-FUEL-002, EMP-EXP-006, EMP-KPI-001 |
| Consumption with a partial tank | BR-MONEY-008, BR-MONEY-009, BR-TRUTH-003, BR-TRUTH-004, EDR-011 | VAL-10, VAL-11, VAL-20 | EMP-FUEL-007, EMP-FUEL-008, EMP-FUEL-013, EMP-METER-006, EMP-KPI-003 |
| Allocation of documents | BR-MONEY-007, BR-TRUTH-001, and the no-double-counting constraint of [06-domain-model](06-domain-model.md) §5 | VAL-09, VAL-12 | EMP-DOC-001, EMP-DOC-007, EMP-DOC-010, EMP-DOC-013 |
| Duplicated cost | EDR-013, BR-REV-006, BR-REV-009, and the one-cost-per-fueling constraint of [06-domain-model](06-domain-model.md) §5 | VAL-13, VAL-32 | EMP-EXP-002, EMP-FIN-002, EMP-FIN-004, EMP-MAINT-006, EMP-STOCK-005 |
| Stale location | BR-TRUTH-006, BR-AUTH-011, BR-AUD-003 | VAL-16 | EMP-GPS-007, EMP-GPS-008, EMP-GPS-012, EMP-GPS-014 |
| Concurrent station claim | BR-TEN-002, BR-AUTH-015, EDR-021 | VAL-17 | EMP-CLAIM-003, EMP-CLAIM-005, EMP-CLAIM-006, EMP-BACK-002 |
| Repeated billing events | BR-AUTH-009, EDR-024, and the billing-event constraint of [06-domain-model](06-domain-model.md) §5 | VAL-18, VAL-35 | EMP-SUBS-002, EMP-SUBS-004, EMP-SUBS-005 |
| Recalculation and auditable corrections | BR-AMD-001, BR-AMD-002, BR-AMD-003, BR-AMD-004, BR-AMD-005, BR-TRUTH-004, BR-REV-005, BR-REV-007, BR-AUD-001, BR-AUD-002 | VAL-19, VAL-20, VAL-26 | EMP-PLAT-003, EMP-PLAT-016, EMP-APPR-006, EMP-FUEL-008, EMP-FUEL-015, EMP-TRIP-015, EMP-METER-004, EMP-FIND-005 |

Two cases named by the request have no row of their own and are covered directly: a race for the last amount of a budget (VAL-14; EMP-BUDG-002, EMP-BUDG-003, EMP-BUDG-004) and a price difference against a reference (VAL-15; EMP-FUEL-014, EMP-KPI-005, EMP-FIND-003). The remaining scenarios (VAL-21 to VAL-31, VAL-33, VAL-37, VAL-38) exist because a milestone gate needs them (§8).

## 3. Fixtures

One synthetic fixture with two fleet organizations is reused by every scenario, so that isolation is tested everywhere and numbers can be compared between tests. All names, numbers and values are invented. No real company, person, plate, CNPJ or private place appears. Vehicles are identified by fleet number; plates are left empty on purpose.

The [version 1 organization fixture](../../contracts/testdata/enterprise/fixture-two-orgs-v1.json)
and the [Go PostGIS harness](../../backend/internal/platform/orgtest/fixture.go)
exist through EMP-PLAT-002. This first fixture contains organizations A and B
and deliberately colliding record identifiers and numbers. The people,
assets, operational records, station organizations and Kotlin consumers
below are added by their owning implementation tasks. Their deterministic
identifiers follow the same fixture version contract.

**Organizations**

| Key | Name | Time zone | State | Used for |
|---|---|---|---|---|
| A | Transportes Exemplo | `America/Cuiaba` (UTC−4) | `ACTIVE` | The organization under test |
| B | Entregas Modelo | `America/Sao_Paulo` (UTC−3) | `ACTIVE` | The organization that must see nothing of A, and whose own data must not change |
| S | Rede de Postos Fictícia | `America/Sao_Paulo` | `ACTIVE` | Station organization, loaded from F7 |
| T | Combustíveis Hipotéticos | `America/Sao_Paulo` | `ACTIVE` | Second claimant for the same station, loaded from F7 |

**Units of A:** `Norte` (`America/Manaus`, UTC−4) and `Sul` (`America/Sao_Paulo`, UTC−3). **Unit of B:** `Centro` (`America/Sao_Paulo`). Cost centre of A: `Operação Sul`.

**People and roles**

| Organization | Employee (registration number) | Unit | Role and scope |
|---|---|---|---|
| A | Carla (0001) | — | `OWNER` at `ORG` |
| A | Ivo (0002) | — | `ADMIN` at `ORG` |
| A | Elisa (0003) | — | `FLEET_MANAGER` at `ORG` |
| A | Diego (0004) | — | `FINANCE` at `ORG`, and `DRIVER` at `SELF` |
| A | Gil (0005) | — | `AUDITOR` at `ORG` |
| A | Ana (0006) | Norte | `SUPERVISOR` at `UNIT` Norte, and `DRIVER` at `SELF` |
| A | Jonas (0007) | Norte | `DRIVER` at `SELF` |
| A | Helena (0008) | Norte | `OPERATIONS_STAFF` at `SELF` |
| A | Bruno (0009) | Sul | `DRIVER` at `SELF` |
| A | Fábio (0010) | Sul | `APPROVER` at `UNIT` Sul |
| B | Rui (0001) | Centro | `OWNER` at `ORG` |
| B | Sara (0002) | Centro | `DRIVER` at `SELF` |
| B | Bruno (0003) | Centro | `DRIVER` at `SELF` |

Bruno is one account with two memberships (A and B). Registration numbers 0001 to 0003 exist in both organizations on purpose: a shared number grants nothing (BR-TEN-002). Unit-scoped roles need EMP-AUTH-006; before it exists the fixture loads only the `ORG` and `SELF` assignments.

**Assets and readings**

| Organization | Fleet number | Kind and product | Tank | Unit | Assigned to | Readings used by the scenarios |
|---|---|---|---|---|---|---|
| A | FR-001 | Truck, diesel S10 | 300 000 mL | Sul | Bruno | 184 320 000 m (trip start), 184 532 000 m (fueling), 184 610 000 m (trip end) |
| A | FR-002 | Light vehicle, gasoline | 50 000 mL | Norte | Jonas | 1 000 000 m, 1 200 000 m, 1 400 000 m |
| B | FR-001 | Van, gasoline | 60 000 mL | Centro | Sara | 52 000 000 m |

The fleet number FR-001 exists in both organizations on purpose.

**Other fixture data**

| Item | Value |
|---|---|
| Public catalog stations | `Posto Fictício Alfa` and `Posto Fictício Beta`, in the same fictitious municipality, with identical community and official fixture prices |
| Receipt file | One JPEG of 412 338 bytes with a fixed SHA-256, and one fiscal document total of R$ 973.50 |
| Price reference | Diesel S10, same municipality, same week, same payment condition: R$ 6.090 per litre (6 090 milli-BRL) |
| Periods of A | October 2026 (`2026-10-01` inclusive to `2026-11-01` exclusive) `LOCKED` on 2026-11-05; November 2026 `OPEN` |
| Budget line of A | Cost centre `Operação Sul`, November 2026, limit R$ 5 000.00, already consumed or held R$ 4 500.00 |
| Intake window of A | 15 days (the default of BR-OFFL-011) |

VAL-01 does not load organization A. It creates A through the product flows, with `Sul` as the first unit, and loads only B.

## 4. Scenarios

Every scenario is written as Given, When, Then. Amounts are integer centavos, quantities millilitres, distances metres and unit prices milli-BRL per litre (BR-MONEY); the arithmetic is shown. A scenario for a later phase uses only shared rules, canonical state names and the roadmap's exit criteria. Where it needs a number that a module file owns (a freshness limit, a tolerance, a field list), the number is set as a fixture parameter and the scenario says so; the module's own acceptance criteria, in section 12 of each module file, run in addition. "Run by" names the gate task; "Built by" names tasks whose tests implement the scenario's parts.

### VAL-01 — Cut C1 end to end

- **Given** an empty enterprise schema, the public catalog fixture, organization B loaded with zero trips, fuelings and documents, the global flag on and the C1 capabilities enabled for A and B.
- **When** (1) Carla signs in to the business web with an existing account and creates `Transportes Exemplo` with time zone `America/Cuiaba` and first unit `Sul`; (2) she registers employees Elisa and Bruno, gives Elisa `FLEET_MANAGER` at `ORG` and invites Bruno, who accepts in the app with his existing account; (3) she registers vehicle FR-001 with a 300 000 mL tank and assigns it to Bruno; (4) Bruno, on an emulator in aeroplane mode, starts a trip with reading 184 320 000 m, adds a stop and a note, records a fueling of 150 000 mL of diesel S10 at 6 490 milli-BRL per litre with R$ 973.50 paid, fill level full, reading 184 532 000 m and the 412 338-byte receipt photo, finishes the trip with reading 184 610 000 m and marks it ready to send; (5) the app process is killed, restarted, and the network is turned on; the first answer is dropped once so that the app resends; (6) Elisa opens the trip timeline and the fueling with its receipt in the panel, approves the fueling and returns the trip with the reason "Informe o motivo da parada"; Bruno corrects and resubmits; Elisa approves; (7) Rui signs in to organization B.
- **Then** after step 2 the app shows enterprise mode with the name `Transportes Exemplo`, and Bruno holds `DRIVER` at `SELF`. After step 4 every record shows "Salvo no aparelho" and nothing exists on the server. After step 5 the server holds exactly one trip, one fueling and one document; each record moved to "Aceito pela empresa"; the stored file has 412 338 bytes and the declared SHA-256; the trip's odometer distance is `184 610 000 − 184 320 000 = 290 000 m`; the fueling has no warning, because `150 000 × 6 490 / 10 000 = 97 350` centavos equals the amount paid. After step 6 the fueling is `APPROVED`, the trip went `SUBMITTED` → `RETURNED` → `SUBMITTED` → `APPROVED` with two revisions kept, Bruno's app shows both outcomes after its next sync, and opening the receipt wrote one access-log entry. After step 7 every list of B (trips, fuelings, documents, review queue, audit trail) is unchanged and contains nothing of A; A's trip, fueling and document identifiers requested through B return `NOT_FOUND`; A's audit trail holds one event per change and no coordinates or document content.
- **Proves** BR-TEN-004, BR-TEN-006, BR-OFFL-002, BR-OFFL-004, BR-OFFL-010, BR-REV-001, BR-REV-005, BR-MONEY-005, BR-AUD-001, BR-AUD-003 · **Profiles** `V-E2E`, `V-SYNC`, `V-ISO` · **Run by** EMP-REL-001.

### VAL-02 — A second organization sees nothing on any surface

- **Given** organizations A and B loaded, with A holding trips, fuelings, documents, export files and jobs.
- **When** each surface of [11-privacy-security](11-privacy-security.md) §10.3 is attempted for A's data with B's identities (Rui, Sara): every organization route with A's path and identifiers; every list with a cursor sealed for A; a file capability issued to a member of A; the same capability after 5 minutes; a job payload naming A while the data belongs to B; an export file of A; a query sent without the organization setting.
- **Then** every route answers `NOT_FOUND` with nothing beyond the error envelope; the cursor and the capability are refused; the expired capability is refused for everyone; the job loads nothing; the export is unreadable; row-level security returns zero rows for the query without the organization setting; every organization response is `no-store`; the logs of the whole run contain none of the seeded secrets, coordinates or document fragments; B's own row counts are the same before and after; the fleet number FR-001 and the registration numbers 0001 to 0003 coexist in both organizations.
- **Proves** BR-TEN-006, BR-TEN-007, BR-PRIV-001, BR-PRIV-002, BR-PRIV-003, BR-PRIV-004, BR-PRIV-008 · **Profiles** `V-ISO`, `V-PRIV` · **Run by** every gate, over the routes added in its phase · **Built by** EMP-PLAT-002.

### VAL-03 — Access ends on the next request after revocation

- **Given** organization A loaded; Diego reads the employee list successfully at 10:00:00 with `FINANCE`.
- **When** (a) Ivo ends Diego's `FINANCE` assignment at 10:00:05 and Diego repeats the request at 10:00:06 with the same session and a cursor sealed at 10:00:00; (b) Ivo suspends Jonas's membership and Jonas sends a request; (c) Ivo offboards Bruno from A and Bruno sends a request to A and one to B; (d) Bruno's phone is revoked as lost and then sends one queued operation; (e) Ivo tries to demote, suspend and offboard Carla, the only `OWNER`.
- **Then** (a) Diego receives `FORBIDDEN` naming `idn.employee.read` and no employee data, and the old cursor is refused; (b) Jonas receives `MEMBERSHIP_SUSPENDED`; (c) Bruno is denied in A with `MEMBERSHIP_ENDED`, his membership in B still works and A's records he authored stay; (d) the operation is `REJECTED` with `DEVICE_REVOKED` and the app erases the local partition at that contact; (e) each attempt fails with `LAST_OWNER` and changes nothing. No case needed a session to expire.
- **Proves** BR-AUTH-014, BR-AUTH-006, BR-AUTH-013, BR-TEN-009, BR-OFFL-012, BR-PRIV-018 · **Profiles** `V-AUTHZ`, `V-ISO` · **Run by** EMP-REL-003.

### VAL-04 — An offboarded driver delivers three fuelings

- **Given** Bruno is offboarded from A at `2026-11-10T12:00:00Z` while his phone is offline with three fuelings declared at `2026-11-09T14:00:00Z`, `2026-11-09T19:30:00Z` and `2026-11-10T09:15:00Z`. The intake window is 15 days, so it closes at `2026-11-25T12:00:00Z`.
- **When** case 1: the phone reconnects at `2026-11-12T08:00:00Z` and also sends a fourth fueling declared at `2026-11-10T13:00:00Z`; case 2 (fresh run): the phone reconnects at `2026-11-26T08:00:00Z`.
- **Then** case 1: each of the three is `ACCEPTED_FOR_REVIEW` with `ACCESS_ENDED_BEFORE_RECEIPT`; three review items are `OPEN`; none is counted in costs until a manager who is not Bruno resolves it; the fourth is `REJECTED` with `MEMBERSHIP_ENDED`; a read request from the same phone is denied, because intake-only access gives no reading right. Case 2: all three are `REJECTED` with `INTAKE_WINDOW_CLOSED`, stay visible on the phone with the reason, and the app says the company must be contacted. In neither case is anything accepted automatically on the strength of the device clock.
- **Proves** BR-OFFL-006, BR-OFFL-009, BR-OFFL-011, BR-TIME-003, UC-OFFL-07 · **Profiles** `V-SYNC`, `V-AUTHZ`, `V-IDEM` · **Run by** EMP-REL-005.

### VAL-05 — Context switch and logout with a pending queue

- **Given** Bruno signed in on one emulator, offline, with memberships in A and B.
- **When** he records two operations in A, switches to B and records one, switches to personal mode, logs out choosing "keep on this device", Sara logs in on the same emulator and syncs, Sara logs out, Bruno logs in again and the network is on.
- **Then** the switcher showed 2 pending for A and 1 for B; no switch changed the organization, membership or device of any operation; Sara saw none of Bruno's three operations and her sync sent none of them; after Bruno's login A received exactly its 2 operations and B exactly its 1, all authored by Bruno; personal data was never touched; nothing was deleted without a question to the user.
- **Proves** BR-TEN-004, BR-OFFL-003, BR-OFFL-013, BR-PRIV-017, UC-AUTH-04, UC-OFFL-05, UC-OFFL-06 · **Profiles** `V-SYNC`, `V-APP`, `V-ISO` · **Run by** EMP-REL-001.

### VAL-06 — Duplicate resend and reused identifier

- **Given** one fueling operation of A with a fixed operation identifier and payload digest (150 000 mL, 97 350 centavos).
- **When** the same batch is sent three times; then the same identifier is sent with the paid amount changed to 97 000 centavos; then a panel review decision is sent twice with one operation identifier.
- **Then** one fueling exists and the three acknowledgements are identical; the changed payload is `REJECTED` with `OPERATION_ID_REUSED` and the stored fueling still says 97 350; the second panel command returns the first result and writes no second audit event.
- **Proves** BR-OFFL-001, BR-OFFL-004, BR-OFFL-005, UC-OFFL-02 · **Profiles** `V-IDEM`, `V-SQL` · **Run by** EMP-REL-001.

### VAL-07 — Two trips started on one vehicle

- **Given** FR-001 of A with no trip in progress; Bruno and Diego each offline.
- **When** Bruno starts a trip on FR-001 declared at 08:00 and Diego starts one on FR-001 declared at 08:05; the server receives Diego's at 10:00 and Bruno's at 10:02; then Elisa and Ivo both try to resolve the review item at the same moment.
- **Then** Diego's trip is `ACCEPTED` and `IN_PROGRESS`; Bruno's is `ACCEPTED_FOR_REVIEW` with `ASSET_BUSY` and is not lost; at no moment do two trips of FR-001 count as in progress; exactly one resolution is applied and the other reviewer receives `CONFLICT`; the resolution is accept, accept with another asset, or refuse with a reason, and it is audited.
- **Proves** BR-OFFL-006, BR-OFFL-007, UC-OFFL-04, BR-REV-003 · **Profiles** `V-IDEM`, `V-SQL`, `V-SYNC` · **Run by** EMP-REL-001.

### VAL-08 — Arithmetic consistency of a fueling

- **Given** 150 000 mL at 6 490 milli-BRL per litre, so `expected_cents = round_half_up(150 000 × 6 490 / 10 000) = round_half_up(97 350.0) = 97 350`, which is R$ 973.50.
- **When** five receipts are recorded: (a) paid 97 350, discount 0; (b) paid 96 850, discount 500; (c) paid 97 000, discount 0; (d) paid 97 349, discount 0; (e) paid 97 348, discount 0. And one rounding vector: 1 000 mL at 6 495 milli-BRL per litre.
- **Then** (a) is consistent: `|97 350 − 97 350| = 0`; (b) is consistent: `96 850 + 500 = 97 350`; (c) gets `ARITHMETIC_MISMATCH`: `|97 000 − 97 350| = 350 > 1`, and is still accepted with the paid total of R$ 970.00 kept as authoritative; (d) is consistent: difference 1; (e) gets the warning: difference 2. The rounding vector gives `1 000 × 6 495 / 10 000 = 649.5`, rounded half-up to 650 centavos. Go and Kotlin return the same results from the same vector file.
- **Proves** BR-MONEY-001, BR-MONEY-003, BR-MONEY-004, BR-MONEY-005, BR-MONEY-011 · **Profiles** `V-MONEY`, `V-DOM` · **Run by** EMP-REL-001 and EMP-REL-004 · **Built by** EMP-PLAT-009.

### VAL-09 — Allocation preserves the whole

- **Given** three amounts to split: 10 000 centavos in three equal parts; 5 centavos in three equal parts; 10 000 centavos by the weights 1, 1 and 4.
- **When** the allocation of BR-MONEY-007 is computed.
- **Then** the first gives floors 3 333, 3 333, 3 333 (sum 9 999), one centavo left, all remainders tie, line 1 receives it: 3 334, 3 333, 3 333, which is R$ 33.34, R$ 33.33 and R$ 33.33 and sums to R$ 100.00. The second gives floors 1, 1, 1 (sum 3), two centavos left, lines 1 and 2 receive them: 2, 2, 1, sum 5. The third gives exact shares 1 666.67, 1 666.67 and 6 666.67, floors 1 666, 1 666, 6 666 (sum 9 998), two centavos left, all remainders tie, lines 1 and 2 receive them: 1 667, 1 667, 6 666, sum 10 000. No part is negative and no centavo is created or lost.
- **Proves** BR-MONEY-007, BR-MONEY-011 · **Profiles** `V-MONEY`, `V-DOM` · **Run by** EMP-REL-004 · **Built by** EMP-PLAT-009.

### VAL-10 — Full-to-full consumption with a partial fueling

- **Given** FR-002 with three approved fuelings of gasoline at 6 100 milli-BRL per litre: an initial full fill of 35 000 mL at 1 000 000 m (R$ 213.50); a partial fill of 20 000 mL at 1 200 000 m (R$ 122.00); a full fill of 20 000 mL at 1 400 000 m (R$ 122.00).
- **When** the consumption interval between the two full fills is computed.
- **Then** the initial fill only opens the interval and is excluded from its fuel; distance is `1 400 000 − 1 000 000 = 400 000 m`, which is 400 km; fuel is `20 000 + 20 000 = 40 000 mL`, which is 40 L; consumption is `400 / 40 = 10 km/L`; cost is `12 200 + 12 200 = 24 400` centavos, R$ 244.00; cost per kilometre is `244.00 / 400 = R$ 0.61`. The interval is `COMPUTED` and shows its method version, anchors and inputs. The figure is not the average of two per-fueling ratios.
- **Proves** EDR-011, BR-MONEY-008, BR-TRUTH-004 · **Profiles** `V-MONEY`, `V-DOM` · **Run by** EMP-REL-004 and EMP-REL-007.

### VAL-11 — A partial-only sequence gives insufficient data

- **Given** an asset with three approved fuelings, all partial: 15 000 mL at 2 000 000 m, 20 000 mL at 2 300 000 m and 18 000 mL at 2 650 000 m; and a second asset whose only two fuelings have an unknown fill level.
- **When** consumption is requested for each asset.
- **Then** both results are `INSUFFICIENT_DATA` with a reason saying that no full fill opens and closes an interval; no number is shown, not zero and not `650 / 53`; the fuelings still count in spend and volume; the day a later full fill arrives the interval that it closes is still not computed without an opening full fill.
- **Proves** EDR-011, BR-TRUTH-003 · **Profiles** `V-DOM`, `V-MONEY` · **Run by** EMP-REL-004 and EMP-REL-007.

### VAL-12 — A document allocation above the document total

- **Given** one fiscal document of A with a total of 97 350 centavos and no allocation (`UNALLOCATED`).
- **When** Diego allocates 60 000 to a fueling, then 30 000 to an expense, then tries 8 000 to a second expense; then two requests allocate the remaining 7 350 at the same moment.
- **Then** after the first two the document is `PARTIALLY_ALLOCATED` with `60 000 + 30 000 = 90 000` allocated and 7 350 left; the third is refused, because `90 000 + 8 000 = 98 000 > 97 350`, and writes nothing; of the two racing requests exactly one succeeds, the document becomes `RECONCILED` with exactly 97 350 allocated, and the loser receives `CONFLICT`. The document never reaches `OVER_ALLOCATED` through an allocation command.
- **Proves** BR-MONEY-007, BR-TRUTH-001 · **Profiles** `V-SQL`, `V-IDEM`, `V-AUTHZ` · **Run by** EMP-REL-004.

### VAL-13 — One fueling, one cost

- **Given** the approved fueling of 97 350 centavos on FR-001, paid by Bruno from his own pocket.
- **When** the fueling is delivered again by the device, the review decision is replayed, Diego records the reimbursement payment of 97 350 centavos, and the same payment command is sent twice.
- **Then** the organization has exactly one expense of 97 350 whose origin is that fueling, one payable of 97 350 to Bruno and one payment of 97 350 that leaves the payable `SETTLED`; the period's cost is 97 350, not 194 700; the payable and the payment appear in the settlement views and never in the cost total; a second expense for the same fueling cannot be inserted.
- **Proves** EDR-013, BR-REV-009, BR-OFFL-004 · **Profiles** `V-SQL`, `V-IDEM` · **Run by** EMP-REL-004.

### VAL-14 — A race for the last R$ 500.00 of a budget

- **Given** the budget line `Operação Sul`, November 2026: limit 500 000 centavos, consumed or held 450 000, so `500 000 − 450 000 = 50 000` available.
- **When** two holds of 30 000 centavos each are requested at the same moment.
- **Then** exactly one hold becomes `ACTIVE` and the available amount becomes `50 000 − 30 000 = 20 000`; the other is refused, or goes to the exception flow when the overrun policy allows one, because `30 000 > 20 000`; at no moment are both `ACTIVE`, which would leave −10 000; releasing the winning hold restores 50 000; the available amount is computed from the rows, never stored as truth.
- **Proves** BR-AUTH-001, the budget constraint of [06-domain-model](06-domain-model.md) §5; rule detail in [modules/expenses-finance](modules/expenses-finance.md) · **Profiles** `V-IDEM`, `V-SQL` · **Run by** EMP-REL-004.

### VAL-15 — A price difference against a comparable reference

- **Given** the approved fueling of 150 000 mL of diesel S10 at 6 490 milli-BRL per litre, and the fixture reference of 6 090 milli-BRL per litre for the same product, municipality, week and payment condition.
- **When** the price comparison runs; a reviewer then marks the finding `JUSTIFIED` with a reason; the detection runs again; and the same fueling is compared in a second run where the only reference is for another product.
- **Then** the difference is `(6 490 − 6 090) × 150 000 / 10 000 = 400 × 150 000 / 10 000 = 6 000` centavos, R$ 60.00, which is `150 × 0.40`; one finding is `OPEN` and its text says a difference to review, never a loss, fraud or saving; it stores the reference, its source, place, period, conditions and the method version; after the decision it is `JUSTIFIED`, and the second detection opens no duplicate. In the second run the result is `INSUFFICIENT_DATA` with the reason that no adequate reference exists, and no finding and no amount are produced.
- **Proves** EDR-030, BR-TRUTH-003, BR-TRUTH-004, BR-TRUTH-005, BR-SRC-001 · **Profiles** `V-DOM`, `V-MONEY`, `V-JOB` · **Run by** EMP-REL-007.

### VAL-16 — A stale live position

- **Given** a trip of A in progress whose latest fix was collected at 10:00:00. Fixture parameter: freshness limit 120 seconds (the default is owned by [modules/gps-navigation](modules/gps-navigation.md)).
- **When** Elisa opens the live map at 10:01:30 and again at 10:02:01; Gil opens it; Rui of B opens his own map; the trip then ends and 24 hours pass.
- **Then** at 10:01:30 the age is 90 seconds and the position is `CURRENT`, shown with its collection time; at 10:02:01 the age is 121 seconds and it is `STALE`, labelled "Desatualizada" with the collection time, never drawn as the current position; Gil receives `FORBIDDEN` naming `gps.location.read_live`; Rui sees nothing of A; each successful read of a position wrote an access-log entry without coordinates; 24 hours after the trip ended the live position row no longer exists.
- **Proves** BR-TRUTH-006, BR-AUTH-011, BR-AUD-003, BR-AUD-004, BR-PRIV-009 · **Profiles** `V-AUTHZ`, `V-PRIV`, `V-WEB` · **Run by** EMP-REL-006.

### VAL-17 — Two concurrent claims for one station

- **Given** station `Posto Fictício Alfa` with no station authority; organizations S and T.
- **When** a member of S and a member of T each submit a claim for the station within the same minute; both complete their proofs; a platform reviewer approves S's claim; a second reviewer sends a decision on the same claim at the same moment.
- **Then** both claims are stored and follow their own states; neither organization holds a station authority or passes a station permission check (`stn.price.publish` is denied) before a platform decision; approving S creates one authority that is `ACTIVE` for S and gives T nothing; T's claim stays for its own decision and T can open a contest case; the second reviewer's command receives `CONFLICT`; every decision is audited with the reviewer's staff identity. Sharing a CNPJ root with the station's operator gave neither claimant anything.
- **Proves** BR-TEN-002, BR-AUTH-015, EDR-021 · **Profiles** `V-IDEM`, `V-AUTHZ`, `V-SQL` · **Run by** EMP-REL-008.

### VAL-18 — The same billing event delivered twice

- **Given** organization A with a subscription in `PAST_DUE` and one provider event, identified by (provider, event identifier), that reports the overdue invoice as paid.
- **When** the event is delivered, delivered again with the same identifier, and delivered a third time concurrently with the second.
- **Then** the subscription moves to `ACTIVE` once; the state history has one transition and the audit trail one event; the second and third deliveries return the stored result; no capability, limit or period is applied twice; no role assignment changes.
- **Proves** BR-AUTH-009, EDR-024, the billing-event constraint of [06-domain-model](06-domain-model.md) §5 · **Profiles** `V-IDEM`, `V-JOB`, `V-SQL` · **Run by** EMP-REL-009.

### VAL-19 — Amendment of an approved fueling in a locked period

- **Given** the approved fueling of FR-001 (unit `Sul`, `America/Sao_Paulo`) with `occurred_at = 2026-11-01T02:30:00Z`, 150 000 mL and 97 350 centavos. Its local time is 2026-10-31 23:30, so its reporting day is 2026-10-31 and it belongs to October 2026, which is `LOCKED`.
- **When** (a) Diego tries to amend the quantity to 145 000 mL and the paid amount to 94 105 centavos; (b) path 1: Carla reopens October with a reason, Diego submits the amendment, Diego tries to approve it, then Carla approves it; (c) path 2 (fresh run, no reopening): the correction is posted as an adjustment in November.
- **Then** (a) fails with `PERIOD_LOCKED` and changes nothing. (b) October is `REOPENED`; the amendment is revision 2 with author, time and reason, linked to revision 1, which stays in effect until approval; Diego's own approval fails with `SELF_APPROVAL`; after Carla's approval revision 2 is in effect with `145 000 × 6 490 / 10 000 = 94 105` centavos, R$ 941.05, the October cost falls by `97 350 − 94 105 = 3 245` centavos, and the audit trail holds the reopening, the amendment and the decision. (c) October still shows 97 350; November shows an adjustment of −3 245 centavos linked to the original fueling; the sum over both periods is 94 105.
- **Proves** BR-AMD-001, BR-AMD-002, BR-AMD-004, BR-TIME-004, BR-REV-003, BR-REV-007, BR-AUD-001 · **Profiles** `V-SQL`, `V-AUTHZ`, `V-MONEY` · **Run by** EMP-REL-004.

### VAL-20 — Recalculation after a correction

- **Given** the interval of VAL-10 as revision 1: 10 km/L and R$ 0.61 per kilometre.
- **When** the partial fueling is amended from 20 000 mL and 12 200 centavos to 25 000 mL and 15 250 centavos (`25 000 × 6 100 / 10 000 = 15 250`), and the amendment is approved by someone who is not its author.
- **Then** until approval the interval stays at revision 1. After approval revision 2 is `COMPUTED` from `20 000 + 25 000 = 45 000 mL` and `15 250 + 12 200 = 27 450` centavos: consumption `400 / 45 = 8.888…`, displayed 8.89 km/L; cost per kilometre `274.50 / 400 = 0.68625`, displayed R$ 0.69. Revision 1 is `SUPERSEDED` and still readable; every indicator that used it names the revision it now uses; a finding that used revision 1 is re-evaluated. Running the recalculation job twice produces no third revision.
- **Proves** BR-AMD-002, BR-AMD-005, BR-TRUTH-004, BR-MONEY-008 · **Profiles** `V-JOB`, `V-DOM`, `V-MONEY` · **Run by** EMP-REL-004 and EMP-REL-007.

### VAL-21 — Authorization matrix conformance

- **Given** the fixture and the matrix of [07-permissions](07-permissions.md) §7.3 as a machine-readable vector (§5).
- **When** the generated suite runs every cell with the cases of §5, and the ten sample rows of §5 run as written.
- **Then** every `O` cell succeeds inside the assigned scope; every `S` cell succeeds on the member's own records and assigned assets and is `FORBIDDEN` elsewhere; every `—` cell is `FORBIDDEN` naming the permission key and returns no record data; a suspended membership, an ended membership, a suspended organization, a capability turned off and a member of organization B each receive the error of the decision order in 07-permissions §10; a route that is registered outside the authorization pipeline fails the suite.
- **Proves** BR-AUTH-001, BR-AUTH-002, BR-AUTH-003, BR-AUTH-004, BR-AUTH-007, BR-AUTH-013, UC-AUTH-05 · **Profiles** `V-AUTHZ`, `V-ISO` · **Run by** every gate, over the permissions delivered so far · **Built by** EMP-AUTH-009.

### VAL-22 — A suspended organization: writes blocked, reads kept

- **Given** organization A `ACTIVE`. Part 2 only: Bruno's phone holds one fueling declared at 09:00.
- **When** a platform operator suspends A at 10:00 with a reason. Part 1: Carla reads the employee list and Ivo tries to create a unit. Part 2, from the phase that has field operations: the phone delivers the 09:00 fueling at 11:00, and then a trip declared at 12:00. Finally A is reactivated.
- **Then** part 1: the read succeeds; the write fails with `ORG_SUSPENDED` and writes nothing; organization B is unaffected. Part 2: the 09:00 fueling is `ACCEPTED_FOR_REVIEW` with `ORG_SUSPENDED_GRACE`, because it was declared before the suspension and arrived inside the 7-day grace period; the 12:00 trip is `REJECTED` with `ORG_SUSPENDED` and stays visible on the phone with its reason. After reactivation writes succeed again; no data and no role assignment was deleted or changed by the suspension.
- **Proves** BR-AUTH-001, BR-OFFL-006, BR-OFFL-010 · **Profiles** `V-AUTHZ`, `V-SYNC` · **Run by** EMP-REL-003 (part 1) and EMP-REL-005 (part 2).

### VAL-23 — Sync resilience: process death, reordering and an interrupted attachment

- **Given** Bruno's emulator offline with a trip start, three logbook events, one fueling with the 412 338-byte attachment and a trip end, each depending on the previous one where the protocol says so.
- **When** (a) the process is killed right after the interface shows "Salvo no aparelho", then restarted; (b) the batch is sent three times; (c) the operations are delivered in reverse order; (d) the attachment upload is cut at byte 200 000 and retried; (e) the upload is sent once with one byte changed; (f) the device clock is set 12 minutes ahead while the limit is 5 minutes.
- **Then** (a) all six operations are in the queue after the restart; (b) one record per operation and three identical acknowledgements each; (c) an operation whose dependency is not yet accepted is answered `RETRY` with `DEPENDENCY_PENDING`, and the final server state equals the in-order state; (d) the stored file has 412 338 bytes and the declared SHA-256 and the document is `STORED`; (e) the bytes are refused and the app retries from its stored file; (f) the operations of the batch are flagged `CLOCK_SKEW` and their declared times are kept, not rewritten. Nothing was dropped without a visible state.
- **Proves** BR-OFFL-002, BR-OFFL-004, BR-OFFL-007, BR-OFFL-015, BR-TIME-006, UC-OFFL-01, UC-OFFL-03 · **Profiles** `V-SYNC`, `V-IDEM` · **Run by** EMP-REL-005 · **Built by** EMP-OFFL-013.

### VAL-24 — A capability switched off and on again

- **Given** organizations A and B with the fueling capability enabled and three approved fuelings in A.
- **When** an operator turns the capability off for A only; Bruno tries a new fueling online; Bruno's phone delivers a fueling declared before the switch; Carla lists and exports A's fuelings; the operator turns the capability on again; then the global flag is turned off and on.
- **Then** with the capability off the feature is hidden in the app and the panel after the next refresh; the online write fails with `ENTITLEMENT_REQUIRED`; the fueling declared before the switch is not discarded and is kept for review (BR-OFFL-006); the three fuelings stay readable and exportable; no role assignment changed; B is unaffected. With the capability on again writes succeed. With the global flag off the organization routes are not served, and turning it on again shows the same rows: nothing was deleted by either switch.
- **Proves** BR-AUTH-009, BR-OFFL-006, and the activation rules of [09-architecture](09-architecture.md) §10 · **Profiles** `V-AUTHZ`, `V-SQL` · **Run by** EMP-REL-002 · **Built by** EMP-PLAT-005, EMP-AUTH-004.

### VAL-25 — Return loop and two approval levels

- **Given** an approval policy of A (fixture parameter): expenses above 50 000 centavos need level 1 by the unit approver and level 2 by finance. Bruno submits an expense of 74 200 centavos, revision 1.
- **When** Fábio returns it with the reason "Falta o comprovante do pedágio"; Bruno attaches the receipt and resubmits as revision 2; Fábio approves level 1 on revision 2; Diego sends a level-2 approval that names revision 1; Diego then approves revision 2.
- **Then** after the return the expense is `RETURNED` with the reason and revision 1 kept; after level 1 it is still `IN_REVIEW` and not in the cost total, and a view that includes it says "includes pending"; the decision on revision 1 fails with `CONFLICT` and changes nothing; after level 2 it is `APPROVED` and the cost total rises by 74 200 exactly once. Bruno cannot decide on his own expense. Variation, when the policy allows a partial approval: with a company amount of 70 000 of the 74 200, the cost counted is 70 000 and the remaining 4 200 is never a company cost.
- **Proves** BR-REV-002, BR-REV-003, BR-REV-005, BR-REV-007, BR-REV-008, BR-REV-009 · **Profiles** `V-AUTHZ`, `V-IDEM`, `V-SQL` · **Run by** EMP-REL-004.

### VAL-26 — A late event and an amendment after a trip is closed

- **Given** the trip of VAL-01, `COMPLETED` and `APPROVED`, with an odometer distance of 290 000 m.
- **When** (a) a toll event that Bruno recorded offline during the trip arrives after the approval; (b) the same event is delivered again; (c) Elisa corrects the final reading from 184 610 000 m to 184 600 000 m with a reason, and Ivo approves the resulting amendment.
- **Then** (a) the event is not dropped and does not change the approved revision in place: it is kept for review with the reason code defined in [modules/trips](modules/trips.md); (b) one event exists; (c) the correction is a new reading that supersedes the old one, which is kept; the trip amendment is a new revision; until approval the distance stays 290 000 m; after approval it is `184 600 000 − 184 320 000 = 280 000 m`; derived figures are recalculated and name the revision used; the audit trail holds the correction, the amendment and the decision.
- **Proves** BR-AMD-001, BR-AMD-002, BR-AMD-005, BR-OFFL-006, BR-AUD-001 · **Profiles** `V-SYNC`, `V-SQL`, `V-AUTHZ` · **Run by** EMP-REL-005.

### VAL-27 — GPS distance against odometer distance

- **Given** a trip with an odometer distance of 290 000 m and a processed track. Fixture parameters: divergence measured against the odometer distance, limit 5 % (both owned by [modules/gps-navigation](modules/gps-navigation.md)).
- **When** the track gives a GPS distance of 262 000 m with one gap of 14 minutes; and, in a second run, 284 000 m with no gap.
- **Then** both distances are stored and shown as separate numbers with their sources, and neither replaces the other; in the first run the divergence is `|290 000 − 262 000| = 28 000 m`, `28 000 / 290 000 = 9.66 %`, above the limit, so a divergence signal asks for review and shows the gap; in the second run it is `6 000 / 290 000 = 2.07 %` and no signal is raised. A trip with no usable track shows the GPS distance as `INSUFFICIENT_DATA`, not zero.
- **Proves** BR-TRUTH-003, BR-TRUTH-004, BR-TRUTH-005, BR-MONEY-009 · **Profiles** `V-DOM`, `V-JOB` · **Run by** EMP-REL-005.

### VAL-28 — A tracking interruption becomes a gap

- **Given** a tracking session `RECORDING` for a trip of A on an emulator, one fix every 10 seconds.
- **When** (a) the location permission is revoked at 10:20:00 and granted again at 10:35:00, and Bruno resumes; (b) the app process is killed at 11:00:00 and reopened at 11:06:00; (c) in a second run only approximate location is granted; (d) the trip is paused and then ended.
- **Then** (a) the session is `INTERRUPTED` and the track has a gap of `10:35:00 − 10:20:00 = 900` seconds with no invented fixes; (b) recording does not resume silently: the app asks Bruno to resume, and the gap is 360 seconds; (c) the track quality is `APPROXIMATE` and is shown as such; (d) no fix is recorded while the trip is paused or after it ends, and the session is `ENDED`. The manifest requests no background location and the location guard in `scripts/check-mobile.sh` passes with the tracking service listed. Behaviour on real devices of several manufacturers is owed evidence (§7).
- **Proves** BR-PRIV-006, BR-PRIV-015, BR-TIME-005, EDR-015 · **Profiles** `V-APP`, `V-SYNC`, `V-PRIV` · **Run by** EMP-REL-005.

### VAL-29 — Trip-aware navigation keeps one trip

- **Given** a trip of A in progress with a destination, a route plan with a planned distance of 42 300 m, a downloaded map region and a tracking session `RECORDING`.
- **When** Bruno starts navigation from the trip screen with no network, leaves the route twice and is rerouted, and arrives; the odometer distance is 44 000 m and the GPS distance 44 100 m.
- **Then** there is still one trip and one tracking session; the stored batches contain no duplicated fix; the planned distance stays 42 300 m and the other two are stored beside it as separate numbers, never added or substituted; route computation and guidance ran on the device and no position was sent to a map provider; the trip screen shows that tracking is active for the whole time.
- **Proves** EDR-016, BR-PRIV-006, BR-TRUTH-004 · **Profiles** `V-APP`, `V-SYNC` · **Run by** EMP-REL-006.

### VAL-30 — A geofence evaluated on the server

- **Given** a circular geofence of A with a radius of 200 m and no dwell time (fixture parameters), and a trip in progress.
- **When** one batch arrives with four fixes at 350 m, 150 m, 120 m and 260 m from the centre, in that order; then the same batch is delivered again.
- **Then** the server produces one entry event at the second fix and one exit event at the fourth; the resend produces no further event and no second alert; each alert shows the collection time of its fix; the evaluation used only received fixes, with no device geofencing and no background location; organization B receives nothing.
- **Proves** BR-TRUTH-006, BR-OFFL-005, BR-PRIV-003 · **Profiles** `V-JOB`, `V-IDEM`, `V-ISO` · **Run by** EMP-REL-006.

### VAL-31 — Maintenance due from a meter and a date

- **Given** FR-001 with a preventive plan every 10 000 000 m or 180 days, whichever comes first (fixture parameters), last done at 180 000 000 m on 2026-06-01. The next occurrence is therefore due at 190 000 000 m or on 2026-11-28 (`2026-06-01 + 180 days`).
- **When** the controlled clock is 2026-11-10 with a latest reading of 184 610 000 m; then a reading of 190 000 000 m arrives on 2026-11-20; then, in a second run with no new reading, the clock reaches 2026-11-28; then a reading of 190 400 000 m is voided and corrected to 189 400 000 m.
- **Then** on 2026-11-10 the occurrence is `UPCOMING` with `190 000 000 − 184 610 000 = 5 390 000 m` left; with the 190 000 000 m reading it is `DUE` by distance; in the second run it is `DUE` by date; after the correction it returns to `UPCOMING` and the computation names the reading it used; an asset with no reading shows `INSUFFICIENT_DATA` for the distance trigger, not "0 km left". The job run twice creates one occurrence.
- **Proves** BR-TRUTH-003, BR-TRUTH-004, BR-AMD-005 · **Profiles** `V-JOB`, `V-DOM` · **Run by** EMP-REL-007.

### VAL-32 — Internal stock counted once

- **Given** an internal tank of A holding 1 000 000 mL valued at 500 000 centavos (R$ 5.00 per litre).
- **When** a purchase of 1 000 000 mL for 600 000 centavos is received into the tank, and then 100 000 mL are dispensed to FR-001.
- **Then** after the receipt the tank holds 2 000 000 mL valued at `500 000 + 600 000 = 1 100 000` centavos, a moving average of R$ 5.50 per litre; the dispensing creates one internal fueling whose cost is `100 000 mL × R$ 5.50 = 55 000` centavos; the fuel cost of the period is 55 000, not `600 000 + 55 000`; the purchase appears as stock and as a payable, never as a consumption cost; the tank then holds 1 900 000 mL valued at `1 100 000 − 55 000 = 1 045 000` centavos.
- **Proves** EDR-013, EDR-029, BR-MONEY-008 · **Profiles** `V-MONEY`, `V-SQL` · **Run by** EMP-REL-007 · **Built by** EMP-STOCK-005.

### VAL-33 — A paying station gains no rank

- **Given** stations `Posto Fictício Alfa` and `Posto Fictício Beta` with identical community observations and official prices; S holds an `ACTIVE` authority over Alfa, an active plan and one announced price of 5 990 milli-BRL per litre.
- **When** the public list, the ranking and the station pages are read; then S's plan is cancelled and the announced price withdrawn, and they are read again.
- **Then** organic ordering, confidence, consensus values, moderation outcomes and review visibility are identical in both reads for both stations; the announced price was shown under its own label with author, validity and conditions, and never entered the community consensus or any average; Beta's position did not move.
- **Proves** BR-SRC-001, BR-SRC-002, EDR-022 · **Profiles** `V-SQL`, `V-API` · **Run by** EMP-REL-008 · **Built by** EMP-STN-010.

### VAL-34 — An agreement shares only its transactions

- **Given** an agreement between A (fleet) and S (station Alfa) with a negotiated price of 6 290 milli-BRL per litre; A issues one fueling authorization for FR-001 limited to 150 000 mL.
- **When** the station redeems it for 150 000 mL; redeems the same code again; a member of S requests A's trips, employees and other fuelings; a member of A requests S's other agreements; the public price endpoints are read.
- **Then** the transaction amount is `150 000 × 6 290 / 10 000 = 94 350` centavos; A has one fueling and one cost of 94 350; S has one receivable of 94 350; the second redemption is refused and changes nothing; S reads only the fields that the agreement lists for that transaction ([modules/station-commerce](modules/station-commerce.md)) and receives `NOT_FOUND` for everything else of A; A receives `NOT_FOUND` for S's other agreements; the negotiated price appears in no public response.
- **Proves** BR-TEN-005, BR-TEN-006, BR-SRC-001, EDR-023 · **Profiles** `V-ISO`, `V-IDEM`, `V-MONEY` · **Run by** EMP-REL-008 · **Built by** EMP-AGREE-009.

### VAL-35 — Effects of a subscription state and a plan limit

- **Given** organization A on a plan with a limit of 10 active vehicles (fixture parameter) and exactly 10 active; Bruno's phone holds one fueling declared before any change.
- **When** Elisa activates an eleventh vehicle; the subscription then leaves `ACTIVE` for the state in which [modules/subscriptions](modules/subscriptions.md) blocks new writes; Carla reads and exports; the phone delivers the fueling.
- **Then** the eleventh activation fails with `LIMIT_REACHED` and the ten vehicles are untouched; in the blocking state new writes are refused while reads and exports work; no role assignment was added, changed or removed by any plan change; the fueling declared before the change is not discarded and is kept for review; the individual Premium of any member is unaffected in both directions.
- **Proves** BR-AUTH-009, BR-OFFL-006, EDR-024 · **Profiles** `V-AUTHZ`, `V-SYNC` · **Run by** EMP-REL-009.

### VAL-36 — No private organization data in a dataset

- **Given** a dataset definition built from public sources, and organization A holding 500 approved fuelings at `Posto Fictício Alfa` with negotiated prices, documents and tracks.
- **When** the dataset is built twice, once with A's records present and once with A's records absent; a source marked as lacking redistribution rights is added; a key of A's Data API requests data of organization B.
- **Then** the two builds are byte-identical, so A's private records contributed nothing; the source without recorded rights is excluded and the build says so; a group below the aggregation threshold (fixture parameter) is suppressed, not published; the cross-organization request returns `NOT_FOUND`.
- **Proves** BR-SRC-005, BR-TEN-006, EDR-025 · **Profiles** `V-ISO`, `V-PRIV` · **Run by** EMP-REL-009 · **Built by** EMP-DATA-007.

### VAL-37 — Capacity benchmark with a synthetic fleet

- **Given** a synthetic fleet of 300 moving vehicles, each sending one batch of 6 fixes every 60 seconds (one fix every 10 seconds), plus the read and write mix of §6, for 30 minutes.
- **When** the benchmark runs against real PostGIS with the API and worker roles on the documented hardware.
- **Then** the input is `300 / 60 = 5` batches per second, `5 × 1 800 = 9 000` batches and `9 000 × 6 = 54 000` fixes; the hypothesis for stored track size is `54 000 × 25 = 1 350 000` bytes. The run records the measured intake p50, p95 and p99, queue delay, table and index growth, CPU and memory, and compares each with the hypotheses of [09-architecture](09-architecture.md) §9. The scenario passes when every figure is measured and recorded and each threshold that was crossed has its listed next step opened as a task. It never passes by restating a hypothesis.
- **Proves** the capacity hypotheses of 09-architecture §9, AS-03 · **Profiles** `V-PERF` · **Run by** EMP-REL-010.

### VAL-38 — Retention expiry and restore

- **Given** a controlled clock; an export file of A created on 2026-11-01 with a 7-day period (expires 2026-11-08); raw track fixes of a trip that ended on 2026-08-01 with a 90-day period (expire 2026-10-30); a legal hold on the records of FR-002; a backup taken on 2026-10-25.
- **When** the clock is 2026-11-10 and the retention job is stopped; then the job runs twice; then the backup of 2026-10-25 is restored.
- **Then** with the job stopped the export and the fixes are already unreadable, because expiry is checked at read time; the job deletes the file and the fixes, keeps the simplified geometry and the trip summary, writes the deletion ledger and deletes nothing under the hold; the second run changes nothing; after the restore the ledger is replayed before the system serves traffic, so the fixes that the backup contained are purged again and are never served; purge events carry identifiers and no content.
- **Proves** BR-PRIV-009, BR-PRIV-010, BR-PRIV-011, BR-PRIV-008, UC-PRIV-02, UC-PRIV-04 · **Profiles** `V-PRIV`, `V-JOB`, `V-SQL` · **Run by** EMP-REL-007 · **Built by** EMP-PRIV-001, EMP-PRIV-004.

## 5. Authorization sampling

The matrix of [07-permissions](07-permissions.md) §7.3 is not sampled when it is tested: every cell is run. "Sampling" names two smaller choices, described at the end of this section.

**From the matrix to tests.**

1. The role catalog `org-roles-v1` and permission registry `org-permissions-v4` are frozen source data. The independent [authorization-catalog-v4.psv](../../contracts/testdata/enterprise/authorization-catalog-v4.psv) records each permission/role cell, coverage (`O`, `S` or `-`) and footnote conditions. Tests consume the literal fixture; expected decisions are never exported from the catalog being tested.
2. Current v4 has 85 registered keys × 13 roles = 1,105 descriptor cells: 215 `O`, 54 `S` and 836 denied. This includes explicit catalog amendments in 07-permissions and registered-but-unassigned keys; the two human presentation tables are not the complete cross-product. A fixture/catalog/documentation mismatch blocks the gate. Historical catalog versions retain their original evidence.
3. Every ordinary organization route declares its permission and complete current-authority descriptor. The full production-router gate checks registrations and OpenAPI in both directions; scoped session/bootstrap/self/device entries have their separate explicit boundaries. The independent [authorization-routes-v1.json](../../contracts/testdata/enterprise/authorization-routes-v1.json) inventories every current route, one real representative per routed permission and every not-yet-routed obligation. Adding a route even under an existing permission requires an explicit fixture/test update. Unguarded, undocumented or incomplete-authority registrations fail before matrix execution ([09-architecture](09-architecture.md) §13).
4. For each cell the generator builds a member of organization A with exactly that role and runs these cases against a route of that permission:

| Case | Setup | Expected |
|---|---|---|
| In scope | Role assigned on the scope that covers the target | `O`: success. `S`: success on an own record or assigned asset. `—`: `FORBIDDEN` naming the key, no record data |
| Own-only | `S` cell, target authored by another member | `FORBIDDEN` naming the key |
| Wrong scope | Role assigned on another unit, cost centre, team or asset | `FORBIDDEN` naming the key |
| Suspended membership | Membership `SUSPENDED` | `MEMBERSHIP_SUSPENDED` |
| Ended membership | Membership `ENDED` | Denied, no data |
| Suspended organization | Organization `SUSPENDED` | Operational/configuration writes denied; authorized reads, access reduction and plan regularization retain the existing source-policy exceptions |
| Capability off | Scoped pilot capability flag off for A | Operational/configuration writes require entitlement; authorized reads, access reduction, plan regularization and restoration retain AUTH-004 exceptions. Global disable denies every operation |
| Other organization | Same role held in B, target in A | `NOT_FOUND`, nothing beyond the envelope |
| Stale session | Otherwise authorized sensitive action without fresh authentication | `FRESH_AUTH_REQUIRED`; an already-denied permission cell remains denied in the existing source gate order |

5. The four footnotes of the matrix are explicit generated cases: an `ADMIN` cannot assign `OWNER`; a driver sees the position and track of own trips only; an auditor reads recorded tracks and never live positions; a fleet manager acts on the fleet side of an agreement only.
6. Separation of duties is tested per record, not per cell: for every review permission one case has the reviewer as the author or the beneficiary and expects `SELF_APPROVAL`.
7. The suite grows with the product. At M1/C1, exercise ORG/SELF only where actual source/feature routes exist. At AUTH-009 opening, 69 ordinary routes use 12 permissions; 15 additional entries use explicit account/session/bootstrap/self/device boundaries. The routed permission matrix has 156 descriptor cells. Three station roles have no persistable F1 authority and remain 36 unavailable-scope cells, leaving 120 current-source cells (41 positive, 79 denied) over10 persistable roles. No routed permission has an `S` descriptor yet. The remaining 73 keys are not yet routed; they are never labeled business acceptance. Unit/cost-centre/team/asset scopes enter with AUTH-006, delegation with AUTH-007, station roles with F7, and each SELF/review business rule with its real producer. The independent inventory and complete-router gate must be updated as the phase adds routes; these opening counts are not future acceptance.

**The two sampling choices.** First, a permission that guards several routes is exercised cell by cell on one representative route; the other routes are covered by the route-to-permission declaration of step 3 and by the isolation harness (VAL-02). Second, ten rows are also written by hand as readable regression tests, so that a mistake in the generator cannot hide a mistake in the matrix:

| # | Actor (role, scope) | Permission | Target | Expected |
|---|---|---|---|---|
| 1 | Ana (`SUPERVISOR`, `UNIT` Norte) | `trip.trip.read` | Jonas's trip in unit Norte | Allowed |
| 2 | Ana (`SUPERVISOR`, `UNIT` Norte; `DRIVER`, `SELF`) | `trip.trip.read` | Bruno's trip in unit Sul | `FORBIDDEN` naming `trip.trip.read`, no trip data |
| 3 | Ana (`SUPERVISOR`, `UNIT` Norte) | `trip.trip.review` | Her own trip in unit Norte | Permission passes; `SELF_APPROVAL` |
| 4 | Bruno (`DRIVER`, `SELF`) | `fuel.fueling.record` | A fueling on FR-001, assigned to him | Allowed |
| 5 | Bruno (`DRIVER`, `SELF`) | `fuel.fueling.review` | Any fueling | `FORBIDDEN` naming `fuel.fueling.review` |
| 6 | Diego (`FINANCE`, `ORG`) | `fin.payment.record` | The reimbursement to Bruno | Allowed |
| 7 | Ivo (`ADMIN`, `ORG`) | `auth.role.assign` | Give `OWNER` to Elisa | `FORBIDDEN` (BR-AUTH-007) |
| 8 | Gil (`AUDITOR`, `ORG`) | `gps.location.read_live` | Any trip in progress | `FORBIDDEN` naming `gps.location.read_live` |
| 9 | Gil (`AUDITOR`, `ORG`) | `gps.track.read` | The recorded track of a finished trip | Allowed; one access-log entry |
| 10 | Rui (`OWNER` of organization B) | `fleet.asset.read` | FR-001 of A, by identifier, through A's path | `NOT_FOUND`, nothing beyond the envelope |

Rows 1–3 are examples 1–2 of 07-permissions §10 and need actual unit scopes/trip routes; their owners remain AUTH-006/TRIP until those sources exist. Rows 4–6 require the real fueling/review/payment routes. Row 7 is exercised through actual role assignment and its last-owner/current-manager constraints. Rows 8–9 require tracking sources and access logging at M4. Row 10 enters with real fleet reads. Until each producer exists, the inventory reports that obligation explicitly; static descriptors or synthetic resources are not proof that its business route works.

## 6. Capacity checks

Every figure in [09-architecture](09-architecture.md) §9 is a hypothesis. Nothing in this plan treats those figures as measurements, and no document may quote them as capacity until EMP-REL-010 has recorded a measurement with its hardware, data size and revision. The benchmark does not need production: it needs real PostGIS, the API and worker roles and a synthetic fleet.

| Check | Synthetic input | Measured | Hypothesis from 09-architecture §9 | If the measurement crosses it |
|---|---|---|---|---|
| Intake at the existing acceptance workload | 300 vehicles moving, one batch per 60 s: 5 batches per second for 30 minutes (VAL-37) | Operation intake p50, p95, p99; pool wait; errors | p95 at or below 500 ms | Tune batch interval and indexes; dedicated dispatcher |
| Intake with a doubled interval | The same fleet with one batch per 120 s: `300 / 120 = 2.5` batches per second, 12 fixes per batch | The same | Halving the rate is the first response to load | Record whether it helps before any infrastructure step |
| Intake above the acceptance workload | 1 000 vehicles: `1 000 / 60 = 16.7` batches per second | The same, and the point where p95 crosses the threshold | None: this finds the limit | Document the measured pilot capacity; scale the host only on the measured signal |
| Track storage | One month for 100 vehicles: `47 520 × 100 = 4 752 000` fixes | Table and index bytes per fix | About 25 bytes per fix in batch rows, about 119 MB a month | Revisit the batch layout; monthly partitions above 20 GB |
| Queue delay | Exports and track processing enqueued during the intake run | Delay from enqueue to start | At or below 5 minutes | Dedicated worker role |
| Panel polling | 20 panel sessions polling the live map every 15 s and queues every 60 s | Share of API requests; latency of the "positions since revision" read | Polling below half of API load | Reconsider server-sent events (EDR-026) |
| Indicator read models | One year of operational rows for 1 000 vehicles (about 3 GB by hypothesis) | Rebuild time and list latency with keyset pagination | None stated | Add only the indexes that the measured queries need |
| Host signals | All of the above | CPU and memory | Existing triggers: CPU above 70 %, memory above 75 % | The steps of `INFRASTRUCTURE_PLAN.md` |

The 10 000-vehicle column of 09-architecture §9 is not benchmarked in this plan: at `10 000 / 60 = 167` batches per second it is beyond one host by hypothesis, and it is a sizing discussion, not a target. Storage cost figures are arithmetic on public prices and are rechecked against the provider chosen at the time (AS-05).

## 7. What cannot be validated locally

These items stay owed after every local gate. A gate record lists them as owed; it never marks them as passed, and nothing is accepted because a plan describes it.

| Kind | What is owed | Why it cannot be local | Who provides it |
|---|---|---|---|
| Device | The tracking interruption matrix on real phones of at least three manufacturers: screen off, app swiped away, reboot, force stop, battery saver, Doze | An emulator does not reproduce manufacturer background behaviour ([18-references](18-references.md) §1 lists what is unverified) | EMP-GOV-006 for the spike; the owner or pilot users for the M4 gate, on devices they consent to use |
| Device | Battery and mobile-data use per hour of tracking; receipt photo legibility from real cameras; storage pressure with many photos | Hardware dependent | The same |
| Device | Keystore-backed protection and the app lock on real hardware, including a device with no secure lock screen | Emulator keystores differ | EMP-GOV-008, then the M1 pilot devices |
| Provider | Object storage behaviour of the real provider (the repository already holds that emulation is insufficient evidence for storage) | Local S3-compatible storage is an emulation | One staging run per storage change |
| Provider | Billing events from a provider's test mode; e-mail and push delivery; tile hosting for the panel map | No provider is configured for the suite | EMP-GOV-009, EMP-GOV-014, EMP-GOV-007 decide; the adapter tasks then record a test-mode run |
| Provider | Fiscal document consultation with a real certificate | Needs an organization's own certificate | EMP-GOV-011 decides; EMP-INTEG-004 records it |
| Legal | Answers to LV-01 to LV-13, the retention schedule, the employee notice, the processing terms and the labour boundary | Engineering cannot declare compliance | A qualified reviewer chosen by the owner, through EMP-GOV-005 |
| Legal | Redistribution rights for public sources | Depends on the source owners | EMP-GOV-016 |
| Owner | Acceptance of each decision in [16-decisions](16-decisions.md); sequencing against the existing roadmap | Only the owner decides | EMP-GOV-001 to EMP-GOV-016 |
| Owner | Real-production certification (the project's G09), Play distribution status and target API level (OQ-16) | Production and store accounts are outside local validation | The project's existing gates |
| Owner | Staffing evidence for station review and support (AS-11); named pilot organizations and their consent | People, not code | EMP-GOV-010, EMP-GOV-015 |
| Field | Usability with real drivers and managers: time to record a fueling, error rates, comprehension of the sync states | Needs real users | Pilot of cut C1, measured as in [02-product-vision](02-product-vision.md) §9 |

## 8. Exit criteria per milestone

A milestone is reached when its gate task records every listed scenario as passed on one named revision, with VAL-02 and VAL-21 rerun over the routes and permissions added in the phase, and with the owed items of §7 listed. The table agrees with [14-roadmap](14-roadmap.md) §3 and with the gate tasks in [backlog/ep20-release](backlog/ep20-release.md).

| Milestone | Gate task | Scenarios that must pass | Other exit conditions |
|---|---|---|---|
| M0 Decisions recorded | None; checked with `V-DOC` | — | EMP-GOV-001, EMP-GOV-002, EMP-GOV-003, EMP-GOV-005, EMP-GOV-008 and EMP-GOV-012 recorded; the remaining governance tasks scheduled; the plan check of §9 reports no problem |
| M1 A company exists with its people | EMP-REL-003 | VAL-02, VAL-03, VAL-21, VAL-22 (part 1) | The last owner cannot be removed through any path; global flag off by default; one internal test organization only |
| C1 First operational cut | EMP-REL-001, then EMP-REL-002 | VAL-01, VAL-02, VAL-05, VAL-06, VAL-07, VAL-08, VAL-21; VAL-24 for activation | Run on real PostGIS and an emulator; runbook and rollback rehearsed; legal validation (EMP-GOV-005) before any real data; named pilot organizations only |
| M2 Assets under control | None | VAL-02 and VAL-21 over the F2 routes | Fleet and meter tasks of F2 done with their own tests; imports of employees and assets usable |
| M3 Every cost has a record, a document and a decision | EMP-REL-004 | VAL-08, VAL-09, VAL-10, VAL-11, VAL-12, VAL-13, VAL-14, VAL-19, VAL-20, VAL-25 | No double counting in the allocation and cost tests; consumption intervals recomputed after corrections |
| M4 Operations are planned and followed | EMP-REL-005 | VAL-04, VAL-22 (part 2), VAL-23, VAL-26, VAL-27, VAL-28 | Tracking spike and legal validation done (EMP-GOV-005, EMP-GOV-006); the real-device interruption matrix recorded, or listed as owed and tracking not enabled for any organization |
| M5 The company sees operations as they happen | EMP-REL-006 | VAL-16, VAL-29, VAL-30 | Map decision recorded (EMP-GOV-007); list views work without the map |
| M6 The company understands and controls its fleet | EMP-REL-007, with EMP-REL-010 | VAL-10, VAL-11, VAL-15, VAL-20, VAL-31, VAL-32, VAL-38; VAL-37 for capacity | Indicator vectors shared by Go and Kotlin; findings lifecycle; measured capacity recorded |
| M7 Stations take part as businesses | EMP-REL-008 | VAL-17, VAL-33, VAL-34 | Stage D override recorded (EMP-GOV-015); staffing evidence recorded or the station front stays off |
| M8 The suite is sold and connected | EMP-REL-009 | VAL-18, VAL-35, VAL-36 | Billing and rights decisions recorded (EMP-GOV-009, EMP-GOV-016); no Data sale for a source without recorded rights |

## 9. Plan consistency check

The block below is generated by [tools/check_plan.py](tools/check_plan.py) and is never edited by hand ([00-conventions](00-conventions.md) §7).

<!-- CHECK-RESULT:BEGIN -->
Last run: 2026-11-14 with `python3 docs/enterprise/tools/check_plan.py --write`.

- Tasks parsed: 348; cut C1: 80.
- Identifiers defined: 545 BR, 167 UC, 145 SCR, 20 IND, 6 FT, 38 VAL, 135 EDR, 12 CX, 135 REQ.
- Problems found: 0.
<!-- CHECK-RESULT:END -->
