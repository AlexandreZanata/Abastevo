# P18 local integration exit — G18 remains blocked

Maintainer requested phase finalization/GitHub publication on 2026-10-01 and confirmed no Mac is available. Publication scope: integrate the validated local slice through PR #67 and mirror its merged SHA; do not mark missing acceptance green or close incomplete issues/milestone. No G18/G09 acceptance, tag, deployment or pilot.

## Reconciled source

Base `fd3dbcd7e5eb7cd8b1ee6e58c72615156011bba9`, merged normally into phase head `766b445066ae9942c4fa4482cd6d5b110e5710d0` before exit. Task commits: 3b302e7 (test-harness mutex + matrix), cf452b1 (local baselines), 111efe6 (security/compatibility review). The only executable diff is the integration-test stub mutex; production code, migrations and dependencies are unchanged. Earlier broad JVM/backend/load/security evidence is linked from task records and remains applicable to unchanged inputs.

## Specialized exit checks on the reconciled tree

- PASS: `go test -race -count=5 -tags=integration -run '^TestPGBindRaceAdmitsExactlyOneOwner$' ./internal/modules/account/adapters` from backend; real digest-pinned PostGIS in a unique tmpfs validation Compose project. Five repetitions passed; container removed afterward. Log /tmp/p18-exit-bind.log.
- PASS: `go vet ./internal/modules/account/adapters`.
- PASS: `bash scripts/check-mobile.sh --static-only`, `bash scripts/check-compat.sh`.
- First attempt blocked before execution: `./gradlew :app:connectedDebugAndroidTest -Pandroid.testInstrumentationRunnerArguments.class=com.anpfuel.app.startup.AppStartupPerformanceTest,com.anpfuel.app.community.CommunityPriceDisplayDeviceTest --no-daemon`; APK installation refused with INSTALL_FAILED_USER_RESTRICTED. Android was physically connected. After maintainer released installation and requested retry, the exact same command PASSED 2/2, failures/errors/skips 0 (JUnit XML); startup-to-home measured 382 ms against the existing 2000 ms test limit. Log /tmp/p18-exit-device-retry.log; Android model 2311DRK48G / Android 16. This startup test measures the cached-home readiness check, not a complete cold-process/frame/heap/battery benchmark.
- iPhone/macOS/Xcode: unavailable, confirmed by maintainer. No native/device result claimed.
- Full private-S3 emulator matrix: earlier digest pull refused; still unproven. No replacement dependency/image or production storage has been introduced to manufacture a pass.

## Gate semantics and remaining work

P18 local implementation/evidence integration is distinct from complete functional acceptance. Issues #60–#62 retain their required feature/device/performance/native proof; PR #67 references them without closing keywords. Milestone 10 remains open. G18 is BLOCKED / NOT_ACCEPTED; G09 remains deferred and uncertified. Required local quick runs once through `finish --required "Quick verification"`; current-head/base remote quick and any reported failing checks must be resolved before merge. Post-merge evidence goes to PR metadata/local record without direct bookkeeping push to main.


Post-merge local record (pending next authorized branch): PR #67 merged as 9c090c6577b9cedb6119f9d5f8f8de96f2c6aa96; required Quick verification / fast / integration SUCCESS on 37e21cc / base fd3dbcd; local full quick PASS once in 12 s. Wiki b4b6c97e575fbe997f6f15cb32c165052d897759 published the merged snapshot. Primary dirty checkout preserved; remote/isolated-clone branch cleanup complete, original occupied local branch retained. G18 NOT_ACCEPTED; issues #60–#62 and milestone 10 remain open.
