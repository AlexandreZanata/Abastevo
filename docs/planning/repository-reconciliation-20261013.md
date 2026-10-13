# Repository reconciliation — 2026-10-13

Status: INTEGRATION_PENDING. User authorization: reconcile all progress branches into protected main with valid CI, preserving every completed change and unfinished draft. This supersedes the earlier chat-only publication restriction. No deployment or release certification is authorized by this integration. Local integration is complete on `codex/main-reconciliation` at `3dfeb9fd`; dev promotion, cumulative PR/CI and guarded merge remain pending (no PR/CI/merge waits during construction per ADR-018).

## Rules before conflict resolution

- B-BR-RECON-01: preserve original commit ancestry, snapshots of detached worktree tips and foreign dirty files. Only retire a branch after its exact tip is an ancestor of integrated main. Keep maintained dev and main.
- B-BR-RECON-02: existing Android schema 11 adds the vehicle photo column and has already been installed. Preserve that migration/schema unchanged; the independently created premium tables move to append-only migration 11→12. Prove actual SQLite upgrade/data retention and merged Room validation before publication.
- B-BR-RECON-03: maintain all existing security/privacy gates. An uncommitted removal of intake assurance is preserved as an explicitly non-executable historical patch, not silently promoted over the tested protections.
- B-BR-RECON-04: reconcile product intent and preserve older artwork/docs as historical assets where newer designs supersede them. No blanket ours/theirs resolution. Run affected meaningful checks and all applicable current-head CI; merge with Quick verification, head/base guards, no bypass.

Private recovery snapshot: `.local/reconcile-main-20261013/all-refs.bundle`, 22 worktree tip archive refs, five dirty-worktree patches and non-ignored untracked documents. Original checkouts/files untouched. Work is isolated in `.worktrees/main-reconciliation`.

[Exact preserved dirty snapshots](history/reconciliation-20261013/README.md) retain all five drafts without overwriting current specifications.

## Integrated merges (local, `codex/main-reconciliation` @ `3dfeb9fd`)

- `games` `b11501b6` → merge `5c5acca1`. Five resolved files keep the Games route/screen/Profile tool and the newer removal of SEARCH/PRICES; PROGRESS keeps current records plus the games paragraph. Targeted games JVM tests passed pre-commit; full `:domain:test` + `:app:testDebugUnitTest` re-run post-integration: 846 tests, 0 failures/errors/skips.
- `refs/heads/landing` `c0b6e408` → merge `04195dab`. Only PROGRESS conflicted; 143 files (landing pages, assets, guides) auto-merged. Landing typecheck/build/check + 45/45 node tests PASS on the merged tree.
- `codex/landing-hero-art` `dbd1a2aa` → merge `83e6ec22`. Five files conflicted (headline/copy/brand/hero style + PROGRESS); every hunk resolved to the newer landing fusion intent ("codex wins", road-life headline, trimmed brand, green emphasis, dropped caption), verified as zero removed lines vs HEAD with hero-art-only additions preserved. Landing suite re-run: 45/45 PASS.
- `codex/landing-local-dev` `ae018992` → ancestry-only merge `3dfeb9fd` (`-s ours`, empty content diff). Its 13 unique 2026-10-11 commits (SEC01-D..SEC04-C) are superseded by newer SEC01..SEC09 + intake assurance + migrations 59..90 on this line; a content merge would revert tested gates (proven: branch tip drops Store intake locking/health/timeout). No security regression; old planning records not promoted.
- Ancestor proof on `3dfeb9fd`: `games`, `landing`, `codex/landing-hero-art`, `codex/landing-local-dev`, `codex/premium-backend`, `codex/mobile-sync-delivery`, `origin/main` (`b7800b2b`), plus retirement candidates `account-keyboard-fix`, `android-dev-preview-key`, `owner-consent`, `ingestion-bench-plan`, `premium-fuel`, `landing-reconcile-dev`. Backend/ has zero diff vs `dca4dbce`; prior premium/Room evidence stands. Full cumulative CI gates the dev → main PR before any merge.
