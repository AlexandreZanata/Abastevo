# UC-015 — Find Nearest Best-Price Station

| Field | Value |
|-------|-------|
| **ID** | UC-015 |
| **Name** | Find Nearest Best-Price Station |
| **Actors** | End User |
| **Layer** | UI → Application → Domain → Infrastructure |

## Goal

From the station list of a fuel product, recommend the **cheapest station close to the user** and open
Google Maps on it, so the user does not have to compare rows manually.

ANP station data has **no coordinates**, so candidate addresses are resolved through Nominatim (BR-021).

## Preconditions

- Station prices for the selected `FuelProduct` are imported and displayed (UC-007).
- Device location permission available — granted during onboarding (UC-012) or requested by this flow.
- Network available when a candidate address is not yet in the geocode cache.
- Search radius configured in Settings (3/5/10/15 km, default 3 km).

## Main flow

1. User taps **Cheapest near me** on the station list.
2. System resolves device coordinates (`DeviceLocation` — ephemeral, never persisted, BR-021):
   the freshest fix available is requested with a bounded timeout; a stale cached fix is only
   reused when it is recent, so a week-old cached position is never used silently.
3. System keeps at most the 8 cheapest stations as candidates (BR-028).
4. For each candidate, system builds the address-only geocoding query (street, municipality, full
   state name, country — **no station trade name**, which Nominatim does not resolve) and resolves
   address → coordinates through Nominatim `/search` (BR-021: local cache, max 1 request per second).
5. System discards candidates priced above the tolerance band (2% above the cheapest), discards
   candidates beyond the user-configured search radius (3/5/10/15 km — Settings, default 3 km,
   BR-028) and picks the nearest remaining candidate; distance ties are broken by lowest price.
6. System emits `StationNavigationRequested` and opens Google Maps with the station query (UC-013).
7. UI shows a progress state on the button while steps 3–5 run.

## Alternative flows

### A1 — Location permission not granted

- **WHEN** the app is not allowed to use location
- **THEN** request the permission
- **AND** when the user denies it, show `stations_nearest_needs_location` and stop (no navigation)

### A2 — Permission granted but no location fix

- **WHEN** the device has no recent location available and no fresh fix arrives within the timeout
- **THEN** show `stations_nearest_no_fix` and stop

### A3 — Geocoding unavailable

- **WHEN** the network fails or the Nominatim throttle cannot be honoured
- **THEN** abort immediately — no partial or guessed result — and show `stations_nearest_failed`
- **AND** the station list stays usable (BR-004)

### A4 — Station detail not imported

- **WHEN** the screen shows the on-demand download prompt instead of a list (UC-007 A1)
- **THEN** this action is not offered, because there are no station rows

### A5 — No candidate address resolves

- **WHEN** every candidate address returns no geocoding hit
- **THEN** show `stations_nearest_failed`

### A6 — No candidate within the search radius

- **WHEN** every geocoded candidate lies beyond the configured radius from the device (BR-028)
- **THEN** show `stations_nearest_failed` — an honest "nothing close enough" instead of a far station

### A7 — Offline

- **WHEN** there is no network
- **THEN** the offline banner is shown (BR-004) and the action fails with `stations_nearest_failed`

## Business rules

- BR-004, BR-021, BR-026, BR-028

## Domain events

- `StationNavigationRequested` — CNPJ and navigation query of the recommended station

## Postconditions

- Google Maps is open on the recommended station.
- Device coordinates are **not** persisted; resolved addresses are cached locally in the geocode cache.
- A Nominatim network failure leaves the imported price data untouched (BR-011).

## i18n keys

- `stations_nearest_action`
- `stations_nearest_needs_location`
- `stations_nearest_failed`
- `geocoding_osm_attribution`
- `stations_navigate_no_app` (UC-013)

## Related documentation

- [uc-007-view-station-prices.md](uc-007-view-station-prices.md) — host screen
- [uc-012-resolve-location-from-device.md](uc-012-resolve-location-from-device.md) — location permission and Nominatim compliance
- [uc-013-navigate-to-station.md](uc-013-navigate-to-station.md) — external navigation and address normalization
