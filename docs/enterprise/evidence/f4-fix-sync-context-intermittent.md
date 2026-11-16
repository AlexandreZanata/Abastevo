# F4 fix — the recurring `EnterpriseSyncContextDeviceTest` intermittent

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PARTIAL — host RED/GREEN, suites, lint and builds PASS; native RED reproduced 3/3; native GREEN (20 class runs) OWED
Integration: INTEGRATION_PENDING

Bounded fix on `codex/enterprise-f4-exp004` (worktree
`/data/dev/worktrees/f4-exp004`, F4 head `d9fdb197`) before EMP-EXP-004.
The only Android device used is the owned emulator `emulator-5588`, every
native run under `flock /data/dev/worktrees/.f4-emulator.lock`; the physical
phone was never addressed. Logs: `/data/dev/worktrees/f4-logs/flake-*`
(scripts in `f4-logs/exp004-scripts/`). ID used: EDR-335 (a product behaviour
changes).

## Symptom on record

FLEET-013 (one failure, then eight passing runs), TRIP-005 (once) and DOC-004
(two of five class runs) saw the class fail in
`myAssetsShowOnlyTheSelectedOrganizationsProtectedCacheOnlineAndOffline` with
exactly "My assets not ready at online; … error=STALE; … context_matches=false;
epoch_matches=false; snapshot=false; count=-1; pulls=0". The logs show a
pattern: the failing run was always the first class run after a long pause
(DOC-004: 01:37 and 04:32 failed, 04:32:58, 04:33:47 and 04:34:23 passed);
runs a few minutes apart passed.

## Reproduction (base APK pair `c73fb159…`/`f02d5b72…`, `flake-base-apks.sha256`)

- `flake-base-a` (09:58–09:59Z): run 1 passed; runs 2 and 3 crashed the
  process before any assertion: "WorkManager needs to be initialized via a
  ContentProvider#onCreate() or an Application#onCreate()" from
  `SystemJobService.onCreate` (`flake-base-a-run2.logcat`). `dumpsys
  jobscheduler` showed periodic WorkManager jobs of the validation package
  (one tagged `QualityReportWorker`) enqueued at ~09:49Z, when another
  worktree's run of `EnterpriseContextDeviceTest` had composed the real app
  graph; the READY job was bound into the next class's process.
- `flake-base-b` (after `cmd jobscheduler cancel` of the package, a harness
  step only): 10:03Z passed, **10:04Z failed with the recorded symptom**
  (STALE at "online", pulls=0), 10:05Z passed. The synthetic session had been
  issued by that 09:48Z login with a 900 s access token, so its refresh window
  (the last 60 s) opened at ~10:02:50Z; the failing run performed the
  refresh, which is why the next run passed.

## Root causes

1. **Product (the recorded STALE).** "Meus ativos" (`MyAssetsFlow.pull`,
   EMP-FLEET-013) asks the delivery proof for a lease
   (`source.deliveryAccount()`), which refreshes the session when the access
   token is expired or in its last 60 s. A same-family refresh saves new
   credentials without moving the fence (`EnterpriseFencedSessionStore` locks
   only on an account or family change), but every full-token lease captured
   before it stops being current (`EnterpriseOperationSessionSource` retains
   a lease only while `sessions.load()` equals its captured session). The
   pull then checked `retained(request)` — still holding the lease that
   `open` had captured for the offline read — before adopting the fresh one,
   so the view was cleared as `STALE` and the feed was never contacted. Every
   first online "Meus ativos" after an idle access token failed until the
   member retried. The trip, fueling and checklist owners already adopt the
   delivery lease immediately, and the sync status owner retires the old
   lease explicitly; only this owner had the order reversed.
2. **Test infrastructure (the process crash).** The validation package keeps
   one JobScheduler namespace across classes and runs. Classes that compose
   the real UI graph (`EnterpriseContextDeviceTest`,
   `EnterpriseInvitationDeviceTest`) initialized WorkManager in their own
   process and the app enqueued its periodic work; those system jobs outlived
   the process and, once due, crashed whichever later class was running
   without WorkManager.

Not causes (checked): ViewModel or owner refresh races (the owner held the
right epoch; only the lease identity differed), device clocks (the synthetic
token expiry is wall-clock seconds and the window matched to the minute),
shared protected-store state between methods (each method's graph erases its
own partitions and keys), the first test method (it never asks for a
delivery proof).

## Fix

- `application/.../fleet/MyAssets.kt`: after `deliveryAccount()` the pull
  checks identity, epoch and background, adopts the fresh lease, and only
  then asks whether the view is current (EDR-335). No assertion or other path
  changed.
- `EnterpriseDeviceTestRunner` initializes WorkManager in `onCreate` (before
  the application's onCreate, the instrumentation thread or any service of
  the process) with a delegating worker factory, and cancels the work of
  earlier classes in `onStart` before the first test;
  `EnterpriseContextDeviceTest` and `EnterpriseInvitationDeviceTest` install
  their own Hilt worker factory instead of initializing WorkManager
  themselves (each test now uses its own graph's factory).
- `EnterpriseSyncContextDeviceTest` (method 2) now starts the online stage
  with the synthetic access token inside its last minute on every run and
  asserts after the online stage that the session was refreshed within the
  same account and family — the refresh path is exercised deterministically
  instead of once per 14 idle minutes. Every previous assertion is kept.

## RED → GREEN

- **Host RED** (`flake-host-red.log`, XML in `flake-host-red-xml/`):
  `EnterpriseMyAssetsFlowTest` 13 tests, 1 failing — the new
  `a same-family session refresh during the pull adopts the fresh lease and
  shows the list` ("A same-family refresh is not a lost context ==> expected:
  <null> but was: <STALE>"). The fixture's leases now retain only while the
  session's credentials are unchanged, like the actual source.
- **Native RED** (`flake-red.summary`, `flake-red-run1..3.log`): the base app
  APK (`c73fb159…`, unfixed product) with the new test APK (`eeb89b50…`):
  3/3 class runs failed with the byte-identical recorded diagnostic ("My
  assets not ready at online; … error=STALE; … pulls=0; failure=NONE"). A
  first attempt crashed in the runner before any test (WorkManager was then
  initialized in `callApplicationOnCreate`, after the instrumentation thread
  had started); the initialization moved to `onCreate` — no expectation
  changed.
- **Host GREEN**: `EnterpriseMyAssetsFlowTest` 13/13; enterprise suites
  (`flake-kotlin-suites-lint.log`): domain 4366, application 158, data 205,
  app 30 tests, 0 failures, errors or skips.
- **Native GREEN** (fixed pair: app `b952a46b…`, test `eeb89b50…`,
  `flake-fix-apks.sha256`): OWED — see Validation.

## Validation

```sh
gradle.sh :application:test --tests 'com.anpfuel.application.enterprise.EnterpriseMyAssetsFlowTest'   # RED 13/1, then GREEN
gradle.sh --continue :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test --tests 'com.anpfuel.application.enterprise.*' \
  :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.enterprise.*' \
  :app:assembleDebug :app:lintDebug :data:lintDebug
    # pass; lint 0 errors (app 354 warnings, data 17, as at the FUEL-011 head; none in a touched source)
gradle.sh -PenterpriseDeviceValidation=true :app:assembleDebug :app:assembleDebugAndroidTest   # pass
flake-loop.sh red 3        # base app + new test: 3/3 fail with the recorded STALE
flake-green.sh             # regressions, then 20 consecutive EnterpriseSyncContextDeviceTest class runs
git diff --cached --check  # clean
```

- **Native GREEN — OWED.** The first attempt (`flake-green.sh`: regressions of
  `EnterpriseContextDeviceTest`/`EnterpriseInvitationDeviceTest`, then 20
  consecutive `EnterpriseSyncContextDeviceTest` runs on the fixed pair) never
  reached the loop: from ~10:33Z the whole emulator VM stalled for ~16
  minutes (one logcat line in that window) while the host had ~1 GB available
  and ~24 GB of swap in use (load ~21, several agents' Gradle and Go race
  runs); at 10:51Z the system server restarted ("System has crashed"), the
  graphics composer HAL then failed binder transactions continuously and
  `adb install` hung (`flake-green-regressions-hang.logcat`). Holding the
  emulator lock, the agent rebooted the emulator; it booted at 11:07Z with
  user 0 `RUNNING_LOCKED` behind the emulator's synthetic lock-screen
  credential, which this agent does not have and must not enter, so every
  instrumentation fails ("WorkManager can't be accessed from direct boot")
  — `f4-logs/EMULATOR_5588_LOCKED_AFTER_REBOOT.md`. Owed once the emulator is
  unlocked: `APKDIR=/data/dev/cache/f4-exp004-apks/fix
  f4-logs/exp004-scripts/flake-green.sh` (two regression classes, then 20
  consecutive class runs; the fixed pair is kept with its checksums).

- No Go, SQL, migration, route, OpenAPI, permission or Kotlin protocol
  change; the Go gates do not apply. Secret/PII review of the staged diff:
  synthetic identifiers and the synthetic validation account only.

## Retained

- Other agents' worktrees still carry the old runner until they merge this
  head; their `EnterpriseContextDeviceTest` runs can still leave periodic
  jobs, which this runner now drops (and no longer crashes on).
- Native GREEN (above) and the regression classes on the fixed pair.
- Host memory pressure can stall the shared emulator (see Validation).
- No other owner needed the change: the trip, fueling and checklist owners
  adopt the delivery lease at once, and `EnterpriseSyncFlow` retires and
  replaces it explicitly.
