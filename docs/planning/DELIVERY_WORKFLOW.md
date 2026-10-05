# Project batch delivery workflow

Policy: [ADR-018](../adr/018-project-batch-delivery.md), explicit user request 2026-10-05. It supersedes per-phase PR/CI/merge/wiki cadence in ADR-013 and older task plans. Main protection remains required; no remote setting changes are implied by a local edit.

## Opening and isolation

Inspect status/diff, Git history and task dependencies before editing. Work sequentially through the authorized scope. Keep one phase branch `codex/phase-NN-slug`; use a worktree when another effort occupies the checkout. Preserve other edits and unmerged work. One atomic commit per task, one phase milestone and real task issues when publication is authorized. Reconcile existing records across open and closed issues by stable task IDs; never duplicate/reopen completed work. Planning-only requests do not create every future issue or authorize product implementation.

Start from clean main or an isolated worktree detached at the verified base:

```sh
bash scripts/git-flow.sh start --phase 25 --slug catalog
```

## Construction loop — no remote wait

Read selected task/rules → document behavior → TDD/DDD implementation → meaningful affected tests and immediate critical risk checks → diff/secret review → evidence/atomic commit → authorized branch backup push. Phase branch pushes do not trigger the current main-only push workflows. Do not open a PR solely to keep a phase moving. Existing drafts may remain review references but are not local progress gates.

No `finish`, CI API polling, fixed waits, aggregate release runs or wiki publication between construction phases. Fix a known failure before dependent work. A phase entry requiring source functionality can consume its preceding tested local checkpoint on the dependency branch; a deployment/public pilot/certified release entry cannot.

## Local phase checkpoint

Run the declared specialized local exit checks once. Commit a regular tracked Markdown evidence file containing these exact lines, followed by actual tested behavior revision, commands/results, risk cases, task IDs, unresolved nonblocking limits and next phase:

```text
Status: LOCAL_DONE
Validation: PASS
```

The declaration is an operator assertion of completed local acceptance, not a shortcut around testing. Do not mark PASS with known missing required local tests. Device/manual and production-environment obligations explicitly deferred by the user remain named and OWED; source acceptance must not impersonate those results.

```sh
bash scripts/git-flow.sh checkpoint --evidence docs/<phase-record>.md
bash scripts/git-flow.sh start --phase 26 --slug discovery --from-checkpoint
```

The helper requires a clean phase branch, committed evidence and matching phase state. It pins HEAD/evidence blob in private worktree Git state and declares INTEGRATION_PENDING. The next phase includes that head by ancestry. Missing/stale checkpoints refuse continuation. Never change the base with `--base` when using `--from-checkpoint`. `--dry-run` is read-only. The original `sync` command merges the recorded parent, not main; use an explicit normal main merge at final closure.

Checkpoint files are local conveniences, not shared authority or new versioned product entities. On a fresh clone/worktree, resume from versioned phase evidence and verify the exact source head before creating state through a new phase start; do not copy another worktree's state blindly. Keep phase issues open until actual integration.

## Final project batch integration

1. Freeze the final cumulative branch containing all authorized phase commits. Confirm complete task range, accepted exit evidence and outstanding obligations. Fetch current main and merge it normally if needed; rerun affected checks after changes. No force push/rebase of published history/admin bypass/direct-main development.
2. Perform the union of end acceptance checks once. The 2026-10-02 deferred Android manual/device batch is due after all P phases; iOS remains explicitly archived. Missing external production requirements leave G09 UNCERTIFIED. Never certify deployment/pilot from local checks.
3. Push the final head and open/reuse one final batch PR to main. List included phases and accepted task issues. `Closes` applies only to completed tasks; no closure of partial work or certification trackers with unmet requirements. If configured review constraints require multiple PRs, prepare them at final closure without imposing earlier construction waits.
4. Mark the final PR ready. Draft jobs are deliberately skipped; drafts can never be merged by the helper. Ready-for-review and subsequent changes run Quick verification plus affected path-selected backend/Android signals. Known non-required failures also require correction; their tests remain part of task acceptance.
5. Once current required checks/reviews are complete, invoke `bash scripts/git-flow.sh finish --pr <actual-number> --required "Quick verification"`. It refuses dirty/wrong/main trees, draft/stale/missing/failed/skipped/cancelled state and main divergence. Pending CI exits immediately; no sleep/poll loop or local aggregate on pending CI. Read check state only when finalization is actionable; do other authorized work or report CI_PENDING instead of staying idle.
6. The helper runs local quick once after remote checks pass, then guarded merge with `--match-head-commit`, no protection bypass. Verify tested PR head/base and workflow identity including GitHub's PR merge-ref mapping in final review; changed inputs invalidate evidence. Preserve commits, verify ancestry in fetched main, then safely delete the merged final branch locally/remotely. Cleanup of earlier stacked branches requires proven merge ancestry and owner/worktree coordination; never delete unmerged or occupied work.
7. Record actual merge/check results, close eligible issues through the merged PR and sync the owned wiki once from merged source. On wiki failure record INTEGRATED / WIKI_PENDING; retry documentation publication only.

Post-merge bookkeeping goes in PR metadata/local pending notes, then in the next useful authorized commit. Do not open a second PR solely to record the first PR or create a commit referencing its own SHA/wiki result. Preserve pending notes when switching worktrees. No new local Git state or plan is proof of remote activation.

## Issues and authorization

Use stable task IDs, source anchor/SHA, B-BR/BUC, dependency checkpoint, risk class, tests/acceptance and rollback in [task issues](templates/TASK_ISSUE.md). Retain one milestone per phase and preserve human comments/assignees/labels/closed state. Reconcile idempotently and paginate. Permission/API errors stop the remote operation without duplicate creation; record ISSUE_PENDING against the local stable task ID and continue authorized local construction instead of waiting for bookkeeping. Record retryable state, not success. Default states: PLANNED → IN_PROGRESS → LOCAL_DONE / INTEGRATION_PENDING → INTEGRATED. Do not fabricate remote IDs.

Verify repository origin and actual protection/permissions before publication. Reuse existing explicit session authorization for routine scoped delivery; reference projects/documents do not grant unrelated authorization. This workflow edit alone does not merge the existing PR #103, deploy, tag or run future features.

## Wiki synchronization — one update per final merged batch

Canonical content is committed Markdown; publish an allowlisted snapshot from the merged SHA, not an uncommitted working tree. Expected wiki target is `AlexandreZanata/brazil-fuel-prices.wiki.git`, resolved/verified independently because it is a second remote. Creation/initialization/publication is part of the separately recorded wiki scope; do not silently enable a disabled wiki. A missing wiki can leave the phase with WIKI_PENDING while `docs/` remains readable in the main repository.

Plan Home, Roadmap, Architecture, Backend API, Domain/Community Rules, Security/Privacy, Operations, Decisions and Progress navigation. Recursively map public `docs/` sections (including backend/security/product/adr) plus selected root README/ROADMAP/TRADEMARKS; exclude operational secrets, local logs and private data by allowlist. Rewrite internal links/anchors to generated pages, code/file links to immutable source permalinks, and image references to the committed public assets. Detect filename collisions; flattening every README to the same name is prohibited.

Use a manifest of **owned generated pages**, source SHA and content hashes. Preserve manual/unmanaged wiki pages. Delete only a previously owned page proven obsolete and unchanged since its last managed version; if a human edited a managed page, surface a conflict instead of overwriting it. Dry run must make zero commits/pushes/mutations. Unchanged snapshot produces no commit. Wiki commit records source SHA; synchronization completion/remote commit is recorded in phase publication evidence. Never use a blanket deletion of all wiki Markdown files.

## Release checkpoints

G09-LOCAL opened functional app work. P24/G24 is accepted by explicit user decision with end manual batch OWED, not manufactured device evidence. Historical P18/G18 stays unaccepted and iOS stays archived. Production P09/G09 requires its complete immutable candidate and real infrastructure/security/privacy/restore/load evidence; public P10 pilot and P11 releases require their separate gates. Unavailable production infrastructure does not prevent local source construction, but does prevent RELEASE_CERTIFIED/deployment/public pilot.
