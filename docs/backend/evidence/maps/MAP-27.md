# MAP-27 — Road-distance EV stops with a reachable alternative

Phase commit: `805353b8` (this evidence commit; docs-only revisions after it do not change code).

Status: LOCAL_DONE
Validation: PASS (local/synthetic charging fixtures; no live or national coverage claim)
Integration: INTEGRATION_PENDING

Parent: `d766dbb4` (MAP-26 LOCAL_DONE). Contract: [MAP_27_CONTRACT](../../../product/MAP_27_CONTRACT.md), written first. No backend migration/API, provider call, live-status claim, paid gate or "guaranteed arrival" is added.

## What exists

- `domain/ev/EvChargingPlan.kt`: typed candidates (site/equipment/connector identity, access, hours with a real zone, exact tariff or UNKNOWN, declared power, static-reference/community provenance), failure scopes SITE/EQUIPMENT/CONNECTOR, routes, plans and bounded prefilter geometry. `EvChargingPlanRules` owns eligibility, per-site duplicate collapse, failure exclusion, exact target-charging energy/cost, failure-state reachability and reserve comparison.
- `application/usecase/ev/PlanEvTripUseCase.kt`: native road main route with ordered user waypoints; bounded corridor candidates; one-per-site primary probes; a real road leg per probe; reserve/arrival checks; target charging; a fallback that starts from the actual primary-arrival SOC with **no** energy from the failed unit; budget/cancellation. It prefers a primary whose bounded fallback is reachable, then falls back to the first reachable stop with an explicit no-fallback warning.
- `maps/engine/NativeEvRoadRouter.kt`: Main-thread native adapter with a generation token, timeout and cancellation. The native controller's NEED_MORE_MAPS is never accepted as a road success.
- `data/repository/RoomEvCandidateSource.kt`: reads only the active admitted snapshot generation in Room through a bounded bbox query; snapshot hours/tariff/operator/report-date fields stay UNKNOWN, never invented. `ChargingDiscoveryRepository.corridor` and `ChargingSnapshotDao.readPlacesInBounds` are bounded (max 500 rows, snapshot cap 10,000).
- No schema/migration change: the bbox query uses the existing snapshot columns. No ANP→community fallback, no straight-line drivable leg, no all-national download.

## Mandatory failure vector

`EvChargingPlanTest.mandatory failure vector...`: capacity 60,000 Wh, initial 30% = 18,000 Wh,
reserve 10% = 6,000 Wh, constant 180 Wh/km, 50,000 m to the primary consumes 9,000 Wh and
leaves 9,000 Wh (1,500 bp). The failed primary provides no energy:

- alternative 20,000 m consumes 3,600 Wh → 5,400 Wh = 900 bp, below the reserve;
- alternative 10,000 m consumes 1,800 Wh → 7,200 Wh = 1,200 bp, energy-reachable.

The test asserts the exact 900/1,200 bp results and reserve comparison. The vector is pure
domain and does not use a measured road distance. The real-road integration is proven
separately below.

## Proofs executed

- `:domain:test --tests '*EvChargingPlanTest*'` — 9/9. Failure vector, PHEV electric portion,
  exact charge energy/cost (UNKNOWN tariff is not zero), timezone-aware/night/overnight hours,
  private/unknown access, incompatible connector, duplicate connectors, failure scopes,
  route prefilter/bbox, invalid/missing inputs, reserve/SOC/capacity.
- `:application:test --tests '*PlanEvTripUseCaseTest*'` — 10/10. Primary+reachable alternative,
  unreachable alternative is not claimed, same-site connector is not independent, private /
  incompatible / closed / unknown metadata, missing snapshot, no route, missing maps,
  replan from primary arrival SOC (including a charger behind the failure), budget exhaustion,
  road warnings/unknown arrival time, PHEV scope.
- `:maps:engine:testDebugUnitTest --tests '*NativeEvRoadRouterTest*'` — 3/3. Late callback
  from an old generation is ignored; missing maps are refused; timeout/cancellation cancels
  the native route.
- `:data:testDebugUnitTest --tests '*EvCandidateMapperTest*'` — 3/3. Exact connector filter,
  power/identity/provenance kept, hours/tariff/report date typed unknown, null coordinates
  skipped, incompatible connector not fabricated from another group.
- `EvTripRealEngineInstrumentedTest` on owned `emulator-5554` (API 35, 16 KB, translated
  ARM64, admitted Parana fixture, offline) — **OK**. A real road main route, real primary
  leg, real detour/alternative leg and a real failure-location replan all return
  `EvRoadLeg.Ready` with positive distances; primary/alternative arrival stays above the
  reserve; the failed site is excluded. Charging points/battery are synthetic and identified;
  no charger coverage is claimed.
- `EvTripUiInstrumentedTest` (MAP-28) also exercised the same real engine + synthetic Room
  snapshot; see [MAP-28](MAP-28.md).

## Limits and not delivered

- The admitted OCM snapshot has no opening hours, tariff, operator or report date; these stay
  UNKNOWN on the real path and the UI explains them. The hours/timezone and tariff rules are
  proven with typed synthetic candidates.
- No live availability/reservation; WORKED/IN_USE is not modelled as a reservation.
- Road closure/restriction is a native route warning, not a closure feed; warnings are
  surfaced but do not become a guarantee of access.
- Candidate fallback checks are bounded (6 probes/6 alternative checks per step by default);
  the first reachable fallback may not be the global best. Budget exhaustion returns an
  explicit incomplete status.
- No elevation/climate modelling, national corridor download, real charger acceptance or
  production road-safety certification. Synthetic fixtures are not described as real charger
  coverage.
- A UI test fixture initially used >12 decimal coordinate text; the existing snapshot contract
  correctly refused it. The fixture was changed to six decimals. This was a fixture issue, not
  a relaxed production rule.
