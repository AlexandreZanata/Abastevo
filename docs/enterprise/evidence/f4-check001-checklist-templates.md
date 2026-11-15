# EMP-CHECK-001 — Checklist templates per asset category

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 task on `codex/enterprise-f4-check001` (worktree
`/data/dev/worktrees/f4-check001`, based on the F4 integration head
`942af96b`, which already carries `org-permissions-v5` granting
`check.template.manage` to OWNER, ADMIN and FLEET_MANAGER at ORG). The
coordinator integrates it into `codex/enterprise-f4` and owns PROGRESS, the F4
execution ledger, pushes and the PR. Real dependencies used: FLEET-002 assets
and categories, FLEET-003 lifecycle, FLEET-005 allocation and AUTH-006
structural scope, FLEET-006 assignments and SELF coverage, the audit runner and
the company HTTP entry. No run, intake, event, job, reference feed or Android
screen is added. Synthetic data only.

Read: the EMP-CHECK-001..004 task blocks ([EP11](../backlog/ep11-maintenance.md));
[maintenance](../modules/maintenance.md) §§1–12 (BR-CHECK-001..018, UC-CHECK-01..06,
§6 checklist run states, §7 permissions, §8 offline, §9 events, §10 limits,
SCR-CHECK-01/04/06); [08-state-model](../08-state-model.md) (Checklist run
row, CHECK-002's); [10-offline-sync](../10-offline-sync.md) §10.3;
[07-permissions](../07-permissions.md) §7.3 and the v5 amendment;
[09-architecture](../09-architecture.md) §2 (the `maintenance` module);
EDR-190/191/192..201; evidence of FLEET-007 (route, matrix and upgrade
precedent), FLEET-006, TRIP-001 and DOC-006 (shared-vector precedent),
METER-003 (oracle grammar).

IDs used: B-BR-EMP-330..335, BUC-EMP-166..168, EDR-213..215 — all of them; no
pre-allocated ID is left unused. Migration `000223_*` only; every test derives
the version from one file-name constant (`checklistMigration`) and builds the
previous chain as `name < thisMigration` (000219 on this base).

## Rules frozen before behaviour

Observed before any change: no checklist, template or maintenance code, table
or route existed; `check.template.manage` was granted (v5) but
`NOT_YET_ROUTED`; the asset-use source's latest body is 000217; categories
(`org_asset_categories`) are immutable and never deleted; DRAFT assets can be
discarded, so a key naming one must count as use or never name one.

Rules B-BR-EMP-330..335 and use cases BUC-EMP-166..168 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-CHECK-001 — Checklist templates per asset category (2026-11-15)";
decisions EDR-213 (module and package placement, shared rule, item identity,
critical items never optional, WARN acknowledgement, error order, policy tie
rule), EDR-214 (persistence, guards, exclusivity, usage allow-list) and EDR-215
(routes, policies, representative, AUDITOR, retained feed) in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

Deviations from the coordinator's starting design, each recorded in an EDR:

- Kotlin package `com.anpfuel.domain.enterprise.maintenance.checklist`, not
  `enterprise.checklist`: maintenance.md names `com.anpfuel.*.enterprise.maintenance`
  and the Go placement nests `maintenance/domain/checklist` like
  `document/domain/fiscal` (EDR-213).
- AUDITOR holds `fleet.asset.read` (`O`), so the applicable read is a
  positive AUDITOR route; only the six template routes are `FORBIDDEN` to it
  (EDR-215). The walk proves both.
- Archive is included (BR-CHECK-002 reads "the ACTIVE one"; UC-CHECK-06
  alternative), and the template revision is the single compare-and-set value
  of every command.

## Shape

- Pure Go `backend/internal/modules/maintenance/domain/checklist`
  (standard library only): `Definition`/`NewDefinition` (bounds, canonical
  sorted target), `Policy`/`NewPolicy`/`DefaultPolicy`, `Evaluate` (answers →
  outcome, issues, critical, acknowledgement), `Resolve` (asset → category →
  organization default over a consistent candidate set), `PolicyAt`,
  `CanonicalID`; errors `ErrInvalid`, `ErrAnswerNotAllowed`,
  `ErrPhotoRequired`, `ErrIncomplete`; `Response` redacted on fmt, JSON and
  slog.
- Kotlin mirror `domain/src/main/kotlin/com/anpfuel/domain/enterprise/maintenance/checklist/Checklist.kt`
  (`ChecklistDefinition.of`, `ChecklistPolicy`, `Checklist.evaluate`,
  `resolve`, `policyAt`, `canonicalId`, `ChecklistFailure`;
  `ChecklistResponse.toString` redacted).
- Oracle `contracts/testdata/enterprise/checklist/checklist-v1.psv` (127 cases:
  TEMPLATE 40, POLICY 11, EVALUATE 38, RESOLVE 22, POLICY_AT 16; SHA-256
  `4231d227e82782a2dedf03655bc69e48168664bd8ef7a10a733ce3d3140165e2`) and its
  README; both consumers assert the digest, the counts, unique IDs, known
  operations and keys, complete expected maps, that every operation and every
  error code has a vector and that every operation has an `INVALID` vector.
- Application `maintenance/application`: `ChecklistAction` (`TemplateRead`,
  `TemplateManage`, `ApplicableRead`) and `Policy` (the applicable read is
  exactly `fleet.Policy(false)`, asserted), `TemplateCommand` (CREATE,
  PUBLISH, POLICY, ARCHIVE; canonical digest), views, `ChecklistAuthority`
  port, `ChecklistService` (live decision of the exact policy and target
  before and after the repository, result validation).
- Migration `000223_organization_checklist_templates.sql` (EDR-214): six
  forced-RLS tables, the guarded head, version/item/target/policy guards, the
  deferred `org_checklist_version_complete` (closed version, exclusivity), the
  asset-use source with one new allow-list line.
- sqlc package `db/queries/maintenance` (own stanza, migrations 004, 020,
  165, 169, 172, 174, 197, 223); store `maintenance/adapters` (authority →
  root lock → receipt replay → one audit event → target checks → effect →
  receipt → authority; reads between two authority checks; the applicable read
  resolves the matching candidates with the pure `Resolve`).
- Authority `organization/adapters/checklist_authority.go`; the company
  authority admits the two template policies.
- HTTP `maintenance/adapters/http` (EDR-215), composed by `enterprisesuite`
  with admission class `check.template` and two authorizations (templates
  without structure; the applicable read with the asset structure and
  assignee sources).
- Contract: 7 paths, 22 schemas, tag `enterprise-checklists` in
  `contracts/openapi/v1.yaml` (patience diff purely additive, +1,174 lines);
  inventory `authorization-routes-v1.json`: `check.template.manage` ROUTED (6
  routes, representative the list), `fleet.asset.read` +1 route (routed
  22→23, not routed 62→61, normal 121→128, special unchanged); router test
  paths +7; authorization matrix +1 representative (`check.template.manage`
  on the template list: 220→230 cells, 85→88 allowed, 135→142 denied; the
  private-state snapshot includes the six new tables); AUTH-011 auditor walk
  128 routes (29 positive including the applicable read, 99 FORBIDDEN).

New allow-list line in `org_asset_current_usage` (on top of the 217 body; the
coordinator reconciles it with FLEET-011's 000222 and TRIP-002's 000220):

```sql
OR(c.conrelid='public.org_checklist_template_targets'::regclass AND c.conname='org_checklist_template_targets_asset_fkey')
```

## RED

Logs in `/data/dev/worktrees/f4-logs/check001-*`.

- Go domain (compiled zero-value stubs returning `errNotImplemented`)
  `go test -race -json ./internal/modules/maintenance/domain/checklist/` →
  `check001-go-domain-red.jsonl`: 7 top-level, 6 failed, and 127/127 shared
  vectors failed; the one passing test exercises only the grammar reader.
- Kotlin (`TODO()` stubs) `:domain:test --tests '…maintenance.checklist.*'`
  (flock) → `check001-kotlin-red.log`, XML in `check001-kotlin-red-xml/`: 133
  tests, 132 failures (vectors 127/128 and unit 5/5); only the grammar method
  passed.
- PostGIS schema RED (migration file moved out) `go test -json -race
  -tags=integration ./internal/modules/organization/adapters -run
  '^TestChecklistTemplate'` → `check001-db-red-schema.jsonl`: 6/6 failed
  (42P01; the upgrade test on the missing migration file).
- PostGIS behaviour RED (migration present, the four store entry points
  stubbed to zero values) → `check001-db-red-behaviour.jsonl`: 6/6 failed at
  their first behaviour check (the upgrade test after the ordinary migrator
  applied 000223, at the draft-target refusal).
- Router RED (routes wired and inventory updated, spec at `HEAD`) →
  `check001-cmd-red.jsonl`: 3 top-level failed
  (`TestAuthorizationInventoryCoversFullProductionRouter`,
  `TestCompanyProductionCompositionIsOptInAndContractChecked`,
  `TestFullProductionRouterEnterpriseContractAndDefaultOff/true`: undocumented
  organization route).
- Contract RED (`TestChecklistTemplate*` in apicontract against the `HEAD`
  spec) → `check001-contract-red.jsonl`: the shapes test failed 18/18 subtests
  and then dereferenced a missing request schema, so the guard test did not
  run.

## GREEN and REFACTOR

- Go domain → `check001-go-domain-green.jsonl`: first implementation 7/7
  top-level and 127/127 vectors under race, 99.3 % statements; the
  `CanonicalID` separator branch was then covered by a characterization test
  (8/8, 100.0 %, `check001-go-domain-cover.out`).
- Kotlin → `check001-kotlin-green.log`: 133/133, 0 failures, errors or skips.
- PostGIS focused (`-race -tags=integration`, ordinary NOSUPERUSER /
  NOBYPASSRLS runtime role) → `check001-db-green.jsonl`: 6/6 top-level passed
  after two fixes found by GREEN: (1) the applicable-candidates query scanned
  a NULL target of an organization default into a string (fixed with
  `COALESCE(…,'')` in the query; the store refused with `UNAVAILABLE` until
  then); (2) the schema fixture statement "add a target to version 2" selected
  a category label that did not exist and inserted zero rows (the statement
  now selects a real category; the expected 55000 is unchanged). One
  expectation was corrected because it was wrong, not weakened: a direct
  insert of a critical-and-optional item into a committed version is refused
  first by the closed-version guard (55000), never reaching the row CHECK; the
  test keeps that refusal and additionally asserts the CHECK
  `NOT(critical AND optional)` definition.
- Matrix and walks (throwaway MinIO project `f4-check001-s3`) →
  `check001-matrix.jsonl`: matrix, auditor walk and the six checklist tests,
  8/8 top-level and 3,534 subtests passed; matrix 230 cells (88 allowed / 142
  denied); auditor 128 routes (29 positive, 99 FORBIDDEN).
- Router and contract GREEN: `cmd/api` passes; `TestChecklistTemplate*` in
  apicontract 2/2 top-level, 46 subtests (`check001-contract-green.jsonl`).
- Added after GREEN as characterization (not counted as RED): application
  `TestPoliciesAreFixedAndTheApplicableReadIsTheFleetAssetRead`,
  `TestTemplateCommandsAreExactPerKind`,
  `TestChecklistServiceBindsDecisionsAndValidatesResults`,
  `TestChecklistValuesAreRedacted`; HTTP
  `TestChecklistRegistrationPinsSevenCompanyPolicies`,
  `TestChecklistRecordsAcceptOnlyTheirPoliciesAndTargets`,
  `TestChecklistRefusalsMapToRegisteredCodes`; store
  `TestChecklistAuditActionsAreOnePerCommandKind`,
  `TestChecklistStoreErrorsKeepOnlyRegisteredRefusals`.
- REFACTOR: absent optional response fields are omitted (the spec has no
  nullable fields); a shadowing `context` parameter was renamed.

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| A run pins its applicable template version and the current policy | PostGIS `TestChecklistTemplateTargetsAndApplicableResolution` (asset → category → organization default → NONE, other context ignored, archived template no longer applies, a new version is pinned after publication, the moved target frees the old one); HTTP applicable read returns the version 2 and policy revision 2 after publish and policy change, without `target_ids`; Go/Kotlin RESOLVE vectors (22) |
| Foreign template (other organization → not found) | store: read and archive through the other organization → `NOT_FOUND`, the other organization lists none; HTTP: read and archive through the other organization's path → 404; foreign asset applicable read → 404; foreign or unknown category/asset target → `NOT_FOUND` (store and HTTP); RLS test: another scope sees 0 rows, a cross-organization head insert → 42501 |
| Invalid answer policy | EVALUATE vectors: `NOT_APPLICABLE` on a non-optional item → `ANSWER_NOT_ALLOWED`; `NOT_OK` without a required photo → `PHOTO_REQUIRED`; unanswered → `INCOMPLETE`; malformed sets → `INVALID`, with the fixed order pinned (6 precedence vectors); invalid critical-item policy: POLICY vectors (bounds 3599/259201, zero, negative, unknown/lowercase/empty mode), `NewTemplateCommand` refusals, HTTP 422 (validity 3599 and 259201, mode `ALLOW`), SQL CHECK; critical-and-optional items refused (TEMPLATE vector, HTTP 422, SQL constraint) |
| A prior revision rewritten after publication | store: version 1 read unchanged after version 2 (name and items); HTTP read `version: 1` after publish; schema test: UPDATE/DELETE/TRUNCATE of versions, items, targets, policy revisions and receipts → 55000; refilling a version 1 position → 23505; adding an item or target to a committed version, re-inserting version 1, a version gap and a policy revision gap → 55000; head context change, moving back a version, skipping a revision, publish and policy at once, deleting the template → 55000 |
| Exclusive targets per context | store: organization default, category and asset taken → `CONFLICT`; another context allowed; archive frees; concurrency `TestChecklistTemplateConcurrentPublicationsTakeATargetOnce`: two creations of one category queued behind a held root lock (both commands observed waiting) — the first queued wins and the other is `CONFLICT` with no row or audit event, in both orderings; a publish and a policy change on one revision race to exactly one change; HTTP target taken → 409 |
| Asset target states | DRAFT and DISPOSED targets → `VALIDATION_FAILED`/`ErrInvalid`; ACTIVE and INACTIVE accepted; SQL guard 23514 |
| Replay / reuse | store and HTTP replay return the first acknowledgement (`replayed: true`); same identifier with another intent → `OPERATION_ID_REUSED`; exactly one audit event and one receipt per accepted command (4/4 after create, publish, policy, archive); refused commands leave no row |
| Stale revision, archived template | store and HTTP → `CONFLICT`; archive twice → `CONFLICT` |
| Authorization (V-AUTHZ) | HTTP: SUPERVISOR (no key) → 403 `check.template.manage`; UNIT-scoped FLEET_MANAGER → 403 on template creation (organization-level only); ORG FLEET_MANAGER archives; Norte supervisor reads the Norte asset's template, Sul supervisor → 403 `fleet.asset.read` on the Norte asset and 200 on a Sul asset (AUTH-006 unit scope); SELF driver reads the assigned asset's template, 403 on an unassigned asset, 403 on template read and publish; matrix representative over 10 roles and 13 variants (missing/expired proof, bad origin/CSRF, other organization, suspended membership, ended/expired assignment, stale authentication, capability off, suspended organization, SELF and UNIT scope); AUDITOR walk: six template routes FORBIDDEN, applicable read positive with an unchanged private snapshot |
| Forced RLS, insert-only | schema test and the upgraded schema (RLS forced on all six tables, owned by the migrator) |
| Populated upgrade 219 → 223, failure and recovery | `TestChecklistTemplatePopulatedUpgradeIsAdditiveAndRecoverable` (ACTIVE and DRAFT assets in two categories, an assignment and a reservation; injected failure leaves schema, usage source and data unchanged; ordinary migrator with only REFERENCES and function ownership; no-op replay; previous image check; 12 triggers, 4 key constraints and every allow-list line; behaviour on the upgraded schema; the asset-use source accepts the new key with unchanged counts) |

## Validation

Environment: worktree backend, Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
`GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`; shared PostGIS control
`127.0.0.1:32775` used only through orgtest isolated child databases; a
throwaway MinIO project `f4-check001-s3` (torn down at the end); Kotlin via the
shared Gradle flock with JDK 17.

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean.
- `GOTOOLCHAIN=go1.27.1 staticcheck` on maintenance, organization,
  enterprisesuite, apicontract, cmd/api, db/queries/maintenance, and with
  `-tags=integration` on maintenance, organization/adapters, orgtest: no
  findings (`check001-staticcheck.log`).
- `sqlc vet` and `sqlc generate` clean (the new `db/queries/maintenance`
  package is committed; regeneration produces no diff).
- OpenAPI `vacuum lint -r contracts/openapi/vacuum-rules.yaml`: 0 errors, 995
  warnings / 644 informs vs 987 / 634 at `HEAD` (`check001-vacuum.log`); the
  patience diff is purely additive.
- Kotlin `:domain:test --tests 'com.anpfuel.domain.enterprise.maintenance.checklist.*'`:
  133/133.
- Affected race/integration union `go test -json -race -tags=integration
  -count=1 -timeout=60m -p 2 ./internal/modules/organization/adapters
  ./internal/modules/fleet/... ./internal/modules/maintenance/...
  ./internal/platform/enterprisesuite/... ./internal/platform/apicontract/...
  ./internal/platform/migrate/... ./internal/platform/orgapi/...
  ./internal/platform/orgtest/... ./cmd/api/...` (with the throwaway MinIO)
  → `check001-db-union.jsonl`: 16 packages (15 ok, `orgtest` has no tests),
  794 top-level / 5,785 subtests passed, 0 failures, 0 skips
  (`organization/adapters` 1,057.4 s; wall 18 min 39 s with other agents'
  suites on the machine).
- Untagged whole module `go test -count=1 -timeout=45m -p 2 ./...` →
  `check001-untagged-all.log`: 219 packages ok, 79 without tests, 0 FAIL
  (includes the four new maintenance packages, apicontract and cmd/api).
- `cmd/worker` integration tests were not run (untouched; their 15 known
  failures need pinned Rust preparer CLIs this machine lacks); nothing in
  this task composes a worker job.
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff
  --check` clean; staged diff reviewed for secrets/PII: synthetic identifiers
  and labels only, the fixed synthetic cursor secrets of existing tests, the
  disposable MinIO credentials only in the log environment file (not
  committed).

## Retained

- Checklist runs: execution offline, the durable operation, the server
  recheck with `PolicyAt` and `TEMPLATE_SUPERSEDED`, damages from `NOT_OK`
  answers, validity and `CHECKLIST_EXPIRED`, blocks on the asset
  (EMP-CHECK-002, then EMP-TRIP-019 for the trip gate).
- The reference-feed extension that ships template versions and policy
  revisions to devices (EMP-OFFL-006, built in parallel); the plain scoped
  applicable read is the source until then.
- Template events (`check.template.published`) and notifications; no
  consumer exists.
- Policy-history and version-list reads for the panel; the panel itself
  (SCR-CHECK-06, EMP-CHECK-004).
- Production grants (README subsection) and live composition review.
- No milestone, phase or gate is claimed.
