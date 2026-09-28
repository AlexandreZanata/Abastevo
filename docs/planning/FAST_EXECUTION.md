# Fast execution card

Read AGENTS, this card, only the current state in [PROGRESS](PROGRESS.md), and the selected ROADMAP task. Read [DELIVERY_WORKFLOW](DELIVERY_WORKFLOW.md) at phase opening or policy changes, not on every microtask. Search historical evidence by task ID; do not reload the entire audit/log/spec collection.

1. **Locate:** `git status --short --branch`, selected task/dependencies, related code and tests. Compare state with Git; resume owned work instead of restarting it. Use one phase branch; worktree only when isolation requires it.
2. **Bound:** state file areas, B-BR/BUC, risk class and smallest meaningful validation. One task at a time; a phase request may continue through its tasks sequentially without per-task permission prompts.
3. **Prove locally:** domain RED/GREEN/REFACTOR; targeted package and affected consumers; formatting and `git diff --check`; no full-backend/Android/image suite for a docs change. Compile-only `go test -run '^$'` is not behavioral-test evidence.
4. **Protect critical behavior now:** real PostGIS for changed SQL/migrations/transactions; race/concurrent replay/idempotency cases where relevant; auth/ownership/negative inputs, exact money/units, media privacy and fail-partial cases when affected. Never defer a known failure or required risk test to G09.
5. **Record once:** at most ~10 lines per task: ID, branch/commit, commands/result, limits, issue/PR links if real and next step. Put durable evidence in the phase record. Keep PROGRESS under about 60 lines; preserve history in a linked archive, not by deletion.
6. **Publish at the right cadence:** authorized phase execution uses one task commit/push, draft PR throughout phase, one phase merge and one wiki update. Keep tasks open until their PR merges. Do not reread remote status between every local edit; inspect failures or closure state when actionable.
7. **Close batch once:** specialized exit gate → planned `finish` (which runs quick locally and verifies required remote head checks) → merge → wiki snapshot. Do not run the same aggregate again before `finish`. No complete release matrix until the designated immutable release candidate.

Rerun checks when their inputs, environment, base merge or related behavior change, or a prior failure remains unresolved. Do not add repeated checks merely to grow an evidence list. Record runtime/capacity honestly: a targeted pass and an integrated phase are not a certified release.

Current activation status and next task are in PROGRESS. Existing workflow requirements remain in force until G01-FLOW switches them safely; this card alone does not disable CI.
