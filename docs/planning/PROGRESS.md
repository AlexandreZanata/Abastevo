# Current execution state

- Updated: 2026-09-28, P04-T01 LOCAL_DONE; P03 INTEGRATED.
- Branch: `codex/phase-04-observations`, based on `6a12dcd`; clean.
- Phase P03 INTEGRATED: PR #4 merged `762615a` → `6a12dcd` (match-head-commit, all checks green); branch `codex/phase-03-anonymous-identity` deleted locally + remotely, verified; merge recorded in PR #4 metadata; wiki WIKI_PENDING. Post-merge main CI runs automatically.
- Main protection ACTIVE: strict, required `[Quick verification]`, PR required, enforce-admins, no force/deletion.
- P04 entry G03 satisfied. Exit G04 pending; no current-price claim at receipt.
- P03-T01…T08: LOCAL_DONE (profile vectors, registration, auth verifier, idempotency, quotas, rotation, durable queue, dispatch). G03 INTEGRATED.
- P04-T01: LOCAL_DONE — `community/domain` immutable Observation (server IDs/times/attribution only, STANDARD sentinel, policy v1, freshness flags, PriceObserved event; stdlib-only with kernel cross-check test); invalid amount/unit/condition/identity/time cases, supersedes link, historical flags; RED proven by dropping the range check, GREEN on restore; manifest extended.
- P04-T02: LOCAL_DONE — `community/domain` validation state machine (RECEIVED→VALIDATING→VALIDATED/REJECTED, VALIDATED→REJECTED moderation-only; persisted command proof on claim, worker-only admit/reject, stable reasons, privileged case-bound invalidation; immutable decisions with sequence + event names; freshness/confidence/disputes kept separate); exhaustive valid/invalid/actor/reason/command matrix; RED proven by dropping the admit guard, GREEN on restore; no new manifest packages.
- G04: P04-T03…T08 NOT STARTED.
- Next: **P04-T03 — Observation persistence**.
- Issues/milestone/PR/wiki: P04 phase PR pending; wiki once per merged phase; no invented IDs.
- G09 NOT STARTED; P10 BLOCKED BY G09; roadmap 78 tasks.

## Evidence pointers

- [Prior P01 command outcomes preserved verbatim](history/P01_FOUNDATION_EVIDENCE.md).
- [Android baseline re-validation](BASELINE_VALIDATION.md#p01-t01-re-validation--2026-09-28).
- [Workflow policy](DELIVERY_WORKFLOW.md), [CI plan](CI_PLAN.md), [fast execution card](FAST_EXECUTION.md), [ADR-013](../adr/013-fast-phase-delivery.md).
- [Phase record template](templates/PHASE_RECORD.md): use one compact state and task evidence entries, not a growing full transcript in this file.
