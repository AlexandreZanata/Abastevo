# F4 — Execution ledger: trips, logbook, sync hardening and location fundamentals

Date: 2026-11-15.
Status: IN_PROGRESS
Validation: PENDING
Integration: INTEGRATION_PENDING

Opened by Claude Code from the coordinator session while the F3 batch
[PR #221](https://github.com/AlexandreZanata/Abastevo/pull/221) (`dev → main`,
head `8c2ee654`) is open with its CI running. Owned branch
`codex/enterprise-f4` in a dedicated worktree on the data volume
(`/data/dev/worktrees/enterprise-f4`), created from `origin/dev` = `8c2ee654`
with no upstream so that nothing reaches `dev` by accident. Only one
`dev → main` PR may be open at a time: F4 is backed up on its owned remote
branch and reaches `dev` only after #221 has merged, by a normal merge with the
then-current `origin/dev` and re-run affected gates. The local `dev` branch in
`.worktrees/main-reconciliation` (three unpublished 2026-10-24 Android commits)
is ignored as instructed. No merge, deployment, real company data, real
location data or physical device is authorized; the only Android device in
scope is the owned emulator `emulator-5588` (AVD `enterprise-f2-api36`).

Scope: the F4 phase of the [roadmap](../14-roadmap.md) — 38 tasks, 65 relative
cycles, milestone **M4** "Operations are planned and followed" with gate
EMP-REL-005 — plus its unfinished cross-phase prerequisites in dependency
order. No F4 or M4 acceptance is claimed from labels, plans or partial work.

## Reconciliation against source (2026-11-15, `8c2ee654`)

Source state at opening:

- **No F4 task exists in source.** There is no trip module, no mission,
  checklist, incident, reservation, implement or tracking code, table or
  route. The permission catalog `org-permissions-v4` already names every F4
  key (`trip.*`, `check.*`, `fleet.reservation.manage`, `gps.*`), but
  `fleet.reservation.manage`, `check.template.manage` and
  `check.incident.manage` are granted to no role.
- **F1/F2/F3 prerequisites already LOCAL_DONE / INTEGRATION_PENDING (not
  reimplemented):** OFFL-002/003/008, IDN-010, IDN-015, IDN-018, ORG-009,
  PLAT-008, PLAT-016, APPR-001, APPR-006, FLEET-002, FLEET-006, METER-001,
  METER-002, DOC-003, PRIV-005, PLAT-015.
- **Dependency-ready F4 tasks at opening (6):** OFFL-009, OFFL-011 (Android),
  OFFL-012, FLEET-007 and CHECK-001 (both also need a catalog grant, see
  below), FLEET-011.
- **Blocked F4 tasks (32):** every TRIP-0xx task (behind the open C1 trip
  chain), CHECK-002/003/004, ROUTE-004, OFFL-013, all GPS tasks and REL-005.

Open prerequisites outside F4 (transitive closure):

| Task | Phase | State at opening | Gates in F4 |
|---|---|---|---|
| EMP-TRIP-001 | F2 (C1) | TODO, ready | every TRIP task, GPS-001 |
| EMP-TRIP-002 | F2 (C1) | TODO | TRIP-007/008/011/012, TRIP-019, everything after |
| EMP-TRIP-003 | F2 (C1) | TODO | TRIP-010, CHECK-003 |
| EMP-TRIP-004 | F2 (C1) | TODO | TRIP-014/015/016 → REL-005 |
| EMP-TRIP-005 | F2 (C1, Android L) | TODO | TRIP-020/021, ROUTE-004, GPS-002 |
| EMP-OFFL-006 → EMP-FLEET-013 | F2 (C1) | TODO, OFFL-006 ready | TRIP-005 |
| EMP-METER-003 → EMP-METER-004 | F2 | TODO, METER-003 ready | TRIP-014 → TRIP-018 |
| EMP-METER-005 → EMP-METER-006 | F2/F3 | TODO | GPS-006 |
| EMP-OFFL-005 | F3 (Android) | TODO, ready | OFFL-013 → REL-005 |
| EMP-DOC-004, EMP-FUEL-011, EMP-EXP-002, EMP-EXP-004 | F3 | TODO | TRIP-020 |
| EMP-GOV-005 | F0 | PACKAGE READY / REVIEW PENDING (human privacy/employment reviewers) | GPS-001 → all tracking → REL-005 |
| EMP-GOV-006 | F0 | TODO (synthetic tracking spike on target devices) | GPS-001 → all tracking → REL-005 |

Critical chains to the M4 gate (EMP-REL-005 needs OFFL-013, TRIP-015,
GPS-006 and GPS-010):

```
TRIP-001 → TRIP-002 → TRIP-004 (+APPR-001) → TRIP-015 (+PLAT-016)
OFFL-005 (Android) → OFFL-013
GOV-005 + GOV-006 → GPS-001 → GPS-004 → GPS-005 → GPS-006 (+METER-006 ← METER-005 ← METER-003)
GPS-001 + TRIP-005 (← OFFL-006 → FLEET-013, TRIP-002/003) → GPS-002 → GPS-010
```

M4 therefore cannot be reached in this batch: GOV-005 needs designated human
reviewers and GOV-006 needs target devices (the physical phone attached to this
machine is excluded; an emulator is not a target device). Both stay
explicit external obligations.

## Execution decision (EDR-196)

Open F4 with its dependency-ready work and the C1 trip chain, each task a
bounded commit built in an isolated owner worktree and integrated here:

1. Wave 1 (parallel, disjoint files): TRIP-001 → TRIP-017 (pure trip domain,
   Go + Kotlin + shared oracle); METER-003 → METER-004 (F2 prerequisites of
   TRIP-014); the `org-permissions-v5` amendment granting
   `fleet.reservation.manage`, `check.template.manage` and
   `check.incident.manage` → FLEET-007.
2. Then the trip persistence chain TRIP-002 → TRIP-003 → TRIP-004 and the
   F4 trip API tasks in dependency order (TRIP-008/011/012/007, TRIP-010,
   CHECK-003, TRIP-009/013/023, TRIP-014/015/016/018), interleaved with the
   ready OFFL-009, OFFL-012, CHECK-001, FLEET-011 and OFFL-006.
3. Android and panel tasks (OFFL-006 → FLEET-013 → TRIP-005, OFFL-011,
   CHECK-002, TRIP-021, ROUTE-004; TRIP-022, CHECK-004) follow their backend
   owners; native acceptance only on `emulator-5588`.
4. GPS-001..015 and REL-005 stay blocked behind GOV-005/GOV-006: no tracking
   domain, intake or service is built ahead of the legal review and the
   target-device spike that decide what may be collected, and no pure-domain
   or partial checkpoint is counted as M4.

Migrations are pre-allocated per task from `000216`; rule, use-case and
decision IDs are pre-allocated per task (rules from 268, use cases from 136,
decisions from 178). Fake trip, tracking or maintenance adapters are not introduced to
reach a later task: an absent source is an explicit, tested "no source" port
(FLEET-003 `NoOpenWork` precedent) replaced by its owning task.

## Allocation (pre-allocated before behaviour)

| Task | Owner worktree / branch | Rules (B-BR-EMP) | Use cases (BUC-EMP) | Decisions (EDR) | Migration |
|---|---|---|---|---|---|
| F4 execution decision | coordinator | — | — | EDR-196 | — |
| EMP-TRIP-001 | `f4-trip001` / `codex/enterprise-f4-trip001` | 268..275 | 136..138 | 178..181 | none |
| EMP-TRIP-017 | same | 276..279 | 139..141 | 182..183 | none |
| EMP-METER-003 | `f4-meter003` / `codex/enterprise-f4-meter003` | 280..285 | 142..144 | 184..186 | 000216 if needed |
| EMP-METER-004 | same | 286..291 | 145..147 | 187..189 | 000216 if unused by METER-003 |
| Catalog `org-permissions-v5` | `f4-fleet007` / `codex/enterprise-f4-fleet007` | 292..293 | — | 190..191 | none |
| EMP-FLEET-007 | same | 294..301 | 148..151 | 192..195 | 000217 |
| EMP-OFFL-009 | `f4-offl009` / `codex/enterprise-f4-offl009` | 302..307 | 152..154 | 197..199 | 000218 if needed |
| EMP-OFFL-012 | same | 308..311 | 155..156 | 200..201 | 000219 if needed |
| EMP-TRIP-002 | `f4-trip001` / `codex/enterprise-f4-trip001` | 312..319 | 157..160 | 202..206 | 000220 |
| EMP-TRIP-003 | same | 320..324 | 161..163 | 207..209 | 000221 |
| EMP-FLEET-011 | `f4-fleet007` / `codex/enterprise-f4-fleet007` | 325..329 | 164..165 | 210..212 | 000222 |
| EMP-CHECK-001 | `f4-check001` / `codex/enterprise-f4-check001` (reassigned) | 330..335 | 166..168 | 213..215 | 000223 |
| EMP-OFFL-006 | `f4-offl009` / `codex/enterprise-f4-offl009` | 336..341 | 169..171 | 216..218 | 000224 if needed |

Unused pre-allocated IDs stay unused and are reported per task.

## Task ledger

State is the source state at opening; the last column lists the open direct
dependencies.

| Task | Title | Type/size | State | Open dependencies |
|---|---|---|---|---|
| EMP-OFFL-009 | Post-revocation intake window and quarantine | API M | TODO | ready |
| EMP-OFFL-011 | Cleanup of revoked sessions and devices on next contact | APP S | TODO | ready |
| EMP-OFFL-012 | Organization-state and plan-change handling for queued operations | API S | TODO | ready |
| EMP-OFFL-013 | Sync resilience suite: duplicates, reordering, partial uploads, process death | TEST M | TODO | OFFL-005 |
| EMP-FLEET-007 | Reservations, blocks and derived availability | API M | TODO | ready (catalog grant) |
| EMP-FLEET-011 | Implements: attach to and detach from a powered asset | API S | TODO | ready |
| EMP-TRIP-007 | Asset-overlap conflict resolution | API M | TODO | TRIP-002 |
| EMP-TRIP-008 | Missions: create, assign, accept and decline | API M | TODO | TRIP-002 |
| EMP-TRIP-009 | Manager-planned trips: destination, reason, customer or project, cargo or passengers | API M | TODO | TRIP-008 |
| EMP-TRIP-010 | Stops and legs: planned versus actual | DB M | TODO | TRIP-003 |
| EMP-TRIP-011 | Pause, resume and cancellation | DOM S | TODO | TRIP-002 |
| EMP-TRIP-012 | Driver handover during a trip | API M | TODO | TRIP-002 |
| EMP-TRIP-013 | Vehicle change within a mission | DOM S | TODO | TRIP-008 |
| EMP-TRIP-014 | Missing final reading and supervisor closure | API M | TODO | TRIP-004, METER-004 |
| EMP-TRIP-015 | Late and duplicate events, and amendments after closure | API M | TODO | TRIP-004 |
| EMP-TRIP-016 | Period lock and authorised reopening for trips | API S | TODO | TRIP-004 |
| EMP-TRIP-017 | Midnight and time-zone crossing rules | DOM S | TODO | TRIP-001 |
| EMP-TRIP-018 | Open trips when a membership ends | API S | TODO | TRIP-014 |
| EMP-TRIP-019 | Checklist gate at trip start and finish | API S | TODO | TRIP-002, CHECK-002 |
| EMP-TRIP-020 | Trip closing conference: expenses and documents before submission | APP M | TODO | TRIP-005, FUEL-011, EXP-004 |
| EMP-TRIP-021 | App: missions inbox and planned-trip execution | APP M | TODO | TRIP-008, TRIP-009, TRIP-011, TRIP-005 |
| EMP-TRIP-022 | Panel: mission planning and assignment board | WEB M | TODO | TRIP-009, FLEET-007 |
| EMP-TRIP-023 | Reservation-to-trip integration and no-show release | DOM S | TODO | FLEET-007, TRIP-008 |
| EMP-GPS-001 | Tracking session domain: trip-bound lifecycle, permission and quality states | DOM M | TODO | TRIP-001, GOV-005, GOV-006 |
| EMP-GPS-002 | Organization trip tracking service | APP L | TODO | GPS-001, TRIP-005 |
| EMP-GPS-003 | Adaptive sampling, local buffer and batch envelope | APP M | TODO | GPS-002 |
| EMP-GPS-004 | Track intake: batch endpoint, de-duplication and segment storage | DB L | TODO | GPS-001 |
| EMP-GPS-005 | Track processing: filtering, gaps, GPS distance and simplified geometry | JOB M | TODO | GPS-004 |
| EMP-GPS-006 | Distance sources on a trip and the divergence signal | DOM S | TODO | GPS-005, METER-006 |
| EMP-GPS-010 | Tracking interruptions: permission, approximate-only, reboot, process death | APP M | TODO | GPS-002 |
| EMP-GPS-012 | Location access permission and access audit | API S | TODO | GPS-004 |
| EMP-GPS-015 | App: tracking transparency | APP S | TODO | GPS-002 |
| EMP-ROUTE-004 | Navigation phase 1: open the in-app route for a trip destination | APP S | TODO | TRIP-005 |
| EMP-CHECK-001 | Checklist templates per asset category | API S | TODO | ready (catalog grant) |
| EMP-CHECK-002 | Checklist execution offline with the critical-item policy | APP M | TODO | CHECK-001 |
| EMP-CHECK-003 | Damage and incident reports with photos | API M | TODO | TRIP-003 |
| EMP-CHECK-004 | Panel: checklists, damages and incidents | WEB M | TODO | CHECK-002, CHECK-003 |
| EMP-REL-005 | M4 gate: trips, logbook, sync and tracking fundamentals acceptance | TEST M | TODO | OFFL-013, TRIP-015, GPS-006, GPS-010 |

## Execution log

Entries are appended per integrated task (commit, evidence, validation).

### EMP-TRIP-001 — trip domain core: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip001-trip-domain.md), commit `68dc0085` (F2/C1 prerequisite).
Pure Go `trip/domain` and Kotlin `enterprise.trip` share the 207-case
`trip-v1.psv` oracle: the closed seven-state operational catalog, the C1
driver-initiated START → IN_PROGRESS → FINISH → COMPLETED lifecycle, an
eligible start context reported as one ordered `INCOMPLETE` list, finish only
by the starting member, honest meter usage (`MEASURED` only for two
non-decreasing readings on one segment, 290 000 m in the VAL-01 example, else
`NOT_MEASURED` with a reason; a finish is never refused for plausibility) and
review as a closed input that a completion never approves. PAUSE/RESUME/CANCEL
are recognised and refused `UNSUPPORTED_COMMAND` until TRIP-011
(EDR-178..181). RED Go 9/10 top-level and 207/207 vectors, Kotlin 216/217
failing → GREEN Go 10/207 under `-race` (100% statements), Kotlin 217 (99.58%
instructions). Reuses `orgtime`, `fleet.AssetRef` and meter reading refs; no
kernel change. Retained: persistence and intake (TRIP-002), logbook
(TRIP-003), review binding (TRIP-004), Android producer (TRIP-005), pause/
resume/cancel (TRIP-011), planned/assigned states (TRIP-008/009).

### EMP-TRIP-017 — midnight and time-zone crossing: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip017-time-crossing.md), commit `87629a4a`. The trip keeps its
start reporting day; finish carries no zone of its own; elapsed time comes only
from the two instants; an optional device run clock (boot id + monotonic ms)
confirms the elapsed time within a caller-supplied tolerance or flags a gap
without inventing a value (EDR-182/183). New 66-case `trip-time-v1.psv`
(trip-v1 unchanged). RED Go 4 new top-level and 65/66 vectors, Kotlin 3/3 unit
and 65/66 vectors (the passing row is the TRIP-001 finish-before-start guard)
→ GREEN Go 14/66/207 (100%), Kotlin 286 trip tests. Retained: the confirmation
tolerance policy owner, the Android run-clock producer (TRIP-005), period locks
keyed on the start day (TRIP-016).

Integration (`7a071b04`): normal merge, only the EDR table conflicted (union
by number); `go build ./...` and `go test -race ./internal/modules/trip/...`
green on the merged tree; agent gates: untagged backend 215 packages, Kotlin
enterprise suite 2571/0/0, staticcheck/vet/gofmt clean, check_plan 0 problems.

### Catalog amendment `org-permissions-v5`: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-auth-catalog-v5.md), commit `2030aad2` (AUTH prerequisite of
FLEET-007, CHECK-001 and CHECK-003/004). Adds 11 `O` cells with standard
requirements only: `fleet.reservation.manage` takes the `fleet.asset.assign`
roles (OWNER, ADMIN, FLEET_MANAGER, SUPERVISOR), `check.template.manage` the
`maint.workorder.manage` roles (OWNER, ADMIN, FLEET_MANAGER) and
`check.incident.manage` OWNER, ADMIN, FLEET_MANAGER, SUPERVISOR, as
maintenance.md §7 proposed (EDR-190/191). Every v4 cell is preserved, AUDITOR
gains nothing, 18 keys stay unassigned; no migration (assignments store role
and scope only) and no Kotlin consumer. RED 10 top-level / 11 subtests →
GREEN 128/1291 race; PostGIS matrix and auditor walk 4/4; union 16 packages
736/5641, 0 failures.

### EMP-FLEET-007 — reservations, blocks and derived availability: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-fleet007-reservations-availability.md), commit `82573cad`.
Migration 000217: forced-RLS reservations moving only through the guarded
transition rows, CONFIRMED windows excluded from overlapping, confirmation over
a block refused, append-only blocks removed only with a reason, command
receipts, an `AFTER UPDATE OF state` trigger cancelling open reservations when
an asset leaves service (`ASSET_LEFT_SERVICE`), and the asset usage allow-list
extended from the 000215 body. EXPIRED/NO_SHOW are derived at read time and
stored once by the `org-asset-reservation-outcome` job; commands settle due
outcomes first. Explicit `NoActiveUse`/`NoMaintenanceHolds` ports are listed in
the availability read's `missing_sources`; pure `DecideStart` and
`StartDecisionDBTX` wait for the trip-start intake (EDR-192..195). Eight routes
(five commands under fresh `fleet.reservation.manage`, three reads under
`fleet.asset.read`); `RESERVATION_CONFLICT` reveals only the window; requests
through `trip.mission.manage` alone are retained for TRIP-023. RED domain 7,
application 6, cmd/api 3, apicontract 1/12, PostGIS 8/8 (no migration) and 8/8
(stubbed store) → GREEN: 8 PostGIS tests (confirmation races both orders with
barriers, confirm/cancel race, job stopped and run twice, foreign asset, unit
scope denial, unreasoned removal, asset leaving service, RLS/guards, populated
215 → 217 upgrade with injected failure and recovery, HTTP 46 assertions);
matrix 220 cells (85/135); auditor walk 118 routes (27/91); race/integration
union 39 packages 1091/7401, 0 failures, 0 skips; untagged 214 packages.
Retained: trip-start intake wiring and `NoActiveUse` replacement (TRIP-002),
mission linkage/FULFILLED (TRIP-023), maintenance holds, notifications, a
configurable no-show tolerance (60 min default), SCR-FLEET-06 (TRIP-022),
Android picker/feed, production grants.

Integration (`59b04947`): normal merge; contract and EDR table conflicts
resolved by union; `go build`/`go vet`, trip/fleet/orgaccess/cmd/api/apicontract
unit tests green on the merged tree.

### Flaky redaction checks fixed (batch hygiene)

FLEET-007's union reported `fleet/domain`
`TestPrivateValuesRedactEveryFormattingAndSerializationSurface` failing once
and passing on reruns. Reproduced on the merged tree: 14 failures in 3000 runs,
all at the slog check. Cause: the test looks for short numeric private markers
(`8500`, `275000`) in slog JSON output whose record time carries nanoseconds,
which contain those digits by chance. Ten redaction tests in fleet, meter,
fueling, expense, trip, document and budget used a timed handler with numeric
markers; they now drop the record time (the `untimed` option DOC-006 already
used). No marker or expectation changed. After the fix: 3000/3000 runs pass;
the affected packages' untagged tests, vet (both tags) and staticcheck pass.

### EMP-METER-003 (F2 prerequisite) — reading validation: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-meter003-reading-validation.md), commit `2669080c`. Pure
validator in Go (`fleet/domain/meter/validation.go`) and Kotlin
(`MeterValidation`) over the 104-case `validation-v1.psv` oracle: below the
previous / above the next reading, an exact nanosecond rate check on both new
pairs, a future instant beyond 300 s of skew, the `RETROACTIVE` note and
fixed-point re-evaluation of suspect neighbours. Migration 000216 adds the
append-only, forced-RLS reading-state history and replaces the 000201
observation guard with one clause changed so an accepted held reading may be
stored `SUSPECT`. The signed intake, the embedded fueling reading and accepted
held reviews validate; a suspect reading is stored (never refused), ACCEPTED at
revision 1, excluded from latest-valid, and its neighbours are re-evaluated in
the same transaction; rollback switch `ANPFUEL_ENTERPRISE_METER_PLAUSIBILITY`
(EDR-184..186). RED Go 6/7 top-level and 104/104 vectors, Kotlin 108/109,
PostGIS 7/9 → GREEN Go 23/459 (validation 100%), Kotlin 2394 enterprise tests,
PostGIS 9 focused; race union 22 packages 793/5213 (MinIO-dependent tests
rerun green). One METER-002 unit expectation that treated a stored SUSPECT as
a mismatch is obsolete under the new rule and now proves acceptance (recorded
in the evidence). Retained: the Android warning screen (AC5 is proven at the
validator), the review-ACK form for suspect standalone readings, per-category
limits and the organization skew setting, rollover/segments (METER-005).

### EMP-METER-004 (F2 prerequisite) — reading correction and void: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-meter004-reading-correction.md), commit `30bfa7c4`, no
migration (000216 already holds actor, reason and supersession). Three routes:
history with supersession chains (`fleet.asset.read`), suspect list and
SUPERSEDE/VOID/CONFIRM decide (`meter.reading.correct`, fresh proof);
`READING_ALREADY_SUPERSEDED` and `AMENDMENT_REQUIRED` (409, a reading of an
approved fueling is refused until an amendment consumer exists); audit events
`meter.reading.corrected|voided|confirmed` without reading content; the
`meter.usage.recalculated` outbox event only when consumers are configured
(none in production); structural scopes resolve `meter.reading.correct` on the
reading's asset (EDR-187..189). RED unit 1/1, PostGIS 5/5 → GREEN 5 PostGIS
tests (22 HTTP assertions), race union 23 packages 855 top-level. Retained:
amendment consumer for approved parents, recalculation consumers (METER-006),
organization-wide suspect queue and panel (METER-007), production grants.

### EMP-OFFL-009 — post-revocation intake window: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-offl009-post-revocation-window.md), commit `fe59fba4`. A device
whose exact enrollment period was revoked `ACCESS_ENDED` (membership end or a
revocation command) delivers intake-only through the existing signed route.
Evidence comes only from server history (EDR-197): the end instant, the
`intake_window_days` setting in force at it (default 15, elapsed seconds,
inclusive) and the database receipt clock; after the window
`INTAKE_WINDOW_CLOSED`; with a pre-end server claim or a declared instant
before the end, held `ACCESS_ENDED_BEFORE_RECEIPT`; otherwise
`MEMBERSHIP_ENDED`/`DEVICE_REVOKED`. Lost or compromised keys are refused for
the whole batch. Intake grants no reads; only late decisions replay byte for
byte. Late fuelings stay `RETRY`/`UNAVAILABLE` (EDR-177) until FUEL-003.
Migration 000218: append-only, forced-RLS decision table re-derived by the
database. RED pure 45/54 named, PostGIS 12/13 top-level → GREEN 16/73; union
18 packages 808/5582 (storage tests rerun with MinIO).

### EMP-OFFL-012 — organization-state disposition of queued operations: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-offl012-org-state-queued-operations.md), commit `aa74b809`.
Before every owner each operation is checked against ORG-009's suspension
assessment, then the plan rule: declared before a suspension and inside the
grace, or during a suspension since ended, held `ORG_SUSPENDED_GRACE`; at or
after its start, `ORG_SUSPENDED`; with no billing yet the plan is the
capability row (EDR-201): work declared before a capability was turned off is
held `PLAN_CHANGED`, later work keeps `ENTITLEMENT_REQUIRED`. Migration 000219:
append-only state decision table with a SQL copy of the suspension assessment
checked against Go by a generated matrix and shared plan-rule vectors. RED
pure 14/20 named, PostGIS 7/9 → GREEN 62/98 focused; union 21 packages
834/5695 with MinIO, 0 failures. The intake now reads `org_suspension_periods`
before every owner (production grant listed in the backend README).

### Merged-tree verification — `942af96b` (TRIP-001/017, catalog v5, FLEET-007, METER-003/004, OFFL-009/012)

METER merge conflicts were resolved as unions (route counts 121 normal /
22 routed / 62 not routed; auditor walk 121 routes, 28 positive, 93
FORBIDDEN; policy allow-lists; sqlc migration list 216 + 217; OpenAPI error
enum); OFFL merged with only the contract conflicting. 216 and 218 redefine
different meter functions. On the merged head: `go build`, `go vet` (both
tags), toolchain gofmt, `sqlc diff`/`vet` clean; untagged
`go test -count=1 ./...` 215 packages ok
(`f4-logs/coord-untagged-942af96b.log`); race/integration union with PostGIS
and a disposable MinIO (`f4-union-s3`) over organization adapters, fleet,
orgsync, membership, orgaccess, orgcapability, fueling, orgexport, review,
trip, document, orgmedia, enterprisesuite, apicontract, migrate, orgtest,
orgapi, cmd/api and cmd/worker: 50 packages, 49 pass, 1274 top-level / 10038
named, 23 min; the only failures are the 5 known `cmd/worker` tests that need
the pinned Rust preparer CLIs (untouched package; CI builds them)
(`f4-logs/coord-union-942af96b.jsonl`). The OFFL evidence's proposed shared
text is applied (10-offline-sync §10.2, backend README grants, BR-IDN-029 and
BR-ORG-031 references, ep04 states).

### EMP-TRIP-002 (F2/C1 prerequisite) — trip persistence and signed intake: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip002-trip-persistence.md), commits `f12558a2` (task) and
`ad60b1c8` (wiring into FLEET-007 availability, after a normal merge of the
F4 head). Migration 000220: forced-RLS trips, insert-only completions (the
IN_PROGRESS → COMPLETED move is the only update a trip row receives), one
active trip per asset enforced by a unique index, held starts/finishes in the
OFFL-008 review mechanism (`org_sync_review_source_valid` gains two trip
branches and keeps the meter one), asset/unit/cost-centre usage sources
redefined from their latest bodies. Signed `trip.start`/`trip.finish` with the
reading recorded in the same transaction; a later admissible start is held
`ASSET_BUSY`/`ASSET_UNAVAILABLE`/`PERMISSION_CHANGED`, a second finish
`TRIP_ALREADY_CLOSED`; finish of an unknown trip `RETRY`/`DEPENDENCY_PENDING`,
finish before start `MALFORMED`, another member's finish `FORBIDDEN`;
`TripAuthority` on the shared history classifier (`trip.trip.start`); a stored
trip is `DRAFT` with no review entry (EDR-202..206). Trips replace FLEET-003's
`NoOpenWork` and FLEET-007's `NoActiveUse`; the start intake calls
`StartDecisionDBTX`. The agent drafted handlers before the integration tests
(recorded in the evidence); RED was run with the frozen tests and migration
against stubbed handler bodies: 9/11 top-level failing → GREEN 13/21 under
race on PostGIS, including concurrent starts in both orders with two devices
and the loser observed waiting on the lock in `pg_stat_activity`. Retained:
resolving and listing held trip items (TRIP-007), finish with changed rights
(stores nothing, RETRY), submission (TRIP-004), Android producer (TRIP-005),
panel reads (TRIP-006), maintenance holds.

### EMP-FLEET-011 — implement attachments: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-fleet011-implement-attachments.md), commit `c76007af`.
Migration 000222: forced-RLS attachment periods with composite FKs to both
assets and a gist exclusion per implement, row guard (IMPLEMENT onto
VEHICLE/MACHINE, ACTIVE/INACTIVE, no retroactive start, detach-only updates, no
deletes), command receipts; attachments count as operations of both assets.
Attach/detach under fresh `fleet.asset.manage` on both assets (EDR-211, UC-FLEET-10
names it), list under `fleet.asset.read`; ports `AttachedImplementsDBTX` /
`PoweredAssetDBTX`; availability makes an attached implement IN_USE through its
powered asset's trips; pure `MainAssetAllowed` (EDR-210..212). RED domain 3,
application 3, router 3, contract, PostGIS 5/5 twice → GREEN 5 PostGIS tests
(race both orders, guards/RLS, populated 217 → 222 upgrade with recovery, HTTP
33 assertions); union 39 packages 1107/7403. Retained (in progress in the
wiring commit of `f4-trip003`): `MainAssetAllowed` at trip start and AC2/AC3
with real trips.

### EMP-OFFL-006 (F2/C1 prerequisite) — reference data feed: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-offl006-reference-feed.md), commit `f3485407`. Signed device
route `POST /v1/orgs/{org_id}/sync/reference` (boundary
`SIGNED_REFERENCE_FEED`, same proof model as the OFFL-008 review results,
EDR-217) returning, for the selected organization only, the member's currently
assigned assets covered by current `fleet.asset.read` as minimised `asset`
entities (identity, revision, kind, usage basis, state, name, fleet number,
plate, accepted products, in-service meters with their open segment, the
covering assignment's kind and end; no serial, unit, cost centre, value, other
drivers, readings or reservations — EDR-216, BR-PRIV-003). Migration 000224:
per-membership append-only change log of digests built on each pull, so
revisions only grow and tombstones reach every older cursor (EDR-218); HMAC
cursors bound to organization, membership, enrollment, epoch and revision; a
foreign or tampered cursor is 404, an unknown epoch or a future revision
`RESET`, a lost grant `RESET FORBIDDEN` with no entities; `confirmed_at` is the
database clock. RED pure 3/3, PostGIS 5/5 → GREEN; 12 concurrent pulls produce
one revision; populated 219 → 224 upgrade with recovery; union 19 packages
906/6030. Retained: Kotlin client and shared wire vectors (FLEET-013),
checklist templates and availability as entity types, paging beyond 200,
read budget, log retention, production grants.

### Merged-tree verification — `6a843325` (adds TRIP-002, FLEET-011, OFFL-006)

Merge fixes made by the coordinator: 000222 redefined `org_asset_current_usage`
from the 000217 body and would have dropped TRIP-002's two allow-list lines and
count terms (000220 runs first) — 000222 now carries the 000220 body plus the
attachment lines; the normal route count and the auditor walk were changed to
121 by both branches and auto-merged, but the union is 124 normal routes and
124 / 29 positive / 95 FORBIDDEN; the `orgtest` grant helper and a duplicated
`jobs` import in `enterprisesuite/sync.go` were merged structurally; the
FLEET-011 HTTP assertion that availability lacks a trip source is obsolete
once TRIP-002 supplies trips and now expects only `MAINTENANCE_HOLD` missing
(no other expectation changed). On `6a843325`: build, vet (both tags),
toolchain gofmt, `sqlc diff`, whole-backend staticcheck clean; untagged
`go test ./...` 217 packages ok; race/integration union with PostGIS and the
disposable MinIO over 20 package roots: 53 packages, 52 pass, 1336 top-level /
10152 named, 25.7 min (`organization/adapters` 1393 s); the only failures are
the 5 known Rust-preparer-gated `cmd/worker` tests
(`f4-logs/coord-union-6a843325.jsonl`).

### EMP-CHECK-001 — checklist templates per asset category: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-check001-checklist-templates.md), commit `256fa3ea`
(reassigned from the FLEET-011 agent, whose context ran out before starting
it). New `maintenance` module: pure `domain/checklist` (template definition,
critical-item policy, answer/outcome rule, applicable-template resolution and
policy-in-force lookup) mirrored in Kotlin
`com.anpfuel.domain.enterprise.maintenance.checklist` over the 127-case
`checklist/checklist-v1.psv` oracle (error codes `INVALID`,
`ANSWER_NOT_ALLOWED`, `PHOTO_REQUIRED`, `INCOMPLETE`). Migration 000223: six
forced-RLS tables — a guarded template head (fixed context; publish, policy
change and archive each bump the single revision under the organization root
lock), insert-only versions, items, targets, policy revisions and receipts; a
deferred check closes each committed version and refuses a target already
named by another ACTIVE template of the same context; only ACTIVE/INACTIVE
assets can be targets. Seven routes: create, publish, policy and archive under
fresh `check.template.manage` (ORG coverage only), template list/read under
`check.template.manage`, and `checklists/assets/{asset_id}/applicable` under
`fleet.asset.read` resolving asset → category → organization default and
returning only the version and policy revision a run pins (EDR-213..215; the
applicable read is a positive AUDITOR route because AUDITOR holds
`fleet.asset.read`). RED Go 6/7 top-level and 127/127 vectors, Kotlin 132/133,
PostGIS 6/6 twice, router 3, contract 18 subtests → GREEN Go 8/127 (100%),
Kotlin 133/133, PostGIS 6/6, contract 2/46; matrix 230 cells (88/142); union
16 packages 794/5785, 0 failures. Retained: checklist runs and their server
recheck (CHECK-002), the trip gate (TRIP-019), the reference-feed extension
(OFFL-006 entity type), template events, policy-history/version reads, panel
(CHECK-004), production grants.

Integration (`08b3e37c`): 000223 redefined `org_asset_current_usage` from the
000217 body and runs after 000220/000222, so it would have dropped the trip,
held-start and attachment lines; its body is now the 000222 body plus the
checklist target line. Route counts merged to 131 normal (23 routed / 61 not
routed, 25 special) and the auditor walk to 131 / 30 positive / 101 FORBIDDEN;
both matrix fixtures keep the attachment store and add the checklist grants;
the quick manifest lists both trip and maintenance packages. Build, vet, gofmt,
`sqlc diff`, `cmd/api`, apicontract and maintenance unit tests green; Kotlin
enterprise domain suite on the merged tree 2813 tests, 0 failures
(`f4-logs/coord-kotlin-08b3e37c.log`).
Merged-tree union on `08b3e37c` (adds `maintenance`): 57 packages, 56 pass,
1361 top-level / 10504 named, 40 min (`organization/adapters` 2212 s under
load); the only failures are the 5 known Rust-preparer-gated `cmd/worker`
tests (`f4-logs/coord-union-08b3e37c.jsonl`); untagged `go test ./...` and
whole-backend staticcheck clean (`coord-untagged-08b3e37c.log`,
`coord-staticcheck-08b3e37c.log`).

### Implement × trip wiring (EMP-FLEET-011 × EMP-TRIP-002): LOCAL_DONE / INTEGRATION_PENDING

Commit `6d62c502` (EDR-219, no migration). The start intake reads the asset
kind and applies `fleet.MainAssetAllowed` next to the DRAFT check, before any
held condition: an IMPLEMENT start is a final `MALFORMED` with nothing stored
(FLEET-011 AC4). `trippg.StartImplements.ImplementsAtStartDBTX` lists the
implements attached at the trip's start through `AttachedImplementsDBTX`,
derived at read time because attachment periods never change toward the past
(AC2). AC3 is proven with real signed trips: an attached implement reads
IN_USE with `powered_asset_id` while the powered asset's trip runs and
AVAILABLE after `trip.finish`. RED 2/2 (implement start ACCEPTED, listing
empty; the AC3 assertions already passed against the composed ports) → GREEN
21/21 trip and attachment PostGIS tests; union 19 packages 889/6296. Retained:
an optional database guard against an implement trip and a consumer of the
start-implements listing.

### EMP-TRIP-003 (F2/C1 prerequisite) — logbook events: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip003-logbook-events.md), commit `b80de813`, migration 000225
(000221 stays unused because 000222/000224 already existed). Signed
`trip.event.record` schema 1 (entity `trip_event`, exact payload `trip_id`,
`type`, `text`): supported NOTE (text required), STOP, ARRIVAL, DEPARTURE;
reserved FUELING, TOLL, PARKING, INCIDENT, DAMAGE, HANDOVER, PAUSE, RESUME,
CANCEL answered RETRY/UNAVAILABLE with no effect until their owners; anything
else MALFORMED. An event before the start is MALFORMED; on a completed trip at
or before the finish it is a late delivery and accepted; after the finish it
is held `TRIP_ALREADY_CLOSED` (the OFFL-008 review hook extended from the
000220 body); another member's event FORBIDDEN; another organization's trip
RETRY `DEPENDENCY_PENDING` like `trip.finish` (forced RLS cannot tell it from
an undelivered trip, EDR-202) (EDR-207..209). Insert-only forced-RLS
`org_trip_events`/`org_trip_held_events` keyed only to trip, operation, author
and enrollment (no usage-source change); no read route (the timeline is the
store read; panel reads are TRIP-006). New 103-case `trip/logbook-v1.psv`
consumed by Go and Kotlin `TripLogbook.kt`. RED Go 101/103 vectors, Kotlin
101, application 3/8, contract 2, PostGIS 6/6 → GREEN domain 16/376 (99.7%),
application 8/27, PostGIS 6/16; Kotlin enterprise 2918, 0 failures; merged
union 910/6615 with one OFFL-006 flake (below). Retained: panel timeline
(TRIP-006), Android producer (TRIP-005), resolution of held events and late
events after approval (TRIP-007/015), reserved types for their owners,
corrections and photos, events with changed rights (RETRY), production grants.

Integration (`20bc047e`): the branch had already merged `08b3e37c`; the merge
into the F4 head was clean (EDR rows re-sorted).

### Reference-cursor flake fixed (EMP-OFFL-006)

The TRIP-003 union saw `TestReferenceFeedSignedRouteRefusesForeignCursors`
fail once: it tampers the last character of an unpadded base64url segment,
and the lenient decoder ignored the two unused bits, so about one run in 16
the tampered cursor opened. Cursor segments are now decoded strictly (only the
canonical spelling opens). RED unit test (a non-canonical MAC spelling opened)
→ GREEN; the route test passed 12/12 repeated race runs; orgsync unit tests
and staticcheck clean.

### EMP-TRIP-004 (F2/C1 prerequisite) — trip submission and review: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip004-trip-review.md), commit `95d5555e`, migration 000226
(append-only, forced-RLS `org_trip_review_entries`). The trip's author sends a
signed `trip.submit` (entity `trip_review`, `expected_revision` = review
revision, payload exactly `{}`, at most the finish as dependency) so the
Android producer can queue it offline behind its finish; only a COMPLETED trip
whose case is DRAFT at that revision is accepted, appending the APPR-001
SUBMIT entry with the ACK, effect and audit; an in-progress or undelivered
trip is `DEPENDENCY_PENDING`, another member `FORBIDDEN`; a case no longer a
draft, or a submission whose rights changed, stores nothing and stays
RETRY/UNAVAILABLE until TRIP-015. Review routes under `trip.trip.review`:
organization queue (ORG grants only; matrix representative), minimal decision
read, and decide with fresh proof and expected revision (OPEN plus
APPROVE/RETURN/REJECT in one transaction, one audit event). A new
`TripStructure` resolves UNIT/COST_CENTRE (stored at start), TEAM (author or
driver at the start receipt) and ASSET grants. RETURNED records the reason
only; the correction loop is TRIP-015 (EDR-220..223). Database guards cover
transitions, reason policy, COMPLETED trip, submitting author and reviewer
separation from author and driver. The agent drafted the implementation before
the tests (recorded in the evidence); RED against stubbed bodies: unit 5/10
new top-level, PostGIS 8/8 and 27 subtests → GREEN unit 33/310; self-approval,
stale revision race both orders, missing reason 422, replay/second decision,
foreign/unknown trip NOT_FOUND, uncovered UNIT/COST_CENTRE/ASSET FORBIDDEN,
AUDITOR/FINANCE/DRIVER FORBIDDEN, populated 224 → 226 upgrade with recovery;
union 33 packages 1054/9336. Retained: resubmission and late/duplicate
submissions, void and amendments (TRIP-015), Android producer and the author's
own result read (TRIP-005), scoped lists and panel (TRIP-006), period lock on
decisions (TRIP-016), an author cancel, a TEAM-scope PostGIS fixture,
production grants.

Integration: merged in a separate integration worktree while the `75c18421`
union ran. OpenAPI owner and schema blocks of TRIP-003 and TRIP-004 were
interleaved by the merge and restored structurally (both owners and all
schemas kept; YAML parses); the operation-owner count both branches set to 5
is 6 on the union (meter, fueling, trip start/finish/event/submit); both
intake handlers are composed; sqlc lists 000225 and 000226. Counts take the
TRIP-004 side: 134 normal routes (24 routed / 60 not routed / 25 special),
auditor walk 134 / 30 / 104, matrix 240 cells (93/147). Build, vet, gofmt,
`sqlc diff`, cmd/api, apicontract and trip unit tests green; focused PostGIS
`Trip|Auditor|AuthorizationMatrix` race tests green.

Merged-tree verification of `75c18421` (wiring, TRIP-003, cursor fix):
untagged `go test ./...` 221 packages ok; race/integration union 57 packages,
56 pass, 1378 top-level / 10642 named, 27.7 min; only the 5 known
Rust-preparer-gated `cmd/worker` tests fail (`f4-logs/coord-union-75c18421.jsonl`).

Merged-tree verification of `cbf68c67` (adds TRIP-004): untagged
`go test ./...` 222 packages ok; race/integration union 58 packages, 57 pass,
1396 top-level / 10823 named, 26.8 min; only the 5 known Rust-preparer-gated
`cmd/worker` tests fail (`f4-logs/coord-union-cbf68c67.jsonl`).

### EMP-FLEET-013 (F2/C1 prerequisite) — app: my assets and asset selection: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-fleet013-my-assets.md), commit `c2e857dc`, no backend
migration, route or OpenAPI change. Kotlin client of the OFFL-006 signed
reference feed: strict request/response parsing, versioned cache format,
delta/tombstone/RESET/FORBIDDEN application, 24 h confirmation and the picker
choice (`domain/.../enterprise/fleet/ReferenceFeed.kt`, a separate 2 MiB
canonical-JSON parse limit); offline read and compare-and-set refresh with one
cursorless retry after 404 (`application/.../MyAssets.kt`); the cache is one
versioned entry sealed in the PRIV-003 protected store, not Room tables —
EDR-076 rejected a second Room store and PRIV-003 forbids plaintext (EDR-224);
signed `EnterpriseReferenceHttpClient`; "Meus ativos" list, read-only detail
and a reusable picker that returns an asset to a calling screen with no
network (unconfirmed or not-available assets need a warning), 53 strings in
eight locales, rollback flag `enterpriseFleetUiEnabled` (EDR-224..227). New
129-row `sync/reference-feed-v1.psv` from an independent oracle, consumed by
Kotlin and by a Go test running the real server parser/encoder. RED domain 7/7
and 129/129, application 12/12, data 11/11, ViewModel 4/4 → GREEN; host
suites domain 2950, application 108, data 176, app 19; lint 0 errors. Native
acceptance on `emulator-5588` only: presentation 6/6, context 2/2 (AC1, AC2
offline, AC4 organization switch, AC5 personal vehicles unchanged),
regressions, and a real-backend three-process run (online pull of two
organizations, force-stop, server-side assignment end, offline cache intact
with the asset marked not confirmed and the picker working, reconnect applying
both tombstones). Incidents recorded in the evidence: an early native run
triggered a real `SESSION_REVOKED` erasure of the synthetic validation account
on the local test API (restored through the product's acknowledged empty
recovery; the test now aligns the session first), and one unreproduced
intermittent failure of `EnterpriseSyncContextDeviceTest` (8 later runs
passed). Retained: last readings and tank capacity (not in the feed),
availability as an entity type, starting operations from the picker
(TRIP-005, FUEL-011, CHECK-002), a background pull, native two-account and
24 h boundary runs, the intermittent, visual/TalkBack review.

Integration (`b96e598d`): only the contract and EDR table conflicted; the
proposed shared text of OFFL-006 and FLEET-013 is applied (10-offline-sync
§10.3, backend and data READMEs, ep04 OFFL-006 state). Merged tree: Go build,
vet, gofmt and orgsync tests green; Android host suites domain 3055,
application 644, data 176, app enterprise 19 tests, 0 failures, and
`:app:assembleDebug` pass (`f4-logs/coord-android-merged.log`).

### EMP-TRIP-008 — missions: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip008-missions.md), commit `f8027a4e`, migration 000227.
Pure mission rules in Go (`trip/domain/mission.go`) and Kotlin
(`TripMission.kt`) over the 238-case `trip/mission-v1.psv` oracle: the
8 states × 7 commands table, create as DRAFT, restore, assign, respond and
cancel; a stale revision is CONFLICT and only the current assignee responds.
Migration 000227: forced-RLS mission row that changes only together with the
history entry for its new revision, append-only history and command receipts;
triggers re-check the transition table, assignee eligibility and that a
response comes from the exact signed operation of the current assignee; the
asset (from 000223) and unit (from 000220) usage functions each keep every
line and add one mission key. Separate sqlc package `db/queries/mission`
(EDR-230). Six routes under `/missions/`: create and list (`trip.mission.manage`
ORG), read/assign/cancel (`trip.mission.manage` covering the stored unit and
asset through a new `MissionStructure`), and `mine/list` (`trip.trip.start`
on the member's own membership); commands need fresh proof, revision CAS and
one audit event; naming a vehicle returns the FLEET-007 start decision as a
warning only. The assignee answers through the signed offline operation
`trip.mission.respond` decided on facts current at receipt: another member or
an ended assignment FORBIDDEN, another organization's mission NOT_FOUND, stale
or already answered CONFLICT, all transport denials storing nothing (the ACK
vocabulary has no CONFLICT rejection); no `mission_id` on `trip.start` yet
(EDR-228..231). RED Go 1/18 top-level and 236/238 vectors, Kotlin 236/238,
router 4, contract 1, PostGIS 9/9 twice → GREEN 238/238 (100%), Kotlin 3157,
PostGIS 9/9; union 24 packages 1041/8597, 0 failures. A lock-order hazard
(response locked the mission before the organization root) was fixed during
GREEN. Counts: 140 normal routes (26 routed / 58 not routed / 25 special),
auditor walk 140 / 30 / 110, matrix 260 cells (102/158), 7 signed owners.
Retained: trip link and reservation linkage (TRIP-021/023), planning
(TRIP-009), vehicle change (TRIP-013), app inbox and mission feed entity
(TRIP-021), board and panel/auditor reads (TRIP-022/006), notifications, a
held form for late responses, production grants.

Incident reported by the TRIP-008 agent: while stopping its own hung RED test
it killed the TRIP-016 agent's `organization/adapters` race union (after
1097 s); that result is void and the TRIP-016 agent was told to rerun it.

Integration (`022b66c1`): only the contract, EDR table and backend README
conflicted (appended sections). Build, vet, gofmt, `sqlc diff`, cmd/api,
apicontract, trip and orgsync unit tests green; Kotlin enterprise domain suite
3294 tests, 0 failures (`f4-logs/coord-kotlin-trip008.log`).

Merged-tree verification of `3345865b` (FLEET-013, TRIP-008): untagged
`go test ./...` 222 packages ok; race/integration union 58 packages, 56 pass,
1413 top-level / 11554 named, 50 min under load. Failures: the 5 known
Rust-preparer-gated `cmd/worker` tests and one run of the pre-existing
`orgmedia` flake `TestPrivateS3OriginalAndMemberBoundAuditedAccess`
("private upload failed orgmedia: unavailable"); that package passed 3/3 on an
immediate `-count=3` race rerun. Its root cause (a conditional PUT on a
keep-alive connection the provider had just closed) is fixed on `dev` by
`82a5f97e` (PR #222), which F4 will merge before delivery
(`f4-logs/coord-union-3345865b.jsonl`).

### EMP-TRIP-016 — period lock for trips: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip016-trip-period-lock.md), commit `6dd09490`, migration
000228 (replaces the 000226 review-entry guard; no table, no usage change). A
trip review decision reads the APPR-006 period of the trip's stored START
reporting day (never device, UTC, finish or receipt day) inside the decision
transaction: OPEN or REOPENED admits, LOCKED refuses `PERIOD_LOCKED` (409) and
writes nothing; replays, reads, the queue and field intake are not gated; late
field facts in a locked month are accepted as facts and their decision waits
for an authorised reopening; no trip adjustment form (EDR-232/233). RED unit 1,
PostGIS 5/5 then 2/5 (the real database gap) → GREEN 12/12: boundary trips
stored on 31 October while their UTC/device/receipt days are November, a stale
reopening withdrawn by a relock, lock against decision in both orders with
lock-wait barriers, direct insert and in-place rewrite refused by the
database, composed HTTP 409, populated 226 → 228 upgrade with recovery. The
agent's first `organization/adapters` union was killed by another agent and is
void; the package was rerun alone (507 / 4139, 0 failures).

### EMP-TRIP-015 — late and duplicate events, amendments after closure: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip015-late-events-amendments.md), commit `9bdd7a45`,
migration 000229. Resubmission: `trip.submit` naming the returned field
revision appends `RESUBMIT` at revision + 1; logbook events are edits only
while the trip is DRAFT or a returned field revision. Late and duplicate facts
— an event inside a submitted, decided or amended trip, or a submission naming
a revision that is no longer the current draft/returned one — are held once
as `STALE_REVISION` (insert-only `org_trip_held_submissions`; the database
refuses a direct logbook fact on such a revision), never applied. Amendments:
`trips/{trip_id}/amendments/propose` under `meter.reading.correct` corrects the
start and/or end reading with a reason; the `orgamend` kernel decides AMEND or
EDIT of a returned amendment, period-gated; review uses the existing decide
route with the amendment's author excluded; approval resolves and supersedes
exactly the corrected readings through a new fleet port
`MeterStore.SupersedeApprovedTx` in the same transaction; the decision context
shows the effective revision and usage per revision — 290 000 m until approval
and 280 000 m after (VAL-26, trips.md §10). METER-004 now treats approved trips
as parents (`AMENDMENT_REQUIRED`); before, it could change an approved trip's
distance in place. Server-only (EDR-234..237). The agent drafted bodies before
the tests (recorded); RED against stubs unit 7, PostGIS 7/7 and 4/4 → GREEN
18/18 after fixing an AMEND kind-parse bug and fixture grants (no expectation
changed); TRIP-004/TRIP-003 tests updated where the new behaviour supersedes
them (resubmit/hold assertions, upgrade test applies the remaining chain).
Merged-tree union on `3a3eca09` (= `3345865b` + both tasks): 29 packages,
1025 top-level / 8242 named, 0 failures; matrix 260 cells (102/158); auditor
walk 141 / 30 / 111; untagged 222 packages; 141 normal routes. Retained:
resolving held `STALE_REVISION` items (TRIP-007), changed-rights events and
submissions (RETRY), adding missing readings, instant/event corrections, void,
METER-004 still correcting readings of submitted-but-unapproved trips
directly, recalculation consumers (METER-006), Android (TRIP-005), panel
(TRIP-006), the period state in the decision context.

Integration (`3a5497e9`): clean merge (the branch already contained
`3345865b`); build, vet, gofmt, `sqlc diff` green; the trip review, period and
amendment grants are added to the backend README. TRIP-002's production
grants are not yet written in the README (retained).

### Sync with dev/main after PR #222 — 2026-11-15

PR #221 merged at `b9d64281` and PR #222 (storage replay fix `82a5f97e`) at
`a0b72533`; `origin/dev` = `82a5f97e`, and the trees of `origin/main` and
`origin/dev` are identical. Both were merged into the F4 batch with normal
merges (`509c22a9`, `25e8b6d6`), no conflicts; build, vet and gofmt green.
The storage fix addresses the `orgmedia` flake seen in the `3345865b` union.

### EMP-CHECK-003 — damage and incident reports with photos: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-check003-incident-reports.md), commit `680505b3`, migration
000230 (redefines the asset and unit usage functions from their 000227 bodies
and the cost-centre one from 000220, keeping every line). Pure
`maintenance/domain/incident` and Kotlin `enterprise/maintenance/incident`
over the 98-case `checklist/incident-v1.psv`; `document-v1.psv` extended to
252 cases for the new link target. Reports are recorded offline through the
signed intake and are accepted independently of their photo upload, whose
storage state stays the document's own; photos link through the DOC-002/003
chain (synthetic data only, real photos legally gated by OQ-04); a report never
books a cost; inside a trip it appears on the timeline through a read join;
handling (acknowledge, resolve with a note, dismiss with a reason) under
`check.incident.manage` with revision CAS and audit (EDR-238..241). RED Go
4/6 top-level and 98/98 vectors, Kotlin 101/103, cmd/api 3, contract 3/3,
trip codec 1, PostGIS 10/10 schema and 10/10 behaviour → GREEN; union 33
packages 1119/8681, 0 failures; untagged 223 packages; storage/document/
orgmedia rerun after the #222 merge 9 packages 124/622. Counts: 145 normal
routes (28 routed / 56 not routed), auditor walk 145 / 30 / 115, matrix 280
cells (112/168), 8 signed operation owners. Retained: `blocks_use` blocking
and release (CHECK-002/004), held reports and late intake, device attachments
and the signed link path (OFFL-005/DOC-004), unlink/retention (DOC-011/012),
work orders from reports (MAINT-003), events/notifications, Android screens,
panel timeline (TRIP-006), team-scope facts, production grants; a report on a
submitted/decided/amended trip is accepted and shown on the timeline but never
joins a TRIP-015 review revision.

Integration (`cb3a23d6`): the branch already contained the F4 head
`25e8b6d6`, so the merge was conflict-free; build, vet (both tags), gofmt,
`sqlc diff` and check_plan green.

### EMP-TRIP-005 (F2/C1 prerequisite) — app: start a trip, log events and finish offline: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip005-app-trip.md), commit `c14b8367`, no migration.
Kotlin builders and a strict decoder for `trip.start`, `trip.event.record`,
`trip.finish` and `trip.submit` (`TripOperations.kt`), the versioned
`trip-book` v1 local index and its rules (`TripBook.kt`, the device refuses a
second unfinished trip on the same asset in the same organization), and a
presentation in which each record's sync state comes only from its own
receipt (`TripPresentation.kt`); application producers, `EnterpriseTripService`
and `TripFlow`; `ProtectedTripStore` writes one sealed RECORD per partition in
the same protected write as each operation; the four trip producers are the
first business producers in the production queue registry; `TripViewModel`,
`TripPanel` and `TripsHomeCard` (start through the FLEET-013 picker, event
grid, finish, ready to send, send now, one status chip and reason per record),
rollback flag `ENTERPRISE_TRIP_UI_ENABLED`, 75 strings in eight locales
(EDR-242..245). New 54-row `trip/trip-operations-v1.psv` from an independent
oracle: the Kotlin builders must produce the exact bytes and the Go parsers
must accept or refuse each row identically. Two backend changes (EDR-244):
`/v1/org-access/contexts/read` now returns the requesting membership's own
`employee_id` (needed by `trip.start`; no SQL, route or permission change),
and a defect found by the native run is fixed — the trip intake accepted only
VALID embedded readings, so a start or finish whose reading METER-003 stored
SUSPECT stayed RETRY/UNAVAILABLE forever (PostGIS RED → GREEN). RED domain
62/63, application 12/12, data 6/14, ViewModel 4/4, Go 2 → GREEN; host suites
domain 3118, application 120, data 183, app 23; union 20 packages 959/8082.
Native acceptance on `emulator-5588` only, against a real local backend in
five processes with force-stop between stages: offline start and STOP,
process death, NOTE, finish and ready to send; organization B sees nothing;
on reconnect each record reaches "Aceito pela empresa" only with its own
receipt and a second send sends nothing; a second member's earlier start on
the same vehicle makes the device's start `ACCEPTED_FOR_REVIEW ASSET_BUSY`,
shown as "Precisa de revisão"; server counts exact (1 completed trip, 1
finish, 2 events, 1 submission, 1 held start, 6 operations and receipts).
Retained/OWED: the author's own review-result read (no server read exists),
held-start resolution (TRIP-007), offline organization re-selection after
process death through the context screen, the current-activity card and
driving layout, event corrections/photos, two physical devices (OWED),
native other-account and 24 h checks of the trip book, departure-reading
plausibility warning, retention of the trip book, the FLEET-013 intermittent,
visual/TalkBack review.

Integration (`4c346a13`): contract, EDR table and trip testdata README
conflicted (appended sections); build and vet with the integration and
`nativefixture` tags, gofmt, `sqlc diff` green; cmd/api, apicontract, trip and
organization unit tests and the focused `Trip|Context|Incident` PostGIS race
tests green; Android host suites domain 3461, application 656, data 183, app
23 tests, 0 failures, and `:app:assembleDebug`
(`f4-logs/coord-android-trip005-merged.log`); the proposed 10-offline-sync
§10.3 and data README text is applied.

## Batch checkpoint — 2026-11-15

Status of this construction batch (all **LOCAL_DONE / INTEGRATION_PENDING**;
none is INTEGRATED, RELEASE_CERTIFIED or an M4 acceptance):

- F2 / cut C1 prerequisites closed for F4 (9): EMP-TRIP-001, EMP-TRIP-002,
  EMP-TRIP-003, EMP-TRIP-004, EMP-TRIP-005, EMP-METER-003, EMP-METER-004,
  EMP-OFFL-006, EMP-FLEET-013.
- F4 tasks done here (10 of 38): EMP-TRIP-017, EMP-FLEET-007, EMP-FLEET-011,
  EMP-OFFL-009, EMP-OFFL-012, EMP-CHECK-001, EMP-CHECK-003, EMP-TRIP-008,
  EMP-TRIP-015, EMP-TRIP-016; plus the `org-permissions-v5` catalog amendment
  and the implement × trip-start wiring (EDR-219).
- Batch fixes: untimed slog redaction checks, canonical reference cursors,
  SUSPECT embedded trip readings accepted, readings of approved trips routed
  through reviewed amendments.

Not done and retained (28 F4 tasks):

- Blocked by governance: EMP-GPS-001, 002, 003, 004, 005, 006, 010, 012, 015
  and the M4 gate EMP-REL-005 — EMP-GOV-005 (legal review with human
  reviewers) and EMP-GOV-006 (tracking spike on target devices; the physical
  phone is excluded and an emulator is not a target device).
- In progress on `codex/enterprise-f4-trip009` (not in this delivery):
  EMP-TRIP-009, EMP-TRIP-013.
- Open: EMP-TRIP-007, 010, 011, 012, 014, 018, 019, 020 (needs FUEL-011 and
  EXP-004), 021, 022, 023; EMP-OFFL-011, EMP-OFFL-013 (needs OFFL-005);
  EMP-ROUTE-004; EMP-CHECK-002, EMP-CHECK-004.
- Per-task retained obligations (production grants including TRIP-002's,
  held-item resolution, panel screens, the author's own trip review-result
  read, OWED two-physical-device and visual/TalkBack checks) are in each
  task's evidence and log entry above.

Final local gates on `bff5a788` (the delivered code; later commits are
record-only): `scripts/check-backend-fast.sh` exit 0 (gofmt, build/vet/unit
tests, whole-backend staticcheck, sqlc vet/diff, OpenAPI lint 0 errors,
tracked and changed-file secret scans) and `make quick-verify` exit 0
(govulncheck "No vulnerabilities found", mobile static guards including no
background location tracking in shipped sources), logs
`f4-logs/coord-check-backend-fast-bff5a788.log` and
`coord-quick-verify-bff5a788.log`. Merged-tree race/integration unions on
each integration head are recorded above (latest full coordinator union on
`3345865b`; the TRIP-015/016, CHECK-003 and TRIP-005 agents ran their unions
on merged trees including every earlier head); the only failures anywhere are
the 5 `cmd/worker` tests needing pinned Rust preparer CLIs (CI builds them)
and the `orgmedia` S3 flake fixed by PR #222. Android host suites on the final
code: domain 3461, application 656, data 183, app enterprise 23 tests, 0
failures; native acceptance only on `emulator-5588`.

## Integration — PR #223 merged (2026-11-15)

[PR #223](https://github.com/AlexandreZanata/Abastevo/pull/223) (`dev` →
`main`, head `f17f57ec`) was merged by the maintainer at 16:31Z as
`93f3a88d`, whose tree is identical to the delivered head. At the last read
before the merge the PR checks `fast`, `test` and `Quick verification`
passed and `integration` was in progress; `release-staging` and `Android
release` then completed successfully on `main` at `93f3a88d`. The final local
race/integration union on `f17f57ec` (71 packages, 70 pass, 1637 top-level /
12618 named; only the 5 Rust-preparer-gated `cmd/worker` tests fail) is
recorded here because no commit could be pushed to `dev` while the PR ran
(`f4-logs/coord-union-final-f17f57ec.jsonl`).

The 19 tasks of the batch checkpoint (9 F2/C1 prerequisites, 10 F4 tasks) and
the catalog amendment are therefore **INTEGRATED** at `93f3a88d`; none is
RELEASE_CERTIFIED, no deployment or production grant is part of the merge, and
M4/F4 remain open with the retained obligations listed in the checkpoint.

## Next batch on `codex/enterprise-f4` (after `93f3a88d`)

### EMP-TRIP-009 — manager-planned missions: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip009-planned-trips.md), commit `b854e4ad`, migration 000231
(rebuilds the unit and cost-centre usage functions from the 000230 bodies).
`PLAN` moves a DRAFT, PLANNED, ASSIGNED or DECLINED mission to PLANNED and
clears the assignee and answer (refused on ACCEPTED, IN_PROGRESS and terminal
missions); a plan holds a destination (free text or an ACTIVE unit), a reason,
an optional ACTIVE cost centre as the customer/project reference and a load
(NONE, CARGO with a description, PASSENGERS with only a count 1..999; free
texts refuse CPF-shaped numbers; no passenger identities); plans are
insert-only rows in `org_trip_mission_plans` and never touch trips, readings or
reviews; new `mission-v2.psv` (317 cases, Go and Kotlin hash-checked) replaces
`mission-v1.psv` for the tests; route `POST /missions/{id}/plan`; reads return
the current plan (EDR-246..249). RED Go/Kotlin 82/317 vectors, router 3,
contract 2/2, PostGIS 4/4 twice → GREEN 317/317, PostGIS 13/13; union 1084 /
9251, 0 failures.

### EMP-TRIP-013 — vehicle change within a mission: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip013-vehicle-change.md), commit `e836412e`, migration 000232
(rebuilds the asset usage function from the 000230 body). Server-only change
on PLANNED, ASSIGNED, ACCEPTED or DECLINED missions, refused while the
assignee has a trip running on the current vehicle (it must finish on its own
vehicle first); the new vehicle must be ACTIVE, not an implement, not running
another trip and not unavailable, in maintenance or blocked at the planned
start; the planner must cover the mission before and after, and a current
assignee must still cover it; each change is an insert-only row in
`org_trip_mission_vehicles` and the database refuses moving a reading or a
trip to another vehicle (EDR-250/251). RED domain 25/25, router 3, contract
1/2, PostGIS 6/6 and 5/6 → GREEN domain 26/26, PostGIS 21/21; union 1092 /
9310, 0 failures; Kotlin enterprise 3541, 0 failures. Counts: 147 normal
routes, auditor walk 147 / 30 / 117, matrix 280 cells (112/168), 8 signed
owners. Retained: app/panel display (TRIP-021/022), vehicle change on
IN_PROGRESS missions once trips link to missions (TRIP-021/023), planned stops
(TRIP-010), re-planning accepted missions, history reads, production grants.

Integration (`4a7c211d`): `origin/main` `93f3a88d` merged (no file change),
then the TRIP-009/013 branch (already merged with `bff5a788`) without
conflicts; build, vet (both tags), gofmt, `sqlc diff`, check_plan, cmd/api and
trip unit tests green. Not delivered: the next cumulative `dev → main` PR.

## Owner decisions for the rest of F4 (2026-11-15)

Relayed request (finish the whole F4 stage, merge it to main and deploy every
app change) and the owner's direct answers in this session: EMP-GOV-005 is
declared reviewed and the tracking features will be in the terms of use, so
EMP-GPS-001..015 are implemented for real (EDR-271; real-device acceptance
OWED, emulator only); PR #224 is merged alone first, then the F4 continuation
in its own PR with valid CI; the enterprise routes are enabled on the VPS only
after the automatic deploy, with a rollback plan and a final confirmation
(EDR-272). The second batch started on `fe7ec66d` (main `93f3a88d` +
TRIP-009/013 + `origin/dev` `db43e949`).

PR #224 (Play Store readiness, head `db43e949`) was merged alone by the
coordinator at 18:33Z as `7ab0b477` under EDR-272: both checks
(`Quick verification`, `test`) completed successfully on the exact head,
`make quick-verify` passed locally on it
(`f4-logs/pr224-quick-verify-db43e949.log`), and the merge used
`gh pr merge --merge --match-head-commit db43e949…`. The F4 branch already
contained that head; `origin/main` was merged for ancestry with no file
change.

### EMP-GPS-001 — tracking session domain: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-gps001-tracking-session-domain.md), commit `dbf02b88`, under
the owner decision EDR-271. Pure Go `tracking/domain` and Kotlin
`enterprise.tracking` over the 359-case `tracking/session-v1.psv` oracle: a
session opens only for an IN_PROGRESS organization trip, by that trip's driver,
from the visible trip screen, while the tracking policy is OPTIONAL or
EXPECTED (personal origin, implicit/boot/worker/push/geofence/server origins,
another member, policy OFF — which also covers an inactive capability or an
employee notice not in force — a trip not in progress, a second session for
the trip and another organization are refused with closed codes); start and
resume carry the device permission and notification visibility explicitly
(NONE or a hidden notification keeps the state and records the visible cause);
pause, typed interruptions, reduced precision and terminal END; FOLLOW applies
the trip's state and never refuses the trip (the trip packages do not import
tracking, checked structurally in both languages); a fix is collectable only
while the bound trip is IN_PROGRESS, driven by the owner and RECORDING; one
quality classification function (EDR-273..276). No coordinates are held. RED
Go 5/7 and 359/359 vectors, Kotlin 363/364 → GREEN Go 7/359 under `-race`
(100%), Kotlin enterprise 3905 tests. Retained: Android service and notice
(GPS-002/015), sampling/buffer and per-fix filter (GPS-003), persistence and
intake (GPS-004), processing/distance (GPS-005/006), session ownership after
handover or supervisor closure (TRIP-012/014), the application policy layer,
real-device acceptance (OWED).

Integration: merged into the F4 branch with only shared docs (EDR table,
contract, manifest) touched; build, vet and the tracking race tests green.

### EMP-OFFL-005 (F3/C1 prerequisite) — attachment upload: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-offl005-attachment-upload.md), commit `cbd24fcc`, no migration.
Photo bytes are sealed `ATTACHMENT` entries in the PRIV-003 protected store
with one sealed `attachment-book` v1 index per partition written in the same
write; capture only beside an operation already queued in that partition;
16 MiB of unsent bytes and 500 entries without eviction; bytes are removed in
the write that records `STORED`; flag `ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED`
off by default (EDR-267). Whole-file retry with the OFFL-003 backoff; local
states `WAITING_FOR_RECORD` → `DECLARED` → `RECEIVED` → `STORED`, `REFUSED`
with a closed code or `FILE_MISSING`; only server answers advance a photo
(EDR-268). The operation's final receipt gates the upload; the DOC-003 link
target is fixed at capture and the server links with the real device actor
(C1 targets INCIDENT and METER_READING) (EDR-269). One signed special route
`POST /v1/orgs/{org_id}/sync/attachments` (`SIGNED_ATTACHMENT_SESSION`)
declares, links and returns a fresh 5-minute capability; bytes go to the
existing capability ingress (EDR-270). New 89-row
`sync/attachment-session-v1.psv`. RED Go 14/16, cmd/api 2/4, PostGIS+MinIO
3/3, Kotlin domain 96/97, application 8/8, data 11/11 → GREEN; union 17
packages 884/7026 (organization adapters 2700 s). Native acceptance on
`emulator-5588` against a real local backend and a throwaway MinIO: offline
trip start with two synthetic JPEGs of its departure reading surviving a
process kill sealed; on reconnect a truncated and a byte-flipped upload are
refused (422) while the record already shows accepted, then reach `STORED` on
retry; a process kill during a held upload resumes with a new session; server
counts exact (3 documents, 3 STORED, 3 links); organization B sees nothing.
Retained/OWED: capture/import screens and producers (DOC-004, FUEL-011,
CHECK-003 Android, TRIP-020), real receipts (OQ-04; no production document
bucket), an encrypted blob store, re-declaration after a closed 24 h session,
TRIP/FUELING/EXPENSE/WORK_ORDER link kinds, OFFL-013, two physical devices.

Integration (`62ed58fd`): contract and EDR table conflicted (appended
sections); build/vet including the `nativefixture` tag, gofmt, cmd/api,
apicontract, orgsync and document unit tests green; Android host suites on the
merged tree domain 4002, application 664, data 195, app 23 tests, 0 failures,
`:app:assembleDebug` (`f4-logs/coord-android-offl005-merged.log`); proposed
shared text applied (10-offline-sync §10.3, data and backend READMEs,
documents.md, 09-architecture).

### EMP-TRIP-011 — pause, resume and cancellation: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip011-pause-resume-cancel.md), commit `4bc35660`, migration
000233 (redefines nine trip/mission functions from their latest bodies:
the 000220 completion guard, finish source/apply, publication and held
source; the 000229 logbook, held-logbook and review-source functions; the
000232 mission-vehicle source). Signed `trip.pause`, `trip.resume` and
`trip.cancel` name the expected trip revision (1 after start, +1 per pause,
resume, cancel or finish) and carry only a `reason` (required for cancel);
IN_PROGRESS ↔ PAUSED, either → CANCELLED (reason) or COMPLETED; a closed trip
holds `TRIP_ALREADY_CLOSED`, a stale revision or disallowed state holds
`STALE_REVISION`, wrong shape / cancel without reason / instant before start
is `MALFORMED`, another member `FORBIDDEN`, an undelivered or foreign trip
`RETRY`/`DEPENDENCY_PENDING`; replay byte for byte. A paused trip keeps its
asset; a cancelled trip frees it at the cancellation, keeps prior facts,
reports `NOT_MEASURED`/`CANCELLED` (no fabricated distance) and closes its
logbook (EDR-252/253). The logbook PAUSE/RESUME/CANCEL types still answer
RETRY/UNAVAILABLE (device vectors unchanged). New 297-case `trip-v2.psv`
replaces `trip-v1` for both languages. RED Go 5/26 top-level and 96/297
vectors, Kotlin 99/865, PostGIS 6/6 → GREEN Go 26/809 (100%), Kotlin 865,
PostGIS 6/16; union 33 packages 1193/8331. Pre-233 upgrade rehearsals use an
opt-out `PreLifecycleImage` flag (production default false). Retained:
Android builders (TRIP-021), supervisor cancel/close (TRIP-014), held-command
resolution (TRIP-007), panel timeline (TRIP-006), production grants.
EMP-TRIP-012 was not attempted (context); its IDs (394..398 / 199..201 /
254..256, migration 000234) stay reserved with the agent's design notes in
the coordinator notes.

Integration (`bc438c2b`): contract and EDR table conflicted; build, vet (both
tags), gofmt, `sqlc diff`, cmd/api, apicontract (11 signed owners), trip,
tracking and orgsync unit tests and the Kotlin enterprise domain suite (4094
tests) green.

### EMP-TRIP-007 — asset-overlap conflict resolution: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip007-conflict-resolution.md), commits `e81d6364` and
`dabde1b3` (alignment with TRIP-011 after merging `45c3baf3`), migration
000237 (no table or foreign key; widens the review-resolution outcome check
and redefines six functions from their latest bodies — the 199 resolution
source/complete and effect source, the 216 observation guard, the 220 trip
source and the 000233 trip completion — and adds `org_trip_review_published`).
Held facts of `trip.start/finish/event.record/submit/pause/resume/cancel` are
listed, read and resolved once under `trip.trip.review` at
`/sync/reviews/trips/{list,read,resolve}` (queue on organization-wide grants;
narrower grants through a new `HeldTripStructure`). Every held item can be
refused with a reason; only a held start can be accepted, becoming the trip it
declared (IN_PROGRESS at revision 1, departure reading at the original
receipt), and only if no other trip of the asset is in progress or paused or
finished/cancelled after the declared start (else `CONFLICT`, nothing
written); reviewing one's own item is `SELF_APPROVAL`, stale/resolved
`CONFLICT`, replay unchanged. The held fact and its original ACK stay
immutable; the driver's signed `review-results` read now answers trip items
(before, a trip item made the poll fail `NOT_FOUND`) (EDR-262..264). RED
application 5/5, cmd/api 4/22, apicontract 34/92, PostGIS 8/8 → GREEN 9/9
PostGIS on the merged tree; union 38 packages 1249/8614, 0 failures. Counts:
150 normal routes, auditor walk 150 / 30 / 120, matrix 280 cells. Retained:
accept-with-another-asset (VAL-07, EDR-262), attaching a refused fact to the
winning timeline (TRIP-006), Android presentation of resolutions, expiry
escalation, production grants.

EMP-TRIP-023 was not started: a mission reference on `trip.start` needs
either an `operation_schema` 2 (the protocol accepts only 1 in Go and the
shared Kotlin vectors) or a schema-1 soft reference with an
`ORPHAN_REFERENCE` hold; reservations have no mission link and no `FULFILLED`
source yet (notes `f4-logs/trip023-analysis.md`). Its IDs (412..414 /
209..210 / 265..266, migration 000238) stay reserved.

Integration (`40ea759d`): the branch already contained `45c3baf3`; clean merge;
build, vet, gofmt, `sqlc diff`, check_plan green.

PR #225 (UGC safety: report topics, report and block users; migration 000239
reserved by that session) was merged by the coordinator at 23:43Z as
`b0ab7d33` on the owner's instruction to merge every open PR with valid CI:
`fast`, `integration`, `Quick verification` and `test` completed successfully
on the exact head `5407e478`, `make quick-verify` passed locally on it
(`f4-logs/pr225-quick-verify-5407e478.log`), and the merge used
`--match-head-commit`. `origin/main` was merged into the F4 branch
(`b55157a2`) without conflicts; 000239 redefines no function used by F4.
The owner also asked for an unattended run: recommended decisions are taken
without questions and recorded here.

### EMP-TRIP-014 — missing final reading and supervisor closure: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip014-supervisor-closure.md), commit `f837b42d` (+ merge
alignment `cc5b5985` onto the 000233 bodies), migration 000235 (insert-only,
forced-RLS `org_trip_closures`; completion guard, logbook and held-logbook
sources and `org_trip_revision` from their 000233 bodies; review-entry source
from 000229 with the closer excluded from review). Routes `trips/open/list`
(ORG grants only) and `trips/{trip_id}/close` (fresh write, revision CAS,
`reason` + `missing_reading_reason`) under `trip.trip.close_for_other`; no
reading, value or distance is accepted or created (usage stays NOT_MEASURED /
READING_ABSENT); the closer is never the author or driver and never reviews
the trip; the driver's later finish and events are held
`TRIP_ALREADY_CLOSED`; PAUSED trips are closable, CANCELLED ones `CONFLICT`; a
closed trip frees its asset without an invented end instant (EDR-257..259).
RED domain 3/3, application 3/4, PostGIS 6/6 → GREEN.

### EMP-TRIP-018 — open trips when a membership ends: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip018-membership-end-open-trips.md), commit `b07973c6`,
migration 000236 (`org_offboarding_guard` from 191 with `TRIP:<id>` flags and
a refusal to leave an open trip unflagged; `org_trip_held_source` from 233
with a late-finish branch). Trips enter the IDN-010 open-responsibilities
source through an explicit port as mandatory `TRIP:<id>` items; offboarding
never closes or deletes a trip; the open-trip list shows the author's
membership state for a scoped supervisor closure; a pre-end late finish is
held `ACCESS_ENDED_BEFORE_RECEIPT`, later work `MEMBERSHIP_ENDED`, a lost
device `DEVICE_REVOKED` for the whole response (EDR-260/261). RED membership
domain 1/1, PostGIS 4/4 → GREEN; the IDN-010 offboarding fixture gained the
`org_trips` grant (no expectation weakened). Final merged-tree union on
`8effb2e8`: 37 packages, 1311 top-level / 9873 named, 0 failures; untagged 224
packages. Counts: 152 normal routes (29 routed / 55 not routed / 26
special), auditor walk 152/30/122, matrix 290 cells (116/174). Note: 000235
and 000236 apply before 000237 by number; no function is redefined by both
sides. Retained: linking a later reading to a closed trip by amendment,
held-item resolution for finishes/events/late finishes, late holders for
start/event/submit, successor handover (TRIP-012), notifications, panel,
TEAM-scope fixture, production grants.

Integration (`05971894`): clean merge; build, vet (both tags), gofmt,
`sqlc diff`, cmd/api, apicontract, trip and membership unit tests green.

### EMP-GPS-004 — track intake and segment storage: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-gps004-track-intake.md), commit `2d53fc84`, migration 000240.
Batches are signed operations through the OFFL-002 intake (six session
operations `gps.session.start|resume|suspend|interrupt|reduce|end` plus
`gps.track.batch` on the `tracking_session` entity; one operation ≤ 8 KiB and
200 fixes; the batch id is the operation id, so replay and
`OPERATION_ID_REUSED` come from the intake) (EDR-277). The closed receipt
vocabulary is kept: policy OFF → REJECTED/FORBIDDEN; other domain refusals and
oversized batches → REJECTED/MALFORMED; another member → whole-request
FORBIDDEN; unknown trip/session → RETRY/DEPENDENCY_PENDING; outside-session,
duplicate and mock counters live on the segment (EDR-278). Four forced-RLS
tables: insert-only `org_tracking_policies` (absent = OFF; the notice must be
in force), `org_tracking_sessions` (unique per trip),
`org_tracking_transitions` (contiguous) and `org_tracking_segments` (integer
arrays plus a database-derived MultiPoint, SRID 4326), serialized on the trip
row lock, with triggers re-checking policy, operation source, recording period,
trip running time, precision and per-session instant uniqueness; a row commits
only with its ACCEPTED receipt (EDR-279/280). Sessions follow the trip's
pause/resume/cancel/finish (TRIP-011); fixes while paused or at/after
cancellation or finish are outside the session in Go and SQL. RED application
8/9 and 70/70, contract 2/2, PostGIS 7/7 → GREEN 9/84, PostGIS 8/19.

### EMP-GPS-012 — location access permission and audit: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-gps012-location-access-audit.md), commit `9b1491e3`, no
migration (000241 unused). `POST /v1/orgs/{org_id}/trips/{trip_id}/track/read`
under `gps.track.read` only: the trip's author counts as the reader's own
record, so DRIVER `OWN_TRIPS_ONLY` and AUDITOR `RECORDED_TRACKS_ONLY` apply;
unit/team scope from the trip's stored structure; authority re-proved inside
the read transaction; ≤ 20 segments per page; every page records
`LOCATION_TRACK` ISSUED then SERVED through the PRIV-005 recorder before the
response, and a failed record serves nothing (UNAVAILABLE) (EDR-281/282). Live
positions stay with GPS-007/008 (F5). RED application 1, inventory/contract
1+1, PostGIS 2/2 → GREEN; union 39 packages 1204/9382, 0 failures; untagged
225 packages. Retained: tracking late intake (OFFL-009), a writer for
`org_tracking_policies` (no backlog owner yet — assigned to the coordinator's
activation work), production grants, the Android producer and batch splitting
(GPS-002/003), processing (GPS-005), retention (GPS-011), live views
(GPS-007..009, F5), real-device acceptance (OWED).

Integration (merge of `codex/enterprise-f4-gps004` onto `b4249772`): route
counts conflicted with TRIP-014 and were merged as the union — 153 normal
routes (30 routed / 54 not routed / 26 special), auditor walk 153 / 31 / 122,
matrix 300 cells (121 allowed / 179 denied), 18 signed owners; the company
policy allow-list keeps both TRIP-014's closure policies and GPS-012's track
policy; `TripStructure` answers `trip.trip.close_for_other` and
`gps.track.read`; three duplicated EDR rows (262..264) from the rebased branch
were deduplicated. Build, vet (both tags), gofmt, `sqlc diff`, cmd/api,
apicontract, tracking and trip unit tests and the focused PostGIS race tests
`Auditor|AuthorizationMatrix|Track|Tracking|Closure|Close|OpenTrip|Offboarding`
(with a disposable MinIO) green.

### EMP-EXP-002 (F3 prerequisite of TRIP-020) — expense persistence and fueling projection: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-exp002-expense-persistence.md), commit `41adb3d6`, migration
000244 (forced-RLS, insert-only `org_expenses`; usage functions from 000232 /
000231 plus one line each). The fueling's expense is created inside the
`fueling.submit` transaction (no consumer exists for the org-event job kind):
category FUEL, paid and company amount equal to the fueling's paid total, its
payer and snapshot, reporting day in the unit (else organization) zone,
SUBMITTED revision 1; neither half can exist alone (a deferred check on
`org_fuelings` and a composite foreign key) (EDR-287/288). DIRECT expenses
through signed `expense.submit` schema 1 (exact keys `asset_id`, `category`,
`paid_total`, `company_amount`, `payer`, `trip_id`; categories TOLL, PARKING,
MAINTENANCE, LEASE, INSURANCE, FINE, OTHER under a named CHECK that EXP-003
replaces; FUEL is MALFORMED; a `trip_id`, attachments, changed rights and
post-disposal dates stay RETRY/UNAVAILABLE); `exp.expense.record` ORG covers the
member's own expense on any asset, SELF only on assigned periods (EDR-289).
A stale projection writes nothing; a newer or differing stored expense is never
overwritten (EDR-290). Kotlin `ExpenseOperations` builder with the 40-row
`expense/expense-submit-v1.psv`. Fuelings accepted before 000244 get their
expense from an idempotent per-organization catch-up whose production trigger
is not wired (only synthetic fuelings exist). RED application 6/7 and 76/77,
PostGIS 21/25 and 12/27, contract 2/2, Kotlin 43/280 → GREEN; union 37
packages 1124/8353 on the merged tree; untagged 229 packages; Kotlin
enterprise 4138. 19 signed owners. Retained: catch-up trigger, FUEL-003/015,
EXP-003/004/005/006, TRIP-020 link, DOC-004 receipts, WORK_ORDER origin,
production grants.

Integration (`a27a30f4`): the branch already contained `0912b48a`; clean
merge; build, vet, gofmt, `sqlc diff`, check_plan green; proposed shared text
applied (backend README, 09-architecture, fuelings §9, 10-offline-sync).

### EMP-TRIP-012 — driver handover during a trip: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip012-driver-handover.md), commit `a30ad444`, migration
000243 (insert-only, forced-RLS `org_trip_handovers`; new
`org_trip_responsible`, `org_trip_responsible_member`,
`org_trip_handover_source`, `org_trip_handover_published`; twelve
redefinitions each from its latest body including 000235/000236/000239/000240,
every original line kept — line log `f4-logs/trip012-migration-lines.log`; no
usage-function line, so 000244 is untouched; 000234 stays unused). Routes
`POST /trips/{trip_id}/handover` (supervisor, fresh `trip.trip.close_for_other`
covering the trip structure, receiver an eligible member assigned to the asset,
revision CAS), `/trips/handovers/list` and `/trips/mine/list`. Responsibility
is derived from the handover rows: the current responsible member gates
pause/resume/cancel/finish/submit/late finish; logbook events, incidents in
trips and tracking facts are checked against the member responsible at the
declared instant; each handover adds one to the trip revision; every past
responsible member is excluded from review; handovers respect supervisor
closures and offboarding flags; after a handover the old tracking owner is
refused and the receiver opens a new session (EDR-254..256). New 87-case
`trip/handover-v1.psv` with Kotlin `TripHandover.kt`. RED Go 5/31 and 81/87,
Kotlin 83/954, PostGIS 7/7 and 4/4 → GREEN Go 36/896 (99.9%), Kotlin 954 and
enterprise 4227, PostGIS incl. races; merged-tree union on `2f031276` + this
change: 55 packages, 1476 top-level / 10379 named, 0 failures; untagged 229
packages. Counts: 156 normal routes (30 routed / 54 not routed / 26 special),
auditor walk 156/31/125, matrix 300 cells (121/179). Retained: Android/panel
handover screens (TRIP-021/022), real-device acceptance.

Integration: the commit's parent is the F4 head `2f031276`; clean merge;
build, vet, gofmt, `sqlc diff`, check_plan green.

### EMP-CHECK-002 — checklist execution offline: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-check002-checklist-execution.md), commit `3e91dab5`, migration
000242 (three forced-RLS run tables and a deferred database recheck; the
000244 usage bodies gained the three run keys at integration so the later
redefinition keeps them). Pure server recheck
(`maintenance/domain/checklist/run.go`) and a signed `check.run.complete`:
a well-formed run is always ACCEPTED with what the device recorded and the
server's outcome by the policy in force from its own history;
`TEMPLATE_SUPERSEDED` is flagged; a disagreement is kept as `review_required`
and never dropped (EDR-283..286). A BLOCKING run blocks the asset through a new
availability cause `CHECKLIST_RUN` (`ChecklistBlockPort`), so a new trip start
is held `ASSET_UNAVAILABLE`; routes: run read (`fleet.asset.read`) and release
(fresh `check.incident.manage`, `SELF_APPROVAL`, CAS, replay, one audit event).
Shared 56-row `checklist/run-v1.psv`. Android: Kotlin builder, sealed
`checklist-book` v1, producer and store, `ChecklistRunScreen` reachable from the
context screen and the trip start form (disabled while BLOCKING), 66 strings ×
8 locales, flag `ENTERPRISE_CHECKLIST_UI_ENABLED`. RED → GREEN across Go
(recheck 7/35 → 16/162, vectors 54/56, availability, contract, router, PostGIS
8/8 twice) and Android (vectors 55/58, book, flow, gateway, store, process
death, ViewModel); merged-tree union on `3fdab0b8` 46 packages 1387/10075, 0
failures; Kotlin domain 4204. Native acceptance on `emulator-5588` (7 stages:
offline BLOCKING run with process death, server agreement, a WARN-vs-BLOCK
disagreement shown as "A empresa recalculou o resultado"; exact server counts)
— run before the emulator-lock rule existed. Retained: the trip gate
(TRIP-019), run photos (DOC-003 `CHECKLIST_RUN` link kind), damages and
incident blocks and the panel (CHECK-004), the feed entity type, events,
protocol-level ACCEPTED_FOR_REVIEW, production grants; OWED: real Hilt UI end
to end, release while the device watches, a second device, visual/TalkBack.

### Tracking policy command (EMP-GPS-002 prerequisite): LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-gps-policy-command.md), commit `4d5159c7`, migration 000247
(receipt table only). `tracking/policy/change` under fresh
`org.settings.manage` (the key the spec names for SCR-GPS-09) and
`tracking/policy/read` under `org.settings.read`; each change writes the next
insert-only `org_tracking_policies` version with a receipt and one
`gps.policy.changed` audit event under revision CAS; OPTIONAL/EXPECTED need a
notice version already in force (a future notice is refused); no notice text is
stored (EDR-302/303). RED unit 3/4, PostGIS 3/4, contract 2+2 → GREEN.

### EMP-GPS-005 — track processing: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-gps005-track-processing.md), commit `90ee8c83`, migration
000246. Storing a session END (or a segment of an ended session) queues
`org-tracking-process` in the intake transaction; the job, under its lease,
writes one insert-only `org_tracking_summaries` revision: fixes filtered and
counted (outside session, accuracy > 50 m, implied speed > 70 m/s), gaps with
reason and detector (SIGNAL_LOST after 180 s), coverage and quality from the
GPS-001 function, haversine distance inside runs only with its method,
Douglas–Peucker at 20 m (≤ 1000 vertices) plus a database-built
MULTILINESTRING; the database re-proves the exact stored input and accepts only
a strictly newer input at the next revision (EDR-298..301). RED unit 16/17,
PostGIS 4/4 → GREEN; union 33 packages 1173/8319 on a tree containing
`2f031276`. Retained: processing for trips ending without the device's
`gps.session.end` (needs a trip-completion consumer), summary reads
(GPS-006/009), purge/trim recomputation (GPS-011), held/late tracking,
production grants, real-device acceptance (OWED).

Integration (CHECK-002 merge `2e3b02e6`; tracking merge `eca80543`): route
counts merged as unions — 160 normal routes (30 routed / 54 not routed / 26
special), auditor walk 160 / 33 / 127, matrix 300 cells (121/179), 20 signed
owners; the router test lists both TRIP-012 and tracking routes; duplicated EDR
rows 262..264 removed again. Build, vet (incl. `nativefixture`), gofmt,
`sqlc diff`, cmd/api, apicontract and tracking unit tests green; proposed shared
text applied (10-offline-sync checklist row, data and backend READMEs).

### EMP-TRIP-022 — panel: mission planning and assignment board: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip022-mission-board.md), commit `ebcf9bd0`, no migration
(000248 unused). Business-web mission board (SCR-TRIP-03) in the organization
workspace: sends nothing until the planner opens it; lists, creates (unit =
the chosen vehicle's current unit), plans, assigns, changes the vehicle and
cancels through the existing routes with schema 1, one fixed operation id per
decision and the revision the planner saw; shows only server-read state with
its read time (ACCEPTED/DECLINED only from a read carrying the answer);
CONFLICT re-reads and never retries, FORBIDDEN/NOT_FOUND clear the board,
FRESH_AUTH_REQUIRED resets the session. SCR-FLEET-06 is delivered as a
read-only vehicle agenda (availability with causes, reservations and blocks,
without purposes, notes or people). One missing backend read was added:
`POST /missions/{mission_id}/assignees/list` under the mission read policy,
reusing the assignment eligibility check and returning only employee id,
display name and registration number (EDR-304..306; the company principal
also needs `SELECT ON org_employee_profiles`). RED router 4, contract 1/2,
PostGIS 3/3, web 8/8 → GREEN router 24/24, contract 2/2, PostGIS 3/3, web
59/59; a local Chromium DOM check found and fixed a focus bug. Union 8 packages
820/7052 on its base. Retained: the actual-API HTTPS browser flow
(`tests/browser.py`) not extended, reservation/block commands and grid
calendar, filters, history, missions without a vehicle, a
`trip.mission.manage` context hint, production grants.

Integration (merge onto `1a566b45`): counts merged to 161 normal routes (30
routed / 54 not routed / 26 special) and the auditor walk 161 / 33 / 128;
build, vet, gofmt, `sqlc diff`, cmd/api, apicontract and trip unit tests
green; business-web typecheck, 59/59 tests, build and the 27-file artifact
check pass on the merged tree.

### EMP-TRIP-023 — reservation-to-trip integration and no-show release: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip023-reservation-trip-link.md), commit `bc118de6`, migration
000249 (000238 unused; six functions redefined from their latest bodies with
a builder that fails on any lost line — `org_asset_reservation_guard` 217,
mission next/transition/guard/complete 231/232, `org_trip_held_source` 243
gaining `ORPHAN_REFERENCE`). Coordinator decision: `trip.start` stays schema 1
and may carry one envelope soft reference `{type:"mission", id}`, eligible only
for an ACCEPTED mission whose assignee is the trip's author and driver, whose
vehicle (if any) is the trip's asset and which has no linked trip; otherwise an
admissible start is held `ORPHAN_REFERENCE` (existing held codes keep
priority). Insert-only `org_trip_mission_links`; the linked start moves the
mission ACCEPTED → IN_PROGRESS and a trigger completes it with the trip's
finish or supervisor closure (a cancelled trip leaves it IN_PROGRESS); oracle
`mission-v3.psv`. New `missions/{mission_id}/reservations/request` (fresh
`trip.mission.manage`, revision) reserves the mission's vehicle for its
window; a confirmed mission reservation admits that mission's start and is
FULFILLED once by the linked start received before the no-show deadline and
declared within `[start − 30 min, end)`; the FLEET-007 outcome job takes the
organization root lock first so release and start resolve to one outcome in
both orders (EDR-265/266). RED oracle 21 vectors, domain/application/cmd/api/
contract, PostGIS 8/8 twice → GREEN 8/25 PostGIS, unit 232/1380; union 37
packages 1313/9276 (one fixture-only fix, `TestTrip*` rerun 104/223).
EMP-TRIP-010 was not started (context); its IDs (475..480 / 242..244 /
307..310, migration 000250) stay reserved.

Integration (`c7e407e6`): conflicts with TRIP-022 and CHECK-002 resolved — both
mission routes kept (162 normal routes; auditor walk 162 / 33 / 129); the
OpenAPI path blocks of `assignees/list` and `reservations/request` were
interleaved by the merge and restored verbatim from their branches after the
contract test caught a lost 400 response; the CHECK-002 integration test call
of `StartDecisionDBTX` gained the new `mission` argument; fleet store fields and
availability tests of both tasks kept. Build, vet (all tags), gofmt,
`sqlc diff`, cmd/api, apicontract, trip, fleet and maintenance unit tests green;
focused PostGIS race tests
`Mission|Reservation|Checklist|TripIntake|TripClosure|Auditor|AuthorizationMatrix|FleetStore|Availability`
with a disposable MinIO: 6 packages, 106 top-level / 4893 named, 0 failures.

### EMP-DOC-004 (F3 prerequisite) — app capture and import: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-doc004-app-capture.md), commit `ab3ff194`, no migration.
`ReceiptCapture.kt` judges the file by its bytes with the OFFL-005 JPEG policy
and keeps the original unmodified; a pure integer legibility hint (resolution,
dark, bright, contrast, blur) never blocks and never claims fiscal validation;
`ReceiptCaptureController`/`EnterpriseReceipts`; `AndroidReceiptOriginals`
(FileProvider camera file deleted once read); SCR-DOC-01 `ReceiptCaptureCard`
with the system camera (`TakePicture`) and photo picker (`PickVisualMedia`),
no new dependency; entry point on trip start/finish records carrying a reading;
flag `ENTERPRISE_RECEIPT_CAPTURE_UI_ENABLED`; 49 strings × 8 locales
(EDR-291..293). RED domain 10/10, application 6/6 + 9/9 → GREEN; native on
`emulator-5588` with a real backend: offline capture → process death →
reconnect, a truncated upload refused while the start was accepted, STORED on
retry, organization B nothing.

### EMP-FUEL-011 (F3 prerequisite) — app fueling offline: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-fuel011-app-fueling.md), commit `ef85f52f`, migration 000245
(`org_document_links.fueling_id` and the `FUELING` link kind with a composite
same-organization foreign key; no function or usage change). Kotlin
`FuelingOperations.kt` (builder + strict decoder mirroring Go `ParseOperation`)
and `FuelingPresentation.kt`; shared 59-row `fueling/fueling-submit-v1.psv`
consumed by Kotlin and the real Go parsers; producer, service and
`FuelingFlow` (the receipt is a second protected write targeting `FUELING`;
warnings never block); `ProtectedFuelingStore` reads retained queue entries;
`FuelingPanel`, flag `ENTERPRISE_FUELING_UI_ENABLED`, 86 strings × 8 locales;
backend `FuelingDocumentTargets` resolver under `fuel.fueling.record`
(EDR-294..297). RED Kotlin domain 62/63, application 8/8, data 1/1, Go resolver
and PostGIS link (503 stub) → GREEN; union 24 packages 1005/9345 with four
fixture-level fixes. Native with a real backend including EXP-002: two offline
fuelings (receipt in the same save and added later), process death,
reconnect → both accepted, one receipt STORED and the other refused (422) then
STORED on retry; server 2 fuelings, 2 documents, 2 FUELING links.

Integration: the DOC-004/FUEL-011 branch conflicted with CHECK-002 in the
Android screens; a dedicated integration agent merged `e7cbf724` and
`c7e407e6` into it keeping all three rollback flags, both trip-screen modes
(departure checklist and reading photo), the checklist, receipt and fueling
owners on the context screen, and a queue registry with trip, checklist and
fueling producers (`835218be`, `ec74050b`, gofmt fix `257b06e8`); EDR rows
re-sorted (306 rows, no duplicates). On `257b06e8`: Go build/vet (all tags),
gofmt, staticcheck, sqlc clean; PostGIS race
`Fueling|DocumentLink|Attachment|Checklist|Expense|Mission|Reservation` 98/222;
Android host suites domain 4366, application 710, data 205, app 30; lint 0
errors; normal and enterprise validation builds; native regression on
`emulator-5588` (trip 6/6, checklist 7/7, receipt 5/5, fueling 3/3, context
6/6, sync-context 2/2). Merged into the F4 branch as `aab550f0` without
conflicts. Retained: real receipts (OQ-04), trip/place/supplier references and
the capacity warning in the app, FUEL-003/004/012/015, DOC-011, TRIP-020, the
recurring `EnterpriseSyncContextDeviceTest` intermittent, two physical devices,
TalkBack/visual review.

### EMP-CHECK-004 — panel: checklists, damages and incidents: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-check004-checklist-incident-panel.md), commit `7467fb2f`, no
migration (000254 unused). One company read `checklists/runs/list` under the
incident-queue policy (`check.incident.manage`, `ORG` grant only, not fresh;
BLOCKING first, then disagreements held for review, newest completion first,
sealed cursor; EDR-328); `contexts/read` hints `doc.document.view_content` for
an `ORG` assignment so the panel never offers a photo it may not open
(EDR-329); `business-web` `checks.ts`/`checkpanel.ts` (EDR-330): availability
always as the server reads it (a failed read is "not confirmed"), release and
report actions with revision checks (`SELF_APPROVAL` named and never retried,
`CONFLICT` re-reads, `FORBIDDEN`/`NOT_FOUND` clear), photos only through the
audited `content/access` + `content/read` path into a saved file (CSP).
B-BR-EMP-506..510, BUC-EMP-258..260, EDR-328..330. RED domain 1/2, apicontract
2/3, cmd/api 3 + 1 subtest, PostGIS 2/2, web 8/8 → GREEN. Task-tree union
903/7596 and untagged 230 packages; merged-tree (TRIP-023) union 1183/9592 with
one load-only TRIP-023 failure (`TestTripMissionReferenceOrphanAndUnsupportedForms`,
3/3 alone).

Integration: merged into the F4 branch as `c219b165` (contract union only;
route counts 163 normal, auditor walk 163/33/130 set by the task branch).
Integration fix `b2dfc674`: the TRIP-022 mission board parser refused the
`CHECKLIST_RUN` availability source that CHECK-002 made the server report, so
the whole availability read failed for a vehicle with a blocking run and its
cause showed as "Outro motivo"; RED 1/9 missions tests → GREEN business-web
68/68, typecheck, build, 29-file artifact check. Focused PostGIS race on
`c219b165` (`cmd/api`, `apicontract`, `orgaccess`, `maintenance`, `document`,
`fleet`, `organization/adapters`, disposable MinIO): 22 packages, 1167
top-level / 9197 named, 0 failures. Retained: blocks from open reports with
`blocks_use`, run-disagreement acknowledgement, unit-scoped queues and filters,
a queue-order index, `CHECKLIST_RUN` document links and work-order links, an
inline photo viewer (CSP decision), the actual-API HTTPS browser flow.

### EMP-TRIP-019 — checklist gate on trip start and finish: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip019-checklist-gate.md), commit `c28e050d`, migration 000255
(insert-only forced-RLS `org_trip_checklist_links` and
`org_trip_checklist_holds` with rule-checking guards; four new functions; no
existing function or usage change). `trip.start` (schema 1) may name its
departure run as one `checklist_run` soft reference beside the TRIP-023 mission
reference and `trip.finish` one return run; a run is required only when a
CHECK-001 departure template applies under the current configuration; only
the author's own unlinked departure run of that vehicle counts, judged at the
declared start from server history (blocking until its release was recorded,
valid for the policy revision's validity). Failures keep the closed ACK
vocabulary (held `ASSET_UNAVAILABLE`, or `ORPHAN_REFERENCE`) and store the
reason (`CHECKLIST_REQUIRED`, `CHECKLIST_BLOCKING`, `CHECKLIST_EXPIRED`,
`ORPHAN_REFERENCE`); a finish links a qualifying return run and is never held
by it. Shared `trip/trip-operations-v2.psv` (61 rows, v1's 54 unchanged)
hash-checked by Go and Kotlin; Android carries the form's valid non-blocking
departure run behind the existing checklist flag. B-BR-EMP-511..516,
BUC-EMP-261..263, EDR-331..334. RED Go application 4, PostGIS 7/7 (codec and
store stubs), Kotlin 7/63 → GREEN; task union 42 packages 1418/9653 (two
fixes/reruns: a TRUNCATE grant in the CHECK-001 schema fixture and a load-only
401 in an unrelated legal-declarations test, 17/17 alone); untagged 230
packages. EMP-TRIP-010 was not started by that agent (IDs stay reserved) and
runs as its own task.

Integration: merged on top of `b2dfc674` as `c68f3537` (union of the
maintenance run queries — CHECK-004's queue and TRIP-019's gate facts; sqlc
output unchanged — contract sections and EDR-328..334, no duplicates; no route
change). Go build/vet (plain, `integration`, `integration nativefixture`),
toolchain gofmt, sqlc vet/diff, check_plan 0 and `git diff --check` clean;
Android host suites on the merged tree: domain 4374, application (enterprise)
158, data 205, app 30, 0 failures, `:app:assembleDebug` passes. Retained: the
gate reason and run in the TRIP-007 conflict view and the panel, linking the
run when a reviewer accepts a held start, the app's return-checklist line on
finish, production grants and native acceptance.

Merge verification for TRIP-019: focused PostGIS race on `c68f3537` (`cmd/api`,
`apicontract`, `maintenance`, `trip`, `fleet`, `fueling`, `orgsync`,
`enterprisesuite`, `organization/adapters`, disposable MinIO): 25 packages,
1190 top-level / 9381 named passed; one failure,
`cmd/api TestPublicProcessIdentityAndSignedWrites` (a public worker poll
timeout under host load, outside enterprise code), passed 3/3 alone on
`7b869304`.

### EMP-METER-005 — meter replacement, reset and rollover: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-meter005-replacement-rollover.md), commit `d3d755db`, migration
000251 (`organization_meter_segments`). A segment closes and a meter is removed
at most once through one guarded update; a reading counts in the segment whose
interval holds its instant; reading rows never change. Go and Kotlin share one
segment model over `meter/segments-v1.psv` (143 cases: cumulative usage, usage
between instants, equivalent display, replace/reset/rollover planners).
`POSSIBLE_ROLLOVER` only on a meter with a modulus and applied only by a
person's `CONFIRM_ROLLOVER`; new route `meters/{meter_id}/change` (REPLACE or
RESET) under `meter.reading.correct`. METER-005 and METER-006 together use
B-BR-EMP-481..490, BUC-EMP-245..249 and EDR-311..317 (the split is in each
evidence file). RED Go domain 2/25 with 143/143 vectors, Kotlin 144/575, PostGIS
6/6 → GREEN.

### EMP-METER-006 — current-value projection and usage-between-instants port: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-meter006-current-value-projection.md), commit `cd45f37a`,
migration 000252 (`organization_meter_current_values`, written by SECURITY
DEFINER triggers in the transaction of every reading change, including
METER-004 decisions, METER-005 boundaries and TRIP-015 amendments; revision =
count of the meter's facts, so a rebuild reproduces it). `ops meter
rebuild-projection --org <uuid>` (operator identity required) and
`MeterStore.UsageBetween` (both anchors, `NO_READING_BEFORE`/`METER_GAP`, the
projection revision, share lock). The concurrency test caught a mixed read
(old revision with new readings), fixed by a separate locking statement.
Existing organizations have no projection until the rebuild runs (forced RLS
stops the migration from listing organizations); until then the port answers
unavailable. RED PostGIS 3/4 → GREEN. The task branch's GPS-006 was not started
there (see GPS-006). Unused: 000253.

Integration: merged on `7b869304` as `7eae7bb1` (contract and EDR union,
EDR-311..317 sorted; both METER-005 and CHECK-004 had moved the route counts
162 → 163, so they were set by hand to 164 normal and the 164/33/131 auditor
walk). Go build/vet (all tags), gofmt, sqlc vet/diff, cmd/api and apicontract
passed; focused PostGIS race on `7eae7bb1` (`cmd/api`, `cmd/ops`,
`apicontract`, `fleet`, `trip`, `maintenance`, `fueling`, `orgsync`,
`enterprisesuite`, `organization/adapters`, disposable MinIO): 26 packages,
1228 top-level / 9564 named, 0 failures. Retained: re-targeting device
readings that name a closed segment, closed-segment history and SCR-METER-04
(METER-007), undoing a rollover or boundary, in-trip usage across a boundary,
consumers of `meter.usage.recalculated`, running the projection rebuild for
existing organizations after deploy, production grants.

### EMP-TRIP-010 — stops and legs, planned versus observed: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip010-stops-legs.md), commit `7599e11e`, migration 000256
(insert-only forced-RLS `org_trip_mission_plan_stops`, admitted only in their
own `PLAN` transaction, deferred receipt check; no function or usage change).
A `PLAN` may carry 1–20 ordered free-text stops (plans without stops keep the
v1 digest byte for byte; with stops a v2 digest). Observed stops and legs are
derived at read time from the trip's start, finish or cancellation and its
accepted `STOP`/`ARRIVAL`/`DEPARTURE` events, never matched to planned stops;
incoherent observations are shown (`ARRIVAL_MISSING`, `DEPARTURE_MISSING`,
`after_receipt`, `after_end`, held events with their code). New read
`trips/mine/{trip_id}/stops/read` under `trip.trip.start` and the own-trips
policy. Shared `trip/stops-v1.psv` (54 rows) hash-checked by Go and Kotlin.
B-BR-EMP-475..480, BUC-EMP-242..244, EDR-307..310. One rule changed during the
task (events after the end are listed, not refused) with a genuine RED on the
old code. Task union 24 packages 1269/10050, 0 failures; untagged 230 ok.

Integration: merged on `7eae7bb1` as `be3d6493` (contract union, EDR-307..310
sorted; counts set by hand to 165 normal and 165/33/132). Retained: panel and
Android presentation, a supervisor's planned-versus-observed read, typed places
and per-stop planned times, matching observations to planned stops, held
review state, a server clock-skew estimate, a finish-after-events guard,
production grants.

### Sync-context intermittent (EDR-335): fixed, LOCAL_DONE

[Evidence](f4-fix-sync-context-intermittent.md), commit `bfc6fcca`. Root cause
in the product: `MyAssetsFlow.pull` checked currency against the lease that
`open()` captured after `deliveryAccount()` had refreshed the session within
the family, so the first online "Meus ativos" in the token's last minute
cleared the view as STALE. It now adopts the fresh delivery lease first, as the
trip, fueling and checklist owners do. Test infrastructure: the device runner
initializes WorkManager with a delegating factory and cancels earlier classes'
work. RED host 1/13 and native 3/3 with the recorded diagnostic → GREEN host
13/13; the 20-run native GREEN is OWED (emulator locked, see below).

### EMP-EXP-004 — app expense with receipt: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-exp004-app-expense.md), commit `db972eef`, migration 000257
(`org_document_links.expense_id` with a same-organization foreign key and
widened checks; `ExpenseDocumentTargets` resolves DIRECT expenses only, under
`exp.expense.record`, author as SELF). Kotlin domain/application/data/app
(`ExpenseFlow`, `ProtectedExpenseStore`, `ExpenseScreen`, flag
`ENTERPRISE_EXPENSE_UI_ENABLED`, 52 strings × 8 locales). B-BR-EMP-517..521,
BUC-EMP-264..266, EDR-336..338. RED Go 1/1 and PostGIS (503 stub), Kotlin
domain 2/283, application 7/7, data 1/1 → GREEN; union 24 packages 1049/8397.
Native OWED.

### EMP-TRIP-020 — closing conference before sending a trip: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip020-closing-conference.md), commit `0dcbcfe1`, no migration
(000258 unused). Device-local conference (EDR-339) over the partition's own
fuelings and expenses for the trip's vehicle between start and finish; per-item
record and evidence state (MISSING, PENDING, AVAILABLE — stored, never
"verified" — or UNREADABLE), amounts per item with no total; submission stays
the first `trip.submit`. B-BR-EMP-522..526, BUC-EMP-267..269, EDR-339..341.
RED domain 3/3, application 2/2 → GREEN. Native OWED.

Integration of the three: merged on `be3d6493` as `78de91be` (contract union,
EDR-335..341 sorted; no route change). Go build/vet (all tags), sqlc, cmd/api,
check_plan clean; Android host suites domain 4579, application 168, data 206,
app 31, 0 failures; lint and `:app:assembleDebug` pass. Retained: EXP-003
categories, EXP-005, real receipts (OQ-04), a server trip reference for
fuelings and expenses, resubmission after RETURN, checklist runs and incidents
in the conference, TalkBack/visual review, production grants (`SELECT` on
`org_expenses` for the resolver, `REFERENCES` for the migrator).

### EMP-GPS-002 — tracking service: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-gps002-tracking-service.md), commit `de8e2eaf` (DI defect fixed in
`d971686f`), no migration. Driver read `tracking/policy/mine` under
`trip.trip.start` over the member's own record (AUDITOR, FINANCE, APPROVER 403;
another organization 404; writes nothing). Kotlin `TrackingOperations` builds
the six `gps.session.*` operations replayed through a sealed tracking book; the
non-exported `OrganizationTrackingService` (location type, `START_NOT_STICKY`)
listens only after a one-time grant armed by the visible trip screen, shows an
ongoing notification with a stop action and a 30-second watchdog; process
death or restart is interrupted as `PROCESS_STOPPED`/`DEVICE_RESTARTED` at the
next opening. `check-mobile.sh` §5 admits exactly this fourth subscription and
still refuses background location, boot receivers and sticky starts. Shared
`tracking/session-operations-v1.psv` (41 rows).

### EMP-GPS-003 — sampling, buffer and batches: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-gps003-sampling-buffer.md), commit `d971686f`, no migration.
`sampling-v1` defaults after a device filter counting mock, stale, inaccurate
and impossible fixes; a sealed `tracking-buffer` survives restarts; gaps
(`SIGNAL_LOST`, `PROVIDER_DISABLED`, `MOCK_LOCATION`) open after 180 s without a
good fix and are never bridged; batches of at most 200 fixes / 8 KiB leave the
buffer in the same write as their operation. A same-millisecond stop/fix
defect was fixed. Shared `tracking/track-batch-v1.psv` (24 rows). GPS-002/003
IDs B-BR-EMP-495..505, BUC-EMP-252..257, EDR-321..327. Task union 32 packages
1199/8524 (two load-only failures, 3/3 alone each); untagged 231 ok; `make
quick-verify` ok. Native OWED with a ready one-line driver.

Integration: merged on `78de91be` as `19e681a3`. Kotlin unions kept every
feature: the trip view state carries DOC-004 photos, TRIP-020 conference
sources and GPS-002 tracking; `TripFlow(source, service, fence, receipts,
related, tracking) { deliver }`; the trip detail keeps photos, closing and the
tracking card; the queue registry adds the `gps.session` producers; all rollback
flags kept. EDR-321..327 inserted in order (the branch still carried the old
EDR-265/266 placement). Counts set by hand to 166 normal and 166/33/133. Go
build/vet (all tags), sqlc, cmd/api and tracking tests pass; Android full module
suites domain 5608, application 739, data 979, app 829, 0 failures; lint, normal
and enterprise validation builds (`-PenterpriseDeviceValidation=true` with both
androidTest APKs) and `scripts/check-mobile.sh` pass. Retained: emulator and
real-device acceptance, Android trip pause/cancel/handover feeds (TRIP-021), a
handover end cause on the server (EDR-256), a server intake for device gap
reasons, battery economy, GPS-010/015, GOV-006, production grants.

### Emulator incident (2026-11-16)

Around 10:33–10:51Z the owned emulator `emulator-5588` stalled under host
memory pressure (~1 GB available, ~24 GB swap) and its system server crashed;
the EXP-004 agent rebooted it holding the emulator lock. It came back with user
0 `RUNNING_LOCKED` behind its synthetic lock-screen credential, which no agent
holds or may enter, and the coordinator does not wipe it or change its lock
settings. Every native acceptance after that point (flake 20-run GREEN, EXP-004,
TRIP-020, GPS-002/003 and later app tasks) is OWED with prepared drivers until
the owner unlocks the emulator once. The physical phone was never used.

Merged-tree verification of `19e681a3` (CHECK-004, TRIP-019, METER-005/006,
TRIP-010, EXP-004, TRIP-020, the sync-context fix and GPS-002/003 together):
focused PostGIS race over `cmd/api`, `cmd/ops`, `apicontract`, `orgaccess`,
`maintenance`, `document`, `fleet`, `trip`, `fueling`, `expense`, `tracking`,
`orgsync`, `enterprisesuite` and `organization/adapters` with a disposable
MinIO: 43 packages, 1504 top-level / 12556 named, 0 failures.

### EMP-OFFL-011 — cleanup of revoked sessions and devices on next contact: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-offl011-revocation-cleanup.md), commit `90bcb693`, Kotlin only, no
migration (000260 unused). A whole-response 403/404 on delivery or a late-intake
code (`ACCESS_ENDED_BEFORE_RECEIPT`, `MEMBERSHIP_ENDED`, `INTAKE_WINDOW_CLOSED`,
`DEVICE_REVOKED`) is only a hint; the worker then asks the exact signed device
contact once. ERASE (lost or compromised) erases only that partition and sends
none of its work again; LOCK (access ended) writes a durable Keystore marker,
refuses every use except delivering sealed unsent work intake-only, and erases
the partition once nothing is unsent or the server closed its intake window.
Before this a LOCK stopped all delivery (eligible pre-end work was erased at
the 30-day expiry) and a lost phone retried its refused queue forever. The app
says "removed" only after verified disposal. B-BR-EMP-536..539,
BUC-EMP-275..276, EDR-348..350. RED 15/15 new tests → GREEN on the first run;
full suites domain 4585, application 723, data 980, app 827; lint 0 errors;
validation build passes. Merged on `112e24b3` as `0adb235d` (contract union);
Android full module suites domain 5614, application 741, data 986, app 829, 0
failures; validation build and `check-mobile.sh` pass. Retained: native
acceptance (OWED, emulator locked), a reboot or clock rollback during
intake-only delivery (re-anchoring from a contact's server time), attachment
bytes not yet stored, suspension, a per-partition "locked, N sending" view,
notifications, the manager-side lost-records message.

### EMP-GPS-006 — distance sources on a trip and the divergence signal: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-gps006-distance-sources.md), commit `db937ab6`, no route and no
migration (000253 unused). Coordinator decision (EDR-318): the backlog types
GPS-006 as DOM (V-DOM, V-MONEY), so it is a pure tracking-domain evaluation plus
a server port, and the first `trip.trip.read` route with its structural, self
and auditor semantics stays with EMP-TRIP-006. The odometer, GPS, planned and
estimated distances stay apart; the basis is the odometer, else a PRECISE GPS
distance, else `INSUFFICIENT_DATA` with a reason; divergence is integer
arithmetic (defaults 2 000 m and 10 %) with `RAISED`/`NOT_RAISED`/
`NOT_EVALUATED` and never a misconduct verdict. `DistanceStore` proves
`gps.track.read` through GPS-012's `CheckTrackTx` (a denial withholds the GPS
source), reads the latest GPS-005 summary, takes the odometer from METER-006's
`UsageBetween` and records the `LOCATION_TRACK` ISSUED/SERVED pair before
answering. EDR-319: a trip finished without its own reading has no odometer
distance (the port's "known 0 m" is not accepted). EDR-320: server-only, no
Kotlin mirror. B-BR-EMP-491..494, BUC-EMP-250..251, EDR-318..320. RED unit
13/13, PostGIS 3/3 → GREEN; task union 21 packages 1098/9083. Merged on
`0adb235d` as `843f647a` (contract union, EDR-318..320 in order); build, vet,
staticcheck, tracking and `cmd/api` tests pass; focused PostGIS race
`TestTripDistance|TestTracking|TestTrack|TestLocation|TestMeter` on the merged
tree passes. Retained: composing the port behind TRIP-006's route, organization
divergence thresholds, UC-GPS-08 and `gps.divergence.detected`, planned and
estimated sources (`NO_ROUTE_PLAN`/`NO_ROUTE_COMPUTATION`), production grants.

### EMP-TRIP-021 — app: missions inbox and planned-trip execution: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-trip021-missions-inbox.md), commit `e2c47185` (after merging
`c68f3537` as `a690216d`), no migration (000259 unused), no server production
change. `TripOperations` carries the mission reference on `trip.start` beside
TRIP-019's departure run, plus `trip.pause`/`trip.resume`/`trip.cancel`;
`TripMissionOperations` signs `trip.mission.respond`; `TripBook` v2 (still reads
v1); `TripMissionInbox` is a sealed copy with its age and the offline
eligibility matrix (refresh online only; a planned trip starts only from a
confirmed acceptance). The queue producer descriptor gained `alone` (answers in
a batch of their own after every other due operation, EDR-343). Shared
`trip/trip-operations-v3.psv` (94 rows; v2's 61 rows byte for byte) consumed by
Go and Kotlin. Flag `ENTERPRISE_MISSION_UI_ENABLED`, strings × 8 locales. The
merge with `19e681a3` (`30a3a99f`) made tracking follow device trip commands (a
pause suspends a recording session as `TRIP_PAUSED`, a cancel ends it as
`TRIP_CANCELLED`) and let the mission gateway accept TRIP-010's `plan.stops`.
RED domain 50/119, application/data 14/14 → GREEN.

### EMP-ROUTE-004 — navigation phase 1: in-app route to a planned destination: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-route004-navigation-handoff.md), commit `cd133382`, no migration.
`TripNavigation` hands an authorized trip/mission destination to the existing
map route engine as `MapRouteTarget.Address`; guidance uploads nothing and
starts no tracking session; a source guard keeps tracking out of the navigation
code. TRIP-021/ROUTE-004 IDs B-BR-EMP-527..535, BUC-EMP-270..274,
EDR-342..347. RED domain 3/3, guard 2/2 → GREEN. Task-tree gates: domain
enterprise 4709, application enterprise 194, data enterprise 219, app
enterprise 36; lint 0 errors; normal and validation builds; Go build/vet (all
tags), staticcheck, trip race 1194 named.

Integration: merged on `843f647a` as `fac846c3` (contract union, EDR-342..347
before EDR-348, the offline-sync paragraph keeps both the missions-inbox and
revoked-partition sentences). Go trip and `cmd/api` pass; Android full module
suites domain 5665, application 749, data 993, app 832, 0 failures; lint,
validation build and `check-mobile.sh` pass. Retained: native runs (OWED,
drivers `trip021-scripts/native-run.sh`, `route004-scripts/native-run.sh`), an
end-to-end run through the real map tab, a final local state for a
transport-denied answer, mission list paging beyond 100, planned stops in the
inbox, unit destinations and next-stop navigation (ROUTE-005..008), server
answer history, TalkBack/visual review.

### EMP-OFFL-013 — sync resilience suite: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-offl013-sync-resilience.md), commit `dd8588c8`, no migration
(000261 unused). Server half `TestSyncResilience*` drives one field day of real
signed operations (mission respond, checklist run, `trip.start` naming mission
and run, events, a meter reading, a fueling, an expense, finish and submit)
through the default runtime's signed TLS intake under a lost response, a
whole-queue resend after a killed client, reversed dependencies, sequential and
concurrent duplicates in both orders behind a held org-root lock, reused
identifiers, a missing acknowledgement, truncated `FUELING`/`EXPENSE` uploads
and cross-organization state; the final effect signature equals the in-order
one. Device host half `EnterpriseSyncResilienceTest` uses the real vault, queue,
producers, queue service and HTTP client with real child-JVM process death. It
found a real defect, fixed RED → GREEN (EDR-352): the Android dispatcher sent a
`trip.start` before its departure checklist run had a final receipt when the run
was in backoff or answered `RETRY` in the same batch, and the server then held
the start `ORPHAN_REFERENCE` with nothing linked; the dispatcher now holds an
operation until every queued operation it names in a soft reference is final.
B-BR-EMP-540..543, BUC-EMP-277..278, EDR-351..353. Merged on `66164d53` as
`98d76e98`; the dispatcher combines the soft-reference holds with TRIP-021's
`alone` producers (a start naming a mission waits for the answer's final
receipt). On the merged tree: PostGIS race
`TestSyncResilience|TestTripMission|TestTripChecklist` passes; Android domain
enterprise 4717, application 752, data 999, app enterprise 36, 0 failures;
lint and the validation build pass. Retained: the native stage (OWED,
`offl013-scripts/native-run.sh`), VAL-23 (f) `CLOCK_SKEW`, proposed shared-doc
text in the evidence file.

### EMP-GPS-010 — tracking interruptions: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-gps010-tracking-interruptions.md), commits `68ad54ce` and
`c0dacaf6` (after merging `fac846c3` as `6e0cbc89`), no migration (000262
unused). Permission loss, approximate-only precision, provider loss, process
death and restart each leave a recorded interruption or gap, never bridged; a
resume needs an explicit visible gesture plus a current policy and
one-collector check on every tap (EDR-356: the device is stricter than the
server); approximate fixes are bounded and never reported as precise; the
sealed buffer splits runs around a pause and accounts for every fix. The
composition with TRIP-021 releases the listener right after a pause or cancel.
RED domain 4/4, application 8/10, collector 1/12, app 1/3 → GREEN.

### EMP-GPS-015 — app: tracking transparency: LOCAL_DONE / INTEGRATION_PENDING

[Evidence](f4-gps015-tracking-transparency.md), commit `763a498d`, no migration
(000263 unused). A tracking indicator in the top bar of every enterprise screen,
a home card and the "Meu rastreamento" panel (what is collected, why, and this
phone's own sessions and gaps; EDR-358: no server read of own sessions exists,
so only this device's records); copy never presents a personal session as
corporate and never promises remote erasure. GPS-010/015 IDs B-BR-EMP-544..551,
BUC-EMP-279..282, EDR-354..359. RED application 4/4, app 2/8 → GREEN.

Integration: merged on `98d76e98` as `a02e3273` (contract union, EDR-354..359
after EDR-353). `1ee50c9e` hardens `scripts/check-mobile.sh`: the agent saw the
static guard fail once under load at the GuidanceService check with an unchanged
manifest (three reruns passed; cause unconfirmed); the eight service-block
checks now read a here-string instead of `echo | grep -q` under `pipefail`;
static guard and `test-gate-selection.sh` (34/34) pass. Retained: native and
real-device acceptance (drivers `gps010-scripts/native-{source,driver}.sh`,
`gps015-scripts/native-driver.sh`; `DEVICE_RESTARTED` proven on the host only),
a local "tracking interrupted" notification, the long-stationary guard
(BR-GPS-016), a server policy check on resume, a server read of own sessions,
SCR-PRIV-01.

GPS-010/015 merged-tree result on `1ee50c9e`: Android full module suites
domain 5671, application 767, data 999, app 842, 0 failures; lint, the
enterprise validation build and `check-mobile.sh` pass.

### EMP-REL-005 — M4 gate: BLOCKED (M4 NOT accepted)

[Evidence](f4-rel005-m4-gate.md), commit `497bfe4c`, documentation only
(B-BR-EMP-552..555, BUC-EMP-283..284, EDR-360..362). The M4 scenario union is
one documented command set (`docs/backend/TEST_STRATEGY.md`, "Enterprise M4
gate scenario union") run on `1ee50c9e`: G1 `organization/adapters` named
scenarios on real PostGIS with `-race` and a disposable MinIO 240 top-level /
5266 named (including the 300-cell matrix and the 166/33/133 auditor walk);
G2 `orgsync`/`orgaudit`/`fleet` adapters 46/117; G3 trip, tracking, orgsync,
fleet domain, maintenance and apicontract 445/4625 plus the `cmd/api`
inventory 4/29; Kotlin enterprise domain 4721, application 214, data 232, app
46; `check-mobile.sh --static-only` ok; 0 failures, 0 skips. Each VAL clause
and M4 exit criterion is mapped to named tests. Not proven (no owner yet):
VAL-04/VAL-22 part 2 held fuelings (FUEL-003), VAL-23 (f) `CLOCK_SKEW`, VAL-27
display and review (TRIP-006, UC-GPS-08), a `trip.start` through the
suspension check, VAL-02 as one cross-surface sweep; entry condition M3
(REL-004) unmet. Device ledger OWED (nothing run): native stages for
OFFL-013, GPS-002/003, GPS-010, OFFL-011 and the task-level items, plus the
real-device interruption matrix on at least three manufacturers. EDR-361: M4
NOT accepted, REL-005 BLOCKED. EDR-362: tracking collects from no real
employee of any organization until a recorded M4 pass with the matrix or a new
explicit owner decision naming the accepted risk (procedural; the policy
already defaults to `OFF` and needs a notice in force; EDR-271 unchanged). The
F4 diff `b0ab7d33..HEAD` scan found no secrets, only the synthetic CPF and
synthetic coordinates, and no images or binaries. Merged as `63726828`.

Backlog bookkeeping at the checkpoint: GPS-002, GPS-003, GPS-010, GPS-015 and
CHECK-003 carried evidence notes but a stale `state TODO`; their state lines
now read LOCAL_DONE (37 of 38 F4 tasks LOCAL_DONE, REL-005 BLOCKED).

## Batch checkpoint — 2026-11-16

Second F4 batch on `codex/enterprise-f4`, built on main `b0ab7d33`. Every F4
implementation task is LOCAL_DONE / INTEGRATION_PENDING; EMP-REL-005 is
BLOCKED and M4 is NOT accepted (EDR-361/362). Nothing here is INTEGRATED or
RELEASE_CERTIFIED. Final gates on the delivered head are recorded below.
