# UC-004 — Search Municipality

| Field | Value |
|-------|-------|
| **ID** | UC-004 |
| **Name** | Search Municipality |
| **Actors** | End User |
| **Layer** | UI → Application |

## Goal

Find a municipality quickly by typing its name instead of browsing by state.

## Preconditions

- Municipality catalog seeded (BR-016) **or** at least one `SurveyWeek` imported (BR-005) during v1→v2 transition.
- FTS index populated from `municipality_catalog` (v2) or `average_price` table (v1).

## Main flow

1. User focuses search field on home or dedicated search screen.
2. User types query string.
3. **WHEN** query length < 2 characters (BR-007)  
   **THEN** show hint, no search executed.
4. **WHEN** query length ≥ 2  
   **THEN** system queries FTS with debounce (300 ms).
5. System returns matches: municipality name + state abbreviation, ranked by relevance (BR-017).
6. User taps a result.
7. System emits `CitySelected`.
8. System navigates to UC-005.

## Alternative flows

### A1 — No results

- **WHEN** FTS returns empty  
- **THEN** show `SEARCH_NO_RESULTS`  
- **AND** suggest browsing by state (UC-003)

### A2 — Ambiguous names

- **WHEN** multiple municipalities share similar names (e.g. "São Paulo" city vs state context)  
- **THEN** always show state in result row  
- **AND** disambiguate by `state + municipality` pair

### A3 — Offline search

- **WHEN** no network  
- **THEN** search works entirely on local FTS (BR-004)

### A4 — City in catalog but no ANP data this week

- **WHEN** user selects a `MunicipalityCatalogEntry` with `DataAvailability.NO_DATA_THIS_WEEK` or `NEVER_IN_ANP`  
- **THEN** navigate to UC-005 and show informative empty state (BR-010)  
- **AND** display subtitle in search results when `NO_DATA_THIS_WEEK` (`search_no_data_this_week`)

## Business rules

- BR-004, BR-005, BR-007, BR-010, BR-012, BR-016, BR-017

## Domain events

- `CitySelected`

## Postconditions

- Selected municipality stored as preference (BR-012).

## i18n keys

- `search_municipality_hint`
- `search_min_chars_hint`
- `error_search_no_results`
- `search_no_data_this_week`
