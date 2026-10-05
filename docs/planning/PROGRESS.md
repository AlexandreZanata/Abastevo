# Current execution state

- 2026-10-05: user authorizes final Android UI, device acceptance and guarded cumulative integration. Branch `codex/phase-32-profile-ui`, inherited P33 `72b9e8f` (P34–P38 already ancestors), normal main merge `e7a1bff`. Existing phase branches/worktrees preserved.
- P32-T01A LOCAL_DONE: public HTTP profile/allowlist and canonical UUID Compose route from server station detail; current badge independent of prices, failed-refresh fallback explicitly stale with management disabled. Profile data/app tests + `assembleDebug` PASS (56s); diff-check and secrets PASS. [Evidence](../mobile/p32-ui-completion.md). Next: P32-T02A real claim transport/SAF.
- Inherited P32/P33 code checkpoints are not complete UI/device acceptance. P33 and catalog backend commits remain unmerged; required current-head/base CI, guarded merge and wiki are pending.
- G24 previously accepted BY USER DECISION (`aedfdd3`), not device evidence; new physical/manual proof remains due. Historical G18 NOT_ACCEPTED; iOS DEFERRED_EXPLICIT_RESUME_ONLY. G09 UNCERTIFIED; no production deployment/tag/public pilot follows from this task.
- Connected-device inventory to verify: runner ADB reports SM-T515 plus emulator; user reports Poco X6. Match physical model/serial before recording device evidence.
- P26 website guides INTEGRATED via PR #117 (`938fc1f`); website task IDs overlap catalog P25/P26 historically. Preserve both source plans and use explicit scope.
- Policy: ADR-018 cumulative final PR/check/merge/wiki, normal main merge, no bypass/history rewrite; critical task tests immediate. Required check: `Quick verification` on verified current head/base, `git-flow.sh finish --required "Quick verification" --pr <actual-number>`.
- Staging last verified trust attempt failed (`curl` 60 / Fortinet); live contracts/provider/private storage/restore/load stay unresolved until actual evidence. Private proof/session data never enter Git/logs/fixtures.
- [Preserved prior current state](history/P32_UI_OPENING_20261005.md), [batch evidence](../release-evidence/batch-p32-p33-closure.md), [workflow](DELIVERY_WORKFLOW.md).
