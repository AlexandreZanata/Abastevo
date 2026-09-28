# Privacy Policy — ANP Fuel Prices

**Last updated:** 2026-06-21  
**App:** ANP Fuel Prices (`com.anpfuel.app`)

## Summary

ANP Fuel Prices does **not collect, transmit, or sell personal data** to app-owned servers. Preferences, vehicle profiles, and imported fuel price data stay on your device. Optional features (device location, local notifications) are described below.

## Data stored on your device

| Data | Purpose | Shared externally |
|------|---------|-------------------|
| Selected city and fuel preferences | Remember your location choice | No |
| Registered vehicles (name, tank size, fuel type, station preference) | Tank fill cost estimates and alerts | No |
| Imported ANP price tables | Offline browsing | No |
| Language preference | UI localization | No |
| Sync and storage settings | App configuration | No |
| Price drop alert preferences | Local notifications after weekly sync | No |
| Reverse geocode cache (rounded coordinates → city) | Avoid repeat Nominatim calls (BR-021) | No |
| Address geocode cache (station address → coordinates) | Avoid repeat Nominatim calls when finding the nearest station (UC-015) | No |

## Data we do not collect

- Name, email, phone number, or account credentials (no login)
- Persistent GPS coordinate history (location is one-shot and not stored)
- Contacts, photos, or other device files
- Advertising identifiers or analytics events (no analytics SDK)

## Optional device location (UC-012)

If you choose **Use my location** during onboarding:

- The app requests Android location permission and reads your position **once** to resolve your municipality.
- Raw latitude/longitude is **not saved** on the device.
- The app may call the public [Nominatim](https://nominatim.openstreetmap.org/) API (OpenStreetMap) over HTTPS: reverse geocoding to resolve your city (UC-012) and forward geocoding of candidate **station addresses** to find the nearest station (UC-015). Station addresses are public ANP data, not personal data.
- A successful result (state + municipality) is saved as your preferred city, same as manual selection.
- Rounded coordinate → city mappings may be cached locally to reduce network use.

You can always choose **Choose manually** and skip location entirely.

## Local notifications (UC-014)

If you enable price drop alerts on a vehicle:

- Notifications are generated **on your device** after new ANP data is imported.
- No notification content is sent to our servers (there is no backend).
- Android may require the **Notifications** permission (Android 13+).

## Network usage

| Destination | When | Data sent |
|-------------|------|-----------|
| **gov.br / ANP** | Sync weekly price tables | None (anonymous HTTP GET of public files) |
| **nominatim.openstreetmap.org** | Optional reverse geocode (UC-012) and station address geocode (UC-015) | Latitude, longitude or station address, app User-Agent |

No third-party analytics or tracking services are used.

## OpenStreetMap / Nominatim attribution

Geocoding results © [OpenStreetMap](https://www.openstreetmap.org/copyright) contributors, via Nominatim usage policy. Attribution is shown in the app where this feature is offered.

## Public ANP data displayed

Station names, addresses, and CNPJ numbers shown in the app come from **public ANP surveys** published by the Brazilian government. The app is **not affiliated with ANP**.

## Children's privacy

The app is a general-audience fuel price reference tool and does not knowingly collect information from children.

## Changes

Material changes to this policy will be noted in the project repository and release notes.

## Contact

Open an issue on the project repository:  
https://github.com/AlexandreZanata/TABELA-ANP-COMBUSTIVEIS/issues

## Open source

Source code is available under the [MIT License](../LICENSE).
