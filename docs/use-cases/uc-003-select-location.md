# UC-003 — Select Location (State + City)

| Field | Value |
|-------|-------|
| **ID** | UC-003 |
| **Name** | Select Location |
| **Actors** | End User |
| **Layer** | UI → Application → Domain |

## Goal

Let the user choose a Brazilian state and municipality to view fuel prices.

## Preconditions

- At least one `SurveyWeek` imported (BR-005).
- `DataReadiness` is `READY`, `PARTIAL`, or `STALE`.

## Main flow

1. User opens location picker from home or search tab.
2. System displays list of 27 `BrazilianState` values (localized display names via i18n).
3. User selects a state.
4. System loads **all catalog municipalities** for the state (BR-016), each annotated with `DataAvailability` for latest `SurveyWeek` (BR-006).
5. User selects a municipality from the list (including cities without current-week data).
6. System validates selection exists in `MunicipalityCatalog`.
7. System persists `preferredState` and `preferredMunicipality` (BR-012).
8. System emits `CitySelected`.
9. System navigates to UC-005 (municipality prices).

## Alternative flows

### A1 — Municipality has no data for selected week

- **WHEN** municipality list item has `DataAvailability.NO_DATA_THIS_WEEK` or `NEVER_IN_ANP`  
- **THEN** allow selection; UC-005 shows informative empty state (BR-010)

### A2 — State has no catalog municipalities

- **WHEN** municipality list is empty for state  
- **THEN** show empty state (BR-010) with message to pick another state

### A3 — User changes location from home

- **WHEN** user already has a preferred city  
- **THEN** picker pre-selects current state/municipality

## Business rules

- BR-005, BR-006, BR-010, BR-012, BR-016

## Domain events

- `CitySelected(municipality, state, surveyWeekId)`

## Postconditions

- Home screen reflects selected municipality.
- Preference survives app restart.

## i18n keys

- `location_picker_title`
- `location_state_label`
- `location_municipality_label`
- `location_empty_state`
- `location_no_data_this_week`
