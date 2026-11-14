# F3 — Authorized follow-on scope and dependency order

Date: 2026-11-13.
Status: PLANNED
Validation: PENDING
Integration: INTEGRATION_PENDING

The maintainer explicitly requests completing the in-progress F2, then the
whole F3: fuelings/receipts, expenses, approvals, budgets, payables/payments,
reimbursements and exports. Any unfinished prerequisites from other phases
must be completed and actually validated before their dependent work starts.
The maintainer also reiterates unattended overnight execution: take the
recommended routine decisions without questions and record them for review.
Use the owned emulator for Android tests while the phone is disconnected.

The canonical roadmap's F3 contains67 tasks and targets M3, entered from C1.
It also owns consumption intervals, document conference/fiscal structure and
allocation, holds, advances, period locks, CSV exports and notifications. Its
tasks already pulled into C1 remain part of F2's actual dependency closure;
do not implement them twice or declare them complete from planning labels.
M3 requires the actual REL-004 allocation/no-double-counting, budget race,
partial-approval/return-loop and corrected-consumption acceptance.

Sequence: continue from completed EMP-OFFL-004 through remaining audited F2/C1 work,
with bounded tasks and immediate critical validation; freeze a truthful F2
local checkpoint; reconcile the actual F3 backlog/dependency graph and source
checkpoints; finish any still-open prerequisites first; then implement F3
in dependency order and complete its actual acceptance. Local checkpoints,
protected integration and release certification remain distinct.

Standing authorized delivery continues through atomic commits/owned-branch
GitHub backups, then the cumulative protected dev → main PR. No construction
CI polling or per-task PR wait is introduced. This extension does not authorize
merge, deployment, real company pilots, real receipt retention or real money
movement. Existing provider/legal/staging gates remain attached to their actual
capabilities; use synthetic local fixtures without fabricating acceptance.

Current source: F2 is IN_PROGRESS; eight completed local checkpoints through
EMP-OFFL-004 are backed up at origin/codex/enterprise-f2.
Next bounded implementation task is EMP-METER-001. No F3 implementation or acceptance
is claimed by this sequencing record.
