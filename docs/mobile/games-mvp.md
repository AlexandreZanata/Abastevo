# Offline Games mobile MVP

Date: 2026-10-11. Status: IN_PROGRESS / INTEGRATION_PENDING.
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
Each completed microtask has a separate atomic commit on `games` and normal
branch push. Do not integrate, merge, publish a wiki, deploy or install on a
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
Gate pass +1, fuel pickup +5; touching a collision boundary is a collision.
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
bests. Best corruption is clamped; two integer keys only, no gameplay history.
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
