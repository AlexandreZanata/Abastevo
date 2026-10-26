# MAP-28 — Electric journey from vehicle and map to saved plan

Status: LOCAL_DONE
Validation: PASS (real app/native flow with synthetic private fixtures; see limits)
Integration: INTEGRATION_PENDING

Parent: MAP-27 local checkpoint. Contract: [MAP_28_CONTRACT](../../../product/MAP_28_CONTRACT.md). No backend/SQL/job/payment/provider change, no new paid safety or contribution gate, no public route/track upload.

## What exists

- `EvTripSection` in the existing native route sheet: app vehicle picker (BEV/PHEV with useful
  battery/connector), SOC, reserve, consumption (Wh/km), charge target, optional PHEV electric
  share; localized decimal-comma parsing; range/format validation; capacity/connector shown;
  unknown values never replaced by convenient defaults. The section is free and uses the
  existing visual containers.
- `EvTripViewModel`: persisted inputs in `SavedStateHandle`, generation invalidation on every
  input/vehicle/route change, explicit primary/alternative/arrival/charge/cost/power/hours/
  provenance and gap display; a failed primary replan starts at its **arrival SOC**, excludes
  the failed site, keeps remaining user waypoints, clears the old result before the new
  calculation and ignores stale callbacks.
- The planner keeps the existing native main route and user stops as ordered waypoints; the
  EV section is shown only when origin and destination exist. Missing maps/snapshot expose a
  Downloads action and an explicit status.
- Saving uses the existing MAP-25 private owner-scoped saved-trip contract. `SavedScreen` now
  has an Open action for route plans; `map/plan/{tripId}` loads the private trip and
  `NativeRouteViewModel.restorePlan` reopens the route sheet with origin/stops/destination/
  avoid. The saved trip stores route points/avoid, not the EV assumptions; the current inputs
  are used when the user replans. The PHEV/ICE/entitlement behavior of the ledger is unchanged.
- Strings were added to all eight locale resource directories (`values`, `pt-rBR`, `es`,
  `fr`, `de`, `ja`, `ru`, `zh-rCN`); `:app:lintDebug` passes without removing translations.
- A debug-only `InstrumentedDbEntryPoint` lets device tests seed the same Room singleton; it is
  not packaged in release.

## Proofs executed

- `EvTripUiInstrumentedTest` on owned `emulator-5554` (API 35, 16 KB, translated ARM64,
  admitted Parana fixture): **OK (20.339 s)**. It seeds a synthetic EV and two synthetic
  charger rows into the same Room DB, plans on the real native map, shows the primary and a
  reachable alternative, marks the primary failed, replans from the arrival state, saves the
  plan through MAP-25, closes the sheet, opens Saved, and reopens the saved plan in the route
  sheet. The fixture data is synthetic and removed in teardown; no real charger coverage is
  claimed.
- `EvTripRealEngineInstrumentedTest`: **OK (3.037 s)**. Real main/primary/alternative/failure-
  location road legs (see MAP-27).
- `EvTripSectionAccessibilityTest`: **OK (2.133 s)**. The real section is rendered at 320 dp
  width with `fontScale = 2.0`; vehicle/fields/plan/stop/alternative/failure controls remain
  reachable, displayed and clickable via Compose semantics.
- `:app:testDebugUnitTest --tests '*EvTripViewModelTest*' --tests '*NativeRouteViewModelTest*'
  --tests '*MapPlanViewModelTest*'` — 11/11. Input persistence/restore, stale generation
  clearing, invalid input/no vehicle/no connector, failure replan site exclusion, plan reopen
  view-model load, restored route points/stops/avoid.
- `:data:testDebugUnitTest --tests '*Charging*' --tests '*EvCandidate*'` and targeted
  `*Ledger*`/`*Saved*` app unit tests pass; no affected price, money, privacy or entitlement
  test was weakened.
- `:app:lintDebug :data:lintDebug :maps:engine:lintDebug` BUILD SUCCESSFUL after fixing literal
  percent signs in the new translated strings.
- Final app debug SHA-256 `18f283c6ebbc62ad69e7b0e8408f533e49eaede5db737050d56621d6a0cfd915`;
  test APK SHA-256 `bc04e1ed3ad5cf7d729434f9f7b5928e27121b8177085802615eb9f7d9bb338b`.
  Room/private fixtures are synthetic; no secret or real location is stored in Git.

## Limits and not delivered

- No claim that the car cannot strand the user. The UI states that arrival is an estimate,
  there is no guarantee, no live availability/reservation and unknown operation stays unknown.
- The OCM snapshot has no hours/tariff/operator/report date; real candidates keep them
  UNKNOWN. No real live charger state was tested.
- The UI test used synthetic Room snapshot rows and a synthetic vehicle; it is not real
  charger coverage, a real payment path or a public saved-plan publication.
- Airplane-mode MAP-28 journey was not re-run in this phase; the route/map offline proof is
  MAP-24 and the charging source is Room-only. The UI test used no network provider.
- Process-death input restoration and saved-plan reopen are proven; a full cold-process
  airplane-mode EV journey and a real two-account/two-device backup/restore are **OWED** (one
  emulator available; the private-backup path belongs to the existing account/MAP-25 scope).
- Actual TalkBack traversal was not run; Compose semantics, focus/click actions and 200%
  font/narrow layout were tested. Manual TalkBack and a physical-device pass remain OWED.
- The existing ledger charge history/cost/monthly/scenario UI was reused, not reimplemented;
  targeted Ledger tests pass. A new real PostGIS→Kotlin→UI price/scenario flow was not needed
  because no backend/price/ledger code changed in this phase.
- Several earlier runs of the long UI test crashed in the emulator framework/translated JIT
  (`MonitoringInstrumentation.queueIdle` SIGSEGV); the API 30 emulator also died during a run.
  The final run above passed; the crashes are disclosed and are not counted as evidence.
