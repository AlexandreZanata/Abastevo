# Phase ID — Execution record

## Current state

- Definition commit and task range:
- Publication scope (LOCAL_ONLY or authorized operations):
- Repository / base SHA / phase branch / optional worktree:
- Milestone / task issue IDs / draft PR (actual values only):
- Next task and blockers:
- Integration status and tested head/base:
- Wiki source SHA, owned-manifest version, remote commit/status:
- Release status (not implied by phase integration):

## Task evidence

One short entry per task: ID; relevant commit/tree; targeted commands and real outcomes; risk cases; unresolved limitations; issue link; next task. Link bulky logs/artifacts. Keep prior evidence; superseding entries identify what changed and why earlier results no longer apply.

## Phase closure

Specialized gate evidence, final quick local/remote result, required reviews/conversations, compare-and-swap merged head, merge SHA, issue closure and wiki sync status. Separate CI_PENDING/WIKI_PENDING from true completion. No complete release claim without the candidate matrix.

Prepare evidence before final checks. Record post-merge results in PR metadata/local state, then preserve the summary in the next authorized branch; no direct-main bookkeeping commit or self-referential SHA/wiki publication loop.
