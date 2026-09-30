# Phase delivery: branches, issues, pull requests and wiki

Status: ACTIVE since G01-FLOW; helpers are present and remote protection was verified on 2026-09-30 with required **Quick verification**, strict up-to-date base and enforced admin protection. [CI_PLAN](CI_PLAN.md) defines the target check cadence; [FAST_EXECUTION](FAST_EXECUTION.md) is the short daily card.

## Reference and adaptation

Reviewed Goyim-Arena at local HEAD `33a458f52016338ac86cd68c34e476491b47307c`: `AGENTS.md`, `.local/GIT_FLOW.md`, `.local/FAST_EXECUTION.md`, `.local/git-flow.sh`, `docs/CI.md`, `Makefile`, `.github/workflows/quick.yml`, `verify.yml` and the PR template. Its operating model is targeted local tests → quick integration check → complete release certification. Its remote quick check still runs on draft PR updates; “full CI at the end” does not mean accepting untested microtasks or red checks.

Adapt the process, not that product's stack, financial rules, repository credentials, labels or P45 numbering. Here G09-LOCAL backend integration permits app work; complete real-production certification G09 is deferred until functional app G18, per ADR-014. Shared scripts, manifests and process instructions belong in versioned `scripts/` and `docs/planning/`, not ignored `.local/`. Local logs/cache may remain ignored. The reference repository's publication authorization does not grant permission for mutations in this repository.

## Units of work and ownership

- One bounded **phase** is an integration batch with one milestone, branch and PR; existing P01…P11 IDs stay stable; new P12…P18 follow the explicit dependency graph. If a phase is too large, define a coherent subphase with explicit task ranges and exit criteria before starting. Do not create one branch/PR per microtask by default or a single branch for the entire backend.
- One **microtask** is one behavior/change, one issue and one atomic implementation commit after its local acceptance tests. Corrections to already published commits use additional explicit fix commits; never rewrite history to manufacture one-commit purity.
- Issues describe task scope and evidence; Git/PRs record actual integration; ROADMAP owns task definitions; PROGRESS is a small current-state index; docs own technical truth; wiki is a generated public reading surface. Avoid maintaining independent copies of the same acceptance rules by hand.
- Phase implementation is complete locally before integration; integration is complete after verified PR merge; release certification is a separate state. A merged phase on `main` is not permission to deploy the backend.

## Isolation and branches

Default: `codex/phase-02-official-catalog`, `codex/phase-03-anonymous-identity`, etc., created from the verified latest `origin/main`. The current planning adjustment uses `codex/phase-01-delivery-plan`; workflow implementation may use `codex/phase-01-delivery-flow`. Neither implies that P01-T13…T17 are already implemented.

Before starting, inspect branch/status/diff, reconcile current-state notes with actual commits and identify ownership of any existing edits. A clean dedicated checkout can use a branch directly. Use a separate Git worktree when another task occupies the checkout, when two authorized efforts need isolation, or when release evidence needs a clean immutable checkout. Do not stash, reset or overwrite someone else's work. No subagents or parallel tasks are implied by this policy.

Update a stale phase with a normal merge of `origin/main`; test the conflict resolutions, create a new commit and revalidate the resulting head. No force push, bypass/admin merge, `--no-verify`, destructive reset/clean or pushing development commits directly to main. After a verified merge, always delete the phase branch locally (`git branch -d`) and remotely (`git push origin --delete`), then verify both deletions; a merged phase branch is never left stale. Unmerged work is never deleted — safe deletion refuses it.

## Phase opening — one preparation pass

1. Confirm phase entry gate, exact task range, branch/base SHA, risk/exit checks and active authorization scope.
2. Find/reuse the milestone and existing issues by stable task IDs across open AND closed records; create only missing records for this phase. Never reopen completed P01 work because a stale document says NOT STARTED.
3. Create/switch to the phase branch. A draft PR can be created once the branch has its first real commit; do not manufacture an empty product change to open it earlier.
4. Link issue IDs, milestone, source task anchors and local validation plan in the PR. Track identifiers once in a versioned phase ledger; no invented issue/PR numbers.

## Per-microtask loop

Read the short state and selected task → implement documented rule/test → run targeted acceptance and risk checks → inspect diff/secret surface → update task evidence/current state → commit only owned files → push the phase branch when remote delivery is authorized.

Keep the PR in draft during the batch. The small remote check may run after each push; it is not necessary to block the next independent microtask waiting for every green notification. A reported failure or broken prerequisite must be investigated and corrected before dependent work, and always before ready/merge. Record scope/commands/outcome in the commit/issue evidence rather than publishing a comment for every tool call. Skip remote status polling during ordinary local edits.

An issue stays open with status LOCAL_DONE/AWAITING_PHASE_MERGE until the phase PR merges. PR `Closes #...` references only tasks whose acceptance criteria really passed. Do not close issues for partial work, a docs-only plan, an unrelated PR or an expected CI result.

Commit prepared task/phase evidence before final verification; identify the tested code tree or preceding behavior commit rather than requiring a document to contain its own commit SHA. Record post-merge check/merge/wiki outcomes in PR metadata and the local phase record, then commit the durable summary on the next authorized branch. Do not push a bookkeeping commit directly to main or trigger a recursive wiki sync just to record the wiki's own commit. A local pending summary must be preserved when switching worktrees.

## Phase closure — integration gate

After the final task, run the phase-specific exit checks once on the final tree, then use the `scripts/git-flow.sh finish --required "Quick verification"` interface to:

1. Reject a dirty/unowned tree, main branch, wrong repository, unresolved scope, missing exit evidence or base divergence.
2. Run the local quick gate once; do not wrap it in a second aggregate quick/full run. Evidence is reusable only when command, environment and all relevant inputs/tree are unchanged.
3. Push final head and mark the existing PR ready; require PR review/conversation resolution according to configured protection.
4. Wait for the named required `Quick verification` check and any other required checks on the **current expected head**. Missing, pending, skipped, neutral, cancelled, timed-out, unavailable or failed required results do not count as success. Verify workflow identity/provider as well as name; a similarly named unrelated status is insufficient.
5. Verify the PR's head SHA equals local expected SHA and its base is up to date. Account for GitHub's PR test-merge SHA: map a run to its PR head/base pair, never confuse a tested merge ref with an arbitrary branch commit. Head/base changes invalidate prior approval evidence.
6. Merge with a compare-and-swap head guard (`--match-head-commit` or equivalent) and merge-commit method to preserve task commits, without overriding protections. Fetch and fast-forward local main only after ancestry checks; then always delete the merged phase branch locally (`git branch -d`) and remotely (`git push origin --delete`) and verify both deletions. Never leave a merged phase branch stale; never delete unmerged work.
7. Record merged SHA/PR/check evidence, let linked issues close, and request wiki synchronization of that merged snapshot. If wiki fails, record INTEGRATED / WIKI_PENDING and retry only documentation publication; do not rerun the backend suite or claim the wiki is current.

When a relevant test fails, fix the cause on the same phase branch and rerun affected checks on the new head. Never reduce thresholds, add skips, suppress errors or hide test absence to accelerate delivery. The script must refuse unknown remote/check state rather than infer green.

## Issue and milestone design

Default issue title: `P02-T01 — Shared ANP fixtures`. Fields: phase/task ID, source anchor and source SHA, goal, scope/exclusions, B-BR/BUC, dependencies, affected areas, risk class, tests-first and validation commands, acceptance, rollback, expected Conventional Commit, and phase/release gate distinction. See [task template](templates/TASK_ISSUE.md).

Milestone: `P02 — Official catalog and ANP ingestion`. Small label set: `phase:P02`, `type:task|bug|docs`, `priority:must|should|later`, `risk:critical|standard|docs`; add `needs-review` only with a defined reviewer workflow. Never imply automated review that did not run.

Reconciliation is idempotent and paginated: list existing records first, match a machine-readable task marker (not a loose title prefix), detect duplicates, preserve user comments/assignees/custom labels and closed state. Managed body sections may be updated if their source changes; do not overwrite human discussion or close unknown issues. A permission/API failure stops remote mutation and records a retryable operation; it must not create duplicates on the next run.

Before the first remote phase, verify origin points to `AlexandreZanata/brazil-fuel-prices`, issue/merge/milestone capabilities and actual branch protection. Once remote phase delivery is authorized in the session, reuse that authorization for routine planned actions; do not request approval per issue/commit. A request only to adjust the plan does not itself publish this edit, create all issues, merge code or push the wiki. Record `publication_scope` as LOCAL_ONLY or the authorized operations; never inherit another repository's authorization.

## Wiki synchronization — one update per merged phase

Canonical content is committed Markdown; publish an allowlisted snapshot from the merged SHA, not an uncommitted working tree. Expected wiki target is `AlexandreZanata/brazil-fuel-prices.wiki.git`, resolved/verified independently because it is a second remote. Creation/initialization/publication is part of the separately recorded wiki scope; do not silently enable a disabled wiki. A missing wiki can leave the phase with WIKI_PENDING while `docs/` remains readable in the main repository.

Plan Home, Roadmap, Architecture, Backend API, Domain/Community Rules, Security/Privacy, Operations, Decisions and Progress navigation. Recursively map public `docs/` sections (including backend/security/product/adr) plus selected root README/ROADMAP/TRADEMARKS; exclude operational secrets, local logs and private data by allowlist. Rewrite internal links/anchors to generated pages, code/file links to immutable source permalinks, and image references to the committed public assets. Detect filename collisions; flattening every README to the same name is prohibited.

Use a manifest of **owned generated pages**, source SHA and content hashes. Preserve manual/unmanaged wiki pages. Delete only a previously owned page proven obsolete and unchanged since its last managed version; if a human edited a managed page, surface a conflict instead of overwriting it. Dry run must make zero commits/pushes/mutations. Unchanged snapshot produces no commit. Wiki commit records source SHA; synchronization completion/remote commit is recorded in phase publication evidence. Never use a blanket deletion of all wiki Markdown files.

## Release checkpoints

P01–P08 and follow-up backend/app phases integrate through targeted tests, specialized phase checks and short CI. G09-LOCAL opens functional app work. P18/G18 proves integrated Android/iOS functionality locally. Only afterward P09/G09 selects one immutable real-production candidate and runs the complete backend/account/social/media/location plus real infrastructure/security/privacy/restore/load matrix. A failed candidate prevents certification/deploy and requires a new candidate after fixes; never move a published tag. Public P10-T09 pilot requires G09 RELEASE_CERTIFIED. P11 commercial releases have separate certification.

See [G01-FLOW tasks in ROADMAP](../../ROADMAP.md#delivery-flow-transition) for the implementation work. Those helpers now exist; use their verified interfaces. Planning future phases does not execute their implementation or create every future issue.
