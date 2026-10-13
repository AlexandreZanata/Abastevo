# Games visual polish and emulator validation

Date: 2026-10-11. Branch: `games`, opening head `b610583`.
Status: SCOPED_EMULATOR_PASS / INTEGRATION_PENDING.
Global acceptance: ACCEPTANCE_BLOCKED (pre-existing domain coverage gate). The user explicitly authorized
GitHub branch backup and emulator validation/polish, superseding the earlier
device deferral for this Games scope only. Preserve all other checkouts,
backend/data/application/iOS code and existing game physics/version.
GitHub accepted the initial seven commits via configured origin and reported
its canonical rename to `AlexandreZanata/Abastevo`; do not change shared remotes.

## Bounded microtasks

- GME-M08: run existing emulator cases, capture baseline and regression findings.
- GME-M09: upgrade native scenes/HUD, readable silhouettes and input feedback.
- GME-M10: upgrade selector/tutorial/results/control hierarchy and game previews.
- GME-M11: emulator regressions, offline navigation/play/pause/records, screenshots,
  scoped unit/lint/APK/size evidence and branch backup.
Each implemented microtask has an atomic commit and authorized branch push.

## Behavior and acceptance contracts

B-BR-GME-P01: presentation/input feedback may change, but `mobile-v1` physics,
collision/scoring/order, fixed clock and bounded records remain unchanged.
A frame must reuse brushes/paths/shaders and bounded decorative objects;
no downloaded assets, new engine/dependency/backend or network requirement.
B-BR-GME-P02: preview art, coherent scene colors, clear primary actions and
score/time labels make each game recognizable. Input has immediate visual
feedback; reduced motion disables decorative animation; mute remains default.
B-BR-GME-P03: text/menu controls remain readable in light/dark themes, portrait,
landscape and 2x font scale; no fixed text height clipping. Existing explicit
pause/resume and failed-record feedback remain intact.
BUC-GME-P01: offline guest sees previews, chooses either game, learns controls,
plays using gestures/buttons, pauses/backgrounds/resumes, finishes and restarts.
BUC-GME-P02: process/activity recreation abandons unfinished games and completed
bests survive; screenshots contain synthetic game data only.

Use only serial `emulator-5580`, API 26 x86_64 / Nexus 5X 1080x1920 @420dpi,
1536 MiB RAM, software graphics, read-only AVD session and no snapshot save.
No physical-phone install, data wipe, account/profile operation, backend call,
PR/merge/wiki/deployment or release certification. Emulator behavior cannot
certify physical-device battery/thermal/audio/TalkBack or real hardware FPS.
The pre-existing global domain coverage failure remains an integration blocker;
keep the threshold and other agents' code unchanged.

## Evidence

GME-M08 LOCAL_DONE: initial four instrumented cases PASS (2.439s). Added a
visual-review case and synchronized Compose/PixelCopy captures; final baseline
5/5 PASS (4.254s), without network (airplane flag 1, Wi-Fi 0, mobile data 0;
no active connectivity agent). `svc wifi disable` did not change the actual
Wi-Fi state on this AVD; verified Settings UI switch was used instead.

Commands: `./gradlew :app:assembleDebugAndroidTest --offline --console=plain
--max-workers=2`; `adb -s emulator-5580 install -r <debug APK/test APK>`;
`adb -s emulator-5580 shell am instrument -w -r -e class
com.anpfuel.app.games.GamesScreenDeviceTest
com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner`.
Captured selector, both tutorials/playfields and pause overlay. PixelCopy
captures contain the current rendered frame, unlike unsynchronized automation
screenshots. Inspected baseline shows text-only selection cards, flat scenery,
a small player silhouette and unused letterbox space. No behavior defect was
found in the existing four cases. Next: M09 native scene/HUD/input feedback.
Diff/secret scan PASS. GitHub branch backup accepted at `b610583`.


GME-M09 LOCAL_DONE (scoped renderer): original shared vector scene painter, cached gradients/brush/path/typeface,
layered Flight landscape and detailed pump silhouettes, dusk Run road/station
and distinct cone/beam/truck silhouettes, mascot visor/feet/shadows, bounded
input rings/pickup glow, localized fitting HUD and three-minute progress bar.
Static preview compositions share this painter without a running session.
Reduced-motion removes decorative trail/feedback/gait/scroll; input haptics honor
system settings. Existing `mobile-v1` domain source is byte-unchanged.
9 games app unit cases PASS; debug and instrumentation APK builds PASS (20s).
5/5 offline API 26 emulator cases PASS (3.945s); current-frame Flight/Run
screenshots visually inspected. No native crash or UI regression in these cases.
M09 lint PASS (zero errors, 177 existing warnings, no games diagnostic),
diff/secret review PASS; next M10 shell polish.


GME-M10 LOCAL_DONE (scoped shell): illustrated adaptive selection cards share native art without frame
callbacks; clear play actions, local-best icon/labels, settings group, centered
scrollable tutorial/pause/results cards over the scene, large primary result,
and labeled 52dp directional/action controls. Inactive control panels disappear
from menus. Material ripple and native input rings provide interaction feedback.
9 affected unit cases PASS; lint/debug/test APK PASS (38s). Visual inspection
found native preview paint escaping its Compose bounds into descriptive text.
Added a pixel regression: RED expected white (-1), actual scene (-14524300).
Explicit native clipping fixes the defect; proportional center-crop avoids
stretching the mascot. Six emulator cases (including regression) passed after
clipping (6.86s); final proportional preview check 6/6 PASS (7.389s).
Final lint/debug/instrumentation APK PASS (39s), zero lint errors and no games
diagnostic. Final preview inspected: clear text background and undistorted
mascot. Diff/secret review PASS. Next M11 expanded acceptance.


## GME-M11 acceptance

Expanded real native interaction cases cover runner swipe/lane buttons/jump/slide,
Flight finish/restart/button/tap, persisted settings, Portuguese dark-theme play,
landscape controls/pause/resume and 2x-font tutorial/start/actions. Ten portrait
cases passed before the final landscape repair (15.58s). The initial landscape
precondition read a stale application configuration; the corrected fixture fixes
UiAutomation rotation, verifies the actual Activity configuration and releases it
in teardown. A subsequent RED case exposed a product defect: starting in
landscape moved AndroidView from Column to Row, detaching it and pausing the
session. Keep the native stage in the same Row slot regardless of game phase;
only the sidebar changes visibility. Final GREEN evidence is recorded below.

Real MainActivity guest navigation Profile → Games was exercised offline.
Background/foreground showed Game paused / Continue. Rotating the Activity
restored the selector and discarded the unfinished game. Some selection buttons
were below the fold in landscape; reachability is verified with scrolling rather
than falsely assuming every button is simultaneously visible. Actual process
force-stop/relaunch (different process IDs) followed by Profile → Games → Flight
returned How to play / Start, without Continue or data wipe.

GME-M11 LOCAL_DONE (scoped emulator/polish): final portrait 10/10 PASS
(15.717s), final landscape 2/2 PASS (3.18s), zero failures/skips. Each fixture
explicitly freezes its required orientation, checks Activity configuration and
releases rotation; the first portrait attempt inherited landscape and timed out
waiting for an offscreen lazy-list selector. The corrected fixture passes without
extending timeouts. Final screenshots were visually inspected. Landscape review
also found labels breaking mid-word in the narrow two-column sidebar. A native
text-layout regression failed against the old APK (expected one line, actual two),
then passed with one-column controls below 280dp. Buttons remain labeled, full
sized and scrollable at enlarged text. The game stage remains attached throughout
start/pause/resume.

Final checks on the unchanged runtime inputs:

- `./gradlew :app:testDebugUnitTest --tests 'com.anpfuel.app.games.*'
  :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest
  :app:verifyReleaseApkSize --offline --console=plain --max-workers=2`:
  PASS (1m9s); 9 affected app unit tests, no failures/errors/skips.
- Full app unit suite previously passed 258/258 before the final responsive
  layout repairs; affected unit/native/build/lint checks reran after those repairs.
- Lint: zero errors/fatals, 177 existing warnings, no Games source/test diagnostic.
- Debug and test APKs installed using `adb -s emulator-5580 install -r`.
  Separate final instrumentation calls used `shell am instrument -w -r -e class`
  with `com.anpfuel.app.games.GamesScreenDeviceTest` and
  `com.anpfuel.app.games.GamesLandscapeDeviceTest`, runner
  `com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner`.
  Success was read from actual `OK (10 tests)` / `OK (2 tests)` output, not
  inferred from shell exit status. Wi-Fi 0 / airplane flag 1 / mobile data 0.
- Unsigned release APK: 4,345,457 bytes (4.14 MiB), below the existing 15 MiB
  limit; +85,604 bytes over 4,259,853-byte source baseline. SHA256:
  `577eada3f6d92796d322ec2ac1ba68c0f9c4bb13a89928fefa8fde893148dc3b`.
- Tested runtime fingerprint: SHA256 of
  `git diff 571f904 -- app/src/main domain/src/main`:
  `310c8614ee31598bb50144261aab5faf3819d30f8c048966ad21836b178d400a`.
  Only documentation/evidence follow these runtime checks.
- [Five original synthetic captures](evidence/games-v1/README.md) include
  Portuguese dark selection, both games, landscape controls and 2x-font tutorial.
  PNGs are documentation assets excluded from the APK, without editing.
- `git diff --check` and repository secret scan PASS; changed documentation
  reviewed for secret patterns. All 12 original `fuel-v1` asset files remain
  byte-identical to `2270da3`; backend/data/application/iOS and domain behavior
  remain unchanged during this polish batch. No dependency addition.

M08 `c758bbb`, M09 `4eb8258` and M10 `9087578` were individually pushed;
M11 publishes the repaired layout, regression cases and this evidence through
configured `origin/games`. The configured shared remote remains unchanged.
[GitHub branch](https://github.com/AlexandreZanata/Abastevo/tree/games).

Global domain coverage remains the unchanged pre-existing integration blocker
(87.2467% vs 90%; baseline 86.5864%). Domain source/tests are unchanged by visual
polish, reusing the 508-case report from the prior matching domain tree (24 games).
No physical-device battery/thermal/FPS/audio/TalkBack claim. No ranking/backend,
PR/merge/wiki or release certification. Source/assets outside Games are preserved.
