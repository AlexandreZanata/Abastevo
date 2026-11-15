# EMP-TRIP-018 — Open trips when a membership ends

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F4 API task (module TRIP, P1, type API, size S) in worktree branch
`codex/enterprise-f4-trip014`, on top of the EMP-TRIP-014 commit `f837b42d`
(migration 000235). When a membership ends through the EMP-IDN-010
offboarding, its trips in progress become flagged open responsibilities and
are resolved by a scoped supervisor's closure; offboarding never closes or
deletes them; the ended member's normal work is refused and a pre-end finish
delivered late is held for review (EMP-OFFL-009). Synthetic data only; no
Android, Kotlin, panel screen, job or deployment change.

Sources read: the EMP-TRIP-018 block ([EP06](../backlog/ep06-trips.md));
[trips](../modules/trips.md) §§6, 9 (UC-TRIP-03); identity BR-IDN-027..029,
UC-IDN-09, SCR-IDN-07 ([identity](../modules/identity.md)); JRN-04
([journeys](../05-journeys.md)); BR-OFFL-006/011/012 and §10.2
([offline sync](../10-offline-sync.md)); REQ-18.4 ([traceability](../17-traceability.md));
the evidence of [IDN-010](f1-idn010-offboarding.md) (reviewed digest,
pending-manager flags, `OPERATIONAL_WORK` as a pending producer, guard 000191),
[OFFL-009](f4-offl009-post-revocation-window.md) (late store, `LateHolder`,
`org_sync_late_end`, the meter late branch), [TRIP-002](f4-trip002-trip-persistence.md)
(held finishes) and [TRIP-014](f4-trip014-supervisor-closure.md); EDR-052,
EDR-197..199, EDR-202..206, EDR-257..259.

IDs used: B-BR-EMP-404..406, BUC-EMP-204..205, EDR-260..261 (all of them).
Migration `000236_organization_trip_membership_end.sql` (append-only).

## Rules frozen before behaviour

Rules B-BR-EMP-404..406 and BUC-EMP-204/205 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-018 — Open trips when a membership ends (2026-11-15)"; decisions
EDR-260 (trips as an explicit open-work port of the IDN-010 source, the guard
completeness check, resolution by the EMP-TRIP-014 closure, successor driver
retained to EMP-TRIP-012) and EDR-261 (late holder of `trip.finish` only)
are in [16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

Design answers:

- **Wiring into IDN-010.** The source existed: `OffboardingItems` (team links
  and settings) feeding a reviewed digest with mandatory flags, and the 191
  guard validating each flag against real source facts; trips were listed
  under the pending producer `OPERATIONAL_WORK`. Trips now join through an
  explicit port (`membership/application.OpenWork`), implemented by
  `trip/adapters.MembershipOpenWork` and wired in the runtime, instead of a
  membership query over a trip table (the sqlc ownership rule). `TRIP:<id>`
  at source revision 1 sorts after the existing keys inside the digest.
  `pending_sources` is unchanged: `OPERATIONAL_WORK` still names the kinds
  not yet produced (records awaiting correction, reviews awaited).
- **Never auto-closed or deleted.** Offboarding only flags; the database
  refuses an offboarding that leaves an open trip of the membership
  unflagged, even if the application source omits it, and trips stay
  insert-only. The open-trip list (TRIP-014) shows `author_membership_state`
  so supervisors find trips left by ended members and close them under their
  scoped authority.
- **Late intake.** The `trip.finish` owner implements the OFFL-009 holder for
  `ACCESS_ENDED_BEFORE_RECEIPT` only; the trip stays in progress. Starts,
  events and submissions keep the OFFL-009 default for owners without a held
  path (retained).

## Implementation

- Migration 000236: `org_offboarding_guard` (191 body, every line kept) admits
  `TRIP:<id>` flags at revision 1 only for a trip `IN_PROGRESS` started by the
  ending membership and refuses an offboarding leaving such a trip unflagged;
  `org_trip_held_source` (220 body, every line kept) gains a late-finish
  branch requiring the exact late source (`org_sync_late_end`), the trip's
  author, a declaration or an earlier claim before the end, the trip's meter
  and time order. No table, key or usage source changes.
- Membership: `HandoverItem.Validate` admits `TRIP:<canonical id>` at
  revision 1 only; `application.OpenWork` port; `OffboardingStore.Work`
  merges the keys (sorted) into the reviewed items of a membership that has
  not ended (the ended view keeps reading the stored flags).
- Trip: `adapters/late.go` (`MembershipOpenWork.OpenWorkKeysTx`;
  `finishHandler.HoldLate`: payload, stored trip of the organization —
  unknown `MALFORMED` as EDR-198 —, author = late source membership, pure
  finish rules, reading meter of the trip's asset, then the held finish);
  `heldCode` admits the late code for a finish (one line in `store.go`);
  queries `OpenTripsOfAuthor` and the author's membership state in
  `OpenTrips`; `OpenTrip.AuthorState`, projected as
  `author_membership_state`.
- Runtime: `enterprisesuite` sets `Work: trippg.MembershipOpenWork{}` on the
  offboarding store; the finish owner's late holder is captured by the
  existing registry (no wiring change).
- Contract: `EnterpriseHandoverItem.key` pattern gains `TRIP:<uuid>`;
  `EnterpriseTripOpenItem.author_membership_state` (ACTIVE, SUSPENDED,
  ENDED, required). No route, inventory, matrix or walk count changes.
- Test provisioning: `orgtest.GrantOffboardingCommands` also grants
  `SELECT(org_id,id,author_membership_id,state) ON org_trips` when the table
  exists (the guard and the port read it).

Shared files touched (for the parallel trip/identity agents):
`membership/domain/offboarding.go`, `membership/application/offboarding.go`,
`membership/adapters/offboarding.go`, `trip/adapters/store.go` (`heldCode`),
`trip/adapters/closure_store.go`, `trip/adapters/http/closure.go`,
`trip/application/closure.go`, `db/queries/trip/closures.sql`,
`platform/orgtest/fixture.go`, `platform/enterprisesuite/routes.go`,
`contracts/openapi/v1.yaml`, contract, EDR table, `ep06-trips.md`.

## RED

- Membership domain (`go test -race -json ./internal/modules/membership/domain -run TestOffboardingTripItems`),
  log `/data/dev/worktrees/f4-logs/trip018-go-red-domain.jsonl`: 1/1 failing
  (a trip item was not a handover item).
- PostGIS, against the migration as written and stubs (`MembershipOpenWork`
  answering no keys, `HoldLate` unavailable, `heldCode` and `Validate`
  unchanged), log `trip018-go-red-integration.jsonl`: 4/4 top-level failing.
  Three are behavioural (the open trip not listed; the offboarding refused by
  the new database guard because the stubbed source omitted the trip). The
  upgrade test failed for a fixture reason (its pre-upgrade population used
  the review intake without setting it); the fixture was corrected (set up
  like the TRIP-014 upgrade test, and its running trip moved to the database
  now so the pre-end finish precedes the offboarding), no expectation
  changed, so no behavioural RED is claimed for that test.
- The apicontract tests were written with the contract blocks and passed at
  once; no contract RED is claimed.

## GREEN and REFACTOR

- Unit: membership and trip packages pass under race.
- PostGIS, logs `trip018-go-green-integration-1.jsonl` → `-3.jsonl`: the
  first GREEN run showed every existing IDN-010 offboarding test failing with
  `register unavailable`: the new guard and port read `org_trips`, which the
  offboarding fixtures had not granted — the fixture grant was added to
  `orgtest.GrantOffboardingCommands` (no expectation changed). Second run:
  every `TestOffboarding*`, `TestRehire*`, `TestEmployeeMerge*`,
  `TestTripClosure*` and the three new behaviour tests pass; third run: the
  corrected upgrade test passes.

## Named negatives and acceptance → tests

| Requirement | Test(s) |
|---|---|
| AC: work flagged and handed over under scoped authority, auditable (BUC-EMP-204, JRN-04) | `TestTripOffboardingFlagsTheOpenTripAndNeverClosesIt` (review lists `TRIP:<id>` at revision 1; another member's review does not; flagged offboarding ends access; the trip row is byte-identical after the end; the flag stays readable; the open-trip list shows `ENDED`; an uncovering ASSET grant cannot close it, the ORG supervisor closes it) |
| Open trip silently skipped or deleted | an offboarding omitting the flag → `OPEN_RESPONSIBILITIES`; a store whose source omits the trip → refused by the database guard, membership still ACTIVE and trip unchanged; the trip row persists after the closure; trips stay insert-only (TRIP-014 schema test: delete 55000) |
| Ended member continues normal work | the ended member's normal intake is refused; a finish and a start declared after the end → `MEMBERSHIP_ENDED`, nothing stored (`TestTripLateIntakeAfterTheMembershipEnds`) |
| Permitted late intake held (OFFL-009) | the pre-end finish → `ACCESS_ENDED_BEFORE_RECEIPT`, one held finish with its reading, no finish, no reading, trip still IN_PROGRESS, replays byte for byte; a late finish of an unknown trip → `MALFORMED` |
| Lost-device intake admitted | `TestTripLateIntakeRefusesALostDevice` (LOST_OR_COMPROMISED revocation, then offboarding: whole-response `DEVICE_REVOKED`, no acknowledgement, nothing stored, trip unchanged) |
| Foreign organization | another organization's owner cannot read the ended member's responsibilities; TRIP-014 foreign closure and list tests |
| Domain | `TestOffboardingTripItemsAreExactAndMustBeFlagged` (canonical id, revision 1 only, malformed keys refused, unflagged trip `OPEN_RESPONSIBILITIES`) |
| Populated upgrade from the 000235 head | `TestTripMembershipEndPopulatedUpgradeFailureRecoveryAndPreviousImage` (injected failure leaves both functions and data; recovery once; no-op replay; previous image compatible; the pre-existing open trip is then flagged by its member's offboarding and its pre-end finish held late) |
| Contract | `TestTripMembershipEndContractShapes` (handover key vocabulary, author state enum and requirement); offboarding and closure contract tests |

## Validation

Same environment as EMP-TRIP-014 (toolchain go1.27.4, shared PostGIS through
`orgtest`, throwaway MinIO `f4-trip014-s3`):

```sh
gofmt -l ./internal ./cmd ./db                                       # empty
go build ./... && go vet ./... && go vet -tags=integration ./...      # clean
GOTOOLCHAIN=go1.27.1 staticcheck [-tags=integration] ./internal/modules/{trip,membership,organization}/... ./internal/platform/{enterprisesuite,apicontract,orgtest}/... ./cmd/api/...   # clean
sqlc vet && sqlc diff                                                 # clean
go test -json -race -tags=integration -count=1 -p 2 -timeout=90m ./internal/modules/organization/adapters ./internal/modules/{trip,membership,fleet,orgsync}/... ./internal/platform/... ./cmd/api/...
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...
python3 docs/enterprise/tools/check_plan.py
git diff --check
```

- Race/integration union (pre-merge tree), log `trip018-db-union.jsonl`
  (35 min 34 s; `organization/adapters` 2131 s): 37 packages, 35 pass and 2
  have no test files, 1267 top-level / 9704 named tests, 0 failures, 0
  skips; it includes every IDN-010 offboarding, rehire and merge suite with
  the trip grant, every TRIP-014 closure test, the late-intake suites and
  the storage-dependent suites with the throwaway MinIO.
- Untagged whole module, log `trip018-untagged-all.log`: exit 0, 223 packages ok, 0 failures.
- Static gates, log `trip018-staticcheck.log`: staticcheck plain and
  `-tags=integration` clean; toolchain gofmt empty; vet (both tags) clean;
  `sqlc vet`/`sqlc diff` clean.
- OpenAPI vacuum: 0 errors, 1071 warnings / 660 informs (unchanged from
  EMP-TRIP-014), log `trip018-vacuum.log`.
- `check_plan.py`: 0 problems; `git diff --check` clean; secret review of
  the staged diff: synthetic data only.
- No Kotlin, Android or deployment change; no Gradle run applies.
- The merge with the TRIP-011 lifecycle (`codex/enterprise-f4`) and the
  merged-tree gates are recorded in the follow-up section below.

## Retained obligations

- A successor driver taking over a running trip (EMP-TRIP-012) and its
  handover flag resolution; notifications to supervisors (UC-IDN-09 step,
  `PENDING_NOTIFICATION_CORE`).
- Late holders for `trip.start`, `trip.event.record` and `trip.submit` of an
  ended period (today `RETRY`/`UNAVAILABLE` until the window closes); the
  resolution of held late finishes (EMP-TRIP-007).
- Submitting a closed trip whose author's membership ended (no author can
  `trip.submit` it); a company-side submission path is a later decision.
- The remaining `OPERATIONAL_WORK` kinds (records awaiting correction,
  reviews awaited from the member) and suspended memberships' trips.
- Production runtime grants (not provisioned here): the offboarding
  principal `SELECT ON org_trips` (guard and port); the signed intake
  principal keeps the OFFL-009 late grants; the migrator owns
  `org_offboarding_guard()` and `org_trip_held_source()`.
- No C1, M4 or F4 acceptance, deployment or real data is claimed.

## Merge with EMP-TRIP-011 — merged-tree validation

See the EMP-TRIP-014 evidence (merge commit `cc5b5985`). For this task: a
`PAUSED` trip is an open responsibility (port, guard flag and completeness
check use `IN_PROGRESS` or `PAUSED`); `TRIP:<id>` keeps the fixed item
revision 1 (the trip's own revision is not a source of the flag, so a pause
during the review does not change it); 000236 takes the 233 body of
`org_trip_held_source`, keeping every line, plus the late-finish branch. The
merged-tree gates and union are recorded in the EMP-TRIP-014 evidence.
