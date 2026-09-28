# UC-005 — View Municipality Fuel Prices

| Field | Value |
|-------|-------|
| **ID** | UC-005 |
| **Name** | View Municipality Fuel Prices |
| **Actors** | End User |
| **Layer** | UI → Application → Domain |

## Goal

Display average, min, and max prices for all available `FuelProduct` values in the selected municipality for the latest (or selected) `SurveyWeek`.

This is the **fuel averages detail** screen. It is reached from Home via **View full price details**. Tapping a `FuelProduct` card on Home does **not** open this screen — it opens UC-007 directly for that fuel (one-tap shortcut).

## Preconditions

- `CitySelected` or preferred location set.
- `AveragePrice` records exist for municipality + `SurveyWeek` (or empty state per BR-010).

## Main flow

1. System resolves municipality, state, and `SurveyWeek` (BR-006).
2. System loads all `AveragePrice` rows for that scope.
3. UI displays:
   - Municipality and state (localized formatting).
   - Survey week date range (localized dates).
   - Last sync timestamp.
   - ANP attribution footer (BR-009).
4. For each `FuelProduct` with data, show:
   - Localized product name (i18n).
   - Average price (currency formatted).
   - Min / max range.
   - Station count surveyed.
5. User taps a fuel row → emit `FuelProductSelected` → navigate to UC-007 (if station detail) or expand detail inline.

> **Entry point note:** Home fuel cards skip this screen and navigate straight to UC-007 with the tapped `FuelProduct` (−1 step). This screen keeps the min/avg/max + station-count comparison view reachable from Home.

## Alternative flows

### A1 — No fuels for municipality

- **WHEN** zero rows for municipality in selected week  
- **THEN** empty state (BR-010) using `dataAvailability`:
  - `NO_DATA_THIS_WEEK` → `prices_empty_no_data_this_week`
  - `NEVER_IN_ANP` → `prices_empty_never_in_anp`
- **AND** show `operationalNote` banner when non-null (Phase 12 week catalog)

### A2 — User switches survey week

- **WHEN** user selects older week from history selector  
- **THEN** reload prices for that `SurveyWeek` (UC-006)

### A3 — Offline

- **WHEN** no network  
- **THEN** show cached prices + offline banner (BR-004)

### A4 — Stale data

- **WHEN** latest week older than 8 days  
- **THEN** show stale banner + refresh action

## Business rules

- BR-004, BR-006, BR-009, BR-010

## Domain events

- `FuelProductSelected` (optional, on row tap)

## Postconditions

- User sees authoritative averages for their location.

## i18n keys

- `prices_title`
- `prices_survey_week_label`
- `prices_last_sync_label`
- `prices_station_count`
- `prices_anp_attribution`
- `fuel_product_ethanol` (etc. per `FuelProduct`)
- `prices_empty_municipality`
- `prices_empty_no_data_this_week`
- `prices_empty_never_in_anp`
