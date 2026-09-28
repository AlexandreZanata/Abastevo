# UC-014 — Fuel Price Drop Alerts

| Field | Value |
|-------|-------|
| **ID** | UC-014 |
| **Name** | Fuel Price Drop Alerts |
| **Actors** | End User, System |
| **Layer** | UI → Application → Infrastructure |

## Goal

Notify the user locally when fuel price for a configured `Vehicle` drops compared to the previous imported `SurveyWeek` in the same municipality.

## Preconditions

- At least two imported `SurveyWeek` records exist (current and immediately prior by import order or `endDate`).
- User enabled `PriceDropAlert` on a vehicle (UC-010).
- Android `POST_NOTIFICATIONS` permission granted (Android 13+).

## Main flow

1. User enables **Price drop alert** on vehicle form (UC-010) and selects price source (`AlertPriceSource` — same semantics as `VehiclePriceSource`).
2. System persists preference and emits `PriceDropAlertConfigured`.
3. **WHEN** UC-001 completes successfully and `activeSurveyWeek` updates (or new week imported):
   - System runs `EvaluatePriceDropAlertsUseCase` (WorkManager chained or post-sync hook).
4. For each vehicle with alert enabled:
   - Resolve current week price per vehicle fuel and `AlertPriceSource` (cheapest station or specific CNPJ).
   - Resolve previous imported week price for same scope.
   - Apply `PriceDropDetectionRule` (BR-025): notify only if `current < previous`.
5. System posts local notification with vehicle name and price context.
6. User taps notification → open Home or Vehicles screen.

## Alternative flows

### A1 — Notification permission denied

- **WHEN** `POST_NOTIFICATIONS` not granted  
- **THEN** persist alert preference  
- **AND** show in-app banner linking to system settings  
- **AND** do not post notifications (BR-025)

### A2 — No previous week data

- **WHEN** only one week imported  
- **THEN** skip evaluation silently

### A3 — Price unchanged or increased

- **WHEN** current price ≥ previous  
- **THEN** no notification

### A4 — Station detail missing

- **WHEN** alert uses station source but station prices unavailable  
- **THEN** fall back to `AveragePrice` minimum for comparison (BR-023)  
- **OR** skip vehicle with debug log

### A5 — User disables alert

- **WHEN** toggle turned off  
- **THEN** emit `PriceDropAlertConfigured`  
- **AND** cancel pending notifications for that vehicle

### A6 — Offline sync

- **WHEN** sync completes from cached download while offline  
- **THEN** evaluation uses local data only (BR-004)

## Business rules

- BR-004, BR-019, BR-023, BR-025

## Domain events

- `PriceDropAlertConfigured`
- `SyncJobCompleted` (trigger for evaluation)

## Postconditions

- User may receive local notifications after weekly data updates.
- No notification data sent to external servers.
- Alert state tied to each `Vehicle` record.

## i18n keys

- `vehicle_price_drop_alert_label`
- `vehicle_price_drop_alert_description`
- `notification_price_drop_title`
- `notification_price_drop_body`
- `notification_permission_rationale`
- `notification_permission_denied_hint`

## Related documentation

- [uc-001-sync-price-tables.md](uc-001-sync-price-tables.md) — sync trigger
- [uc-010-manage-vehicles.md](uc-010-manage-vehicles.md) — configuration UI
- [../privacy-policy.md](../privacy-policy.md) — local notifications disclosure
