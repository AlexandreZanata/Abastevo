# MAP-19 — Fast station prices and honest source switching

Current assessment: [2026-10-25 remediation](MAP-15_20_REMEDIATION_20261025.md) supersedes earlier integration/acceptance claims. Historical evidence below is retained.

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (JVM sheet ViewModel, real-phone shell)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at MAP-18 plus MAP-19 changes.
Date: 2026-10-25. Search/filters remain MAP-20.

## Behavior frozen

- Tap → bottom sheet → full profile/route/report. The sheet shows name,
  service type (fuel/charging/combined, text + pictogram, never color alone),
  selected source, exact product/unit/price/date, location quality/access and
  three real actions: native-preview route (explicit external interim until
  MAP-22), existing station profile (`map/station/...` keeps MAP selected)
  and existing contribution (`capture?stationId&fuel`).
- ANP mode shows station-specific ANP observations for the chosen survey
  period; absence is explicit ("No price for this station"), never a silent
  community fallback. Community mode shows the validated projection with its
  date/conditions; rapid ANP↔Community switches discard stale responses by
  generation.
- Charging markers stay discoverable in ANP mode with "ANP does not supply
  electric tariffs" and an explicit "Show Community" switch; the selector is
  never silently disabled. No-price locations stay discoverable via the
  accessible list.
- Exact money/units preserved (`PriceAmount` BigDecimal, no float invention);
  municipal aggregates, wholesale, LPG cylinder and kWh/min/session tariffs
  are never relabeled as per-liter. Single lookup per selection (no N+1).
- Unknown is not zero/free; premium/additive stay distinct by product.

## Business Rules & Use Cases (Review Remediation)

- **B-BR-MAP-PRICE-WIRE-01**: Production `SheetPriceSource` (`RealSheetPriceSource`) must resolve prices from real repositories/use cases (`StationPriceRepository`, `PriceTableRepository`, `GetServerStationDetailUseCase`, `MunicipalityCatalogRepository`, `GetCommunityPriceGroupsUseCase`, `CityCommunityFeedReader`, `ChargingObservationGateway`, `UserPreferencesRepository`, `VehicleRepository`) using canonical UUID identity, selected fuel product, unit, source, and survey period. ANP uses the station's observation in the selected survey period; Community uses the validated non-expired projection (or EV community observation for charging). Exact decimal money (`BigDecimal` / integer milli-BRL) is preserved; absence of price (`"no-price"`) and lookup failure (`"load-failed"`) are distinct; electric tariffs are never invented in ANP (`"anp-no-tariff"`) and sources never fall back to each other.
- **B-BR-MAP-SHEET-RACE-01**: Every `MapSheetViewModel.select(...)` and `clear()` call must increment `generation` and cancel any active lookup `Job` immediately at entry — before early-return branches (such as charging in ANP) and before the 100 ms debounce. Stale completions must be rejected by `gen != generation` even when the underlying provider does not cooperate with coroutine cancellation.
- **B-BR-MAP-PRICE-COND-01**: `SheetPrice` must transport `conditionKind` (and optional `conditionQualifier`) and `MapBottomSheet` must display the condition/qualifier explicitly. Conditional community prices (`LOYALTY`, `APP`, `CASH`, `OTHER`) and conditional EV charging observations (`OFF_PEAK`, `PEAK`, `MEMBER`) are never converted to `STANDARD`; `STANDARD` is preferred when available, and when `STANDARD` is absent/unavailable/expired a valid conditional group is presented with its exact condition/qualifier requirement. Unrecognized or `UNKNOWN` conditions/connectors, incompatible units (`L` vs `M3` vs `KG_13`), divergent fuel products, incompatible EV connectors (when a connector filter is active), and malformed or divergent survey periods (never silently falling back to another period when a non-blank period is invalid) are rejected before presentation.
- **BUC-MAP-19-01**: Selecting a station or charger on the map, switching ANP ↔ Community (via top filter chips or the sheet's "Show Community" button), or changing selected product/period reloads the sheet for the active UUID without removing/reinserting pins, and profile/report actions respect the selected fuel product or route charging markers to `charging-community/{placeId}` instead of fuel capture.
- **BUC-MAP-19-02**: Selecting a station with conditional-only prices, multiple price groups/conditions, or EV charging observations with connector/condition constraints shows the exact product, unit, amount, date, and condition qualifier (or explicit absence when incompatible), without exposing conditional prices as generic `STANDARD`.


## Implementation

- `app/ui/map/MapSheetViewModel.kt`: immediate generation invalidation and job cancellation before branches/debounce, ANP-tariff guard for charging, explicit absence vs load-failure reasons.
- `RealSheetPriceSource` + `MapSheetModule.kt`: production Hilt binding wired to real ANP/Community/EV repositories and user/vehicle preferences.
- `app/ui/map/MapBottomSheet.kt`: ModalBottomSheet with name, services, source, exact price/unit/date, stale/failure/absence distinction, quality, route/profile/report actions.
- `MapScreen.kt` + `AnpNavGraph.kt`: selection and source/product/period changes reload the sheet; profile/report pass the active fuel product for fuel stations and route charging stations to `map/charging/{placeId}`.

## Verification executed

JVM (`:app:testDebugUnitTest`, 5 cases):

- ANP price shows exact product/unit/date;
- absence explicit without fallback; source switch never paints old response;
- rapid switches discard stale (ANP 5.99 vs Community 6.10);
- charging in ANP explains missing tariff without lookup;
- no-price locations stay discoverable with UUID preserved.

Phone shell proof reuses MAP-17/18 `MapScreenInstrumentedTest`; full sheet
wiring (profile/route/report navigation) is exercised in MAP-20 gate.

## Limits

- The earlier default-absence binding was replaced by the real source binding;
  current source/condition/unit/period regressions are in the remediation record.
  Live on-device station-price acceptance still needs admitted staging data.
- Native route preview (`buildCarRoute`) owns MAP-22; the interim external
  `geo:` action is explicit and never claimed as native navigation.
