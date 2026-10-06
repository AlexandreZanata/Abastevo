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
