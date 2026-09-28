# UC-011 — Estimate Tank Fill Cost

| Field | Value |
|-------|-------|
| **ID** | UC-011 |
| **Name** | Estimate Tank Fill Cost |
| **Actors** | End User |
| **Layer** | UI → Application → Domain |

## Goal

Show the estimated cost to fill each registered `Vehicle` tank on the home screen, using prices for the active `SurveyWeek` and the user's preferred municipality.

## Preconditions

- At least one imported `SurveyWeek` (BR-005) for price resolution.
- Preferred location set for meaningful estimates (BR-012).
- `ActiveSurveyWeek` resolved (BR-019).

## Main flow

1. User opens **Home** after location and sync are configured.
2. System loads registered `Vehicle` list (UC-010).
3. **IF** no vehicles registered:
   - Render fixed placeholder card below survey week label and above fuel price cards.
   - Show optimized CTA copy inviting user to track real fill-up cost.
   - Card tap navigates to UC-010.
4. **IF** one or more vehicles registered:
   - For each vehicle in `sortOrder`, compute `TankFillCostEstimate`:
     - Resolve unit price per `VehiclePriceSource` and BR-023.
     - Multiply by `TankCapacity`.
   - Render one card per vehicle (BR-024):
     - **Large** formatted total cost (primary visual).
     - Vehicle display name on secondary line.
     - Tank capacity in parentheses via i18n (e.g. `(50 L)`).
   - Card tap navigates to UC-010 for edit.
5. Section position is **below** ANP survey week metadata and **above** municipality `FuelPriceCard` list.

## Alternative flows

### A1 — Cheapest station mode

- **WHEN** `VehiclePriceSourceMode` is `CHEAPEST_STATION` and station detail exists  
- **THEN** use minimum `StationPrice` for vehicle fuel in preferred municipality  
- **ELSE** fall back to `AveragePrice.minimum` (BR-023)

### A2 — Specific station mode

- **WHEN** mode is `SPECIFIC_STATION` and CNPJ has price for active week  
- **THEN** use that `StationPrice`  
- **ELSE** show card with unavailable price state and hint (BR-010)

### A3 — No price data

- **WHEN** neither station nor average price available  
- **THEN** show vehicle card with empty price state (BR-010), not generic error

### A4 — Offline

- **WHEN** no network  
- **THEN** compute from cached prices (BR-004)  
- **AND** show offline banner if already shown on home

### A5 — User changes active week

- **WHEN** user changes `activeSurveyWeek` via week chip  
- **THEN** recalculate all estimates for new week

## Business rules

- BR-004, BR-010, BR-019, BR-022, BR-023, BR-024

## Domain events

- None directly (reads vehicle and price data; vehicle changes emit UC-010 events)

## Postconditions

- User sees personalized tank fill cost per vehicle for the active survey week.
- Layout slot remains stable whether placeholder or filled cards are shown.

## i18n keys

- `home_tank_fill_cost_title`
- `home_tank_fill_cost_placeholder_title`
- `home_tank_fill_cost_placeholder_body`
- `home_tank_fill_cost_liters_format`
- `home_tank_fill_cost_unavailable`
- `home_tank_fill_cost_station_name` (optional subtitle when specific station used)

## Related documentation

- [uc-010-manage-vehicles.md](uc-010-manage-vehicles.md)
- [uc-005-view-municipality-prices.md](uc-005-view-municipality-prices.md)
