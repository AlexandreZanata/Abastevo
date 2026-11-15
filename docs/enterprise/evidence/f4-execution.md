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
