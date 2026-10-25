# MAP-19 — Fast station prices and honest source switching

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

## Implementation

- `app/ui/map/MapSheetViewModel.kt`: generation-checked single lookup,
  ANP-tariff guard for charging, explicit absence reasons.
- `SheetPriceSource` + `MapSheetModule.kt`: Hilt binding defaults to
  explicit absence (never invents); tests inject exact fixtures.
- `app/ui/map/MapBottomSheet.kt`: ModalBottomSheet with name, services,
  source, exact price/unit/date, quality, route/profile/report actions.
- `MapScreen.kt`: selection opens the sheet; route uses explicit `geo:`
  external intent (never counted as native guidance); profile uses
  `mapStationPage`; report uses `capture`.

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

- Default Hilt `SheetPriceSource` returns explicit absence; full ANP week +
  community projection wiring owns its follow-up and must not invent prices.
- Native route preview (`buildCarRoute`) owns MAP-22; the interim external
  `geo:` action is explicit and never claimed as native navigation.
