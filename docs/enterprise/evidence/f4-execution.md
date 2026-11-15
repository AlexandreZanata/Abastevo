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
