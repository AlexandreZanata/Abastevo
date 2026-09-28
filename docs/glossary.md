# Domain Glossary

> Ubiquitous language for the ANP Fuel Prices project.
> Code identifiers **must** match these terms exactly.

## Business Terms

| Term | Definition |
|------|------------|
| **PriceSurvey** | A weekly ANP fuel price collection for a fixed date range (Sunday–Saturday). |
| **SurveyWeek** | The date range of a PriceSurvey (start date + end date). |
| **FuelProduct** | A fuel type tracked by ANP (enum). |
| **GeographicScope** | Aggregation level: `NATIONAL`, `REGION`, `STATE`, `MUNICIPALITY`, `STATION`. |
| **AveragePrice** | Statistical summary (mean, min, max, std dev) for a FuelProduct at a GeographicScope. |
| **RetailStation** | A licensed fuel retail point identified by CNPJ. |
| **StationPrice** | The price charged by a RetailStation for a FuelProduct on a collection date. |
| **PriceTable** | The downloadable XLSX file published by ANP for a SurveyWeek. |
| **PriceTableType** | Either `WEEKLY_SUMMARY` or `STATION_DETAIL`. |
| **SyncJob** | A background task that discovers, downloads, and imports new PriceTables. |
| **MunicipalityCatalog** | Persistent IBGE baseline (~5 570 municipalities) merged with ANP-published aliases for national search (v2). |
| **MunicipalityCatalogEntry** | A single municipality in the catalog: `state`, `municipality`, optional `ibgeCode`. ANP week availability is derived at query time, not stored. |
| **MunicipalityLocationKey** | Stable identity pair (`state` + `municipality`) for catalog matching and availability checks. |
| **DataAvailability** | Query-time annotation: `HAS_DATA`, `NO_DATA_THIS_WEEK`, or `NEVER_IN_ANP`. |
| **SearchMatchType** | FTS match quality tier for ranking: `EXACT_PREFIX`, `ACCENT_NORMALIZED`, `TYPO_TOLERANT`, `SUBSTRING`. |
| **SurveyWeekCatalogEntry** | Metadata for one ANP listing week block: `surveyWeek`, summary/station URLs, optional `publishedAt`, optional `operationalNote` (v2). |
| **SurveyWeekSelectionMode** | How the user chose a week in UC-009: `LATEST` or `SPECIFIC`. |
| **ActiveSurveyWeek** | User preference (`UserPreferences.activeSurveyWeek`) for the week currently displayed and synced (BR-019). |
| **Vehicle** | User-owned car profile: display name, `TankCapacity`, single `FuelProduct`, and `VehiclePriceSource`. |
| **TankCapacity** | Value object — tank capacity in liters (> 0, ≤ 200). |
| **TankFillCostEstimate** | Derived value: unit price × `TankCapacity` for the active `SurveyWeek`. |
| **VehiclePriceSourceMode** | How unit price is resolved: `CHEAPEST_STATION` or `SPECIFIC_STATION`. |
| **VehiclePriceSource** | `VehiclePriceSourceMode` plus optional `Cnpj` when mode is `SPECIFIC_STATION`. |
| **PriceDropAlert** | User preference on a `Vehicle` to notify when fuel price drops vs previous imported week. |
| **AlertPriceSource** | Price reference for alerts — same semantics as `VehiclePriceSourceMode` (+ optional CNPJ). |
| **DeviceLocation** | Ephemeral latitude/longitude from Android location APIs; not persisted as PII (UC-012, UC-015). |
| **GeoCoordinates** | Validated latitude/longitude pair (WGS-84) used for station distance calculation (UC-015). |
| **ReverseGeocodeResult** | Resolved `BrazilianState` and municipality name from coordinates. |
| **AddressGeocode** | Resolution of a normalized address into `GeoCoordinates` through Nominatim `/search` (`AddressGeocodeRepository`, UC-015). |
| **NearestStationRecommendation** | Selected `RetailStation` for UC-015: station, unit price, and distance in meters from the device. |
| **StationNavigationQuery** | Normalized address string plus municipality and state for external map apps. |
| **GeocodingAttribution** | OSM/Nominatim attribution requirement whenever geocoding is used — reverse (UC-012) or forward (UC-015) (BR-021). |

## FuelProduct Enum

| Value | ANP source label (Portuguese) | Unit |
|-------|-------------------------------|------|
| `ETHANOL` | ETANOL HIDRATADO / ETANOL | R$/liter |
| `GASOLINE_REGULAR` | GASOLINA COMUM | R$/liter |
| `GASOLINE_PREMIUM` | GASOLINA ADITIVADA | R$/liter |
| `DIESEL_S500` | OLEO DIESEL / DIESEL S500 | R$/liter |
| `DIESEL_S10` | OLEO DIESEL S10 / DIESEL S10 | R$/liter |
| `CNG` | GNV | R$/m³ |
| `LPG_P13` | GLP | R$/13 kg |

## BrazilianRegion Enum

`NORTH`, `NORTHEAST`, `CENTRAL_WEST`, `SOUTHEAST`, `SOUTH`

## BrazilianState

27 federative units. Stored as enum with `name`, `abbreviation` (e.g. `SAO_PAULO`, `SP`).

## VehiclePriceSourceMode / AlertPriceSource

Shared enum values:

| Value | Meaning |
|-------|---------|
| `CHEAPEST_STATION` | Minimum `StationPrice` for vehicle fuel in preferred municipality |
| `SPECIFIC_STATION` | Price at a chosen `RetailStation` identified by `Cnpj` |

## Domain Events

| Event | Fired when |
|-------|------------|
| `PriceTableDiscovered` | A new weekly file URL is found on the ANP page |
| `PriceTableDownloaded` | A PriceTable file is saved locally |
| `PriceTableImported` | Rows are parsed and persisted to local DB |
| `PriceTableImportFailed` | Parsing or validation failed |
| `SyncJobCompleted` | A SyncJob finishes (success or partial) |
| `CitySelected` | User selects a municipality for price lookup |
| `FuelProductSelected` | User selects a fuel to view detail or stations |
| `SyncRequested` | User or system triggers a sync (`MANUAL`, `SCHEDULED`, `FIRST_LAUNCH`) |
| `StationDetailRequested` | User requests on-demand station-level download |
| `PreferencesUpdated` | User changes a local preference |
| `CacheCleared` | User clears local data (`ALL` or `STATION_DETAIL_ONLY`) |
| `SurveyWeekSelected` | User selects a survey week for sync and display (UC-009) |
| `VehicleRegistered` | User saves a new `Vehicle` (UC-010) |
| `VehicleUpdated` | User edits an existing `Vehicle` (UC-010) |
| `VehicleRemoved` | User deletes a `Vehicle` (UC-010) |
| `PriceDropAlertConfigured` | User toggles or changes alert source on a vehicle (UC-014) |
| `DeviceLocationResolved` | Reverse geocode succeeded and matched `MunicipalityCatalog` (UC-012) |
| `StationNavigationRequested` | User taps navigate on a station row (UC-013) |

## Business Rules

### BR-001 — Valid Survey Week
**GIVEN** a PriceTable file name  
**WHEN** parsed for date range  
**THEN** start date must be before or equal to end date  
**AND** range must be ≤ 7 days

### BR-002 — Fuel Product Normalization
**GIVEN** a raw product label from ANP (Portuguese)  
**WHEN** imported  
**THEN** it must map to exactly one `FuelProduct` enum value  
**AND** unmapped labels must be logged and skipped (not crash)

### BR-003 — Immutable Price History
**GIVEN** an imported StationPrice or AveragePrice  
**WHEN** the same ANP file is re-imported with corrections  
**THEN** a new record is created referencing the SurveyWeek  
**AND** the previous record is never deleted or overwritten

### BR-004 — Offline Read
**GIVEN** no network connectivity  
**WHEN** user requests prices for a previously synced city  
**THEN** cached data must be returned  
**AND** UI must indicate stale/sync status

### BR-005 — Search Requires Imported Data
**GIVEN** the local database has zero imported `SurveyWeek` records  
**WHEN** user attempts search or location browse  
**THEN** the app must redirect to sync/onboarding  
**AND** must not execute an empty FTS query

### BR-006 — Default Survey Week
**GIVEN** multiple `SurveyWeek` records exist  
**WHEN** the app needs a default week for display  
**THEN** it must select the most recent successfully imported week by `endDate`  
**AND** never a failed or partial-import week unless explicitly chosen by the user

### BR-007 — Minimum Search Length
**GIVEN** the municipality search field  
**WHEN** the query has fewer than 2 characters  
**THEN** no FTS query is executed  
**AND** a hint is shown to the user

### BR-008 — Station Detail Opt-out
**GIVEN** `syncStationDetail` is enabled (default: `true`)  
**WHEN** UC-001 sync runs for a survey week  
**THEN** the system must download and import `STATION_DETAIL` together with `WEEKLY_SUMMARY`

**GIVEN** the user has disabled `syncStationDetail` and no local `StationPrice` exists  
**WHEN** user opens station list  
**THEN** the app must offer on-demand download  
**AND** must not auto-download station files in background sync

### BR-009 — ANP Data Attribution
**GIVEN** any screen displaying price data  
**WHEN** rendered to the user  
**THEN** ANP source attribution must be visible  
**AND** link to official ANP page must be available in settings

### BR-010 — Empty Municipality Is Not an Error
**GIVEN** a valid municipality with no rows for the selected `SurveyWeek`  
**WHEN** prices are requested  
**THEN** show empty state UI  
**AND** do not throw or show generic error

### BR-011 — Sync Failure Preserves Cache
**GIVEN** a sync job fails at any stage  
**WHEN** the failure is handled  
**THEN** existing imported data must remain intact  
**AND** no rollback of previously imported weeks

### BR-012 — Preferred Location Persistence
**GIVEN** user selects a municipality  
**WHEN** selection is confirmed  
**THEN** persist `preferredState` and `preferredMunicipality` locally  
**AND** restore on next app launch as default home location

### BR-013 — Station Detail Retention Window
**GIVEN** `stationDetailRetentionWeeks` is set to N  
**WHEN** retention cleanup runs after import  
**THEN** delete `StationPrice` rows older than the N most recent weeks  
**AND** never delete `AveragePrice` summary rows in the same operation

### BR-014 — Wi-Fi Only Background Sync
**GIVEN** `autoSyncOnWifi` is true  
**WHEN** WorkManager scheduled sync runs  
**THEN** require unmetered network constraint  
**AND** manual sync may proceed on metered with optional user confirmation

### BR-015 — Single Active Sync Job
**GIVEN** a `SyncJob` is in a non-terminal state  
**WHEN** another sync is requested  
**THEN** reject or queue the second request  
**AND** never run two imports concurrently

### BR-016 — Municipality Catalog Completeness
**GIVEN** IBGE catalog loaded  
**WHEN** user searches  
**THEN** every federative unit municipality appears in the index regardless of current week data  
**AND** catalog must contain at least 5 570 entries

### BR-017 — Intelligent Search Ranking
**GIVEN** query ≥ 2 characters  
**WHEN** FTS runs  
**THEN** rank matches as: exact prefix > accent-normalized prefix > typo-tolerant token prefix > substring  
**AND** always tie-break by state abbreviation (alphabetical)

### BR-018 — Week Selection Before Sync
**GIVEN** `autoDownloadLatestWeek` is disabled (BR-020)  
**WHEN** `UserPreferences.activeSurveyWeek` is null  
**THEN** show week picker before download starts  
**AND** do not trigger UC-001 until a week is selected

### BR-019 — Active Survey Week
**GIVEN** user selected week W and summary data for W is imported  
**WHEN** any price screen resolves the display week  
**THEN** use W instead of BR-006 default-latest  
**AND** keep W until user changes `activeSurveyWeek` or clears cache

### BR-020 — Auto-Download Latest Survey Week
**GIVEN** `autoDownloadLatestWeek` is enabled (default: `true`)  
**WHEN** onboarding completes, app cold-starts, or background sync runs  
**THEN** discover the ANP catalog, set `activeSurveyWeek` to the newest entry, and trigger UC-001 for that week  
**AND** skip the week picker unless the user disabled this preference

**GIVEN** the user disabled `autoDownloadLatestWeek`  
**WHEN** no `activeSurveyWeek` is set  
**THEN** require manual week selection (BR-018) before sync

### BR-021 — Nominatim Usage Compliance
**GIVEN** a reverse geocode request  
**WHEN** calling the public Nominatim API  
**THEN** send a custom `User-Agent` identifying the app  
**AND** enforce max 1 request per second (client-side throttle)  
**AND** cache successful rounded `(lat, lon)` → municipality results locally  
**AND** never run periodic bulk geocoding in background  
**AND** display OSM attribution where geocoding is used

### BR-022 — One Fuel Product per Vehicle
**GIVEN** a `Vehicle`  
**WHEN** saved  
**THEN** exactly one `FuelProduct` is bound  
**AND** tank fill cost uses only that product's price

### BR-023 — Tank Fill Cost Price Source
**GIVEN** a vehicle with `CHEAPEST_STATION` mode  
**WHEN** station detail exists for the active week  
**THEN** use the cheapest `StationPrice` for that fuel in the preferred municipality  
**ELSE** fall back to `AveragePrice.minimum` if available  
**AND** show informative empty state if neither exists (BR-010)

**GIVEN** a vehicle with `SPECIFIC_STATION` mode  
**WHEN** the chosen CNPJ has a price for the active week  
**THEN** use that `StationPrice`  
**ELSE** apply the same fallback chain as above

### BR-024 — Multiple Vehicles on Home
**GIVEN** the user registered N vehicles (N ≥ 1)  
**WHEN** home renders the tank fill cost section  
**THEN** show one card per vehicle in registration order  
**AND** preserve stable vertical layout (placeholder height matches filled card)

### BR-025 — Price Drop Alert Eligibility
**GIVEN** `PriceDropAlert` enabled for a vehicle  
**WHEN** a new `SurveyWeek` is imported and evaluated  
**THEN** compare current price source vs the previous imported week for the same municipality and fuel  
**AND** emit a local notification only if current < previous  
**AND** never notify without `POST_NOTIFICATIONS` granted

### BR-026 — Station Navigation Query
**GIVEN** a `RetailStation` address from ANP  
**WHEN** the user taps **Navigate**  
**THEN** build a `StationNavigationQuery` with normalized address plus municipality and state  
**AND** open the system geo app chooser (Maps, Waze, or other handlers)

### BR-027 — Maximum Registered Vehicles
**GIVEN** the user already has 3 saved vehicles  
**WHEN** attempting to add another  
**THEN** reject the operation with an informative UI message  
**AND** do not persist a fourth vehicle

### BR-028 — Nearest Best-Price Station
**GIVEN** imported station prices for one `FuelProduct` and the user's `DeviceLocation`  
**WHEN** the user requests the nearest best-price station (UC-015)  
**THEN** consider at most 8 cheapest candidates  
**AND** resolve candidate addresses through Nominatim with local cache and max 1 request per second (BR-021)  
**AND** accept only candidates priced within 2% above the cheapest  
**AND** accept only candidates within the user-configured radius (3/5/10/15 km, default 3 km) of the device location  
**AND** select the nearest candidate inside that band and radius, breaking ties by lowest price  
**AND** report failure — never a partial guess — when no candidate resolves, geocoding fails, or no candidate lies within the radius  
**AND** never persist device coordinates

## Acronyms

| Acronym | Meaning |
|---------|---------|
| ANP | Agência Nacional do Petróleo, Gás Natural e Biocombustíveis |
| IBGE | Instituto Brasileiro de Geografia e Estatística |
| LPC | Levantamento de Preços de Combustíveis (Fuel Price Survey) |
| CNPJ | Cadastro Nacional da Pessoa Jurídica (Brazilian company tax ID) |
| CNG | Compressed Natural Gas (GNV in Brazil) |
| LPG | Liquefied Petroleum Gas (GLP in Brazil) |
| OSM | OpenStreetMap — data provider for Nominatim geocoding |
