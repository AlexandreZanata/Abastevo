# Community-first PRM planning validation — 2026-10-13

Continuation 2026-10-14: the subsequent explicit request authorized source implementation and GitHub PR. PRM15–PRM20 and PRM21 local source acceptance are recorded in [execution evidence](../backend/evidence/community-safety/PRM_COMMUNITY_FIRST.md); staging/device execution remains OWED and PRM22 DEFERRED. The original planning-only record below is historical, not the current execution state.

Status: PLANNING_LOCAL_DONE / IMPLEMENTATION_PLANNED

Scope: the maintainer requested additional phases in the PRM plan for execution by another agent. [Canonical amendment and tasks](PREMIUM_BACKEND_APP_PLAN.md#2026-10-13-community-first-planning-amendment--prm15prm22); [reusable handoff prompt](PRM_COMMUNITY_FIRST_PROMPT.md). No implementation task is accepted by this document.

## Opening source and isolation

The primary checkout was dev at e22824db, with an unrelated uncommitted edit in backend/internal/modules/evidence/adapters/store.go. It was inspected and preserved. The PRM plan is absent from that dev revision but exists on codex/premium-backend at a104ed80, alongside ongoing Premium implementation. Its clean worktree and actual task checkpoints were inspected without editing it. This amendment uses a separate detached planning worktree from a104ed80; only documentation changes are included. Reconcile/cherry-pick the scoped planning commit into the future maintained-dev implementation checkout without importing unrelated Premium code.

The original 62 task IDs and OneSignal's 17 task IDs are preserved. New IDs are PRM15-T01 through PRM22-T03: 24 tasks, eight phases, 103 total tasks/23 phases. PRM15–PRM21 contain 21 initial-policy tasks; PRM22 contains three deferred future tasks. Numbering does not impose a billing/paywall dependency on the free-community track.

## Inspected implementation facts

- backend/internal/modules/community/application/photo_capture.go uses PhotoCaptureRadiusM = 150 and normal CheckPhotoCaptureLocation admission, requiring location, reviewed station geometry and valid fix classification. DevelopmentPreview is a separate timed/allowlisted policy. Normal community-first capture must be a new explicit policy rather than promotion of that preview bypass.
- backend/internal/modules/community/application/submit_location.go already permits absent optional observation location; supplied claims are verified and may yield proximity/travel signals. Therefore removing the camera barrier alone is insufficient: downstream policy/trust weighting must be audited without claiming every observation currently requires GPS.
- app/src/main/kotlin/com/anpfuel/app/capture/CaptureOcrViewModel.kt exposes city-scoped searchPreviewStation through the existing Directory gateway and has normal/development authorization branches. The requested normal search can reuse this experience without recreating the removed standalone search page or requiring developer credentials.
- tools/station-prep and the Rust plan already distinguish registry identity/address from optional coordinate enrichment. PRM16 explicitly starts with regression/inventory and changes only demonstrated gaps; it does not assume all parser rows currently require coordinates.
- COMMUNITY_SAFETY_EXECUTION_PLAN.md keeps helpfulness separate from price consensus/confidence. The amendment preserves that separation while requiring a usable no-proximity community-validation path and existing reports/authorized moderation.

These are source observations, not fresh runtime/device/staging evidence. Old location tests/evidence remain historical for their tested revision. Active contract supersession is an explicit PRM15 implementation prerequisite; no runtime requirement was silently changed here.

## Local document validation

Performed against the final planning files:

- Programmatic task inventory: original PRM00–PRM14 task headings/bold IDs unchanged; 24 unique sequential PRM15–PRM22 IDs; 103 unique task definitions total; all new tasks have dependencies/rules, outcome/areas, tests first and acceptance/recovery.
- Local Markdown link/path/anchor checks for the three amendment documents and the changed PROGRESS entry; referenced source areas confirmed present at the inspected revisions.
- Policy/ordering review: backend before consuming Android, no mandatory GPS/150m initial barrier, no fabricated proximity trust, no paid participation, no helpfulness-to-price conflation, versioned legacy compatibility, strict >90% plus product validation and explicit future authorization.
- Scoped documentation diff/whitespace and high-confidence secret-surface review. No credentials, real contributor locations or production images introduced.

Results: programmatic Python document audit **PASS** (79 previous task definitions unchanged; 24 new tasks with all four required fields; 103 unique IDs/23 phases; local paths/anchors valid). `bash scripts/validate-repo-baseline.sh` **PASS**, including its tracked secret guard. `git diff --check` and new-document `git diff --no-index --check /dev/null <document>` checks **PASS**. The separate scoped pattern scan covers the new Markdown files as well as tracked planning changes. Manual policy/dependency review **PASS**; all new runtime tasks remain unstarted.

No backend/Rust/Android suite, national coverage measurement, provider/device/staging operation, new issue/PR, push/merge/wiki/deployment or policy activation is performed or claimed by this planning amendment. Existing unrelated implementation/check failures remain owned by their original evidence; no new passing runtime assertion replaces them.

## Next action

Have the implementation agent read PRM_COMMUNITY_FIRST_PROMPT.md after an explicit scoped construction request. Reconcile this planning amendment with current maintained dev and actual source, then start PRM15-T01. Preserve current in-flight work and validate affected dependencies before behavior. Keep PRM22 deferred; the user's >90% condition is a future prerequisite, not a measured fact or automatic switch.
