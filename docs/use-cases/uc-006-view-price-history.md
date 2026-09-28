# UC-006 — View Price History

| Field | Value |
|-------|-------|
| **ID** | UC-006 |
| **Name** | View Price History |
| **Actors** | End User |
| **Layer** | UI → Application → Domain |

## Goal

Show how average prices for a selected `FuelProduct` in a municipality changed across imported `SurveyWeek`s.

## Preconditions

- `showPriceHistory` preference is true (default).
- At least **2** imported `SurveyWeek`s for same municipality + fuel (otherwise show single-point message).

## Main flow

1. User opens history from UC-005 fuel detail.
2. System queries immutable `AveragePrice` history (BR-003) ordered by `SurveyWeek.startDate`.
3. UI renders trend list or simple chart (avg price vs week).
4. User can tap a week to view full UC-005 snapshot for that week.

## Alternative flows

### A1 — Only one week available

- **WHEN** single `SurveyWeek` in DB  
- **THEN** show message: "History available after more weekly syncs"

### A2 — Gap in weeks

- **WHEN** missing intermediate weeks (user didn't sync)  
- **THEN** show only available weeks, no interpolation (honest data)

## Business rules

- BR-003, BR-006

## Domain events

- None (read-only)

## Postconditions

- User understands price trend from stored ANP history only.

## i18n keys

- `history_title`
- `history_insufficient_data`
- `history_week_label`
