# ADR-020: Maintained dev and protected main

Status: Adopted by explicit user request, 2026-10-06.

## Decision

Consolidate the completed construction branches through cumulative PR #122, preserving commit ancestry. Delete obsolete local/remote branch refs only after their tips are contained in merged main. Dirty historical worktrees retain their files and revision; detach their branches without overwriting or deleting drafts. Keep only main and dev as maintained branches.

Subsequent source changes start on dev synchronized with main, are committed and pushed to dev, and reach main through a protected PR. No direct development push to main. The same current-head/base checks, meaningful affected tests and no-bypass rules remain mandatory. This supersedes ADR-018 phase branch creation/cleanup cadence for future ordinary work, while preserving existing task evidence and release gates. dev is retained and fast-forwarded after guarded merges.

An existing clean delivery branch may use `bash scripts/git-flow.sh finish --base origin/main --pr NUMBER --required "Quick verification"`. The explicit base is restricted to origin/main, permits recovery from stale private phase state, and does not skip remote head/base, draft, required-check, auth or merge guards. Legacy phase finish still deletes only its verified merged branch. dev finish retains dev.

The user's request to close all existing issues is an administrative backlog reset. Implemented source tasks close through the actual merged PR; incomplete acceptance/release/pilot trackers close as not planned, with obligations preserved in [the consolidation record](../planning/repository-consolidation-20261006.md). Closure never certifies incomplete runtime/security/storage/device acceptance. No deploy/tag/pilot is implied.

## Operating sequence

1. `git fetch origin`, `git switch dev`, and `git merge --ff-only origin/main` when clean and behind. If main and dev diverge, normal merge and revalidate affected behavior; never reset or force push.
2. Make one bounded change with immediate risk tests, record truthful evidence, commit and push dev.
3. Reuse the dev → main PR; final readiness requires the affected local checks and all applicable current-head remote checks.
4. Run guarded finish with the configured required context; main and dev are then synchronized. Preserve manual wiki pages and mirror the merged source once when publication is authorized.

## Validation

The git-flow harness covers state-free explicit-base delivery, rejection of arbitrary bases and missing checks, zero-mutation dry runs, retained local/remote dev, synchronization and legacy branch/worktree cleanup. Production private representation is deliberately unavailable until its integration is proved; see B-BR-PROFILE-CONTAINMENT-01.
