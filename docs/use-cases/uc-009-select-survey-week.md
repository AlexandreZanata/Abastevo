# UC-009 — Select Survey Week

| Field | Value |
|-------|-------|
| **ID** | UC-009 |
| **Name** | Select Survey Week |
| **Actors** | End User |
| **Layer** | UI → Application → Domain |
| **Introduced** | v2 (Phase 12) |

## Goal

Let the user choose **which ANP survey week** to download or browse — defaulting to the latest discoverable week — before sync starts, matching the gov.br listing page structure.

## Preconditions

- Network available for catalog discovery (offline users with a prior selection use cached `activeSurveyWeek`).
- No other `SyncJob` in active state when sync is triggered (BR-015).

## Main flow

1. User opens the app on first launch, taps **Change week**, or has no `activeSurveyWeek` preference while `autoDownloadLatestWeek` is disabled (BR-018).
2. System runs `DiscoverSurveyWeekCatalogUseCase` (Phase 12.3) or reads cached catalog metadata.
3. System shows **WeekPickerScreen** with weeks ordered newest-first (gov.br parity).
4. User taps **Use latest week** (`SurveyWeekSelectionMode.LATEST`) or selects a specific historical week (`SPECIFIC`).
5. System persists `activeSurveyWeek` in user preferences (BR-019).
6. System emits `SurveyWeekSelected(surveyWeek, selectionMode)`.
7. System triggers UC-001 scoped to the selected `SurveyWeek` only (summary mandatory; station per BR-008 / preference).
8. On successful import, UI navigates to location selection or home with **Active week chip** visible.

## Alternative flows

### A1 — Returning user with imported active week

- **WHEN** `activeSurveyWeek` is set and summary data exists locally  
- **THEN** skip week picker on launch  
- **AND** home shows active week chip (tap opens picker sheet)

### A2 — Active week not yet imported locally

- **WHEN** user selects week W that is not in local DB  
- **THEN** start targeted sync for W  
- **AND** show sync progress until summary import completes

### A3 — Catalog discovery fails

- **WHEN** ANP listing page cannot be fetched  
- **THEN** show structured error with retry  
- **AND** if any week was previously imported, allow browsing cached weeks without changing selection

### A4 — Operational note on selected week

- **WHEN** `SurveyWeekCatalogEntry.operationalNote` is non-null  
- **THEN** show collapsible banner on week picker and/or home (i18n `week_picker_operational_note`)

### A5 — Auto-download latest week enabled (BR-020)

- **WHEN** `autoDownloadLatestWeek` is true (default)  
- **THEN** skip week picker on onboarding and cold start  
- **AND** run `AutoDownloadLatestWeekUseCase` to select catalog latest and sync (UC-001)

## Business rules

- BR-001, BR-006, BR-011, BR-015, BR-018, BR-019, BR-020

## Domain events

- `SurveyWeekSelected`
- `SyncRequested` (scoped to selected week)
- `PreferencesUpdated` (`activeSurveyWeek` key)

## Postconditions

- `UserPreferences.activeSurveyWeek` reflects user choice.
- Target week's `WEEKLY_SUMMARY` imported when sync succeeds.
- All price screens resolve display week via BR-019 (active week overrides BR-006 default).

## UI requirements

- i18n keys: `week_picker_title`, `week_picker_latest`, `week_picker_updated_at`, `week_picker_operational_note`, `week_picker_download_week`, `active_week_label` (Phase 12.4)
- Week row shows pt-BR date range label matching gov.br headings
- **Latest** chip on first catalog entry

## Related documentation

- [user-business-logic.md](../user-business-logic.md) — v2 week picker pillar
- [data-sources.md](../data-sources.md) — URL patterns and listing HTML structure
- [uc-001-sync-price-tables.md](uc-001-sync-price-tables.md) — extended for target week
- [uc-002-onboarding.md](uc-002-onboarding.md) — week picker after intro slides (Phase 12.3)
