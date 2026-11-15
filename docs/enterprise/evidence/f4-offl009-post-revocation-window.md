# EMP-OFFL-009 — Post-revocation intake window and quarantine

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F4 API task (module OFFL, phase F4, P1, size M) on
`codex/enterprise-f4-offl009`, opened from `codex/enterprise-f4` `5767ca27`
(origin/dev `8c2ee654` plus the F4 ledger; migrations through `000215`).
Synthetic data only; no Android, panel, job, route or release change.

Sources read: the EMP-OFFL-009 block of the backlog; [offline sync](../10-offline-sync.md)
§§4, 5 (UC-OFFL-07), 8 and 10.2; identity §§4–5 (BR-IDN-027..029, UC-IDN-09
and the worked offboarding example); [validation](../15-validation.md) VAL-04;
the [OFFL-002 ledger](f2-offl002-intake-ledger.md), [OFFL-008 review](f2-offl008-real-review.md)
(including the Claude Code closure), [IDN-010 offboarding](f1-idn010-offboarding.md),
[IDN-015 device registry](f1-idn015-device-registry.md), the
[METER-002 full closure](f2-meter002-persistence-intake.md) (held
`PERMISSION_CHANGED` decided by EDR-118 continuity) and
[FUEL-002](f3-fuel002-fueling-persistence.md) (EDR-177: changed-right
fuelings store nothing and stay `RETRY`/`UNAVAILABLE`); ORG-008 settings
(`policies.intake_window_days`, 0–60, default 15) and ORG-009's
`AssessSuspension` grace boundary.

IDs used: B-BR-EMP-302..307, BUC-EMP-152..154, EDR-197..199 (all of them).
Migration `000218_organization_late_intake.sql` (append-only).

## Observed before any change

- The signed route required a current `ENROLLED` device: an ended membership
  answered `404 NOT_FOUND` and a revoked device `403 DEVICE_REVOKED` for the
  whole batch, so a pre-end reading could never be delivered (BR-IDN-029 gap).
- Migration 194 forces `org_device_enrollments.last_sync_at` NULL and
  `ContactDevice` persists nothing: the server records **no** last contact, so
  it cannot be evidence. The server does hold `org_sync_operations.claimed_at`
  (immutable claim time), the membership `ended_at`, the enrollment
  `revoked_at` and its revocation class/source, and versioned settings with
  `effective_at`.
- The 201 held-meter guard admits a held reading only for an `ENROLLED`
  enrollment of an `ACTIVE` membership; the panel review validator and OpenAPI
  enum knew only `PERMISSION_CHANGED`/`ASSET_UNAVAILABLE`.
- The ACK vocabulary (OFFL-001) and migration 196 already contain
  `ACCESS_ENDED_BEFORE_RECEIPT`, `MEMBERSHIP_ENDED`, `INTAKE_WINDOW_CLOSED` and
  `DEVICE_REVOKED`; no producer existed.

## Rules frozen before behaviour

Rules B-BR-EMP-302..307 and BUC-EMP-152..154 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
section "EMP-OFFL-009 — Post-revocation intake window and quarantine" and
decisions EDR-197..199 in [16-decisions](../16-decisions.md), both appended
before any behaviour. The evidence rule (EDR-197): E is the membership
`ended_at` for a membership-sourced revocation, else the enrollment
`revoked_at`; W is the window of the settings version in force at E (default
15); R is the database clock of the final ACK. R > E + W·86 400 s →
`INTAKE_WINDOW_CLOSED` (end instant included, as ORG-009's grace); else a
pre-end server claim, or else a declaration before E → held
`ACCESS_ENDED_BEFORE_RECEIPT`; else `MEMBERSHIP_ENDED`/`DEVICE_REVOKED`. The
declared instant can only choose review over refusal. Fuelings keep EDR-177
(no held path until FUEL-003): their eligible late work is
`RETRY`/`UNAVAILABLE` with no claim, their refusals are final.

## Implementation

- **Pure decision** (`orgsync/application/late.go`): `LateAccess`,
  `LateEvidence`, `LateFacts`, `DecideLate` (EDR-197), the optional owner port
  `LateHolder` (captured by the registry like the review proof), `LateStore`
  and `Service.IntakeLate` (the existing intake loop, refactored once to choose
  the normal or late store method after the unchanged preflight).
- **Store** (`orgsync/adapters/store_late.go`): `ProcessLate` in one scoped
  transaction: `ConfirmLate` (source) plus the SQL window function; replay of
  an existing late decision returns the stored ACK after re-proving the late
  row, the stored mirrors/audit (shared `storedAcknowledgement`, extracted from
  `Process` without semantic change) and the held proof; a normal-path ACK is
  `ErrUnavailable` (no receipt read); same id/other digest is unstored
  `OPERATION_ID_REUSED`; a held decision needs an installed holder and review
  proof (else unavailable, no claim) and ready dependencies (else
  `DEPENDENCY_PENDING`, claim kept); the final receipt re-decides with fresh
  evidence and rolls back on any change; decision row and ACK commit with the
  audit; no effect is ever written.
- **Source** (`membership/adapters/device_late.go`, query
  `LateIntakeEnrollment`): account → personal proof → capability snapshot
  (enabled, EDR-108 order) → root `ACTIVE` → the exact owned period (account,
  key, contributor, fingerprint) → `REVOKED`/`ACCESS_ENDED` with the key never
  `LOST_OR_COMPROMISED` in the organization → end evidence → proof recheck.
  Lost/barred → `ErrDeviceRevoked`; enrolled/unknown/foreign → not found.
- **Route** (`orgsync/adapters/http/http.go`): the sealed period is read from
  the first operation with the full parser; the late path is entered only when
  the current check refuses the key as revoked/ended, or the sealed period
  differs from the current one (rehire); the late source is confirmed again
  before the response. `access.ErrEntitlementRequired` now maps to its bounded
  `ENTITLEMENT_REQUIRED` code instead of `UNAVAILABLE`.
- **Meter owner** (`fleet/adapters/meter_intake_late.go`, new file):
  `HoldLate` validates payload, target in the organization and service window
  without current authority, then stores the OFFL-008 held fact with the late
  code. Shared-file edits are one line each: `meter_review_hold.go` admits the
  late code in `HoldMeterReadingTx`; `fleet/application/meter_review_service.go`
  lists it in the panel view validator.
- **Migration 000218**: `org_sync_late_disposition` (pure, shared vectors),
  `org_sync_late_end`, `org_sync_late_window`; the forced-RLS, append-only
  `org_sync_late_intakes` with a BEFORE guard re-deriving E/W/disposition, a
  deferred publication guard (ACK, review item, no effect), a deferred ACK guard
  for the four late codes, and the 201 `org_meter_held_source` body with one
  late branch. Ordinary acknowledgements read no new table (nested branch).
- **Composition** (`enterprisesuite/sync.go`): `ConfirmLate` from the device
  store and the route's `Late` source. **OpenAPI**: the meter review view code
  enum gains `ACCESS_ENDED_BEFORE_RECEIPT`; the intake description states the
  late path (no path, status or schema change). **Shared vectors**:
  `contracts/testdata/enterprise/sync/late-intake-v1.psv` (54 rows: DECIDE 47,
  EVIDENCE 7), consumed by Go `DecideLate` and by the PostgreSQL function.
- **Test fixtures**: `orgtest.GrantLateIntake`; `devicesFixtureOn`,
  `syncFixtureOn`, `lifecycleIntakeFixtureOn` (the existing constructors now
  delegate unchanged) so the upgrade rehearsal builds on the previous image.

## Acceptance on real PostGIS (`-race -tags=integration`)

| Backlog item | Test(s) |
|---|---|
| Eligible pre-end operations inside the window enter review (held, never applied) | `TestLateIntakeHoldsPreEndReadingsAndRefusesLaterOnesFromServerHistory` (E−6 h, E−30 min, E−1 ns held; review item, held reading, version-1 proof, decision row with E/W; no reading/effect; panel queue lists them with author) |
| A reviewer who is not the author decides; nothing automatic | `TestLateIntakeHeldReadingIsResolvedOnlyByAnotherReviewer` (accept applies once with original author, enrollment and receipt; late replay keeps the held ACK) |
| Closed window refused; window from server history | `TestLateIntakeWindowIsTheSettingsInForceAtTheEnd` (window 0 at E → `INTAKE_WINDOW_CLOSED`; later 60-day settings do not reopen; ancient/forged and post-end declarations also closed); pure and SQL boundary vectors |
| Lost or compromised devices refused | `TestLateIntakeRefusesLostKeysForeignAndCurrentPeriods/{lost-period,key-barred-in-a-later-period}` (whole-response `DEVICE_REVOKED`, nothing stored, barred after a rehire LOST revocation, also for replay); signed route `TestLateIntakeSignedRouteRefusesALostDeviceAsAWhole` (403, private state unchanged) |
| Intake grants no reads | `TestLateIntakeSignedRouteHoldsAndRefusesThroughTheDefaultRuntime` (signed review-results read refused after the hold; pre-end normal receipt → `RETRY`/`UNAVAILABLE`); `…HoldsPreEnd…/pre-end-normal-receipt-is-not-reopened` |
| Negative: foreign organization | `…/current-and-foreign-periods` (another organization's scope: not found, nothing stored in either); unknown target → `MALFORMED` (`…/unknown-target-is-malformed`) |
| Negative: forged time treated as proof | `…/forged-early-declaration-is-only-evidence` (held only, receipt from the DB clock); `TestLateIntakeDeclaredTimeNeverAppliesOrMovesTheWindow`; oracle `forged-*`; window test (forged early declaration still closed) |
| Negative: new operation declared after the end | E, E+1 ns, E+1 h → `MEMBERSHIP_ENDED`; device command end → `DEVICE_REVOKED` (`TestLateIntakeDeviceCommandEndUsesTheRevocationInstant`) |
| Negative: access restored through replay | replays ×2 of every late decision byte for byte; `OPERATION_ID_REUSED` unstored; barred key cannot replay its held ACK; 12-way `TestLateIntakeConcurrentIdenticalDeliveriesKeepOneDecision` |
| Server claim evidence and dependency order | `TestLateIntakeEarlierClaimIsEvidenceAndDependenciesKeepTheirOrder` (pre-end claim + fast device clock → `HOLD_CLAIM`; unclaimed fast clock → `MEMBERSHIP_ENDED`; unready parent → `DEPENDENCY_PENDING` with claim kept, no decision) |
| Intake racing the membership end, both orders | `TestLateIntakeRacesTheMembershipEndInBothOrders` (observed root-lock waits; end first → whole refusal then late hold; intake first → accepted before E, late resend reads nothing) |
| Organization state stays whole (OFFL-012 owns grace) | `TestLateIntakeKeepsOrganizationRefusalsWhole` (capability off → `ENTITLEMENT_REQUIRED`, nothing stored) |
| Database guards | `TestLateIntakeSchemaRefusesFabricatedDecisionsAndLateCodes` (normal path cannot publish late codes or hold a late fact; held-meter late branch refuses current/declared-after sources 23514; forced RLS, UPDATE/DELETE/TRUNCATE 55000; wrong E/source/window/disposition 23514; no ACK → refused at commit) |
| Shared oracle on PostgreSQL | `TestLateIntakeSQLDispositionMatchesTheSharedOracle` (47 rows × UTC, America/New_York, America/Sao_Paulo) |
| Populated upgrade | `TestLateIntakePopulatedUpgradeFailureRecoveryAndPreviousImage` (populated 215 head with an accepted reading and an ended period; injected failure leaves schema/data unchanged; ordinary migrator recovery once; no-op replay; previous image compatible; 201 checks preserved; the pre-upgrade ended period then delivers late) |

## Validation — RED → GREEN and gates

Environment: backend cwd, Go 1.27.4, `GOCACHE=/data/dev/cache/f4-go-build`,
shared PostGIS `127.0.0.1:32775` through orgtest child databases. Logs in
`/data/dev/worktrees/f4-logs/offl009-*`.

- Rules first (contract section and EDR rows above), then compiled stubs
  (`DecideLate` returning an error, `ProcessLate`, `CheckLateIntakeDevice*`
  and meter `HoldLate` returning unavailable, no route change), the frozen
  oracle and the migration as written.
- RED pure `offl009-go-red-pure.jsonl` (`go test -json ./internal/modules/orgsync/application -run TestLateIntake`):
  2 top-level failing; 54 named: 45 fail, 9 pass (the INVALID rows, whose
  expected error the stub already returns).
- RED PostGIS `offl009-go-red-integration.jsonl` (`go test -json -race -tags=integration -p 2 ./internal/modules/orgsync/adapters ./internal/modules/organization/adapters -run '^TestLateIntake'`):
  13 top-level: 12 fail, 1 pass (the SQL oracle consumer: the migration
  precedes behaviour, as FUEL-002's upgrade test); 6 named: 4 fail, 2 pass
  (the intake-first race and the normal-path late-code guard, both proving
  unchanged current behaviour). Every failure is behavioural: quarantine
  unavailable, lost device and capability refusals not reached, signed route
  404 instead of 200/403. A first RED attempt had two fixture defects,
  corrected before this record and not counted: a normal-path reading
  declared before the fixture's retained ADMIN grant (EDR-118 holds it) and a
  missing `org_settings_commands` grant for the settings change.
- GREEN corrections that touched tests (none weakened an expectation): the
  oracle row `forged-ancient-declared-held` caught that the minimum OrgTime
  instant is Go's zero time, so `LateFacts.Declared` became a pointer (absence
  = nil); a test query used an ambiguous `USING` join; two count assertions
  also counted the meter installation's opening reading and now count intake
  readings only; the signed-route fixture lacked the held-review owner's
  INSERT grants (granted exactly as `meterReviewStore` does).
- Added after GREEN: `TestLateIntakeEarlierClaimIsEvidenceAndDependenciesKeepTheirOrder`.
  Mutation check instead of RED: disabling the claim branch of `DecideLate`
  makes it fail (`offl009-claim-mutation-red.jsonl`: Go decides ENDED, the
  database guard re-derives HOLD_CLAIM and refuses the row); restored.
- Affected race union (`go test -json -race -tags=integration -count=1 -p 2 -timeout=60m`
  over organization adapters, orgsync/..., membership/..., fleet/...,
  enterprisesuite, apicontract, migrate, cmd/api): `offl009-race-union.jsonl`,
  18 packages, 808 top-level / 5582 named nodes, 21m24s; 802 top-level pass
  and 6 fail only for `explicit disposable ANPFUEL_TEST_STORAGE_ENDPOINT
  required` (auditor matrix, document links/upload, three export tests). With
  a throwaway MinIO (`infra/compose.validation.yml`, project
  `f4-offl009-s3`) those 6 pass with 86 named nodes
  (`offl009-storage-rerun.jsonl`; the AUTH-011 walk still reports 110 routed
  normal routes). Late tests in the union: 16 top-level / 73 named, 0 failures
  (1.5–5.7 s each); organization adapters 1281.7 s, apicontract 974.9 s.
- Untagged `go test -count=1 -p 2 -timeout=45m ./...`: 214 packages ok, exit 0,
  1m49s (`offl009-untagged-all.log`).
- Toolchain `gofmt -l` empty; `go build ./... && go vet ./... && go vet -tags=integration ./...`
  pass; staticcheck (GOTOOLCHAIN go1.27.1) plain and `-tags=integration` on
  orgsync, membership, fleet, organization, enterprisesuite, apicontract and
  orgtest: no findings (`offl009-static.log`); `sqlc vet` and `sqlc generate`
  reproduce with only the intended changes (orgsync `late.sql.go`, the
  `claimed_at` column of `ReadOperation`, membership `LateIntakeEnrollment`);
  `git diff --check` clean; `check_plan.py` 348 tasks / 545 rules / 167 use
  cases, 0 problems.
- No route, route inventory, authorization matrix or AUTH-011 walk change;
  OpenAPI change limited to one enum value and description text (the
  apicontract package passes in the union). No Android, Kotlin, job or worker
  change; no emulator or device was used.
- Secret/PII review of the staged diff: synthetic identifiers and the
  disposable MinIO/PostGIS fixture credentials only in local commands, none in
  the diff; no real name, CNPJ, CPF, plate or location.

## Retained (with reasons; not part of this task's acceptance)

- Held late fuelings: FUEL-003 owns the held-fueling review owner; it then
  implements `LateHolder` for `fueling.submit` (EDR-177/198). Until then an
  eligible late fueling is `RETRY`/`UNAVAILABLE` and, if the window closes
  first, becomes `INTAKE_WINDOW_CLOSED` — FUEL-003 should land before any
  production late intake of fuelings.
- Suspended/archived organizations and a disabled base capability keep their
  whole-response refusals for late sources; EMP-OFFL-012 decides their
  disposition (EDR-199).
- A pre-end operation whose normal ACK was lost before the end stays
  `RETRY`/`UNAVAILABLE` for the ended device (the company holds the record);
  the client presentation of that case is retained for the Android owner.
- Android: the app must keep sending sealed pending operations after a
  `LOCK` contact verdict and present `ACCESS_ENDED_BEFORE_RECEIPT`,
  `MEMBERSHIP_ENDED`, `INTAKE_WINDOW_CLOSED` and `DEVICE_REVOKED` per operation
  (codes already in the frozen vocabulary; no client code changed here).
- Membership suspension (BR-OFFL-011 "ends or is suspended"): a suspended
  membership keeps its enrolled period; its intake disposition is not decided
  here.
- Notifications to reviewers about late items (NOTIF owner), production grants
  for the intake principal (proposal 3 below), panel screens and release.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. `docs/enterprise/10-offline-sync.md` §10.2, after the fueling sentence:
   "A device whose exact sealed period was revoked `ACCESS_ENDED` delivers it
   intake-only: from server history only (end instant, window in force at
   it, receipt), late operations are held `ACCESS_ENDED_BEFORE_RECEIPT` when
   their owner has a held path, else `RETRY`/`UNAVAILABLE`, or refused
   `MEMBERSHIP_ENDED`/`DEVICE_REVOKED`/`INTAKE_WINDOW_CLOSED`; a key revoked
   lost or compromised is refused as a whole (EMP-OFFL-009, EDR-197..199)."
2. `docs/enterprise/backlog/ep04-sync.md` EMP-OFFL-009: state `TODO` →
   `LOCAL_DONE` (only if the coordinator accepts the validation above).
3. `backend/README.md`, intake grants: "Migration 218 adds forced-RLS,
   append-only `org_sync_late_intakes`. The signed intake principal needs
   `SELECT, INSERT ON org_sync_late_intakes` and `SELECT ON
   org_settings_versions`; the migrator needs `REFERENCES` on
   org_sync_operations, org_sync_acknowledgements, org_memberships,
   org_device_enrollments and org_settings_versions, `TRIGGER` on
   org_sync_acknowledgements and ownership of `org_meter_held_source()`.
   Nothing here provisions live grants."
4. `docs/enterprise/modules/identity.md` BR-IDN-029: append "(EMP-OFFL-009:
   B-BR-EMP-302..307)".
