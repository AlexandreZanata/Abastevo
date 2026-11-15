# EMP-OFFL-012 — Organization-state and plan-change handling for queued operations

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F4 API task (module OFFL, phase F4, P1, size S) on
`codex/enterprise-f4-offl009`, after EMP-OFFL-009 (`fe59fba4`, migration
`000218`). Synthetic data only; no Android, panel, route, job or release
change.

Sources read: the EMP-OFFL-012 block of the backlog; [offline sync](../10-offline-sync.md)
§10.2 (`ORG_SUSPENDED_GRACE`, `ORG_SUSPENDED`, `PLAN_CHANGED` rows);
[organizations](../modules/organizations.md) BR-ORG-030/031 and UC-ORG-08;
[07-permissions](../07-permissions.md) BR-AUTH-009; [validation](../15-validation.md)
VAL-22, VAL-24 and VAL-35; the [ORG-009 suspension](f1-org009-suspension.md)
evidence and its `AssessSuspension`/`AssessSuspensionTx` port; the
[AUTH-004 pilot entitlements](f1-auth004-pilot-entitlements.md) and
[PLAT-005 capability flags](f1-plat005-capability-flags.md) evidence; the
EMP-OFFL-009 late path (this branch) and the OFFL-002/008 ledgers.

IDs used: B-BR-EMP-308..311, BUC-EMP-155..156, EDR-200..201 (all of them).
Migration `000219_organization_state_intake.sql` (append-only).

## Observed before any change

- A SUSPENDED organization refused every signed batch as a whole
  (`403 ORG_SUSPENDED`): VAL-22 part 2's pre-suspension fueling could never be
  kept for review, and after reactivation work declared during the
  suspension applied as if the suspension had not existed.
- A disabled base capability made the owner refuse every operation as a whole
  (`ENTITLEMENT_REQUIRED`), including work declared before the downgrade
  (VAL-24/35 require it to be kept for review).
- ORG-009 already owns the suspension rule (`AssessSuspension`: recorded
  periods, the database receipt, a 7-day grace including its end, review for
  facts declared during a resolved interval) and an owning-transaction port
  (`SuspensionStore.AssessSuspensionTx`); no intake consumed it.
- No billing exists. The only recorded plan state is the capability row
  (`org_capabilities`: enabled, revision, `updated_at`); its audit events do
  not name the capability, and a same-state update also advances it.

## Rules frozen before behaviour

Rules B-BR-EMP-308..311 and BUC-EMP-155..156 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
section "EMP-OFFL-012 — Organization-state and plan-change handling for queued
operations" and decisions EDR-200/201 in [16-decisions](../16-decisions.md),
appended before behaviour. EDR-200: organization state decides before every
owner through ORG-009's assessment; holds use the EMP-OFFL-009 typed held path
with the state code; `ORG_SUSPENDED` is a final per-operation refusal; while
suspended a non-empty batch reaches only this assessment (an empty batch keeps
`ORG_SUSPENDED`); decisions replay through their own row. EDR-201: the plan is
the capability row — disabled at receipt after a change makes `updated_at` the
downgrade instant; earlier work is held `PLAN_CHANGED`, later work keeps the
owner's `ENTITLEMENT_REQUIRED`; a never-enabled or absent capability denies.

## Implementation

- **Pure** (`orgsync/application/state.go`): `StateDecision` (shape
  validation, `Same`), `PlanState`, `DecidePlan` (EDR-201).
- **Store** (`orgsync/adapters/store.go`, `store_state.go`): one hook in
  `Process` before the owner's write admission: `AssessState` (composed) →
  `processState` (claim, ready dependencies or `DEPENDENCY_PENDING`, owner
  `HoldLate` with the state code or a final refusal, audit, the decision
  re-taken at the final receipt, state row, ACK; never an effect). Replay of
  a state decision and a reused identifier with a state row go through the row
  (`replayState`, `stateRecorded`), never owner read authority. Without
  `AssessState` the store behaves exactly as before.
- **Assessor** (`organization/adapters/intake_state.go`):
  `IntakeStateAssessor` = ORG-009 `AssessSuspensionTx` (REVIEW → hold
  `ORG_SUSPENDED_GRACE`; REFUSE `ORG_SUSPENDED` → per-operation refusal; other
  refusals → whole-response codes) then the capability row through
  `orgcapability.Store.PlanStateDBTX` (new read-only method) and `DecidePlan`.
- **Suspended source** (`membership/adapters/device_suspended.go`;
  `devices.go`/`device_intake.go` gain one `allowSuspended` flag threaded
  through `signedForState`, every other check unchanged): the current member's
  exact enrolled device of a SUSPENDED organization. Composition: the intake
  `ConfirmSource` falls back to it only on `ErrOrgSuspended`; the signed route
  uses it only for a batch with operations and reconfirms it before the
  response.
- **Owner**: the meter `HoldLate` and `HoldMeterReadingTx` accept
  `ORG_SUSPENDED_GRACE` and `PLAN_CHANGED` (one helper, `meterRestrictedHeld`);
  the panel view validator lists them; the held-meter guard needs no change
  (state holds come from a current enrolled source).
- **Migration 000219**: `org_sync_suspension_disposition_of` (SQL mirror of
  `AssessSuspension` for facts with no server-opened entity) and its table
  wrapper, `org_sync_plan_disposition` (shared vectors), the forced-RLS,
  append-only `org_sync_state_intakes` with a BEFORE guard re-deriving the
  condition from history at the receipt, a deferred publication guard (ACK,
  review item, no effect) and a deferred ACK guard for the three state codes.
- **OpenAPI**: review view code enum gains `ORG_SUSPENDED_GRACE` and
  `PLAN_CHANGED`; the intake description states the state assessment. Shared
  vectors: `contracts/testdata/enterprise/sync/state-intake-v1.psv` (20 PLAN
  rows) with `STATE.md`.
- **Test fixtures**: `orgtest.GrantStateIntake`; `syncFixtureOn` grants it,
  because the composed intake now reads suspension history before every owner.

## Acceptance on real PostGIS (`-race -tags=integration`)

| Backlog item | Test(s) |
|---|---|
| Earlier work follows the grace rule; new forbidden work stays denied | `TestStateIntakeSuspensionGraceNewWorkAndReactivation` (declared before S → held `ORG_SUSPENDED_GRACE`; S, S+1 ns, S+1 min → `ORG_SUSPENDED`; replays while suspended; after reactivation work declared in the resolved interval is held and new work applies; memberships, roles and enrollments unchanged) |
| Plan-change review rule | `TestStateIntakePlanDowngradeHoldsEarlierWorkOnly` (declared before the downgrade → held `PLAN_CHANGED` with capability revision and instant; at/after → owner `ENTITLEMENT_REQUIRED`, nothing stored; restored capability applies; no authority change) |
| Negative: old client clock bypasses the write block | forged declaration just after installation → held only, receipt from the DB clock; post-suspension declarations refused; post-downgrade declarations still denied; oracle `forged-*` rows |
| Negative: downgrade changes membership | `authority()` snapshot equal before/after suspension, reactivation and plan changes |
| Negative: foreign history | `TestStateIntakeSchemaGuardsAndForeignHistory` (another organization suspended: this organization's work applies) |
| Negative: replay turns held work into acceptance or duplicate cost | byte-identical replays (also after reactivation and while disabled), `OPERATION_ID_REUSED` without owner authority, `TestStateIntakeConcurrentIdenticalDeliveriesKeepOneDecision` (12-way, one claim/decision/held fact) |
| Concurrent state change | `TestStateIntakeRacesTheSuspensionInBothOrders` (observed root-lock waits: suspension first → held; delivery first → applied, then suspended) |
| Signed route (VAL-22 part 2) | `TestStateIntakeSignedRouteWhileSuspended` (empty batch 403 `ORG_SUSPENDED`; batch → held + refused; resend byte-identical; after reactivation new work applies and the old ACKs are unchanged) |
| Database guards | `…SchemaGuards…` (owner path cannot publish `ORG_SUSPENDED`; state rows append-only 55000; fabricated decisions 23514; no ACK → refused at commit) |
| Shared oracle and SQL mirror | `TestStateIntakePlanOracleIsCompleteAndDecidesFromTheCapabilityRow` (Go), `TestStateIntakeSQLPlanDispositionMatchesTheSharedOracle` (SQL), `TestStateIntakeSQLSuspensionMirrorsAssessSuspension` (generated matrix vs the ORG-009 domain, open and resolved periods, grace boundary ±1 µs) |
| Populated upgrade | `TestStateIntakePopulatedUpgradeFailureRecoveryAndPreviousImage` (populated 218 head; injected failure, ordinary recovery, no-op, previous image, forced-RLS ownership; state hold works after) |

## Validation — RED → GREEN and gates

Environment as EMP-OFFL-009. Logs in `/data/dev/worktrees/f4-logs/offl012-*`.

- Rules first, then compiled stubs (`DecidePlan` returning an error, the
  assessor returning the zero decision, the suspended source returning
  `ORG_SUSPENDED`, no route change), the frozen plan oracle and migration
  000219 as written.
- RED pure `offl012-go-red-pure.jsonl`: 2 top-level (oracle test fails; the
  shape validator test passes — it is a pure value check written with the
  shape); 20 named: 14 fail, 6 pass (the INVALID rows).
- RED PostGIS `offl012-go-red-integration.jsonl` (`-race -tags=integration
  -p 2 … -run '^TestStateIntake'`): 9 top-level: 7 fail, 2 pass (the SQL plan
  oracle and the SQL suspension mirror: schema precedes behaviour); 2 named:
  1 fail, 1 pass (the delivery-first race, unchanged behaviour). Failures are
  behavioural: suspended source refused (`ORG_SUSPENDED` whole), plan work
  refused by the owner, signed route 403 instead of 200. A first RED attempt
  did not run: the migration as written had a `CASE … THEN` inside a plpgsql
  `IF` condition (`syntax error at end of input`, found by applying the full
  chain to a scratch database in the PostGIS container); parenthesizing it is
  the only change, not counted as RED.
- GREEN focused `offl012-go-green-1.jsonl`: all state tests pass; the
  composed default runtime then needed the test role's read of
  `org_suspension_periods` (the intake now reads suspension history before
  every owner), granted once in `syncFixtureOn` through
  `orgtest.GrantStateIntake`. Focused regression of every intake suite
  (`^Test(StateIntake|LateIntake|MeterIntake|FuelingIntake|SyncIntake|OwnReviewResults|OrganizationSync)`):
  `offl012-go-green-2.jsonl`, 62 top-level / 98 named, 0 failures.
- Affected race union (`-race -tags=integration -count=1 -p 2 -timeout=60m`
  over organization adapters, orgsync/..., membership/..., fleet/...,
  orgcapability/..., enterprisesuite, apicontract, migrate, cmd/api, with the
  throwaway MinIO of `infra/compose.validation.yml` project `f4-offl009-s3`):
  `offl012-race-union.jsonl`, 21 packages, 834 top-level / 5695 named nodes,
  0 failures/skips, 21m30s (organization adapters 1288.1 s, apicontract
  977.1 s, cmd/api 105.8 s). State tests 11 top-level / 28 named and the
  EMP-OFFL-009 late tests 16 top-level, all passing on the final code.
- Untagged `go test -count=1 -p 2 -timeout=45m ./...`: 214 packages ok, exit 0,
  3m19s (`offl012-untagged-all.log`).
- Toolchain `gofmt -l` empty; `go build ./... && go vet ./... && go vet -tags=integration ./...`
  pass; staticcheck (go1.27.1) plain and `-tags=integration` on orgsync,
  membership, fleet, organization, orgcapability, enterprisesuite, apicontract
  and orgtest: no findings (`offl012-static.log`); `sqlc vet` and
  `sqlc generate` reproduce with only the new state queries; `git diff --check`
  clean; `check_plan.py` 348 tasks / 545 rules / 167 use cases, 0 problems.
- No route, route inventory, authorization matrix or AUTH-011 walk change
  (the walk still passes in the union); OpenAPI change limited to two enum
  values and description text. No Android, Kotlin, job or worker change; no
  emulator or device was used.
- Secret/PII review of the staged diff: synthetic identifiers only; the
  disposable MinIO/PostGIS fixture credentials appear only in local commands.

## Retained (with reasons; not part of this task's acceptance)

- A capability turned off and on again does not reconstruct its past off
  interval: the row keeps only the current state, so work declared while it
  was off and delivered after it returns is decided by the owner (EDR-201). A
  recorded plan history belongs with the subscription/billing owner (SUBS).
- Plan limits ("limit reached after a downgrade", VAL-35 `LIMIT_REACHED`) have
  no counter source yet; only the capability state is modelled.
- New forbidden work after a downgrade keeps the owner's whole-response
  `ENTITLEMENT_REQUIRED`, so a batch mixing pre- and post-downgrade work
  withholds the held ACKs until the device drops or delays the later item
  (OFFL-008 L2, whole-batch denial).
- State holds precede the owner's permission classification: a member who
  never held the permission gets a held item a reviewer refuses (EDR-200).
- Late intake (EMP-OFFL-009) in a suspended organization or with the
  capability off stays refused as a whole (EDR-199); entity-opened
  ("server-opened") grace has no owner producer yet (trips).
- Fuelings: no held path until FUEL-003 (state holds `RETRY`/`UNAVAILABLE`).
- Android presentation of `ORG_SUSPENDED_GRACE`, `ORG_SUSPENDED` and
  `PLAN_CHANGED` (frozen codes; SCR-ORG-12 notice still retained).
- Production grants (proposal 2 below), panel screens and release.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. `docs/enterprise/10-offline-sync.md` §10.2, after the OFFL-009 sentence:
   "Before any owner, the organization's recorded suspension history holds
   work declared before a suspension and received inside its grace
   (`ORG_SUSPENDED_GRACE`), refuses later work (`ORG_SUSPENDED`), and keeps
   reviewing work declared during a resolved suspension; work declared before
   its capability was turned off is held `PLAN_CHANGED` (EMP-OFFL-012,
   EDR-200/201)."
2. `backend/README.md`, intake grants: "Migration 219 adds forced-RLS,
   append-only `org_sync_state_intakes`. The signed intake principal needs
   `SELECT, INSERT ON org_sync_state_intakes` and `SELECT ON
   org_suspension_periods` (read before every owner); the migrator needs
   `REFERENCES` on org_sync_operations, org_sync_acknowledgements and
   org_capabilities and `TRIGGER` on org_sync_acknowledgements. Nothing here
   provisions live grants."
3. `docs/enterprise/backlog/ep04-sync.md` EMP-OFFL-012: state `TODO` →
   `LOCAL_DONE` (only if the coordinator accepts the validation above).
4. `docs/enterprise/modules/organizations.md` BR-ORG-031: append
   "(EMP-OFFL-012: B-BR-EMP-308..311)".
