# Games visual polish and emulator validation

Date: 2026-10-11. Branch: `games`, opening head `b610583`.
Status: IN_PROGRESS / INTEGRATION_PENDING. The user explicitly authorized
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
