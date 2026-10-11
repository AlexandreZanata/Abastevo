# Games — lightweight offline arcade and verified Brazil ranking

2026-10-11 scoped follow-up: the user selected the offline Android MVP on an
isolated `games` branch. [Mobile contracts and evidence](../mobile/games-mvp.md)
own that implementation. The dated planning/draft status below is historical;
backend ranking/sync and the full device/performance matrix remain deferred.

Date: 2026-10-09. Owner: separate optional Games workstream, IDs `GME-*`.
Status: PLANNING_LOCAL_DONE; GME-00 asset draft LOCAL_DONE / INTEGRATION_PENDING.
Runtime status: Games tab, playable games, sync and leaderboard NOT_IMPLEMENTED.

## Authorized scope and dependencies

The user requested a separate professional plan for an in-app Games tab, two
simple offline arcade games, fuel-themed 3D assets and a Brazil leaderboard.
**This delivery implements only the initial art assets and planning.** Later
tasks require explicit implementation selection; this document does not enable
an app tab, add an engine/SDK, migrate a database, contact players or publish.

The source baseline is dev `4725ebcb65bc065f6c6db3c40929b38b7548880a`, with
recorded origin/main already an ancestor at opening. Main checkout contained
another task's uncommitted Rust audit/progress. This work is isolated in the
managed `games-assets-plan` worktree, initially detached at that dev revision.
Owned changes must later be reconciled into maintained dev without overwriting
that work; final publication remains protected dev → main under
[ADR-020](../adr/020-maintained-dev-main-delivery.md) and
[DELIVERY_WORKFLOW](DELIVERY_WORKFLOW.md). No remote issue/PR/check was opened or
refreshed for this local art/planning request.

This optional extension does not reorder [P34–P38](ANDROID_VPS_PLAN.md), accept
G18/G24/G09, expand the Rust ingestion task or resume iOS. Backend contracts and
critical tests precede Games consumers that submit ranked runs. Offline art and
simulation work can be independently selected after the existing G09-LOCAL
source integration; live ranking requires its own staging acceptance at
`https://teste.abastevo.com.br`, never readiness inferred from this plan.

## Product proposal

The future **Games** destination contains game selection, a clearly labeled
local personal best and a cached Brazil leaderboard with its last update time.
Choose its navigation placement in GME-01 after reviewing the existing tabs;
adding another crowded bottom-navigation item is not presumed.

- **Fuel Flight** (working title): one-tap impulse/gravity arcade. A fuel-drop
  character passes pairs of original pump/road obstacles and collects fuel
  tokens. A short tutorial, immediate restart, pause, mute and reduced motion
  keep the game simple.
- **Fuel Run** (working title): three-lane runner with lane changes, jump and
  slide, using original fuel collectibles and station/road props. Start with
  a fixed perspective, pooled obstacle sets and bounded speed progression.

These are genre references, not permission to copy Flappy Bird/Subway Surfers
characters, names, sound, logos, art or level layouts. Distinct accessibility
labels and silhouettes supplement category colors. No score affects fuel
prices, contribution validation, community trust or paid status. Free account
registration and community participation remain independent.

## Behavior contracts to freeze before runtime implementation

- **B-BR-GME-01 / offline:** either game launches, teaches, plays, pauses,
  restarts and records a personal best in airplane mode, including first use
  after installation. No login, asset download, attestation or ad is required
  to play. App-background pause is automatic; process death never fabricates a
  completed run. Initial ranked games do not resume a mid-run snapshot after
  process death; local bests and completed pending submissions survive.
- **B-BR-GME-02 / authority:** the client score, wall clock, local database and
  locally held key are untrusted. National ranking accepts only a server
  recomputed, policy-eligible result. An offline personal record is not shown
  as verified until accepted; unrankable runs remain usable local records.
- **B-BR-GME-03 / separation:** per-game, rules-version and season leaderboards;
  deterministic tie policy frozen in GME-01. Updates and old physics never
  silently mix. No paid advantage, reward conversion or trust weighting.
- **B-BR-GME-04 / custody:** pending submissions bind to the existing owner,
  key and environment. Account/origin changes cannot send another owner's
  runs. Use existing signed-command and session mechanisms only after their
  suitability is audited; do not create a second account or tenant system.
- **B-BR-GME-05 / privacy:** public results expose only an approved public
  alias and game metrics. No email, raw device ID, GPS, gameplay timing trace
  or private account/key data in public API/logs. A Brazil-wide list needs no
  GPS; the product must define eligibility without claiming verified physical
  presence or nationality. Cache, trace retention, deletion and moderation
  requirements are decided before collection, not after deployment.
- **B-BR-GME-06 / limits:** all simulation, trace, storage, queue and network
  work has explicit caps. Reject malformed or excessive submissions before
  expensive verification; never let untrusted score claims allocate arbitrary
  memory or CPU.

**BUC-GME-01:** guest opens Games offline → chooses a game → plays → personal
best saved. **BUC-GME-02:** eligible signed-in player uses a previously obtained
ranked-run ticket offline → completed input trace queued → connectivity returns
→ signed upload/verification → explicit accepted/rejected receipt and refreshed
rank. **BUC-GME-03:** first-install/no-ticket/expired-ticket/offline guest still
plays; record remains local with a clear explanation of ranking eligibility.
**BUC-GME-04:** network loss, logout, owner/origin switch, duplicate delivery or
process death preserve safe state and never yield duplicate leaderboard writes.

## Technology recommendation

**Default: Kotlin game core + Android 2.5D rendering, within the existing app.**
Kotlin is the programming language; Canvas/libGDX/Godot are different rendering
or engine choices. Keep simulation pure and independent of Compose, graphics,
network, Room and frame rate. Retain current modules and `com.anpfuel`; decide
whether a small game-core submodule is justified only in GME-01.

For Fuel Flight and a simplified fixed-camera runner, render the 3D masters
offline into transparent sprite atlases, then draw with a dedicated Android
Canvas/SurfaceView renderer hosted by the Compose navigation/shell. Use a fixed
simulation tick and render interpolation; Compose handles menus/settings/HUD
state without rebuilding a large UI tree on every simulation tick. Synchronize
rendering to display timing and pause all frame work when hidden. Avoid building
a generic game engine. The official Android game-loop guidance describes
display synchronization and separating rendering from expensive UI-thread work:
[Android game loops](https://developer.android.com/games/develop/gameloops).

This preserves 3D-looking artwork while keeping gameplay 2D/2.5D, which is the
recommended starting tradeoff for extreme lightness. The delivered GLBs remain
editable source art; they are not automatically copied into the APK. Sprite
atlases/animations are a later task: neither the contact sheet nor the GLBs are
claimed to be a ready-to-play animated character pack.

If **real-time 3D** is essential, benchmark **Kotlin + libGDX** for the runner
before choosing the renderer for both games. It supports Kotlin; confirm the
specific glTF loader and its maintenance/license/security needs separately:
[libGDX with Kotlin](https://libgdx.com/wiki/jvm-langs/using-libgdx-with-kotlin).
Writing a bespoke OpenGL renderer plus GLB loader is not the default professional
shortcut; it adds maintenance for no demonstrated need.

**Godot** is a viable alternative when visual scene authoring, richer 3D levels
or a game-centric workflow outweigh the embedded-engine cost. Its Android AAR
can be embedded in an existing app:
[Godot Android library](https://docs.godotengine.org/en/stable/tutorials/platform/android/android_library.html).
Its MIT license is compatible with this repository, while third-party notices
still apply: [Godot license](https://godotengine.org/license/).
Unity/native C++/Rust are not proposed for these first two simple Android games:
no measured bottleneck or engine-specific requirement warrants that added
runtime/toolchain here. No engine-size or performance number is assumed; measure
the same representative scene on the same device before adopting a dependency.

## Offline play and cheating: achievable guarantees

**Absolute cheat prevention and fully offline client control cannot both be
guaranteed.** A modified client can fabricate a valid input sequence, run bots,
inspect cached seeds, simulate ahead, reset state or choose its best attempts.
Encryption, obfuscation, local HMACs and Keystore signatures do not establish
that a human honestly played: the device must process the game while offline.
Server replay proves that a trace follows the published rules, not that a
human produced it in real time. Changing engine or programming language does
not remove this limit.

Recommended compromise: fully offline gameplay and local records, plus
**asynchronous verified national ranking** for eligible attempts. The backend
does bounded validation work when a player reconnects, never per-frame streaming.
"All processing local" therefore describes active gameplay, not zero backend
computation for trustworthy shared results. If zero backend verification is a
hard requirement, a trustworthy national ranking is not deliverable.

Proposed ranked protocol, to be specified/tested in GME-01/GME-03:

1. While online and authenticated, obtain a small capped batch of server-issued
   one-use tickets containing ticket ID, owner/key/environment binding, game,
   exact rules version, seed and server-controlled acceptance deadline. Use
   established server authentication/signature primitives, never custom crypto.
2. Play offline with the fixed tick, explicitly versioned integer/fixed-point
   physics, PRNG and collision rules. Record only tick-indexed actions and final
   duration, not video or per-frame snapshots. Catch-up work is capped;
   rendering frame rate never changes scoring. Trace size and run duration have
   strict limits, with longer practice allowed only by a separate local mode if
   the product later requests it.
3. Queue the trace locally with its immutable ticket/version and retry using the
   existing bounded outbox pattern. No ticket, expired acceptance window or
   unsupported rules version leaves the local record intact, but cannot be
   silently upgraded to ranked. No retroactive ticket for a run played before
   ticket issuance; deadline checks use server time at receipt, not phone time.
4. At reconnect, submit the bound trace and claim through authenticated signed
   requests. Apply payload, account/IP and verification-cost budgets before
   replay. If selected, request fresh Play Integrity evidence bound to the
   submission digest; a network/provider failure leaves verification pending
   or unranked according to the frozen policy, never blocks offline play.
5. The Go backend independently replays the trace using the same deterministic
   specification and shared golden vectors. Reject invalid ordering, impossible
   actions, timing/rate violations, overflow, unknown versions and any score
   differing from its computation. Capped tick counts prevent verification DoS.
6. After validation, atomically consume the ticket and store the receipt/result.
   Same owner/ticket/content retry returns the original receipt; changed-content,
   cross-owner/environment/key or simultaneous reuse cannot create another
   score. The exact rejected-ticket consumption policy must resist brute-force
   retries and be frozen before implementation. Read rankings from a bounded,
   indexed projection with stable pagination and cached public reads.

Google documents `requestHash` submission binding, replay protections and
server-side verdict verification:
[Play Integrity standard requests](https://developer.android.com/google/play/integrity/standard).
An attestation obtained on reconnect is evidence about that request/app state;
it does **not** retrospectively prove honest offline play. Preserve server
idempotency even when attestation tokens are replay-protected. Play distribution,
provider availability, quotas, costs and failure policy require a dedicated
decision; no SDK or live service is enabled by this plan.

Residual risks: automated valid traces, seed search, ticket stockpiling, selective
discard of bad attempts and compromised clients remain possible. Single-use
tickets and issuance caps limit submissions, not prove exactly one offline
attempt occurred. Apply server-owned seasons, rate limits and review of anomalous
top scores, with an appeal path before punitive actions. Stronger competition
can later use online-start challenges or an online-only mode, explicitly trading
away unrestricted offline ranking eligibility. Do not advertise "cheat-proof".

## Sequential delivery tasks

**GME-00 — asset draft + separate plan (this delivery).** Eight texture-free GLB
masters, generator, provenance/hashes, interactive offline viewer and rendered
contact sheet. Preserve active fuel art and package size. Acceptance: loader and
geometry checks, repeat generation, source hash checks, visual inspection,
budget/link/secret/whitespace checks. No game, animation or APK claim.

**GME-01 — product/specification + renderer spike (PLANNED).** Freeze B-BR/BUC,
navigation, controls, collision shapes, exact ticks/PRNG/physics, tie/season policy,
offline eligibility and ranked threat model. Benchmark one representative scene
in Canvas 2.5D versus libGDX only if real 3D is required. Record dependency
need/license/security/maintenance, APK delta, memory and frame times. No full game
or leaderboard implementation before the decision.

**GME-02 — deterministic simulation contract (PLANNED).** One game at a time;
write Kotlin domain RED → GREEN → REFACTOR and cross-language golden vectors
before UI. Fixed-width arithmetic/overflow/rounding, PRNG, collision, event order
and terminal tick are specified explicitly. Acceptance covers collisions, exact
score, seed reproducibility, invalid input, frame-rate independence and bounded
trace/run lengths. Keep Go replayer parity fixtures independent of rendering.

**GME-03 — backend ranked-run boundary (PLANNED).** Implement the selected
game's Go verifier, tickets, authenticated intake, idempotent result and public
leaderboard reads. Define exact routes/OpenAPI/schema here, not speculative
current endpoints in this plan. Forward-only migrations, real PostGIS-backed
repository tests, auth/key/owner/env negative tests, consume/receipt transaction,
concurrent reuse, malformed/oversized trace, replay/DoS, version expiry and
retention/recovery tests are immediate. Backend contracts precede Android sync.

**GME-04 — Fuel Flight + Games entry (PLANNED).** Deliver sprites, UI/renderer,
pause/restart/accessibility/audio and first-install airplane-mode play. Tests:
touch/render/physics separation, background/process death, no network dependency,
bounded storage and safe navigation/back. No national-rank acceptance yet.

**GME-05 — ranked outbox and Brazil leaderboard (PLANNED).** Consume the tested
backend contract; owner-bound durable queue, bounded retries, explicit pending /
accepted / unranked states and stale-cache labels. Test offline→online, duplicate
send/ack loss, key/access expiry, logout/account/origin switch, provider failures,
old rules and corrupted queue. Live staging acceptance is separately recorded.

**GME-06 — simplified Fuel Run (PLANNED).** Repeat GME-02/GME-03 acceptance for
the runner before its ranked consumer. Three lanes, original assets, pooling,
no open world/heavy physics engine. Test lane/jump/slide/collision/tick-boundary
cases and fairness across display rates; never reuse a Flight board for Run.

**GME-07 — optimization and scoped acceptance (PLANNED).** End device matrix,
airplane-mode first run, thermal/battery runs, fonts/contrast/color-vision review,
retention/security/failure acceptance, final protected batch delivery. Integrate
to dev preserving other work; required current-head `Quick verification` and
guarded `finish --required "Quick verification"` at authorized final publication.
One merged-SHA wiki mirror; no CI polling during construction. G09/release and
remaining commercial/manual acceptance obligations are unchanged.

## Proposed performance budgets (targets, not measured game results)

- Art masters now: at most 3,000 triangles and 128 KiB per GLB; eight-file GLB
  pack at most 1 MiB. Documentation HTML/PNG are excluded from runtime packaging.
- Future two-game shipping-content target: at most 2 MiB added compressed APK
  size; current `:app:verifyReleaseApkSize` **15 MiB total** limit remains. Confirm
  raster atlas size before committing to a renderer; engine/native ABI overhead
  counts, and 3D masters must not also ship if only sprites are consumed.
- 60 fps target on selected devices (16.7 ms frame budget), capped 30 fps low
  power option; profile long-frame percentiles and thermal stability. Simulation
  remains fixed-rate regardless of display/quality setting. No FPS promise until
  measured on named low-end and existing POCO devices.
- Candidate incremental memory cap: 32 MiB after warm-up. Limit live objects,
  atlas memory (decoded RGBA, not compressed file size), pooled particles and
  simultaneous GLB loads; no allocation-heavy per-frame path.
- Candidate ranked run limit: 3 minutes / 64 KiB uncompressed trace, with
  separately capped tick/event count. Candidate submission storage cap: 1 MiB.
  Freeze issuance/retention/deadline/retry values and visible full-queue behavior
  in GME-01; do not silently evict acknowledged-pending runs.

Measure cold launch, APK before/after, decoded texture/GPU/heap memory, p50/p95/p99
frame times and per-minute energy/thermal behavior in repeatable scenes. Publish
actual device/build/revision and test conditions; do not extrapolate artwork
file sizes into game performance guarantees.

## Current deliverables and next action

See [asset pack](../assets/games/fuel-v1/README.md),
[manifest](../assets/games/fuel-v1/manifest.json),
[interactive 3D review](../assets/games/fuel-v1/preview.html) and
[rendered contact sheet](../assets/games/fuel-v1/contact-sheet.png).
GME-00 verification is recorded in the asset README/current PROGRESS.
Next: review the V1 art and select GME-01; choose 2.5D as the default unless
real-time 3D is a product requirement. No later GME implementation is implied.
