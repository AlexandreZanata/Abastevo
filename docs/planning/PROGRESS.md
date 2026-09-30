# Current execution state

- Updated: 2026-09-30. User authorized phase publication/push; ADR-014 defers real G09 until functional app G18.
- Baseline: P09 INTEGRATED via merged PR #14 (`6f4f048` from `c51fa03` + `8c5e795` + planning/wiki guard). Issues #11/#12/#15 closed; Quick verification + fast/integration/test SUCCESS on `2b1a55a`; wiki `136c6f9` (112 owned pages).
- G09: RELEASE / DEFERRED_UNTIL_APP_FUNCTIONAL, tracker [#13](https://github.com/AlexandreZanata/brazil-fuel-prices/issues/13). G09-LOCAL INTEGRATED: corrections + required current-head CI permit P12 app work; not production certification.
- Current phase: P12 KMP foundation, branch `codex/phase-12-kmp-foundation` from `origin/main 6f4f048`; milestone 3; issues #16–#20; draft [PR #21](https://github.com/AlexandreZanata/brazil-fuel-prices/pull/21) (P12-T01 pushed as `27aa2ad`). Scope: authorized issues/push/draft PR/guarded merge/wiki; no deploy/tag/Release.
- Current task: P12-T05 foundation acceptance/gates, issue [#20](https://github.com/AlexandreZanata/brazil-fuel-prices/issues/20), LOCAL_DONE with G12 PARTIAL (3/4: pins, Android parity 475+169 tests, portable vectors GREEN; Swift/macOS build BLOCKED). New `check-mobile.sh` bounded selection wired into quick-verify (test-gate 13/13), zero dep/manifest drift, no merge claimed. Evidence: [P12-T05 acceptance](../mobile/p12-t05-foundation-acceptance.md).
- Current task: P12-T01 toolchain/feature baseline, issue [#16](https://github.com/AlexandreZanata/brazil-fuel-prices/issues/16), LOCAL_DONE. Pins frozen (Kotlin 2.0.21/AGP 8.7.3/Gradle 8.10.2/KSP 2.0.21-1.0.28/min 26/target 35); Android baseline 597 tests pass (430 domain/app + 167 data/app, 1 skipped), `assembleDebug` SUCCESS. Evidence: [P12-T01 baseline](../mobile/p12-t01-toolchain-baseline.md). iOS needs macOS/Xcode 26.4; not claimed from Linux.
- Publication/tooling: `scripts/issues.sh` gh-2.45 create fix (23/23 harness); created labels `phase:P12`/`type:task`/`priority:must`/`risk:standard`.
- User choices: 280-char comments/replies; FREE email-code/Google/Apple. Next: macOS session (P12-T04 run per `iosApp/README.md`), then phase finish/merge → P13 (#P13-T01…). P12 issues stay open until merge.
- Evidence pointers: [local runtime](../release-evidence/p09-local-runtime-validation.md), [P09-T04](../release-evidence/p09-t04-functional-planning.md).

## Preserved history

- [P01–P09 history](history/P01_P09_PROGRESS_20260930.md), [P09 local-only state](history/P09_LOCAL_PROGRESS_20260930.md), [original roadmap snapshot](history/ROADMAP_BEFORE_MULTIPLATFORM_20260930.md).
- [Delivery workflow](DELIVERY_WORKFLOW.md), [CI cadence](CI_PLAN.md), [fast execution](FAST_EXECUTION.md), [functional plan](MOBILE_DELIVERY_PLAN.md).
