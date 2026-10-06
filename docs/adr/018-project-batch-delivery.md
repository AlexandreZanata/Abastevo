# ADR-018 — Project batch construction and deferred remote integration

Date: 2026-10-05. Status: ADOPTED by explicit maintainer request; local implementation, remote rollout pending.

## Context

The maintainer requested construction to continue through all planned project phases without waiting for PR checks or merges between phases. The supplied screenshot shows repeated 150–300 second sleeps and a second PR solely to record the first merge. ADR-013's per-phase integration cadence now delays useful work. Existing task acceptance and release obligations remain necessary.

## Decision

1. Build one bounded task at a time, with atomic commits on `codex/phase-NN-slug`. Retain phase boundaries, issues and milestones, but defer PR creation, remote CI inspection, merges and wiki publication to the final project construction batch. Authorized branch pushes are backups, not integration. No phase-branch push triggers the current main-only push workflows.
2. After passing immediate task and specialized local phase checks, commit a phase record with `Status: LOCAL_DONE` and `Validation: PASS`. `git-flow.sh checkpoint --evidence docs/...md` pins the clean head and evidence blob in private worktree state. It declares INTEGRATION_PENDING, never INTEGRATED. The helper checks the declaration; the operator must actually run and record the tests.
3. Start the next authorized phase with `start --from-checkpoint`. Its branch contains the previous checkpoint by ancestry. Changed heads, dirty trees and missing acceptance records refuse continuation. Local source dependencies may consume these tested checkpoints; deployment, public pilot and release dependencies still require their original integration/certification gates. This policy does not authorize implementing new product scope.
4. During construction, do not create/wait for a phase PR, poll check APIs, invoke `finish`, run aggregate release CI or sleep for fixed 150/300 second intervals. Existing draft PRs remain review references; they are not prerequisites for the next local phase. Known failures must be fixed before dependent work.
5. At the end of the authorized project phases, freeze one cumulative candidate containing the phase commits. Merge latest verified main normally into that branch if needed; rerun affected acceptance. Open/reuse one final batch PR to main with the accepted task range and truthful issue closure references. Split final review only if protection/review constraints require it; no intermediate merge dependency during construction.
6. Run the end batch acceptance matrix once, including the owed Android manual/device batch when the user makes the device available. Preserve the 2026-10-02 instruction to defer device/emulator runs until all P phases finish; iOS remains archived until explicit resumption. Missing real infrastructure means G09 UNCERTIFIED, not failed permission to continue unrelated local construction. Integration and production certification remain different results.
7. Ready-for-review triggers required Quick verification and relevant backend/PostGIS/Android signals. Reject drafts, missing/failed/skipped/cancelled required checks, stale head/base and unresolved reviews. `finish --required "Quick verification"` performs a bounded snapshot, exits on pending CI, and runs local quick only after remote checks pass. No polling loop, admin bypass, force push or direct development push to main.
8. Preserve task commits with a guarded merge. Close accepted task issues through the actual merged final PR; certify no partially completed issue. Mirror canonical docs to the owned wiki once from the final merged snapshot. Failed wiki publication stays WIKI_PENDING and retries only publication.
9. Record merged SHA/check/wiki outcomes in PR metadata or local pending notes, then include the durable summary in the next useful authorized commit. Never open a recursive bookkeeping-only PR to record another PR, its own SHA or its wiki commit.

## Verification and rollout

Synthetic Git/fake-GitHub tests cover refusal and continuation, worktree state isolation, zero remote operations at checkpoints, and protected final integration. Workflow checks cover drafts, ready-for-review and branch push triggers. Required main protection remains Quick verification; its remote settings are not changed by this local edit. YAML and rules become GitHub behavior only after authorized publication/integration. Existing runs are not cancelled by this plan.

This decision supersedes only ADR-013's cadence and contradictory per-phase PR/merge/CI/wiki instructions in older task plans. Immediate TDD/DDD and auth/privacy/money/SQL/migration/job risk tests remain required. Historical evidence stays historical. LOCAL_DONE / INTEGRATION_PENDING / INTEGRATED / WIKI_PENDING / RELEASE_CERTIFIED are distinct.

Contracts: [fast execution](../planning/FAST_EXECUTION.md), [delivery workflow](../planning/DELIVERY_WORKFLOW.md), [CI cadence](../planning/CI_PLAN.md).
