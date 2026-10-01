# Current execution state

- Updated: 2026-10-01. User authorized phase publication/push; ADR-014 defers real G09 until functional app G18.
- Baseline: P09 INTEGRATED via merged PR #14 (`6f4f048`); issues #11/#12/#15 closed; Quick verification + fast/integration/test SUCCESS; wiki `136c6f9` (112 owned pages).
- G09: RELEASE / DEFERRED_UNTIL_APP_FUNCTIONAL, tracker [#13](https://github.com/AlexandreZanata/brazil-fuel-prices/issues/13). G09-LOCAL INTEGRATED (corrections + required CI); not production certification.
- P12 KMP foundation INTEGRATED via merged PR #21 (`bf40a44` from `82dd951`; match-head-commit, Quick verification success; branch deleted locally + remotely, verified); issues #16–#20 closed by the merge; wiki `5c0426f` (117 owned pages from `bf40a44`). G12 ACCEPTED-BASIC per owner decision 2026-09-30 (pins/Android-parity/portable-vectors/static gates green; macOS device confirmation deferred to release, never claimed).
- P13 free accounts/recovery INTEGRATED via merged PR #27 (`fb5ac45`; match-head-commit, Quick verification success; branch deleted locally + remotely, verified); issues #22–#26 closed by the merge; wiki `9d699f1` (from `fb5ac45`). G13 holds with device evidence deferred to release per owner decision.
- P14 station fuel feedback INTEGRATED via merged PR #33 (`47bdffe` from `b72a7e3`; match-head-commit, Quick verification success; branch deleted locally + remotely, verified); issues #28–#32 closed by completion comments; wiki `1ac13fc` (138 owned pages from `47bdffe`). G14 holds per exit battery (real-DB export/erase, stats/tally rebuild equality, erase races green).
- P15 lightweight photos and 24-hour audit expiry INTEGRATED via merged PR #39 (`207936b` from `6280dbc`; match-head-commit, Quick verification success; branch deleted locally + remotely, verified); issues #34–#38 closed by completion comments; wiki `5778f42` (138 owned pages from `207936b`). G15 holds with device evidence deferred to release per owner decision (recorded, never claimed).
- P16 location integrity across Android and iOS INTEGRATED via merged PR #44 (`cb131b0` from `90ed059`; required Quick verification + fast/integration/test SUCCESS on head; branch deleted locally + remotely, verified); issues #40–#43 closed; wiki `f13b029` (151 owned pages from `cb131b0`). G16 holds (simulated GPS blocks claims; UNKNOWN honest; device matrices deferred, never claimed).
- P10 functional app integration INTEGRATED via merged PR #54 (`be680d1` from `470c7ff`; required Quick verification + test SUCCESS on head; branch deleted locally + remotely, verified); issues #45–#52 closed by the merge; #53 pilot stays OPEN deferred until G18/G09; wiki `285be47` (from `be680d1`). G10-LOCAL holds (T01–T08 contracts/cache/keys/OCR/outbox/UI/confirm/offline green; lint 23 pre-existing/P13; app connected MIUI-gated CI-bound, never claimed).
- Current phase: P17 social app functionality and iPhone parity (T01–T04), branch `codex/phase-17-social-iphone-parity` from `origin/main be680d1`; milestone 9; issues #55–#58; draft PR #59. Entry: G10-LOCAL + G13–G16. Scope: authorized issues/push/draft PR/guarded merge/wiki; no deploy/tag/Release.
- Current task: P17-T04 cross-platform lifecycle and privacy exit (issue #58) LOCAL_DONE, phase implementation complete awaiting exit gate + merge: same-fixture revocation/erasure/expiry/denial/no-PII exit (JVM 6/6 green, XCTest 6 Mac-gated), full Android battery + static/diff/secrets green. Evidence: [P17-T04 record](../release-evidence/p17-t04-lifecycle-exit.md) + [mobile exit](../mobile/p17-t04-lifecycle-exit.md). Next: phase exit gate → guarded merge of PR #59 (P17 INTEGRATED).
- Publication/tooling: `scripts/issues.sh` gh-2.45 create fix (test-issues 23/23); phase labels/milestones created through P17 (milestone 9, issues #55–#58, `phase:P17` label created).
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

- P00-T02 LOCAL_DONE on the same isolated branch/base (uncommitted): native SVG (~23 KB), Android XML and iOS vector PDF saved under docs/assets/brand; original PNG preserved. Evidence/imports: docs/assets/brand/README.md and vector-provenance.json.
- Vector checks: 32–4096 px render/visual inspection, SVG safety, SVG/Android path-gradient parity, Android aapt2 compile/link and PDF zero-raster inventory PASS; no device/runtime integration. Next: review brand preview, then authorized publication separately; banner approval remains pending.

- P00-T03 LOCAL_PREVIEW_DONE / AWAITING_VISUAL_APPROVAL (same isolated branch; uncommitted): reference uppercase text traced as independent SVG; V2 README combines both native SVG assets. Original-logo bytes/geometry unchanged; SVG safety, renders and glyph IoU 0.990820 PASS. Evidence: docs/brand/IDENTITY.md / wordmark-provenance.json. No remote publication or runtime change.
