# Offline Games mobile MVP

Date: 2026-10-11. Status: SOURCE_IMPLEMENTED / ACCEPTANCE_BLOCKED / INTEGRATION_PENDING.
Branch: `games`; isolated worktree `.worktrees/games`; source dev `571f904`.
User explicitly requests this branch instead of maintained dev construction.
Main `a349d7c` is an ancestor of source dev. Other worktrees and uncommitted
community/security work remain owned by their original agents and untouched.
Original GME-00 art/specification is copied byte-for-byte from `2270da3`;
its detached source checkout is preserved. No backend, iOS or dependencies.

## Selected phases and microtasks

1. GME-M01: preserve asset source and freeze mobile-only contracts/navigation.
2. GME-M02: Flight deterministic simulation, meaningful RED then GREEN tests.
3. GME-M03: Run deterministic simulation, meaningful RED then GREEN tests.
4. GME-M04: bounded frame/session lifecycle and per-game local-best policy.
5. GME-M05: native Canvas renderer, original procedural MVP art and controls.
6. GME-M06: Compose selector/tutorial/settings, profile entry, local storage.
7. GME-M07: regression/build/lint/size and scoped acceptance evidence.
These microtasks cover the mobile portions of GME-01/GME-02/GME-04/GME-06
and GME-07 source/build checks. Full original GME-07 device/performance/delivery
acceptance remains unaccepted.
Each implemented microtask has a separate atomic commit on `games`. Remote
backup pushes remain PUSH_PENDING after automatic approval review refused the
unverified external destination; explicit destination authorization is required. Do not integrate, merge, publish a wiki, deploy or install on a
user phone as part of this isolation request. Integration remains pending.
GME-03 backend and GME-05 ranked sync are explicitly outside authorized scope.

## Frozen B-BR / BUC

B-BR-GME-M01: Both games run from bundled code on first installation without
network, login, location, downloads, ads, rewards or changes to prices/trust.
Entry is Profile → Games, retaining the existing three bottom destinations.
All results say local personal best; no Brazil board or verified rank is shown.
BUC-GME-M01: guest chooses a game, reads its tutorial, starts, plays, finishes,
sees its score and saved best, restarts or returns to game selection.

B-BR-GME-M02: rules `mobile-v1`; 60 integer ticks/second; maximum 10,800 ticks
(three simulation minutes). xorshift32 PRNG, unsigned shifts, wrapping Int
operations only in PRNG, zero seed mapped to 1. All gameplay arithmetic is
bounded integer arithmetic. One action per tick; impossible actions are ignored.
Collision precedes collection/pass score, then terminal time limit. Scores are
nonnegative. No traces, tickets, accounts or network queue are collected.
Flight world 1000×1600, player (220,800), radius 42, gravity +2/tick, impulse -22,
velocity cap +30; bounds 0..1500. Gates width 130, gap 480, speed 6/tick,
first at tick 120, then every 110 ticks; center 480..1120 inclusive from PRNG.
Player collision uses a radius-42 AABB; gate edges are inclusive. Fuel uses
an AABB half-size 28. Gate pass +1, fuel pickup +5; touching a collision boundary is a collision.
Run: lanes -1/0/+1, start center; jump 48 ticks, slide 36 ticks, mutually
exclusive. Entities spawn first at tick 90 then every 75 ticks at depth 1800;
speed min(20,10+tick/900). Impact crosses depth 80; cone requires active jump,
beam active slide, truck requires another lane, fuel gives +5. Distance gives
+1 per 60 ticks. Lane changes clamp, avoiding wrap; seed chooses lane and kind.
Independent lane choices always leave two lanes open at each encounter.

B-BR-GME-M03: READY → RUNNING → PAUSED/FINISHED. Background, detach and Back
pause immediately; resume requires a user action. Restart resets seed/score and
clock. Process death abandons unfinished runs; no save-state run snapshot.
Only FINISHED scores can update monotonic bounded per-game/per-version local
bests. Best corruption is rejected; two integer record keys and three boolean settings,
no gameplay history. Failed disk commits remain visibly unsaved and retryable.
BUC-GME-M02: background → no ticks/audio → foreground pause overlay → resume.
BUC-GME-M03: process recreation → selector; completed best survives.

B-BR-GME-M04: Canvas View hosted in Compose; Choreographer display callbacks,
fixed tick accumulator capped at five catch-up ticks, excess debt dropped
without changing score rules. Pause resets time. 30/60 render option changes
only draw cadence. No Compose tree rebuild per tick; menu state updates on
phase changes. Live entities bounded to eight per game; no GLBs in runtime.
Procedural original shapes are intentionally MVP placeholders for later sprite
atlases from preserved masters. Reduced scenery option removes decorative
scroll; visible instructions and labeled 48dp buttons supplement gestures.
Mute defaults on; optional short synthesized cues, no audio asset/network.
Accessibility controls support tap, lane buttons, jump/slide and keyboard.

## Renderer decision and acceptance limits

Choose Android Canvas 2D/2.5D with existing Compose shell. Real-time 3D is not
required for this body/MVP; libGDX comparison is therefore not selected. No new
engine, dependency or license. GME-00 masters remain documentation assets.
Target added APK ≤2 MiB and existing total ≤15 MiB, measured during final
build. 60fps / 32 MiB / battery and thermal targets are not measured guarantees.
Hardware, TalkBack, airplane-mode installation and thermal acceptance remain
owed unless actually exercised. Existing deferred device policy is preserved.
Tests must prove deterministic collisions/scoring, limits, frame partitioning,
lifecycle/no fabricated completion and best isolation. Compile Android UI tests
and run scoped unit suites, debug lint/assembly, release size, secret/diff checks.
No Go replayer parity or national ranking acceptance is implied by local vectors.

## Evidence

GME-M01: original assets preserved; contract recorded before runtime code.

GME-M02 LOCAL_DONE: Flight pure Kotlin; RED 4 failures/5 tests against stub,
GREEN 7 tests including unsigned PRNG reference, inclusive collision edges,
500-tick vector (17 points) and 10,800-tick time-limit vector (576 points).
Command: `./gradlew :domain:test --tests 'com.anpfuel.domain.games.FuelFlightTest' --offline --console=plain`.
Diff and tracked secret checks PASS. Push PENDING: automatic approval review
refused unverified remote egress; only local commits are authorized to proceed.

GME-M03 LOCAL_DONE: Run pure Kotlin; RED 7/7 against stub; GREEN 14 combined
Flight/Run tests, including exact tick-261 impact, expired defenses, single
fuel collection, lane clamping, seed reproducibility, bounded entities and
three-minute/speed caps. Independent Run reference: seed 19, avoidance trace,
10,800 ticks → score 255, lane 0, speed 20. Command:
`./gradlew :domain:test --tests 'com.anpfuel.domain.games.Fuel*Test' --offline --console=plain`.
Diff/secret review PASS. No backend replayer or network behavior.

GME-M04 LOCAL_DONE: session and fractional 60Hz clock; RED 8/8 against
stubs, GREEN 22 combined tests. 30/60/90/120Hz produce exactly 60 ticks/second;
long stalls cap at five ticks, background debt/input discarded, terminal state
frozen, restart clean, recreation starts READY. Local best accepts only valid
FINISHED scores and never decreases. Command: `./gradlew :domain:test --tests
'com.anpfuel.domain.games.*' --offline --console=plain`. No unfinished snapshot
or trace persistence. Diff/secret checks PASS.

GME-M05 LOCAL_DONE (source/compilation): native View/Canvas renderer, fixed
aspect scaling/letterboxing, interpolation, original procedural art, cached HUD,
display callbacks removed on pause/detach, capped draw cadence, muted-by-default
optional short audio cues, reduced decorative motion, keyboard/touch controls.
4 gesture tests pass (threshold, dominant direction, invalid floats, short taps).
`./gradlew :app:testDebugUnitTest --tests 'com.anpfuel.app.games.GameplayInputTest'
:app:compileDebugKotlin --offline --console=plain --max-workers=2` PASS.
Android compilation PASS; native visual/audio/hardware acceptance remains OWED.
No new dependency or runtime art file. Diff/secret checks PASS.

GME-M06 LOCAL_DONE (source/unit): Profile → Games additive route, both selectors,
English/Portuguese tutorial, READY/PAUSED/FINISHED overlays, labeled controls,
landscape side controls and scrollable large-font menus, lifecycle observer,
settings and per-game/version local records. Disk writes run on IO, survive UI
disposal while process alive, acknowledge commit failure and offer retry.
Only scores/options persist; Android backup already excludes shared preferences.
8 games app tests + 7 navigation regressions PASS, Kotlin compilation PASS:
`./gradlew :app:testDebugUnitTest --tests 'com.anpfuel.app.games.*' --tests
'com.anpfuel.app.navigation.NavigationTabTest' --offline --console=plain --max-workers=2`.
No changed existing tool, account gate, bottom tab, permission or backend file.
Diff/secret checks PASS. Native UI/device acceptance remains OWED.


GME-M07 SOURCE_IMPLEMENTED / ACCEPTANCE_BLOCKED: added two clock/session
boundary regressions, failed-commit cache restoration regression, four isolated
Android instrumentation cases (guest selection, background pause, large fonts,
real local-record persistence/corrupt preference type), and all 42 game strings
in the existing eight locales. Native playfield accessibility focus is hidden
under overlays. No lint suppression, dependency or acceptance threshold change.
Score labels precede their numbers to avoid incorrect singular/plural forms.
Android SharedPreferences updates memory even when disk commit returns false;
restore its previous cache value so retry cannot falsely acknowledge that score.

### Final validation and blocker

Environment: JDK 17, AGP 8.7.3, Kotlin 2.0.21, SDK/target 35, min SDK 26;
offline dependency cache, max two workers. Tested behavior: parent `fe53990`
plus this microtask's source/test/resource diff. The final commit contains that
exact runtime tree and this evidence; no backend/data/application/iOS changes.
Runtime diff fingerprint (`git diff 571f904 -- app/src/main domain/src/main`,
including staged locale additions), SHA-256:
`33953e6bc4abaa1f8ce311ae8816c1c931d1d0f2a9dbd660140893200bd13ea9`.

- `./gradlew :domain:jacocoTestCoverageVerification --offline
  --console=plain --max-workers=2`: all 508 domain cases PASS (24 games),
  zero failures/errors/skips; coverage verification FAILS at 87.2467%, below
  the unchanged 90% global minimum. Games package instructions 1105/1120
  (98.6607%), lines 161/162, branches 132/137.
- Baseline source `571f904`, exported with `git archive` to a disposable
  `/tmp` checkout, reproduces the same verification failure: 484 tests PASS,
  instruction coverage 16764/(16764+2597) = 86.5864%. Games head covers
  17869/(17869+2612). Existing uncovered domain code is outside this user's
  preservation/mobile scope. This is an unresolved pre-existing acceptance
  blocker, not a waived check; no integration or all-gates-PASS claim.
- Full `:app:testDebugUnitTest` ran 258 tests with zero failures/errors/skips
  before final translation/tooling/cache-rollback adjustments. Final affected
  `:app:testDebugUnitTest --tests 'com.anpfuel.app.games.*'` rerun: 9/9 PASS,
  including real-Android-cache failure semantics. Navigation regressions 7/7
  passed during GME-M06; navigation source is unchanged since that check.
- Final command: `./gradlew :app:testDebugUnitTest --tests
  'com.anpfuel.app.games.*' :app:lintDebug :app:assembleDebug
  :app:assembleDebugAndroidTest :app:verifyReleaseApkSize --offline
  --console=plain --max-workers=2`. PASS (3m 7s); final score-label-only resource update reran
  `./gradlew :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest
  :app:verifyReleaseApkSize --offline --console=plain --max-workers=2`: PASS (34s).
- `:app:lintDebug`: zero errors, 177 existing warnings; no games diagnostic.
  All 42 keys/placeholders exist in English, pt-BR, German, Spanish, French,
  Russian, Japanese and Simplified Chinese.
- `:app:assembleDebug` and `:app:assembleDebugAndroidTest`: PASS. Four added
  device tests compile; none executed on hardware/emulator. APK existence is
  build evidence, not device/gameplay/performance acceptance.
- Baseline release APK: 4,259,853 bytes. Final release APK: 4,329,073 bytes
  (4.13 MiB), delta +69,220 bytes (67.60 KiB). Both shipping target added
  <=2 MiB and configured overall size <=15 MiB PASS. Release SHA-256:
  `e0544abd2e5cf4bdd68812a545185c4789194a6c6e8528f7be2a693c3031ce23`.
- `git diff --check`, tracked secret scan and scoped secret-surface review:
  PASS (including staged files and changed documentation). Twelve preserved
  asset-pack files remain byte-identical to
  `2270da3`; no production photos, identities, traces or secret material added.

### Remaining obligations and next action

No complete LOCAL_DONE declaration is issued for GME-M07 while global coverage
fails. Device playability, rotation/process death, TalkBack, airplane-mode first
installation, FPS/heap/thermal/battery and full visual/contrast acceptance remain
OWED under the recorded device deferral. Procedural placeholders are the MVP
body; production sprite atlases/animation polish remain future work.
GME-03 server verification and GME-05 ranked sync/leaderboard are DEFERRED by
explicit mobile-only scope. No network queue, endpoint, migration or backend
service was added. Remote backup is PUSH_PENDING. Integration requires later
reconciliation with current dev, resolving the global acceptance blocker and
protected delivery; no merge, PR, wiki, deployment or release certification here.
