# UC-013 — Navigate to Station

| Field | Value |
|-------|-------|
| **ID** | UC-013 |
| **Name** | Navigate to Station |
| **Actors** | End User |
| **Layer** | UI → Application → Domain |

## Goal

Let the user open an external maps app (Google Maps, Waze, or system geo handler) to find a `RetailStation` from the station price list.

## Preconditions

- UC-007 station list displayed with at least one row.
- Preferred municipality and state available (manual UC-003 or UC-012).

## Main flow

1. User views station list (UC-007).
2. Each row shows station name, brand, normalized address snippet, price, and **Navigate** action.
3. User taps **Navigate** on a row.
4. System builds `StationNavigationQuery` (BR-026):
   - Normalize ANP `RetailStation.address` (trim, collapse whitespace).
   - Append preferred user municipality and state abbreviation.
   - Format query for geo search (e.g. `{address}, {municipality} - {state}, Brazil`).
5. System emits `StationNavigationRequested`.
6. System launches `Intent.ACTION_VIEW` with geo URI (`geo:0,0?q=...`) so user picks Maps, Waze, or other installed app.
7. External app opens; ANP Fuel Prices does not track navigation outcome.

## Alternative flows

### A1 — No maps app installed

- **WHEN** no activity resolves geo intent  
- **THEN** show error toast with i18n key `stations_navigate_no_app`

### A2 — Blank or low-quality ANP address

- **WHEN** address normalization yields insufficient text  
- **THEN** build query from station name + user municipality + state  
- **AND** still open chooser

### A3 — User municipality differs from station municipality

- **WHEN** station row municipality differs from preferred location (edge case in ANP data)  
- **THEN** prefer station's own municipality in query  
- **AND** include state from `RetailStation`

## Business rules

- BR-009, BR-026

## Domain events

- `StationNavigationRequested`

## Postconditions

- User can reach third-party navigation without leaving price context permanently.
- No coordinates stored by this app (external app handles routing).

## i18n keys

- `stations_navigate_action`
- `stations_navigate_no_app`
- `a11y_station_navigate`

## Related documentation

- [uc-007-view-station-prices.md](uc-007-view-station-prices.md) — station list host screen
