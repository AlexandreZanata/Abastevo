# ADR-013 — Fast delivery by phase with separate release certification

Date: 2026-09-28. Status: **ADOPTED policy; automation and remote activation pending G01-FLOW (P01-T13…T17).**

## Context

The maintainer requested the delivery structure used by the local Goyim-Arena project: isolated phase branches, task commits/issues, phase PRs, concise context and a documentation wiki. Repeating a complete backend/infra/mobile suite for every microtask would slow development. Deferring all meaningful tests until the end would conceal critical failures.

The fuel repository already contains P01-T01…T12 foundation commits and path-aware backend/PostGIS/Android workflows. This decision extends that implementation; it does not pretend those workflows have already changed or that the backend is release-ready. Previous task evidence is preserved in [the P01 archive](../planning/history/P01_FOUNDATION_EVIDENCE.md).

Reference inspected: `/home/iiii/PESSOAL-PROJETOS-ALEXANDRE/Goyim-Arena/`, HEAD `33a458f52016338ac86cd68c34e476491b47307c`; AGENTS, `.local/GIT_FLOW.md`, `.local/FAST_EXECUTION.md`, phase helper, quick/full workflows, Makefile and CI documentation. This is design provenance, not a runtime dependency or authority to publish into either repository.

## Decision

1. Work on one bounded task at a time, on `codex/phase-NN-slug`; use a worktree when a shared checkout would interfere. One phase milestone/draft PR contains task issues and atomic Conventional Commits. Use additional commits for corrections, without rewriting published work.
2. Run focused behavioral tests before each implementation commit. Authentication, privacy, money/units, transactions, migrations, jobs and destructive-operation controls keep immediate adversarial/integration checks.
3. Integrate each phase after its specialized exit evidence and a bounded quick gate. The required quick CI runs on all PRs, including drafts and docs-only changes, plus main. Do not wait for every draft push if independent work can proceed safely; never merge with a known failure or absent required result.
4. Certify the complete backend and infrastructure on an immutable P09 candidate after P01–P08 merge. Full security, restore, load and compatibility acceptance remains mandatory. G09 alone releases Android feature work in P10; P10/P11 have their own release matrices.
5. Verify required check identity, SUCCESS, exact current PR head/base and configured reviews before a guarded merge. No admin bypass, force push, disabled tests or success inferred from missing/skipped checks.
6. Keep Markdown canonical. Update progress briefly per task and issues as needed; synchronize wiki once per merged phase using owned-page manifests, immutable source SHAs, rewritten links and conflict detection. Preserve manual wiki pages. Retry WIKI_PENDING without rerunning unrelated application tests.
7. Implement shared helpers in versioned `scripts/`, using synthetic fixtures/fake remotes before authorized live checks. Reconciliation must be idempotent; dry runs and LOCAL_ONLY mode must make no remote writes. Record session publication scope and reuse existing authorization without prompting per microtask.

Detailed contracts: [delivery workflow](../planning/DELIVERY_WORKFLOW.md), [CI plan](../planning/CI_PLAN.md), [fast execution](../planning/FAST_EXECUTION.md). G01-FLOW introduces the new required check and migrates protections without a gap; until then existing workflows and requirements remain binding.

## Alternatives and consequences

- Full CI per microtask provides broad feedback but repeats expensive, unrelated work. Reserve the broad matrix for release and preserve focused critical checks during implementation.
- No tests until release gives late feedback on price, identity and data-loss errors. It is rejected; quick compilation is also not a substitute for behavioral tests.
- One PR per microtask adds integration/wiki overhead. Use one phase PR, keeping reviewable task commits and a bounded phase scope.
- Copying the reference helper verbatim would import repository-specific settings, ignored local files and unsafe wiki cleanup assumptions. Reimplement only the required lifecycle with dry-run, duplicate/conflict and exact-head tests.

Phase integration no longer implies complete-system certification. Records must distinguish LOCAL_DONE, INTEGRATED, WIKI_PENDING/SYNCED and RELEASE_CERTIFIED. A broader defect can still be found at release; mandatory task checks, required quick tests and an explicit candidate matrix reduce that risk without claiming it is eliminated. Measure quick duration and adjust selection based on evidence without weakening acceptance.

2026-09-30 amendment: G01-FLOW is active. ADR-014 supersedes the G09-before-mobile dependency; full real-production certification waits for G18. Immediate critical checks and fast phase delivery remain mandatory.

2026-10-05 amendment: [ADR-018](018-project-batch-delivery.md) supersedes the per-phase PR/remote CI/merge/wiki cadence with local phase checkpoints and final project batch integration. Immediate task/risk tests and real release certification remain mandatory. Historical activation evidence above is unchanged.
