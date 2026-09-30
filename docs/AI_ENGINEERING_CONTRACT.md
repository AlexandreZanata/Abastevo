# AI engineering contract

Scope/authorization comes from the active user request. Start at AGENTS and the [fast execution card](planning/FAST_EXECUTION.md); do not reread the whole repository for each microtask.

## Sources of truth

Product owns behavior; DOMAIN_MODEL/COMMUNITY_PRICING_SPEC own backend rules; BUC specs own orchestration; ADRs own durable decisions; OpenAPI owns wire syntax; migrations own schema; tests demonstrate contracts. Current Git/PR/check evidence owns delivery state. ROADMAP defines tasks, PROGRESS indexes current state, phase records preserve evidence, and wiki mirrors committed docs rather than creating a second technical authority.

Known legacy conflicts remain in CURRENT_STATE_AUDIT. ADR-014 supersedes the production-before-mobile restriction; ADR-015 defines KMP/native boundaries. ADR-013 and DELIVERY_WORKFLOW supersede the old one-PR-per-task / repeated-full-check interpretation. A process plan is not proof that workflows/protection or remote publication are active.

## Per-task workflow

1. Inspect status/diff, current state, selected task/dependencies and relevant implementation/tests. Identify owned scope; resume or isolate with branch/worktree without changing others' work.
2. State B-BR/BUC, file areas, risk class and minimal meaningful checks. Record bounded assumptions; seek user input only for required unresolved decisions, not routine microtask transitions within an authorized phase.
3. Document behavior, write a failing meaningful test, implement minimally and refactor inside scope.
4. Run task/risk checks once against final relevant inputs. Re-run affected checks only after changes/failures. Never substitute a compile-only or zero-test result for behavior evidence. Review diff, sensitive changes and `git diff --check`.
5. Update compact task evidence/state. When phase publication is authorized, commit owned files atomically and push the phase branch; keep its PR draft and issue open until verified merge. Fixes after publication are new commits.

## Safety boundaries

- Backend/infra integration before app improvements through G09-LOCAL; actual G09 production release is deferred until G18. Functional gates and production certification are distinct. Preserve Android modules/packages, offline data and license.
- No invented API/field, silent money rounding, swallowed error, false source/price fallback or dropped unknown ANP data.
- Pure deterministic domain tests; real PostGIS for changed transactional/geo/schema behavior. Auth/ownership, replay, idempotency, concurrency, media/privacy and money invariants are immediate risk checks.
- No test deletion/skip/useless mock, reduced threshold or ignored failure to make CI green. Regression fixes include tests; expensive checks can be scheduled but relevant known failures cannot be deferred.
- One coherent task/commit; one phase PR with its reviewed task range. Do not accumulate unrelated refactors or implement speculative packages.
- Dependencies outside established needs require recorded alternatives, maintenance/license/security/build/lock-in analysis and ADR when architectural.
- Applied migrations are immutable; explicit forward changes and recovery. No manual production patch, destructive cleanup, force push or admin/verification bypass.
- No secrets/real private data in Git, public wiki, logs or fixtures; no private GPS/media in domain events. Read needed configuration only and redact diagnostics.
- No custom crypto primitive, purchased trust weight, infinite retry or public evidence bucket.

## Completion levels

**DOD-1 / task LOCAL_DONE:** selected behavior documented; meaningful targeted/risk tests and affected compile/static/contract/migration checks pass; scoped diff/secret review and recovery are recorded; no critical TODO or unsupported claim; issue/phase record reflects actual state. Documentation tasks use document checks. Local completion is enough to proceed to the next dependent task on the same phase branch when its prerequisite is satisfied; it is not merge/release certification.

**Phase INTEGRATED:** all included task acceptance and specialized phase checks pass, local quick and required remote checks/reviews verify the current expected PR head/base, guarded merge succeeds and linked issues close. Wiki synchronization has its own explicit pending/synced status. No duplicate full suite for each microtask or phase.

**Release RELEASE_CERTIFIED:** full required matrix and external operational evidence validate the immutable merged release candidate at the designated release checkpoint (functional G18 first, real-production P09/G09 afterward, then public P10 pilot/P11 as applicable). A new relevant code/config change invalidates affected evidence. No stable tag/deploy based only on quick CI.

## Publication scope

[DELIVERY_WORKFLOW](planning/DELIVERY_WORKFLOW.md) owns branch/issue/PR/wiki policy. Honor existing session authorization for phase delivery without per-action prompts. A planning edit alone does not execute publication or transfer authorization from Goyim-Arena. Record actual remote identifiers and source SHA; never fabricate issues, checks, reviewer approval or wiki completion. Preserve manual wiki content and human issue discussion.
