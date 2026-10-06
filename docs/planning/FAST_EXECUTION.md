# Fast execution card

Current maintained-branch policy: [ADR-020](../adr/020-maintained-dev-main-delivery.md), explicitly requested 2026-10-06, supersedes the phase branch lifecycle below for future work. Develop on `dev`, integrate to protected `main` through a PR, and retain/synchronize dev after merge. For existing clean delivery or dev, use `bash scripts/git-flow.sh finish --base origin/main --pr NUMBER --required "Quick verification"`; every security/current-head/base/check guard still applies. Historical phase evidence and incomplete runtime/release obligations remain preserved. The phase commands below describe the earlier construction batch.

Next source task: [P34-T01 Android/VPS connection](ANDROID_VPS_PLAN.md), under ADR-019. Staging origin is `https://teste.abastevo.com.br`; do not resume the public pilot or national importer before app-first scope.

[ADR-018](../adr/018-project-batch-delivery.md), adopted 2026-10-05, supersedes per-phase PR/CI/merge waits. Read AGENTS, this card, current [PROGRESS](PROGRESS.md), selected ROADMAP task and relevant code/tests. Read [DELIVERY_WORKFLOW](DELIVERY_WORKFLOW.md) at opening/policy change; search archives by task ID.

1. **Locate:** inspect status/diff and actual commits; resume owned work. Isolate another occupied checkout with a worktree. Never overwrite/stash another person's edits.
2. **Bound:** one task at a time, B-BR/BUC, risk and meaningful acceptance. Only authorized product scope; a workflow change is not authorization for new features.
3. **Prove locally:** domain RED → GREEN → REFACTOR; affected tests/consumers, compilation/format/static checks, `git diff --check` and secret review. Compile-only is not behavior evidence.
4. **Protect now:** immediate real PostGIS tests for SQL/migrations/transactions; negative auth/privacy/ownership, money, replay/idempotency, concurrency and jobs when affected. Fix known failures before dependent work.
5. **Commit:** one atomic task commit; authorized phase-branch push as backup. Keep issues open with LOCAL_DONE / INTEGRATION_PENDING. No PR needed to keep building; do not poll CI or sleep waiting for it.
6. **Checkpoint:** run local specialized phase exit once, commit truthful evidence with `Status: LOCAL_DONE` and `Validation: PASS`, then `bash scripts/git-flow.sh checkpoint --evidence docs/<phase-record>.md`. This pins the current clean head; the helper does not run or certify the declared tests.
7. **Continue:** `bash scripts/git-flow.sh start --phase NN --slug name --from-checkpoint`. The new branch includes its tested parent. No remote checks, PR creation, merge or wiki sync between phases. A changed checkpoint requires affected revalidation. Local dependencies may consume that checkpoint; release/deployment/pilot gates are unchanged.
8. **Close project batch:** after the planned authorized phases finish, reconcile latest main normally, perform the end acceptance matrix, open/reuse the cumulative final PR and mark ready. Required remote Quick verification/reviews and current head/base gate the guarded merge. Invoke `finish --required "Quick verification"` once checks are complete; it exits on pending state without waiting or running local quick. Then mirror merged docs once to wiki.

No repeated aggregate suite for record-only edits; rerun affected checks after actual input/environment/base changes or failures. Post-merge SHA/wiki notes go into PR metadata/local pending notes and the next useful commit, never a separate recursive bookkeeping PR. Keep PROGRESS concise; linked records preserve detail/history.

Android emulator/device validation remains deferred until all P phases finish by the 2026-10-02 user directive; the manual batch remains OWED. iOS is archived until explicit resumption. G09 production certification, deployment/tag/public pilot remain separately gated. Local checkpoints do not certify production or declare remote activation.
