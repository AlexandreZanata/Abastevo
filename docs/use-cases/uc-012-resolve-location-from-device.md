# UC-012 — Resolve Location from Device

| Field | Value |
|-------|-------|
| **ID** | UC-012 |
| **Name** | Resolve Location from Device |
| **Actors** | End User, System |
| **Layer** | UI → Application → Infrastructure |

## Goal

Optionally detect the user's municipality on first launch using device location and Nominatim reverse geocoding, skipping manual state/city selection when successful.

## Preconditions

- First onboarding completed through UC-001 sync success (UC-002).
- `locationPromptCompleted` preference is false.
- Network available for one-time Nominatim call (or valid cache hit).

## Main flow

1. After successful first sync, system shows **location prompt** step (extends UC-002):
   - Explain benefit: auto-select city for prices.
   - Actions: **Use my location** / **Choose manually**.
   - OSM/Nominatim attribution visible (BR-021, `GeocodingAttribution`).
2. User taps **Use my location**.
3. System requests runtime location permission (`ACCESS_FINE_LOCATION` or coarse fallback).
4. **IF** granted:
   - System obtains one-shot `DeviceLocation` (lat/lon).
   - System calls `ReverseGeocodeRepository` → Nominatim reverse API (BR-021).
   - System maps response to `ReverseGeocodeResult` (state + municipality).
   - System validates municipality against `MunicipalityCatalog` (BR-016).
   - System invokes UC-003 `SelectLocationUseCase` to persist `preferredState` and `preferredMunicipality`.
   - System emits `DeviceLocationResolved` and `CitySelected`.
   - System sets `locationPromptCompleted = true`.
   - Navigate to Home (skip manual UC-003 picker).
5. **IF** denied or geocode fails:
   - Fall back to UC-003 manual flow without blocking.
   - Set `locationPromptCompleted = true`.

## Alternative flows

### A1 — Choose manually

- **WHEN** user taps **Choose manually**  
- **THEN** navigate to UC-003 state + municipality picker  
- **AND** set `locationPromptCompleted = true`

### A2 — Municipality not in catalog

- **WHEN** Nominatim returns unknown or non-Brazilian result  
- **THEN** show brief message  
- **AND** fall back to UC-003

### A3 — Rate limit or network error

- **WHEN** Nominatim unreachable or throttled  
- **THEN** fall back to UC-003  
- **AND** log error without crash

### A4 — Cached geocode

- **WHEN** rounded coordinates already cached locally  
- **THEN** skip network call (BR-021)  
- **AND** proceed with cached `ReverseGeocodeResult`

### A5 — Returning user

- **WHEN** `locationPromptCompleted` is true  
- **THEN** never show location prompt again  
- **AND** user changes city only via UC-003 / search

## Business rules

- BR-012, BR-016, BR-021

## Domain events

- `DeviceLocationResolved`
- `CitySelected` (via UC-003 persistence)

## Postconditions

- Preferred municipality may be set without manual picker.
- Device coordinates are **not** persisted (ephemeral `DeviceLocation` only).
- Successful reverse geocode results cached by rounded coordinates (BR-021).

## i18n keys

- `onboarding_location_prompt_title`
- `onboarding_location_prompt_body`
- `onboarding_location_use_device`
- `onboarding_location_choose_manual`
- `onboarding_location_permission_rationale`
- `onboarding_location_failed`
- `geocoding_osm_attribution`
- `settings_geocoding_attribution`

## Related documentation

- [uc-002-onboarding.md](uc-002-onboarding.md) — extended with location prompt step
- [uc-003-select-location.md](uc-003-select-location.md) — manual fallback
- [../adr/003-nominatim-reverse-geocode.md](../adr/003-nominatim-reverse-geocode.md)

## External API

- Nominatim public reverse endpoint: `https://nominatim.openstreetmap.org/reverse`
- Usage policy: https://operations.osmfoundation.org/policies/nominatim/
