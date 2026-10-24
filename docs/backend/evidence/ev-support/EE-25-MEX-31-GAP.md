# MEX-31 / EE-25 — Master execution gap closure

Status: LOCAL_DONE (bounded correction; energy/scenario execution OWED)
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `82ea53c3` (maintained dev, EE-22). Date: 2026-10-23.
Repository gaps found: the master screen only rendered an OFFICIAL
deep-link; EV place intents showed an interpretation with no vehicle
context, and energy/scenario intents showed a route label only. Those
are interpretation outputs, not completed journeys.

## Delivered

- Master charging intent now loads free local vehicles, keeps only
  BEV/PHEV profiles with a battery, and opens the existing compatible
  charging screen with that vehicle's context. One vehicle opens
  directly; several show labelled choices; none shows a free "add an
  electric vehicle" action instead of inventing a vehicle.
- The stale charge route is cleared when a new question is interpreted,
  so a later answer cannot open an old vehicle.
- Energy-price and scenario-cost intents now expose an explicit
  execution-pending notice instead of a tappable-looking route with no
  destination. The interpretation remains visible; no price, tariff or
  total is fabricated.
- Six strings in all eight locales; no new dependency, backend call or
  schema.

## Why the remaining execution is not implemented

- Reported energy price needs a place-level aggregate over community
  observations (accepted EE-12/13 store only per-place history; no
  national/UF energy-price ranking endpoint exists). Reading all
  histories per place would be an unbounded phone-only sweep.
- Complete scenario cost needs a selected place tariff with energy/
  time/parking/session components and known eligibility; there is no
  stored provider tariff to feed the EE-11 engine, and inventing one
  would violate EV-11/12. The interpretation therefore stays visible
  with an explicit OWED notice.
- These are missing backend/product inputs, not a client bug hidden by
  a generic error.

## Actual checks

- Master ViewModel tests: single electric vehicle opens CHARGING with
  its id; no electric vehicle shows the add-vehicle state; energy and
  scenario intents set explicit pending notices; existing suggestion,
  ranking, official, ambiguous and adversarial tests remain green.
- `:app:testDebugUnitTest` master selector PASS; `:app:lintDebug`
  zero errors; `:app:assembleDebug` PASS; 8-locale key parity.
- `git diff --check` and secret scan PASS.

## Limits

- EE-25's reported-energy-price and scenario execution remain
  OWED/blocked on the concrete backend inputs above. This record does
  not claim EE-25 complete and does not convert interpretation into a
  result.
- Device validation remains OWED under the standing deferral.

Next: EE-27 end-to-end acceptance over the delivered local journeys.
