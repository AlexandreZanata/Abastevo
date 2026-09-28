# UC-002 — Onboarding and First Launch

| Field | Value |
|-------|-------|
| **ID** | UC-002 |
| **Name** | Onboarding and First Launch |
| **Actors** | End User |
| **Layer** | UI → Application |

## Goal

Introduce the app value proposition and guide the user through survey week selection and the first data sync.

## Preconditions

- App installed, no imported `SurveyWeek` (`DataReadiness.EMPTY`).

## Main flow

1. User opens app for the first time.
2. System shows onboarding screens (max 3):
   - Official ANP data source (BR-009).
   - Weekly updates, offline after sync.
   - Survey week selection overview (v2).
3. User taps **Get started** on the last intro slide.
4. System runs UC-009 week catalog discovery. If `autoDownloadLatestWeek` is enabled (BR-020, default), auto-selects latest week and syncs; otherwise shows the week picker (BR-018).
5. When week picker is shown: user taps **Use latest week** or selects a specific historical week.
6. System persists `activeSurveyWeek`, emits `SurveyWeekSelected`, and triggers UC-001 scoped to the selected week (`SyncRequested(FIRST_LAUNCH)`).
7. On `SyncJobCompleted` success, system sets onboarding complete flag.
8. System navigates to UC-012 location prompt (optional GPS) or UC-003 if user skips GPS, or home if `preferredMunicipality` already exists.

## Alternative flows

### A1 — User skips sync (offline first open)

- **WHEN** user dismisses sync on onboarding or week picker  
- **THEN** remain in `EMPTY` state  
- **AND** home shows empty state with prominent sync CTA

### A2 — Sync fails on first launch

- **WHEN** UC-001 fails  
- **THEN** show error with retry on week picker  
- **AND** do not mark onboarding complete until at least one successful summary import

### A3 — Week catalog discovery fails

- **WHEN** ANP listing cannot be fetched during onboarding  
- **THEN** show structured error with retry on week picker  
- **AND** user may go back to intro slides or skip sync (A1)

### A4 — User chooses manual location after sync

- **WHEN** user declines device location on UC-012 prompt or taps **Choose manually**  
- **THEN** navigate to UC-003 manual state + city picker  
- **AND** set `locationPromptCompleted = true`

### A5 — Device location resolves city successfully

- **WHEN** user grants location and reverse geocode matches catalog  
- **THEN** persist preferred municipality via UC-003 and navigate to Home  
- **AND** set `locationPromptCompleted = true`

## Business rules

- BR-005, BR-009, BR-011, BR-018, BR-019, BR-020, BR-021

## Domain events

- `SurveyWeekSelected` (v2, via UC-009)
- `SyncRequested(FIRST_LAUNCH)`
- `PreferencesUpdated` (`activeSurveyWeek`, `onboardingCompleted`, `locationPromptCompleted`)

## Postconditions

- `onboarding_completed` preference is true only after first successful summary import.
- `activeSurveyWeek` reflects user choice when sync was started (BR-019).
- User understands data is weekly and from ANP.

## i18n keys

- `onboarding_title_welcome`
- `onboarding_body_anp_source`
- `onboarding_body_offline`
- `onboarding_action_get_started`
- `week_picker_title`, `week_picker_latest`, `week_picker_updated_at`, `week_picker_operational_note` (UC-009)
- `onboarding_location_prompt_title`, `onboarding_location_prompt_body`, `onboarding_location_use_device`, `onboarding_location_choose_manual` (UC-012)
- `geocoding_osm_attribution` (UC-012)

## Related documentation

- [uc-009-select-survey-week.md](uc-009-select-survey-week.md) — week picker after intro slides (Phase 12.3)
- [uc-012-resolve-location-from-device.md](uc-012-resolve-location-from-device.md) — optional GPS step after sync
