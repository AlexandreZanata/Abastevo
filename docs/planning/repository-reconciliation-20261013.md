# Repository reconciliation — 2026-10-13

Status: IN_PROGRESS. User authorization: reconcile all progress branches into protected main with valid CI, preserving every completed change and unfinished draft. This supersedes the earlier chat-only publication restriction. No deployment or release certification is authorized by this integration.

## Rules before conflict resolution

- B-BR-RECON-01: preserve original commit ancestry, snapshots of detached worktree tips and foreign dirty files. Only retire a branch after its exact tip is an ancestor of integrated main. Keep maintained dev and main.
- B-BR-RECON-02: existing Android schema 11 adds the vehicle photo column and has already been installed. Preserve that migration/schema unchanged; the independently created premium tables move to append-only migration 11→12. Prove actual SQLite upgrade/data retention and merged Room validation before publication.
- B-BR-RECON-03: maintain all existing security/privacy gates. An uncommitted removal of intake assurance is preserved as a private patch, not silently promoted over the tested protections.
- B-BR-RECON-04: reconcile product intent and preserve older artwork/docs as historical assets where newer designs supersede them. No blanket ours/theirs resolution. Run affected meaningful checks and all applicable current-head CI; merge with Quick verification, head/base guards, no bypass.

Private recovery snapshot: `.local/reconcile-main-20261013/all-refs.bundle`, 22 worktree tip archive refs, five dirty-worktree patches and non-ignored untracked documents. Original checkouts/files untouched. Work is isolated in `.worktrees/main-reconciliation`.
