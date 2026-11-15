# EMP-FLEET-011 — Implements: attach to and detach from a powered asset

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F4 task on `codex/enterprise-f4-fleet007`, fast-forwarded to the
integrated batch head `5a206693` (catalog v5 and EMP-FLEET-007 already
merged). Real dependencies used: FLEET-002 assets (kinds, tanks, activation
with meters), FLEET-003 lifecycle states, FLEET-005 allocation (structural
scope), AUTH-006 structural grants, the FLEET-007 availability read and its
active-use port. No trip exists on this branch: EMP-TRIP-002 (migration 000220)
is built in parallel and will replace `NoActiveUse` (EDR-212).

Read: task block `ep05-fleet-meters.md` EMP-FLEET-011; `modules/fleet.md` §3
(implement attachment), §4 (BR-FLEET-002, BR-FLEET-033), §5 (UC-FLEET-10), §6.2
(row 2 "or as an attached implement"), §7 (implements under
`fleet.asset.manage`), §8, §9 (`fleet.implement.attached/detached`);
`modules/trips.md` (no implement rule beyond BR-FLEET-033); EDR-161, 164, 192–195.

IDs used: B-BR-EMP-325..329, BUC-EMP-164..165, EDR-210..212 (all of them).
Migration `000222_*` only; tests derive it from one constant
(`attachmentMigration`) and build the previous chain as `name < thisMigration`.

## Rules frozen before behaviour

Observed: assets have closed kinds (`VEHICLE`, `MACHINE`, `IMPLEMENT`); the
kind changes only while `org_asset_current_usage` counts no reading or
operation; no attachment storage, route or port exists; the availability read
answers row 2 only from the asset's own active use (`NoActiveUse` = no source).
The coordinator suggested `fleet.asset.assign`; UC-FLEET-10 and fleet.md §7
name `fleet.asset.manage`, which this task follows (EDR-211).

Rules B-BR-EMP-325..329 and BUC-EMP-164..165 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-FLEET-011 — Implements: attach to and detach from a powered asset";
decisions EDR-210..212 in [16-decisions](../16-decisions.md).

## Shape

- Domain `fleet/domain/attachment.go`: `ValidAttachmentKinds`,
  `MainAssetAllowed` (AC4, `ErrImplementMainAsset`), `ValidNewAttachment`,
  `DetachAt` (explicit or server end; before attach/past/after planned end
  refused; `ErrAttachmentEnded`); `UseFact.Via`/`Cause.Via` carry the powered
  asset of an implement's use.
- Application `fleet/application/attachments.go`: commands with scoped digests,
  `AttachmentService` (fresh `fleet.asset.manage` on the path asset, list under
  `fleet.asset.read`; the FLEET-007 guard is now the shared `guardedCall`),
  `AttachedImplementsPort`, `ErrAttachmentOverlap`.
- Migration `000222_organization_implement_attachments.sql` (EDR-210).
  New `org_asset_current_usage` lines on top of the 217 body, for the
  coordinator's reconciliation with TRIP-002's 000220:
  `OR(c.conrelid='public.org_asset_implement_attachments'::regclass AND c.conname='org_asset_implement_attachments_implement_fkey')`,
  `OR(c.conrelid='public.org_asset_implement_attachments'::regclass AND c.conname='org_asset_implement_attachments_powered_fkey')`
  and the operations term
  `+(SELECT count(*) FROM public.org_asset_implement_attachments i WHERE i.org_id=selected_org AND(i.implement_id=selected_asset OR i.powered_asset_id=selected_asset))`.
- Store `fleet/adapters/attachments.go` (+ sqlc `db/queries/fleet/attachments.sql`):
  authority (path asset) → root lock → receipt replay → one audit event →
  authority of the other asset → kinds → transaction instant → effect →
  receipt → authority; the exclusion refusal maps to `ErrAttachmentOverlap`;
  the port; the availability facts of an implement add its powered asset's
  active use (`Via`).
- HTTP `fleet/adapters/http/attachments.go`: `attach` (201), `detach`, `list`;
  availability causes may carry `powered_asset_id`.
- Contract: 3 paths, 5 schemas and `powered_asset_id` on the availability
  cause in OpenAPI; inventory `fleet.asset.manage` +2 and `fleet.asset.read`
  +1 routes (routed 22, not routed 62, normal 118→121); router test +3 paths;
  AUTH-011 auditor walk 121 routes (28 positive incl. the list, 93 FORBIDDEN);
  matrix unchanged at 220 cells (its private-state snapshot includes the two
  new tables).
- Fixture: `orgtest.GrantMeterUsage` grants
  `SELECT(org_id,implement_id,powered_asset_id)` on the attachments table to
  every runtime role (the usage source now reads it in discard and kind
  checks).

## RED

Logs `/data/dev/worktrees/f4-logs/fleet011-*`.

- Domain (zero-value stubs) → `fleet011-domain-red.jsonl`: 3 new top-level
  failed (kinds/eligibility, periods/detach, implement through powered
  asset), 31 / 163 passed.
- Application (stubs) → `fleet011-app-red.jsonl`: 3 new failed, 54 / 119
  passed.
- Router (routes wired, spec and inventory at the previous state) →
  `fleet011-cmd-red.jsonl`: 3 failed (inventory, company composition, full
  router).
- Contract (`TestFleetAttachment*` against the previous spec) →
  `fleet011-contract-red.jsonl`: failed.
- First PostGIS run: 3/3 failed on a migration syntax error (a mangled RLS
  policy line in the generated file; fixed by copying the 217 line) — a RED for
  the wrong reason, so the genuine schema/behaviour REDs below were recorded
  after the fix.
- PostGIS schema RED (migration moved out) and behaviour RED (store entry
  points stubbed) over the five attachment tests → `fleet011-db-red-schema.jsonl`,
  `fleet011-db-red-behaviour.jsonl`: 5/5 failed in each (schema: the
  missing tables / migration file; behaviour: zero-value store entry points —
  the first behaviour attempt exposed that the race test left its lock
  holder open after a failed wait, so it now rolls back and cancels in a
  cleanup and the RED was rerun to completion).

## GREEN

- Domain and application race ok; HTTP and job/contract unit tests ok.
- PostGIS (`-race -tags=integration`, ordinary runtime role), 5/5:
  `TestFleetImplementAttachmentsOverlapPortAndAvailability`,
  `TestFleetImplementAttachmentConcurrentRace`,
  `TestFleetImplementAttachmentSchemaGuards`,
  `TestFleetAttachmentPopulatedUpgradeIsAdditiveAndRecoverable`,
  `TestFleetAttachmentsThroughComposedCompanyHTTP` (33 HTTP assertions).
- Matrix and auditor walk (throwaway MinIO `f4-fleet011-s3`) →
  `fleet011-matrix.jsonl`: 2/2, 3,380 subtests.
- Two test-expectation fixes during GREEN (not behaviour): the harrow was
  still attached until its planned end (a fresh implement now proves "no
  powered asset → available"), and the receipt count is 5, not 6.

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| AC1 overlapping attachment → `CONFLICT` | store (refused whole: no row, receipt or audit; overlap with a planned end; attach from the switch instant allowed); concurrent, both orders with barriers (`TestFleetImplementAttachmentConcurrentRace`: the first queued wins, the second `ErrAttachmentOverlap`); SQL 23P01; HTTP 409 |
| AC2 trip start lists attached implements | through the port `AttachedImplementsDBTX` (both implements on the tractor at the instant; the truck's implement only from the switch instant; half-open boundary); the trip-side listing is retained for EMP-TRIP-002 |
| AC3 implement `IN_USE` with its powered asset's trip | domain `TestAttachedImplementFollowsItsPoweredAsset`; PostGIS availability read with a test active-use source answering a trip on the tractor: the attached implement `IN_USE` naming the tractor, an unattached implement `AVAILABLE`, the tractor's own cause without `Via`; with `NoActiveUse` both missing sources are listed |
| AC4 `IMPLEMENT` refused as main asset | domain `TestAttachmentKindsAndMainAssetEligibility` (`MainAssetAllowed`); call site retained for EMP-TRIP-002 |
| Attaching to an implement | store and HTTP `VALIDATION_FAILED`; SQL 23514 |
| Another organization's asset | store `NOT_FOUND` for a foreign implement and a foreign powered asset; HTTP 404; cross-org insert 42501; RLS without the setting sees nothing |
| Two concurrent attachments | race test above |
| Detach before the attach instant | domain (explicit end before start, in the past, after a planned end); store `VALIDATION_FAILED`; HTTP 422; SQL 55000 when the end precedes the start |
| Scope on both assets | HTTP: a Sul `FLEET_MANAGER` is `FORBIDDEN` on a Norte implement and when attaching a Sul implement onto a Norte tractor |
| Kind fixed by an attachment | store and upgraded schema: changing the implement's kind → P7903 |
| Replay / reuse | store and HTTP replay; reuse → `OPERATION_ID_REUSED` |
| Populated upgrade 217 → 222, failure, recovery | `TestFleetAttachmentPopulatedUpgradeIsAdditiveAndRecoverable` (assets, an assignment and a reservation; injected failure leaves schema, usage source and data unchanged; ordinary migrator with REFERENCES and function ownership only; no-op replay; previous image check; RLS, triggers, exclusion and every allow-list line; attach, exclusion, admission and kind lock on the upgraded schema) |

## Validation

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean.
- `GOTOOLCHAIN=go1.27.1 staticcheck` on fleet, organization adapters,
  enterprisesuite, apicontract, cmd/api, db/queries/fleet and with
  `-tags=integration` on organization adapters, fleet and orgtest: no
  findings (`fleet011-staticcheck.log`).
- `sqlc generate` (fleet stanza + 000222), `sqlc vet`, `sqlc diff` clean.
- OpenAPI vacuum: 0 errors; 975 warnings / 642 informs (the batch head with
  FLEET-007 had 969 / 633; the per-route pattern of 3 new routes); patience
  diff purely additive (+381).
- Affected race/integration union (same 13 paths as FLEET-007, throwaway MinIO)
  → `fleet011-db-union.jsonl`: 39 packages ok, 1,107 top-level / 7,403
  subtests, 0 failures, 0 skips (`organization/adapters` 1,173.5 s). The
  FLEET-007 availability tests needed one fixture change: once 222 is
  installed, the availability read of an implement reads the attachment
  table, so `reservationStore` grants `SELECT` on it when installed (the
  production grant is in the README text).
- Untagged whole module → `fleet011-untagged-all.log`: 215 packages ok, 78 without tests, 0 FAIL.
- `check_plan.py` → 0 problems; `git diff --check` clean; secret/PII review of
  the staged diff: synthetic identifiers and the fixed synthetic cursor secret
  only.

## Retained

- EMP-TRIP-002: record the implements attached at a trip start through
  `AttachedImplementsDBTX`, apply `MainAssetAllowed` in the start eligibility,
  and replace `NoActiveUse` (which also makes AC3 live end to end).
- No automatic detach on deactivation/disposal (no rule; EDR-210).
- Implement-specific meters and checklists per implement (task scope out);
  an app action for frequent swaps (task risk note); panel screens.
- Production grants: the Fleet principal needs `SELECT, INSERT, UPDATE ON
  org_asset_implement_attachments` and `SELECT, INSERT ON
  org_asset_attachment_commands`; every principal that discards an asset or
  changes its kind needs `SELECT (org_id, implement_id, powered_asset_id)` on
  the attachments table.
- No milestone, phase or gate is claimed.
