# Runtime role, grant inventory and VPS activation runbook (EMP-REL-002 prerequisite)

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded delivery in worktree `/data/dev/worktrees/f4-activation`, branch
`codex/enterprise-activation`, created from the final F4 head `b17dc3d8`. The
coordinator integrates it, owns PROGRESS, the F4 ledger, pushes, the PR and
every VPS action; this branch prepares and proves only. Nothing was pushed,
deployed or applied to the VPS, the deploy agent, k3s or any live database.
Synthetic data only. EMP-REL-002 itself stays TODO (EMP-REL-001, the VAL-24
switch rehearsal and the activation are owed); no milestone, gate,
INTEGRATED or RELEASE_CERTIFIED state is claimed.

## Problem (as verified by the coordinator, read-only)

The VPS (`abastevo-temp`, `ANPFUEL_ENV=staging`) runs the API and worker with
one `ANPFUEL_DATABASE_URL` whose login is the container superuser (BYPASSRLS,
owner of every table) and `ANPFUEL_ENTERPRISE_ENABLED` unset. Every
organization transaction calls `requireSafeOrgRole`
(`backend/internal/platform/database/org_scope.go`), which refuses a session
user that is, or is a member of, a superuser/BYPASSRLS/CREATEROLE role or owns
an RLS table. Turning the switch on with that login mounts the routes but
answers every organization operation as unavailable — now proven by a test
(below). No canonical production grant inventory existed: the per-task
README "grants" paragraphs were column lists for single tasks, and the
fixtures granted narrow privileges per test.

## Rules read

`AGENTS.md`; `docs/planning/FAST_EXECUTION.md`; `docs/enterprise/00-conventions.md`;
the agent brief; `backend/README.md` "Private Empresas activation (EMP-PLAT-005)",
"Final PR202 review corrections" (the paragraph that requires the full grant
inventory before enabling the switch for real accounts), "Organization write
admission (EMP-PLAT-014)" (SELECT/INSERT/UPDATE on `identity_rate_windows`,
cleanup scheduling owed) and every F2–F4 grants paragraph; `backlog/ep20-release.md`
EMP-REL-001/002; `15-validation.md` VAL-24 and §8; `16-decisions.md`
EDR-271/272/361/362 (and EDR-317 for the operator-only meter rebuild);
`infra/k8s/staging/` (README, `api-deployment.yaml`, `render.py`,
`test-manifests.py`); `infra/deploy/agent/` (README, `apply.sh`,
`runtime/k8s.sh`: the agent changes only images, pull policy, strategy and
annotations; its schema, backup and migrate Jobs read `ANPFUEL_DATABASE_URL`
through `envFrom` of the DB secret; `rollout undo` targets the revision
recorded before its own image change); `org_scope.go`; `orgtest/fixture.go`;
`cmd/api` and `cmd/worker` composition (`cmd/worker` composes the enterprise
export, reservation-outcome, tracking and document jobs only with
`ANPFUEL_ENTERPRISE_ENABLED`, so the worker needs the runtime login and the
switch too).

## Rules frozen

Contract section "EMP-REL-002 — Runtime role, grant inventory and staging
activation runbook" in `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`:
B-BR-EMP-556 (ordinary runtime login that owns nothing), B-BR-EMP-557 (derived,
reviewed, converged inventory), B-BR-EMP-558 (owner DSN for schema work,
runtime DSN for the serving containers, re-run after releases), B-BR-EMP-559
(scoped, reversible, data-preserving activation); BUC-EMP-285 (provision and
activate), BUC-EMP-286 (roll back). Decisions EDR-363 (derivation and
exceptions), EDR-364 (later migrations: default privileges + re-run, version
guard), EDR-365 (proof strategy and script mode), EDR-366 (deployment wiring,
pre-flight gates and scope). Migration 000264 was not needed (no schema
change); all pre-allocated IDs are used.

## Shape

- `backend/internal/platform/runtimegrants` (new, stdlib + pgx):
  `analyze.go` (`Usage`, `AddSQL`, `AddExpression`), `sources.go`
  (`ProductionContext`, `Closure`, `ScanPackage` — go/parser, constant folding),
  `catalog.go` (`LoadCatalog`), `inventory.go` (`Build`, `ReviewedExceptions`),
  `render.go` (`Render`, `ForRole`, `ScriptPath`).
- `infra/db/runtime-role.sql` (generated, checked in; schema 000257: 357 of 359
  relations granted — 134 `SELECT, INSERT`, 150 `SELECT, INSERT, UPDATE`, 34
  `SELECT, INSERT, DELETE`, 34 all four, 3 `SELECT`, 2 `SELECT, UPDATE`; no
  access to `official_summary_prices`, `org_fuel_products_v1`; 5 sequences;
  0 explicit functions; `org_meter_current_rebuild(uuid)` operator-only).
- `backend/internal/platform/orgtest`: `ProvisionScriptRole`,
  `ScriptRuntimePool`, opt-in script mode (`ANPFUEL_TEST_RUNTIME_GRANTS=script`).
- `backend/cmd/api`: `runtime_role_integration_test.go` (new) and the script
  mode of `testPublicProcessIdentityAndSignedWrites`.
- `infra/k8s/staging/api-deployment.yaml` + `test-manifests.py`
  (`test_runtime_login_and_enterprise_switch`), `infra/k8s/staging/README.md`.
- Runbook `docs/operations/ENTERPRISE_ACTIVATION_RUNBOOK.md`; README section
  "Runtime role and grant inventory (EMP-REL-002 activation)"; status note on
  the EMP-REL-002 backlog block.

## RED

Tests were written first; the implementation was then replaced by compiled
stubs (zero values) for the run and restored afterwards.

- Unit (`go test ./internal/platform/runtimegrants`): 7 of 7 top-level tests
  FAIL, 14 of 15 subtests FAIL; the passing subtest is the negative "plain
  prose ignored" case, which a no-op satisfies by construction.
  Log: `/data/dev/worktrees/f4-logs/activation-red-unit.jsonl`.
- Integration (`-tags=integration`): 9 of 9 top-level FAIL (the two DB tests
  fail on the missing script and empty inventory).
  Log: `/data/dev/worktrees/f4-logs/activation-red-integration.jsonl`.
- Manifests: `test_runtime_login_and_enterprise_switch` against the `HEAD`
  manifest: 1 error (`KeyError: 'ANPFUEL_DATABASE_URL'`), 4 other tests pass.
  Log: `/data/dev/worktrees/f4-logs/activation-red-manifests.log`.
- The composed process test was written against the generated script; its
  first runs failed on harness issues (a map query without `source`, the
  enterprise export store refusing a loopback bucket — see Findings) and then
  passed without any change to the inventory.

## GREEN / REFACTOR

- Unit: 7/7 PASS (15 subtests). Integration package: 9/9 PASS, including
  `TestRuntimeScriptMatchesHead` and
  `TestRuntimeScriptProvisionsOrdinaryRoleAndRefusesUnsafeUse`.
- `cmd/api` `TestRuntimeRoleServesProductionProcessAndEnterprise`: PASS
  (31.5 s), with its superuser subtest PASS.
- Refactor: the test-only privilege fingerprint evaluates
  `has_sequence_privilege` only for sequences (the planner may otherwise call it
  on other relations); script-provisioned test roles get a 120 s
  `lock_timeout`/`idle_in_transaction_session_timeout` so a test whose lock
  observer watches the fixture login fails instead of hanging.

## Proof under the runtime role

`TestRuntimeRoleServesProductionProcessAndEnterprise` (fresh head database,
migrated as the owner, role provisioned only by the script, real API process
`run()` with `ANPFUEL_ENTERPRISE_ENABLED=true` and the forum on):

| Area | Exercised |
|---|---|
| Current VPS state | API as the superuser: routes mounted, company login works, organization creation answers `503 UNAVAILABLE`, no organization row is left |
| Readiness | `/health/ready` 200 (schema ledger read with `SELECT` only) |
| Public reads | community summary and feed, station search, detail, prices, official prices, nearby, map assertions, forum categories and posts, charging history |
| Public signed writes | two contributor enrollments, observation submit, idempotent retry, owner read 200, other contributor 404 |
| Accounts | key account creation and login, session refresh, premium status, cloud status |
| Enterprise (cookie plane) | unauthenticated 401; company login; organization creation; setup discovery; settings read with capability off; unit create refused `ENTITLEMENT_REQUIRED`; capability enabled through the audited store as the runtime login; organization activation; unit; employee; invitation issued with one-time delivery; vehicle asset; odometer installation; asset activation; latest reading |
| Signed sync | account binding, device enrollment (signed), one `meter.reading.record` through `/sync/operations` → `ACCEPTED`; replay returns the byte-identical acknowledgement |
| Isolation | foreign organization `settings/read` 404 `NOT_FOUND`; foreign member reading the asset 404 `NOT_FOUND` |
| Worker | real `cmd/worker` binary, same login and switch, settles a queued `org-asset-reservation-outcome` job (stored `EXPIRED`, job `done`) |

`TestRuntimeScriptProvisionsOrdinaryRoleAndRefusesUnsafeUse`: second
application leaves the effective privileges identical; a widened role
(`TRUNCATE`/`DELETE` on audit, a column `UPDATE` on the ledger) is converged
back; attributes ordinary, password stored as a SCRAM verifier; the guard
admits the role and refuses the owner; RLS hides the other organization; and
`42501` for `TRUNCATE org_audit_events`, `DELETE`/`UPDATE org_audit_events`,
`DELETE org_sync_operations`, `TRUNCATE accounts`, insert/delete on
`schema_migrations`, `org_meter_current_rebuild`, `org_pending_owner_count`,
`CREATE TABLE`, `ALTER TABLE accounts`, `SET ROLE pg_read_all_data`; no
SECURITY DEFINER function executable; no column-level grant survives; the
script refuses another schema version, a role that owns an object and the
owner itself.

## Script mode over the existing suites (EDR-365)

`ANPFUEL_TEST_RUNTIME_GRANTS=script`: orgtest pools log in as a role
provisioned only by the script; grants addressed to `RuntimeRole` land on the
unused fixture login, so a passing test proves the script alone served it.

- Public process tests (`./cmd/api -run TestPublicProcess`, with a throwaway
  MinIO `f4-activation-s3`): `TestPublicProcessIdentityAndSignedWrites`,
  `TestPublicProcessDevelopmentPhotoPreview`, `TestPublicProcessCommunityActions`
  PASS with the API **and the worker** (photo pipeline, evidence sweep,
  observation decisions, consolidation, account deletion, cloud erase) as the
  script role. Log: `activation-script-mode-public.jsonl`.
- Run 1 (18 orgtest packages, no storage): companysession 11/11,
  fleet 23/23, orgaccess 36/36, orgaccess/http 4/4, orgnotify 9/9,
  orgnotify/http 3/3, orgapi 28/28 PASS; the other packages are listed with
  their classification below. `organization/adapters` was stopped after 119
  tests because `TestExpenseSecondCostForOneFuelingRacesBothOrders` hung on an
  unbuffered channel after its lock observer (watching the fixture login)
  timed out; the stuck sessions of the script role were in this run's own
  disposable database and were terminated. Log: `activation-script-mode-1.jsonl`.
- Run 2 (`organization/adapters`, `document/adapters`, `orgexport/adapters`,
  `orgexport/adapters/lists` with the throwaway MinIO, skipping the 53 tests
  whose lock observers watch the fixture login by name — the regex is in
  `activation-script-mode-skip.txt`): 610 PASS, 58 FAIL — `organization/adapters` 571/626 run, `document/adapters`
  18/19, `orgexport/adapters` 14/15, `orgexport/adapters/lists` 7/8; every
  storage-dependent test that failed in run 1 for the missing bucket now ran. Log:
  `activation-script-mode-2.jsonl`.

Classification of every script-mode failure (no expectation was changed and
no failure is a missing runtime privilege):

| Class | Run 1 (17 packages other than `organization/adapters`) | Run 2 | Tests |
|---|---|---|---|
| Needs the disposable bucket (rerun with MinIO in run 2, where they pass) | 13 | 0 | `document/adapters` ×7, `orgexport/adapters` ×6 |
| Seal test now refused by the missing privilege (`42501`) before its trigger (`55000`, `0A000`, a `P7xxx` or `23503` code): the test grants `UPDATE`/`DELETE`/`TRUNCATE` to the fixture login to prove the trigger, the script withholds it | 4 | 40 | e.g. `TestSensitiveAccessIsolationAppendOnlyAndDatabaseGuards`, `TestAuditReadIsolationAndImmutability`, `TestForcedScopeAndAppendOnlyRecords`, `TestIntakeSchemaRejectsInvalidClaimsAndEmptyMutation`, `TestChecklistTemplateSchemaGuardsAndIsolation`, `TestExpenseSchemaIsForcedRLSAndInsertOnly`, `TestFuelingSchemaIsForcedRLSAndInsertOnly`, the trip, mission, meter, tracking, settings, team, unit, cost-centre, period, reference-feed and suspension schema tests |
| Fault injection by revoking from (or setting the time zone of) the fixture login, which the script role does not use | 8 | 11 | `TestFailedDetailWriteLeavesNoAuditEvent`, `TestAuditedChangesAreAtomicAndReplaySafe`, `TestCapabilityAuditAndCommitFailuresRollBackTogether`, `TestCapabilityGuardJoinsExistingAuditTransaction`, `TestPublicationFailureRollsBackEveryRequiredEffect`, `TestConsumerFailureCommitAndExpiredLeaseLeaveNoReceipt`, `TestListExportsFailWithoutPartialOutput` (both runs), `TestRequiredAxisAndCommitFailuresRollbackThenRecover`, `TestAuditedContentIsMemberBoundShortLivedAndFailsClosed`, `TestAccountContextsCurrentOwnSourcePaginationAndScopeIsolation`, `TestCreatedSetupDirectoryConcurrentReadsAndStorageFailure`, `TestDocumentUploadSessionAndAuditedContentOverTheCompanyAPI`, `TestListExportsThroughActualCompanyHTTPAndWorker`, `TestSettingsLegacyReceiptsPreserveCompleteIntent`, `TestTrackReadServesNothingWhenTheAccessLogFails`, `TestTripDistancesComposeTheMeterAndTheTrackUnderTheTrackGrant`, `TestAuditWriteFailureServesNothing`, `TestMeterReviewExpiryIsElapsedSecondsUnderNonUTCSessionDST` |
| Lock observer watching the fixture login by name (`pg_stat_activity.usename`) | 3 | skipped (53) | `TestMembershipCommitFailureAndLockCancellationReturnNoSuccess`, `TestBindingLosesToObservedAccountDeletion`, `TestCancellationDuringObservedDatabaseLockDoesNotChargeQuota`; in the stopped first pass of `organization/adapters` also `TestChecklistRunReleaseRaceLeavesOneReleaseInBothOrders`, `TestContactChangeFreshProofAfter*Wait`, `TestDeviceContactProofExpiresWhileWaitingForRoot`, `TestExpenseSecondCostForOneFuelingRacesBothOrders` (hung) and others of the 53 |
| Test-only direct writes standing in for an owner that does not exist yet (asset unit/cost-centre reallocation: no runtime SQL inserts into `org_asset_cost_centre_periods` or updates `org_asset_unit_periods`) | 0 | 5 | `TestFleetAllocationArchiveGuardsCountCurrentAndFuturePeriods`, `TestFleetAllocationHistoryThroughComposedCompanyHTTP`, `TestFleetAllocationPortReproducesPermissionsExampleThree`, `TestFleetAllocationRefusesArchivedDraftAndBrokenPeriods`, `TestStructuralScopesActualCompanyHTTPExampleThree` |
| Column-level grant of the task is table-level in the script; the trigger still refuses (`55000` instead of `42501`) | 0 | 1 | `TestIncidentReportSchemaIsForcedRLSAppendOnlyAndGuarded` ("description rewritten") |
| Operator-only function correctly refused to the runtime role | 0 | 1 | `TestMeterProjectionRebuildEqualsIncremental` (EDR-317/363) |
| Test inspects the fixture login itself | 1 | 0 | `TestOrganizationTemplateIsolation` |

All remaining tests of those packages passed with the script role alone
(run 1: 220 in the 17 packages; run 2: 610, superseding the stopped first
pass of `organization/adapters`, 101 PASS / 18 FAIL of the same classes). The 53 observer tests and every failing test above run in the
default mode of the normal gates, where their expectations hold.

## Findings that affect activation

1. **R2 endpoint must be HTTPS when the switch is on.** `orgexport.NewS3`
   refuses a non-HTTPS endpoint unless a loopback fixture option is set, and
   `cmd/api`/`cmd/worker` pass the R2 configuration to the export store when
   enterprise is enabled, so an `http` endpoint makes both processes exit at
   startup. The rollout would never become ready (the previous pod keeps
   serving with `maxUnavailable: 0`). The runbook prints only the scheme as a
   pre-flight gate.
2. **No production path enables the `organizations` capability.** The audited
   store primitive exists, but no route or `ops` command reaches it, so every
   organization created on the VPS stays capability-off and its writes answer
   `ENTITLEMENT_REQUIRED`. The VPS smoke therefore stops at creation, settings
   read and that refusal; the full flow is proven only in the test above.
3. **Any signed-in account can create a capability-off organization** under
   the synthetic DPA once the switch is on; organization name, unit name and
   the creator's membership are stored. Accepted inside the owner's EDR-272
   decision for synthetic verification; rollback is the switch.
4. **Grant convergence is manual after a release** whose script changed
   (EDR-364); a new table is covered by default privileges, a new privilege on
   an existing table is not until the re-run.
5. **`identity_rate_windows` cleanup is still unscheduled** (EMP-PLAT-014);
   pre-existing for the public API, more rows with organizations.

## Validation commands and results

All from `backend/` with the brief's environment (toolchain go1.27.4,
`ANPFUEL_TEST_DATABASE_URL` on the shared PostGIS, storage variables of the
throwaway MinIO when stated).

| Command | Result | Log (`/data/dev/worktrees/f4-logs/`) |
|---|---|---|
| `gofmt -l` (go1.27.4) on `runtimegrants`, `orgtest`, `cmd/api` | empty | — |
| `go build ./... && go vet ./... && go vet -tags=integration ./...` | PASS (re-vetted after the last change) | `activation-vet.log` |
| `staticcheck` plain and `-tags=integration` on `runtimegrants`, `orgtest`, `cmd/api` | no findings | — |
| `go test -race -tags=integration ./internal/platform/runtimegrants` | 9/9 top, 15 sub PASS | `activation-green-runtimegrants-race.jsonl` |
| `go test -race -tags=integration ./cmd/api` (default mode, includes the new test and the public process tests) | 36/36 top, 28 sub PASS (163 s) | `activation-green-cmdapi-race.jsonl` |
| final reruns after the last source change: `-tags=integration ./internal/platform/runtimegrants ./internal/platform/database`; `./cmd/api -run TestRuntimeRoleServesProductionProcessAndEnterprise` | 23/23 top, 45 sub PASS; 1/1 PASS | `activation-green-final-*.jsonl` |
| `ANPFUEL_TEST_RUNTIME_GRANTS=script go test -tags=integration ./cmd/api -run TestPublicProcess` (MinIO) | 3/3 PASS | `activation-script-mode-public.jsonl` |
| script mode run 1 (18 orgtest packages, `-p 2`) | see above | `activation-script-mode-1.jsonl`, `-failures.txt` |
| script mode run 2 (4 packages, MinIO, `-skip` regex) | 610 PASS, 58 FAIL classified | `activation-script-mode-2.jsonl`, `-failures.txt`, `activation-script-mode-skip.txt` |
| `go test -p 2 -count=1 -timeout=45m ./...` (untagged) | 232 packages ok, 0 FAIL | `activation-untagged.log` |
| `python3 infra/k8s/staging/test-manifests.py`; `make check-infra test-infra` | 5/5 OK; infra-config 7/7 | `activation-infra.log` |
| `python3 docs/enterprise/tools/check_plan.py` | 0 problems | — |
| `git diff --check`; staged diff secret/PII review | clean; no password, DSN with credentials, token or personal data (test secrets are generated hexadecimal at run time; the MinIO values are the repository's published CI fixture and only in the local log environment) | — |

Not run: the default-mode `organization/adapters` race union (no migration,
route, usage source or authorization change; the orgtest default path is
unchanged — the new code runs only under the opt-in variable — and run 2
exercised the package as a whole), `orgmedia` S3 suites, the native-preparer
tests, `scripts/test-deploy.sh` (no deploy tooling touched), Kotlin (none
touched).

## Named negatives mapped to tests

| Negative | Test |
|---|---|
| Superuser login refused by `requireSafeOrgRole` (current VPS state) | `TestRuntimeRoleServesProductionProcessAndEnterprise/superuser…`, `TestRuntimeScriptProvisionsOrdinaryRoleAndRefusesUnsafeUse` |
| Runtime cannot `TRUNCATE`/`DELETE`/`UPDATE` append-only rows | `TestRuntimeScriptProvisionsOrdinaryRoleAndRefusesUnsafeUse` |
| Runtime cannot call operator-only or closed functions | same (`org_meter_current_rebuild`, `org_pending_owner_count`) |
| Runtime cannot run DDL, create objects or change role | same |
| Another organization stays not found | process test (both directions), RLS check in the package test |
| Stale or unsafe inventory refused | `TestBuildRefusesStaleOrUnsafeInputs`, `TestRuntimeScriptMatchesHead` |
| Script applied on another schema version, to an owning role or to the owner | `TestRuntimeScriptProvisionsOrdinaryRoleAndRefusesUnsafeUse` |
| Capability-off organization write | process test (`ENTITLEMENT_REQUIRED`) |
| Migrate/backup/schema Jobs and registry job keep the owner DSN | `test_runtime_login_and_enterprise_switch` |

## Runbook

[ENTERPRISE_ACTIVATION_RUNBOOK](../../operations/ENTERPRISE_ACTIVATION_RUNBOOK.md):
agent lock, pre-flight (release carries the script for the live schema, R2
scheme, owner superuser, PUBLIC without CREATE, revision recorded), verified
backup with the agent's backup Job (`activation-*`), script as the owner,
password with statement logging off, new secret key, strategic patch of both
containers, rollout status, smoke, `permission denied` count, release;
rollback by deleting the two entries; re-run after releases.

## Retained

- The VPS activation itself (coordinator, after the automatic deploy of the
  integrated release and the owner's final confirmation, EDR-272).
- Regenerate `infra/db/runtime-role.sql` on the integrated tree whenever it
  carries migrations or SQL beyond `b17dc3d8` (one command; the test fails
  until then).
- An audited operator path for the `organizations` capability (no production
  route or command exists).
- Wiring the grant convergence into the release pipeline (EDR-364); until
  then the documented re-run after releases whose script changed.
- `identity_rate_windows` cleanup scheduling and measured production quota
  classes (EMP-PLAT-014); separate API and worker roles if ever needed.
- A reviewed operator login for `ops meter rebuild-projection` (it needs an
  ordinary, non-owner role with `EXECUTE` on the rebuild; never the runtime
  role).
- EMP-REL-001, the VAL-24 switch rehearsal, the live zero-diff of the staging
  manifests after activation, and the M4 device matrix before any tracking
  (EDR-361/362).
- `orgmedia` S3 suites and the 15 native-preparer integration tests were not
  run (outside this change; the latter need pinned Rust CLIs absent here).
