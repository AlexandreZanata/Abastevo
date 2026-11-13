# F2 — Fleet, meters and the first enterprise flow

Date: 2026-11-13.
Status: IN_PROGRESS
Validation: PENDING
Integration: INTEGRATION_PENDING

The maintainer requests the entire F2, not only an initial fleet task. Continue
on owned `codex/enterprise-f2` from tested/published dev
`fac002e7d507a230d4947a3ed563269ad48f8c8e`. Main `6a19bcc7` is an ancestor;
cumulative dev → main PR203 remains open. Other occupied checkouts are preserved.
[Phase tracker215](https://github.com/AlexandreZanata/Abastevo/issues/215) retains
the whole requested scope; [first task216](https://github.com/AlexandreZanata/Abastevo/issues/216)
tracks the first domain checkpoint.
At opening the parent's required Quick passed; backend/Android CI was still
running. No construction polling or merge/deployment is authorized by this phase.

## Scope and dependency audit

All34 F2 tasks remain open at entry:

- IDN-014; AUTH-006; OFFL-006/008/014.
- FLEET-001/002/003/004/005/006/008/012/013.
- METER-001/002/003/004/005/007.
- TRIP-001/002/003/004/005/006; PUB-003.
- IMPORT-001/002/003/005/006; REL-001/002.

Their closure includes25 unfinished prerequisite tasks:

- F1: AUTH-009, ORG-014, PRIV-003, PUB-001, OFFL-001/002/003/004/007/010.
- F3: APPR-001/005, DOC-001/002/003/004/014/015, FUEL-001/002/003/011/012,
  OFFL-005 and PRIV-005.

The formal M1 gate additionally requires GOV-013, IDN-006/007/008 and REL-003.
These64 unfinished nodes are not completed by the preceding invitation/context
UI. The backlog task blocks remain canonical status. Pure domains may start
under roadmap §5; exposing operations and declaring M1/M2 requires their actual
owning source and acceptance. C1's existing-account invitation path does not
require managed access, but that does not imply full M1 acceptance.

Execute one dependency-ready task at a time, testing and committing its source
before dependent work. Begin FLEET-001, followed by protocol/access/onboarding/
protection prerequisites and the remaining ready domains. Then install real
asset/meter/assignment/intake/reference producers, the web/app/import flows,
minimal trips/fuelings/documents/review and C1 acceptance. Do not expose a vehicle
as ACTIVE with fake meter presence or keep an empty open-work adapter once trips
exist. All operations retain their creation snapshots and immutable ownership.

## Current bounded task

EMP-FLEET-001 is LOCAL_DONE / INTEGRATION_PENDING at2daea515:
[domain acceptance](f2-fleet001-asset-domain.md),133 shared vectors.
EMP-OFFL-001 is LOCAL_DONE / INTEGRATION_PENDING:
[protocol acceptance](f2-offl001-sync-protocol.md),260 shared vectors and matching
component-only OpenAPI. EMP-AUTH-009 is LOCAL_DONE / INTEGRATION_PENDING,
[authorization conformance](f2-auth009-authorization-conformance.md):120 actual
current-source HTTP cells and complete route inventory, tested commit4743e0b9.
EMP-ORG-014 is LOCAL_DONE / INTEGRATION_PENDING at efd7e886:
[company onboarding/settings acceptance](f2-org014-onboarding-settings-panel.md).
Those four checkpoints are backed up at origin/codex/enterprise-f2; the current
PR203 dev head still contains the earlier batch. EMP-PRIV-003 is now
LOCAL_DONE / INTEGRATION_PENDING:
[protected contact/generation completion](f2-priv003-protection-completion.md).
B-BR-EMP-104..107, BUC-EMP-54/55 and EDR-068..070 precede those changes.
Exact signed source, committed generation/floor and durable maintenance pass
ordinary PostGIS/race/host and actual API36 emulator acceptance. Twenty-two
native executions cover sixteen methods, including real source/keys, OS
backup/transfer, credential, clock/reboot and worker boundaries. Actual product
startup and normal/opt-in builds/lint pass; accepted ports now bind the real
client/shared store. Five checkpoints are local;59 audited nodes remain open.
Next: open the next dependency-ready task from this tested committed head.
Use phase tracker215 and cumulative PR203, without an intermediate PR or
remote-check wait. No asset persistence, intake route or queue was introduced
by the completed checkpoints. The full F2 scope remains in progress;
M1/M2/C1 is not accepted by these checkpoints.

## Acceptance and retained gates

Each critical SQL/auth/money/privacy change immediately gets negative,
failure/recovery/concurrency/ordinary real-PostGIS acceptance. Domain work uses
shared RED→GREEN vectors. Validate affected compilation/format/static checks,
diff and secret surfaces per task. Run the final union/Quick once after the
construction batch, then publish through the protected existing dev → main PR.

The latest maintainer instruction permits the emulator because the phone is
currently disconnected, superseding the earlier physical-only instruction for
this execution. Use isolated synthetic fixtures and preserve other applications
and data. Actual backup/transfer, secure-unlock, clock/key-loss
and current-contact requirements must be proved, not inferred from host or UI
tests. The protected-store/contact bindings remain closed until that acceptance.
Legal retention of real receipts, real storage-provider behavior, production
runtime credentials/grants and real pilot/deployment retain their external gates.
REL-002 permits a named synthetic local rehearsal; it does not authorize a real
company pilot. No LOCAL_DONE/M1/M2/C1 claim is made at opening.
