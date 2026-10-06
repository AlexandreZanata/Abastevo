# P32-UI-HOME-MINIMAL — bounded Home cleanup

Status: LOCAL_DONE
Validation: PASS

User scope (2026-10-06): remove the blank strip above the bottom navigation
icons and temporarily remove the Home "Update price" floating action. Code
changes only; no ADB/device interaction after the user's clarification.

Base: `9fe9af3`, the clean `codex/ui-explore-footer-spacing` worktree head,
including the previous footer removal and compact three-tab navigation.
Isolated branch: `codex/phase-32-home-minimal`. The occupied P32 checkout
and existing UI worktree are preserved.

Presentation contract: B-BR-C01/BUC-C01 discovery retains price/source and
staleness metadata; B-BR-C02/BUC-C02 contextual capture remains available.
The shell owns bottom-navigation/system padding and consumes it before child
screen scaffolds calculate their safe area. No hard-coded system-bar height,
new dependency, backend, domain, storage or permission change.

Checks: existing HomeViewModel and NavigationTab unit tests, debug APK build,
Android instrumentation source compilation, lint, diff/secret-surface review.
Light/dark visual acceptance uses the supplied screenshots as the reference;
runtime visual verification remains unperformed under the code-only request.

Remote issue/PR/push/merge/wiki are outside this request. Integration pending.
Rollback: revert this task's presentation commit, retaining the inherited UI.

## Source validation

`ANDROID_HOME=/data/dev/android/sdk/Sdk ./gradlew :app:testDebugUnitTest
--tests '*HomeViewModelTest' --tests '*NavigationTabTest' :app:assembleDebug
:app:compileDebugAndroidTestKotlin :app:lintDebug --console=plain` PASS,
2m 1s. HomeViewModel: 4/4; NavigationTab: 6/6; zero failures/errors/skips.
APK and instrumentation source compilation PASS. No instrumentation executed.
Lint: zero errors, 162 warnings. One unused accessibility-string warning follows
the temporary FAB removal; its existing translations are retained for restoration.
The initial command failed before task execution because ANDROID_HOME and
ANDROID_SDK_ROOT differed; using the configured SDK path resolved it without
repository/environment configuration edits.

`git diff --check` PASS. Scoped added-source secret review PASS: only the
inset-consumption import, modifier and comments were added. No device install,
launch or visual verification after the code-only clarification. The inherited
three-tab geometry/colors and system navigation safe area are preserved.

## Follow-up — symmetric bottom navigation spacing

2026-10-06: the user subsequently authorized ADB installation on the Poco.
The `fa18d04` APK was installed with `install -r`, opened successfully and
visually checked in dark mode: no Home floating action or duplicate inset strip.
This supersedes the earlier code-only runtime limitation for that scoped check.

New bounded request: add a small top gap matching the existing 8 dp gap
between the tab group and the lower system safe area. Use one shared 8 dp
vertical-padding value above icons and below labels; retain system insets,
the three tabs and the Home action removal (B-BR-C01 / BUC-C01–C02).
Checks: existing NavigationTab tests, debug APK build, lint, diff/secret review;
install on the explicitly identified Poco and inspect the resulting Home.
Follow-up validation: PASS / LOCAL_DONE / INTEGRATION_PENDING.

`ANDROID_HOME=/data/dev/android/sdk/Sdk ./gradlew :app:testDebugUnitTest
--tests '*NavigationTabTest' :app:assembleDebug :app:lintDebug --console=plain`
PASS (52s), 6 tests, zero failures/errors/skips; lint zero errors, the same
162 warnings. `git diff --check` and scoped secret-surface review PASS.

Poco Android APK updated with `adb -s <identified-phone> install -r`; Success.
Cold launch: Status ok, TotalTime 1137ms. Dark-mode Home screenshot inspected:
small top margin added, lower margin/system safe area preserved, all three
tabs visible, Home update action absent. Screenshot retained only in `/tmp`;
no device data committed. Other connected surfaces were not targeted.
APK SHA-256: `28ec8df2ba30d5b054b7487c5c277207af39df2c4b3928f4301a008eae73ec2d`.
Light-mode runtime/full commercial acceptance were not claimed by this check.
