# P01-T18 — Project batch flow validation

Date: 2026-10-05. Scope: explicit user cadence change; no backend/app behavior, remote merge, release or deployment.

Status: LOCAL_DONE
Validation: PASS

## Source and isolation

Branch: `codex/phase-01-project-batch-flow`; worktree: `.worktrees/abastevo-project-batch-flow`; original base `4e1c73a`. Existing primary follow-up/pilot work and the uncommitted station planning worktree are preserved. Remote main advanced to `59ea845` during this work (PR #103 merged by the other effort); this task did not perform that merge. Final local branch reconciles that source normally before delivery.

Implemented: ADR-018/canonical instructions, local phase checkpoints and parent branches, worktree-private state, nonblocking finalization, draft refusal and main freshness/branch guards, final local quick after remote success, draft job conditions and ready events in three workflows. Removed the duplicate Makefile quick recipe while changing the helper test target. No runtime dependency added: helpers use Bash/Git/Python standard library; the cadence harness has no external Python dependency.

## Targeted results

- RED: the new checkpoint/continuation harness failed against the original helper (unknown checkpoint command).
- GREEN: `make test-flow` passes — 32 legacy/finalization assertions, local checkpoint/dirty/missing/stale/dry-run/dependency ancestry/worktree-isolation scenarios, and 2 static workflow cadence tests.
- The checkpoint harness installs a failing `gh` stub: any remote call during phase construction fails the test. It proves no CI/PR/merge API call is needed to continue locally.
- Pending required CI invokes neither local quick nor merge. Final successful synthetic integration runs local quick once. Linked finalization leaves the other worktree's main untouched and safely deletes the merged final branch.
- `bash -n scripts/git-flow.sh scripts/tests/test-git-flow.sh scripts/tests/test-project-batch-flow.sh`: PASS.
- `shellcheck` on those three shell files: PASS, no findings.
- Workflow YAML parsed independently with the locally available PyYAML BaseLoader; 3 files valid. This parser is a local validation tool, not a new project dependency. Cadence tests assert trigger/job policy but do not emulate GitHub's event engine.
- Changed documentation link targets, `git diff --check` and secret-surface review: PASS. No credential, private image, GPS or production data added.

## Limits and next action

LOCAL_DONE / INTEGRATION_PENDING. GitHub workflow rollout is PENDING; required protection was not edited or bypassed. Existing runs/PRs/issues/wiki were not changed by this task. Synthetic checks are not live GitHub evidence. Continue authorized source phases from exact local checkpoints; final project closure prepares the cumulative PR and verifies actual required current head/base/reviews/workflow identity before merging. Do not run release/Android/device/PostGIS campaigns for this process-only change. User-deferred Android manual validation remains OWED; iOS remains archived; real-production G09 remains UNCERTIFIED.

A checkpoint validates the committed operator acceptance declaration, not every product criterion. Keep actual task/risk results in versioned phase evidence. After a relevant code/environment/base change, rerun affected checks before creating another checkpoint. Post-merge notes ride the next useful commit; do not open a bookkeeping-only follow-up PR.
