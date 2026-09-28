# UC-010 — Manage Vehicles

| Field | Value |
|-------|-------|
| **ID** | UC-010 |
| **Name** | Manage Vehicles |
| **Actors** | End User |
| **Layer** | UI → Application → Domain |

## Goal

Let the user register, edit, and remove one or more `Vehicle` profiles used for tank fill cost estimates and optional price drop alerts.

## Preconditions

- App reachable (no sync required to open the screen).
- Tank fill cost display on home (UC-011) requires preferred location and imported prices.

## Main flow

1. User opens **Vehicles** from home placeholder card (UC-011), navigation chip, or settings shortcut.
2. System loads all saved `Vehicle` records ordered by `sortOrder`.
3. User taps **Add vehicle**.
4. User enters:
   - **Display name** (e.g. car model nickname).
   - **Tank capacity** in liters (`TankCapacity`).
   - **Fuel product** — exactly one `FuelProduct` per vehicle (BR-022).
   - **Price source** (`VehiclePriceSource`):
     - `CHEAPEST_STATION` — cheapest `StationPrice` in preferred municipality for active week.
     - `SPECIFIC_STATION` — user picks a `RetailStation` by CNPJ from UC-007 list filtered by vehicle fuel.
   - **Price drop alert** toggle (`PriceDropAlert`, UC-014) — optional, default off.
5. System validates inputs (non-blank name, valid `TankCapacity`, fuel selected, CNPJ required when mode is `SPECIFIC_STATION`).
6. System persists vehicle locally and emits `VehicleRegistered` or `VehicleUpdated`.
7. User returns to previous screen; home refreshes tank cost section (UC-011).

## Alternative flows

### A1 — Edit existing vehicle

- **WHEN** user taps a vehicle in the list  
- **THEN** open form pre-filled  
- **AND** on save emit `VehicleUpdated`

### A2 — Delete vehicle

- **WHEN** user confirms delete  
- **THEN** remove record, emit `VehicleRemoved`  
- **AND** home removes corresponding tank cost card (BR-024)

### A3 — Maximum vehicles reached

- **WHEN** user attempts to add beyond UI limit (3 vehicles, BR-027)  
- **THEN** show informative message  
- **AND** do not persist

### A4 — Specific station unavailable

- **WHEN** mode is `SPECIFIC_STATION` but station detail not synced  
- **THEN** allow saving preference  
- **AND** UC-011 shows fallback or empty state per BR-023

### A5 — No preferred location

- **WHEN** user configures `CHEAPEST_STATION` or `SPECIFIC_STATION` without preferred municipality  
- **THEN** allow saving vehicle  
- **AND** home tank cost card shows hint to select location (UC-003)

## Business rules

- BR-010, BR-022, BR-023, BR-024, BR-027

## Domain events

- `VehicleRegistered`
- `VehicleUpdated`
- `VehicleRemoved`
- `PriceDropAlertConfigured` (when alert toggle or source changes)

## Postconditions

- Vehicle profiles persist across app restarts.
- Each vehicle binds exactly one `FuelProduct`.
- Home shows one tank cost slot per vehicle when UC-011 data is available.

## i18n keys

- `vehicle_screen_title`
- `vehicle_name_hint`
- `vehicle_tank_capacity_hint`
- `vehicle_fuel_product_label`
- `vehicle_price_source_label`
- `vehicle_price_source_cheapest`
- `vehicle_price_source_specific`
- `vehicle_select_station_label`
- `vehicle_price_drop_alert_label`
- `vehicle_add`
- `vehicle_edit`
- `vehicle_delete`
- `vehicle_delete_confirm`
- `vehicle_max_reached`
- `nav_vehicles`

## Related documentation

- [uc-011-estimate-tank-fill-cost.md](uc-011-estimate-tank-fill-cost.md) — home display
- [uc-007-view-station-prices.md](uc-007-view-station-prices.md) — station picker
- [uc-014-fuel-price-drop-alerts.md](uc-014-fuel-price-drop-alerts.md) — alert toggle
