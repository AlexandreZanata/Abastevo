# Current execution state

- Updated: 2026-10-01. User authorized phase publication/push; ADR-014 defers real G09 until functional app G18.
- Baseline: P09 INTEGRATED via merged PR #14 (`6f4f048`); issues #11/#12/#15 closed; Quick verification + fast/integration/test SUCCESS; wiki `136c6f9` (112 owned pages).
- G09: RELEASE / DEFERRED_UNTIL_APP_FUNCTIONAL, tracker [#13](https://github.com/AlexandreZanata/brazil-fuel-prices/issues/13). G09-LOCAL INTEGRATED (corrections + required CI); not production certification.
- P12 KMP foundation INTEGRATED via merged PR #21 (`bf40a44` from `82dd951`; match-head-commit, Quick verification success; branch deleted locally + remotely, verified); issues #16–#20 closed by the merge; wiki `5c0426f` (117 owned pages from `bf40a44`). G12 ACCEPTED-BASIC per owner decision 2026-09-30 (pins/Android-parity/portable-vectors/static gates green; macOS device confirmation deferred to release, never claimed).
- P13 free accounts/recovery INTEGRATED via merged PR #27 (`fb5ac45`; match-head-commit, Quick verification success; branch deleted locally + remotely, verified); issues #22–#26 closed by the merge; wiki `9d699f1` (from `fb5ac45`). G13 holds with device evidence deferred to release per owner decision.
- P14 station fuel feedback INTEGRATED via merged PR #33 (`47bdffe` from `b72a7e3`; match-head-commit, Quick verification success; branch deleted locally + remotely, verified); issues #28–#32 closed by completion comments; wiki `1ac13fc` (138 owned pages from `47bdffe`). G14 holds per exit battery (real-DB export/erase, stats/tally rebuild equality, erase races green).
- P15 lightweight photos and 24-hour audit expiry INTEGRATED via merged PR #39 (`207936b` from `6280dbc`; match-head-commit, Quick verification success; branch deleted locally + remotely, verified); issues #34–#38 closed by completion comments; wiki `5778f42` (138 owned pages from `207936b`). G15 holds with device evidence deferred to release per owner decision (recorded, never claimed).
- P16 location integrity across Android and iOS INTEGRATED via merged PR #44 (`cb131b0` from `90ed059`; required Quick verification + fast/integration/test SUCCESS on head; branch deleted locally + remotely, verified); issues #40–#43 closed; wiki `f13b029` (151 owned pages from `cb131b0`). G16 holds (simulated GPS blocks claims; UNKNOWN honest; device matrices deferred, never claimed).
- Current phase: P10 functional app integration after G09-LOCAL (G10-LOCAL T01–T08; T09 pilot deferred until G18/G09), branch `codex/phase-10-functional-integration` from `origin/main cb131b0`; milestone 8; issues #45–#53; draft PR #54. Entry: G16 (P16 integrated). Scope: authorized issues/push/draft PR/guarded merge/wiki; no deploy/tag/Release.
- Current task: P10-T08 integrated offline and release checks (issue [#52](https://github.com/AlexandreZanata/brazil-fuel-prices/issues/52)) LOCAL_DONE: i18n backfill 21 community keys x6 locales (P10 MissingTranslation zero) + CaptureScreen copy to resources + 2 community_privacy_* notices in vote panel + heading semantics + v4→v6 chain test + V5→V6 strengthened + seeder all migrations + outage/additive-field tests + 2 real on-device bugs fixed (dead station_id index dropped from MIGRATION_4_5; Vehicle test id-compare; coroutines-test dep for androidTest), full ./gradlew test green, assemble green, data connected GREEN on physical device (44 tests, 0 failed), lint 23 = 22 auth P13 + 1 pre-existing, app connected compiles but MIUI install-gated (CI-bound). Evidence: [P10-T08 record](../release-evidence/p10-t08-offline-release.md). Next: phase exit gate → finish → merge (G10-LOCAL).
- Publication/tooling: `scripts/issues.sh` gh-2.45 create fix (test-issues 23/23); phase labels/milestones created through P10 (milestone 8, issues #45–#53, `phase:P10` label created).
- User choices: 280-char comments/replies; FREE email-code/Google/Apple.

## Preserved history

- [P01–P09 history](history/P01_P09_PROGRESS_20260930.md), [P14 task detail](history/P14_PROGRESS_20260930.md), [P15 task detail](history/P15_PROGRESS_20260930.md), [P09 local-only state](history/P09_LOCAL_PROGRESS_20260930.md), [original roadmap snapshot](history/ROADMAP_BEFORE_MULTIPLATFORM_20260930.md).
- [Delivery workflow](DELIVERY_WORKFLOW.md), [CI cadence](CI_PLAN.md), [fast execution](FAST_EXECUTION.md), [functional plan](MOBILE_DELIVERY_PLAN.md).

## Isolated brand preview (2026-10-01)

- Task P00-T01: selected name abastevo and supplied logo; local documentation plus README artwork awaiting visual approval.
- Branch codex/phase-00-abastevo-brand-preview; worktree .worktrees/abastevo-brand-preview; base origin/main be680d1. Ongoing P17 checkout remains separate.
- Publication scope LOCAL_PREVIEW_ONLY; no remote issue/PR/merge/wiki update. This preview does not advance a functional or production gate.
- Artifact: docs/assets/brand/abastevo-readme-banner-v1.png; README.brand-preview.md. Original logo copied byte-for-byte; no runtime code changed.
- Evidence/approval source: docs/brand/IDENTITY.md. State LOCAL_PREVIEW_DONE / AWAITING_VISUAL_APPROVAL; not INTEGRATED.
