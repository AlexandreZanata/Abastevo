# DRV-18 — Recovery and clock continuation

Status: IN_PROGRESS. This slice is not DRV-18 acceptance.

## Behavior before implementation

B-BR-DRV-06/07 and BUC-DRV-02 require paused time to remain separate from work,
monotonic clocks bound to the same boot, and explicit review of an interrupted
interval. A restored running shift must not silently credit the interval or
restart GPS. A reboot, clock discontinuity, or missing checkpoint yields a gap;
known previously credited time is preserved. Review can leave a gap unknown;
the UI must not imply its elapsed interval is verified work. Invalid actions
and stale writes must not silently transition a shift.

The current provisional local checkpoint policy is 60 seconds maximum between
checkpoints and 1 second wall/monotonic tolerance, matching existing domain
regression vectors. These conservative values are not a field-validated policy
or acceptance of battery, process lifecycle, concurrent starts, or complete
workday behavior. Failure, rotation, device and accessibility proof remain owed.

## Implementation limitations to verify

A newly created view model conservatively treats an existing RUNNING shift as a
process recovery, including navigation away and back in the same process. The
60-second policy can cause a review when background scheduling delays a
checkpoint; it is not an approved or measured field threshold. A negative wall
clock gap can only be confirmed as zero in this local screen; other durations
remain pending. Session checks before suspension do not serialize account changes
with the Room commit. No lifecycle or device acceptance is implied.

## Current local validation

The focused app JVM test and app/data debug compilation passed, and the focused
app test and compilation passed again after removal of unsafe automatic GPS
stop calls. Five `DriverLocalStoreTest` Room cases passed
on the owned API35 emulator through direct `adb` instrumentation. The first
Gradle connected-instrumentation attempt failed because Gradle reported no
connected device; that attempt is not a pass. Mobile static guards and
`git diff --check` passed.

## Outstanding proof

This remains a partial slice: complete manual workday, process kill/reboot,
rotation/background, gap review, accessibility and account changes need actual
UI/device tests. GPS permission revocation, service lifecycle, MAP coexistence,
field distance/battery, and policy/distribution acceptance remain unproved.
Checkpoint interval and tolerance need an admitted operational policy and field
measurement; transient scheduling delays may require conservative gap review.
Local Room atomicity does not serialize an account change with the transaction.
No complete DRV-18 or release/field acceptance is claimed.

Commands used (the app test and app compilation were repeated after the GPS stop removal):

```sh
./gradlew :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.driver.work.DriverWorkShiftTransitionTest' :app:compileDebugKotlin :data:compileDebugKotlin --no-daemon --no-parallel --max-workers=2 --console=plain
bash scripts/check-mobile.sh --static-only
adb install -r data/build/outputs/apk/androidTest/debug/data-debug-androidTest.apk
adb shell am instrument -w -r -e class com.anpfuel.data.local.DriverLocalStoreTest com.anpfuel.data.test/androidx.test.runner.AndroidJUnitRunner
```

Before the subsequent DRV-21 implementation, source inspection identified the
following GPS risks; this records the earlier state, not current acceptance: `DriverGpsService.finish()` launches a
write in a service scope that `onDestroy()` cancels; a repeated `begin()` can
replace the listener reference without removing the previous listener; and
`finish()` looks up the current owner instead of binding the initiating owner.
No device test of these observations is claimed. They are independent DRV-21
obligations and are not covered by the shift tests above.

Automatic GPS stop calls added during this continuation were removed: the
existing stop helper can start a service even when none is running, including
from background, and can fail after a Room commit. Consequently GPS may
continue when a shift pauses or enters recovery; this is unsafe and remains a
critical DRV-21 obligation. Source inspection also identifies the cancellation,
listener and owner-binding risks above. GPS lifecycle and owner fixes plus
actual failure and device tests are required before either mode is accepted.
The same earlier inspection found the service wrote `complete=true` and
`hasGap=false` even with no usable fixes, gaps or outliers, and bypassed the
existing GPS quality model. GPS distance/coverage must not be accepted as proven
until that path is corrected and exercised; no device result is implied.

## Real screen and Room continuation

The actual `DriverWorkScreen` and `DriverWorkViewModel` were exercised with
file-backed Room and synthetic owners on an owned API35 emulator: **3/3 passed**
after increasing the emulator memory to 4 GiB. The tests exercise two quick
start requests (one stored shift), pause/resume without crediting the paused
interval, closure and a second shift, recovery when a new ViewModel is created,
and explicit reload as another owner while preserving the first owner's row.

Two earlier instrumentation attempts ended with `Process crashed` and no test
results; emulator logs showed the low-memory killer terminating the app with
2 GiB configured. They were not passes and do not establish a code failure.
This test does not kill the app process, rotate or recreate the Activity, reboot
or simulate an account change during an outstanding Room commit. It does not
establish a complete two-shift workday with personal kilometres or GPS behavior.
Those proofs and the other obligations above remain open.

The passing emulator was `anpfuel-16k` API35, started read-only without a
snapshot and with `-memory 4096`. Commands:

```sh
./gradlew :app:assembleDebug :app:assembleDebugAndroidTest --no-daemon --no-parallel --max-workers=2 --console=plain
adb install -r app/build/outputs/apk/debug/app-debug.apk
adb install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
adb shell am instrument -w -r -e class com.anpfuel.app.ui.driver.work.DriverWorkScreenRoomTest com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
```

## Subsequent owner and legacy-gap correction

After the 3/3 screen run, review found that a ViewModel initialized for owner A
could load a persisted RUNNING shift for owner B without explicit recovery, and
legacy rows marked `reviewed=true` with no reviewed duration were hidden despite
their unknown time. The local continuation binds initialization to the owner,
fences action refreshes to the captured owner, and treats such legacy gaps as
pending until an explicit duration is recorded. The legacy data are preserved.

Two new real Room/screen regressions cover owner A → B with B's existing
running shift and the previously reviewed flag with unknown work duration.
The earlier 3/3 run predates these source and test changes; the later six-case
rerun is recorded below. Account observation while the screen is idle and account changes
during a suspended database commit remain unresolved and unaccepted.

## Owner, legacy-gap and current-vehicle checkpoint rerun

On the owned API35 emulator configured with 6 GiB RAM, all six
`DriverWorkScreenRoomTest` scenarios passed, including the subsequent owner
switch, legacy unknown-gap and current-vehicle checkpoint regressions. This
supersedes the earlier 3/3 evidence for these changed inputs only. In the same
instrumentation run a GPS test failed; the aggregate result is **6 PASS / 1
FAIL**, not a passing batch. The GPS failure and its correction require a
separate successful rerun. There is still no process-death, rotation, reboot,
account-change-during-commit or complete-workday acceptance.

A preceding 4 GiB attempt stopped during its third case and produced no
complete batch result. In the 6 GiB run,
`DriverGpsServiceTest.explicitStopPersistsIncompleteOwnerScopedObservation`
failed before exercising the intended behavior with `ClassCastException`:
the Hilt singleton did not implement the test-only dependency interface. This
is an observed test-configuration failure, not evidence of successful GPS stop,
owner isolation or persistence. A corrected test and successful rerun remain
required.

## Lint and corrected GPS test follow-up

The initial Android lint run **FAILed** with 28 `MissingTranslation` findings
for the driver hub and charging-fee resources. All 28 keys were added in each
of the seven existing non-default locales; XML parsing, key parity and
`git diff --check` passed. At that point the corrected resources still required
a lint rerun; the later result is recorded separately below.

The six screen/Room scenarios passed before the subsequent resource-only
changes. The corrected GPS instrumentation emitted an individual test status
of 0, but the instrumentation process crashed before its final result. That
observation is **not an accepted GPS test pass**; a later isolated complete
rerun is recorded separately below. GPS lifecycle and ownership, field and battery, account
change during an active operation, process death, and full-workday acceptance
remain open.

The failed lint was run using:

```sh
./gradlew :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --no-daemon --no-parallel --max-workers=2 --console=plain
```

The incomplete corrected GPS attempt ran on the API35 emulator with 4 GiB:

```sh
adb shell am instrument -w -r -e class com.anpfuel.app.ui.driver.DriverGpsServiceTest com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
```

It emitted individual status `0`, followed by
`INSTRUMENTATION_RESULT: shortMsg=Process crashed`, with no complete successful
instrumentation result.

## Subsequent isolated GPS and lint reruns

After the seven locale corrections, `:app:lintDebug`, both debug APK assemblies
and the mobile static guard passed. This is a subsequent result; the initial
28-error lint failure above remains part of the record. On the owned API35
emulator configured with 4 GiB, the corrected isolated GPS test completed with
the runner's final **OK (1 test)**. Fine-location permission was granted before
the test. This proves only the exercised synthetic stop/owner-scoped persistence
case; it does not turn the earlier crashed run into a pass or certify DRV-21.
The six DRV-18 screen/Room cases passed before the later resource-only and debug
test-dependency changes. No new six-case rerun is claimed for those edits.

```sh
./gradlew :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --no-daemon --no-parallel --max-workers=2 --console=plain
bash scripts/check-mobile.sh --static-only
adb shell pm grant com.anpfuel.app.debug android.permission.ACCESS_FINE_LOCATION
adb shell am instrument -w -r -e class com.anpfuel.app.ui.driver.DriverGpsServiceTest com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
```

GPS security and lifecycle work remains open: session changes while no location
callback arrives, session changes during a Room transaction, permission loss
without a callback, automatic stopping on pause/end, durability failures, GPS
quality/coverage, coexistence, physical battery and field proof. See the
[GPS lifecycle contract](../../../product/DRV_21_GPS_LIFECYCLE_CONTRACT.md).
The inherited global domain coverage gate also remains open: the last recorded
measurement was 81.81%, below the required 90%, and was not remeasured here.
No DRV-18, DRV-21, release or field acceptance is claimed.

## New shift/GPS integration — validation initially pending

The source subsequently changed to integrate GPS stopping with shift pause,
end and recovery. All JVM, app build, lint, static and Android test results
above precede this integration and **do not validate the new source**. The
At that point relevant targeted tests, app compilation, lint and static checks
were pending; their later results are recorded below. Earlier failures and
passes remain historical evidence, and safety, lifecycle and field obligations
remain open until supported by appropriate proof.

## Targeted checks after shift/GPS integration

The focused `DriverWorkShiftTransitionTest` and `DriverGpsLifecycleTest` JVM
tests, app and instrumentation APK assemblies, app lint, mobile static guard
and `git diff --check` passed after the integration. The combined Android run
on the owned `anpfuel-16k` API35 emulator, read-only without snapshot and
configured with 6 GiB, completed **OK (8 tests)** after the explicit fine
location permission grant. Earlier failures and crashes remain historical
results, not reclassified.

```sh
./gradlew :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.driver.work.DriverWorkShiftTransitionTest' --tests 'com.anpfuel.app.ui.driver.work.DriverGpsLifecycleTest' :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --no-daemon --no-parallel --max-workers=2 --console=plain
bash scripts/check-mobile.sh --static-only
git diff --check
adb shell pm grant com.anpfuel.app.debug android.permission.ACCESS_FINE_LOCATION
adb shell am instrument -w -r -e class com.anpfuel.app.ui.driver.work.DriverWorkScreenRoomTest,com.anpfuel.app.ui.driver.DriverGpsServiceTest com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
```

These are targeted results only. The 60-second checkpoint gap and one-second
wall tolerance used locally are taken from test vectors and have not been
formally admitted as operational policy; the driver contracts require a
caller-supplied reviewed policy. Session/logout changes are not serialized with
a suspended Room commit, and GPS must still prove behavior when the account or
permission changes without a callback. Quality, real device battery, field,
process-death, accessibility and complete-workday proof remain open. The last
recorded global domain coverage was 81.81% against a 90% gate and was not
remeasured in this slice. DRV-18 and DRV-21 remain IN_PROGRESS; no batch,
integration, release or field acceptance is claimed.
