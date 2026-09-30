# Current execution state

- Updated: 2026-09-30. User authorized phase publication/push; ADR-014 defers real G09 until functional app G18.
- Baseline: P09 INTEGRATED via merged PR #14 (`6f4f048`); issues #11/#12/#15 closed; Quick verification + fast/integration/test SUCCESS; wiki `136c6f9` (112 owned pages).
- G09: RELEASE / DEFERRED_UNTIL_APP_FUNCTIONAL, tracker [#13](https://github.com/AlexandreZanata/brazil-fuel-prices/issues/13). G09-LOCAL INTEGRATED (corrections + required CI); not production certification.
- P12 KMP foundation (branch `codex/phase-12-kmp-foundation`, draft [PR #21](https://github.com/AlexandreZanata/brazil-fuel-prices/pull/21), milestone 3, issues #16–#20): T01 pins, T02 portable money/text/identity + golden vectors, T03 outbox/offline ports, T04 Swift shell (IMPLEMENTED_UNVERIFIED, macOS BLOCKED), T05 acceptance with G12 PARTIAL 3/4. Unmerged; Mac run still gates P12 merge/G12.
- Current phase: P13 free accounts/recovery, branch `codex/phase-13-free-accounts` from `origin/main 6f4f048`; milestone 4; issues #22–#26; draft PR pending first push. Entry: G12 PARTIAL per owner decision (backend account work proceeds while P12-T04 macOS run pends; Mac run still gates P12 merge, G12/G13, native login). Scope: authorized issues/push/draft PR/guarded merge/wiki; no deploy/tag/Release.
- Current task: P13-T02 email access-code backend (issue [#23](https://github.com/AlexandreZanata/brazil-fuel-prices/issues/23)), sliced T02A/B/C per oversize rule. T02A LOCAL_DONE: service + hasher + memstore, 12 unit tests incl. 16-way race green, no DB. Evidence: [P13-T02A record](../release-evidence/p13-t02a-account-service.md). Next: T02B Postgres + migration + real-DB integration.
- Publication/tooling: `scripts/issues.sh` gh-2.45 create fix (test-issues 23/23; same fix rides both phase branches, merges cleanly); P12 labels/milestone plus `phase:P13`/milestone 4 created.
- User choices: 280-char comments/replies; FREE email-code/Google/Apple. Next: P13-T02 email access-code backend (#23).

## Preserved history

- [P01–P09 history](history/P01_P09_PROGRESS_20260930.md), [P09 local-only state](history/P09_LOCAL_PROGRESS_20260930.md), [original roadmap snapshot](history/ROADMAP_BEFORE_MULTIPLATFORM_20260930.md).
- [Delivery workflow](DELIVERY_WORKFLOW.md), [CI cadence](CI_PLAN.md), [fast execution](FAST_EXECUTION.md), [functional plan](MOBILE_DELIVERY_PLAN.md).
