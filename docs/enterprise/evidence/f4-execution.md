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
