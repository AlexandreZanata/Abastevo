# MAP-28 implementation contract

Scope: the minimum real EV journey UI over the proven MAP-27 planner and existing private
MAP-25 storage. This record consolidates the rules the implementation and tests were built
against; it is not a new permissions, pricing or backend contract.

B-BR-MAP28-01 (units and input): vehicle capacity is the existing useful Wh; SOC, reserve and
charge target are percent converted exactly to basis points; consumption is Wh/km converted to
milli-Wh/km; PHEV electric share is percent converted to permille. Decimal comma and dot are
accepted with bounded scale. Invalid, missing, non-finite or out-of-range values stay an
explicit error; there is no silent zero/default and no float money.

B-BR-MAP28-02 (result honesty): the screen shows the native road distance/time, primary,
bounded alternative, arrival SOC, target, charge energy, minimum charge time, exact-or-unknown
tariff/cost, declared power, provenance and gaps. It states that traffic, elevation, live
availability, reservation, guaranteed power and guaranteed arrival are not modelled. UNKNOWN
never renders as a convenient fake value.

B-BR-MAP28-03 (save/download): saving uses the existing MAP-25 owner-scoped route-plan contract
(points + avoid); it does not imply that maps or charging snapshots were downloaded. The Saved
screen can reopen a route plan through the private loader and route-sheet restore. Downloads
stay the MAP-24 flow; no silent network or national base download.

B-BR-MAP28-04 (failure/replan): marking the primary failed invalidates the old result before
the new calculation, starts from the actual arrival SOC at that charger with no energy from
the failed unit, excludes the failed site and preserves only remaining user waypoints. A stale
callback/generation cannot republish the old plan. The UI does not claim a fallback when the
planner has no evidenced one.

B-BR-MAP28-05 (offline/private): candidate reads use the active Room snapshot only; the screen
works from already-admitted maps/references/prices and exposes missing map/snapshot states.
EV inputs persist in SavedStateHandle; private saved plans remain owner-scoped under MAP-25.
No route/track is published and no new tenant or public upload is introduced.

B-BR-MAP28-06 (existing accounting): the EV section is free; no new paid privacy/safety/
contribution restriction is added. Existing ICE/PHEV/free/entitlement, ledger charge fields,
exact prices and MAP-25 owner rules are reused, not replaced. No backend/money/SQL change is
made without its own negative/concurrency/PostGIS tests.

B-BR-MAP28-07 (accessibility/locales): controls have 48 dp targets, visible text semantics,
vertical scroll for 200% font/narrow screens, light/dark theme, and strings in all eight
current locales. Start/stop navigation and any recording remain explicit separate actions;
reading or planning never starts GPS/TTS/recording.

BUC-MAP28: vehicle + map origin/waypoints/destination + SOC/reserve/consumption/target ->
real plan with primary/alternative/gaps -> save private route -> close -> open Saved -> reopen
route sheet -> mark primary failed -> replan from arrival SOC -> save/reopen. No guarantee that
the user will not strand or that a charger is available.

Limits: no real two-account/two-device backup run, manual TalkBack traversal or airplane-mode
process-death journey was available in this local batch; those remain owed and are recorded in
the MAP-28 evidence. Synthetic Room/vehicle fixtures used for the device proof are labelled.
