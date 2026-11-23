# Android and iOS functional parity

Baseline: Android and backend at `e77339125a71f07c3fbcdaed6a087a53ceff04f4`.

Acceptance is open. Each supported Android journey needs an equivalent iOS entry point, the same API or local business rule, and evidence for success, refusal, recovery and persistence where applicable. Use the same versioned fixtures in [contracts/testdata](../contracts/testdata). A screen, model, manifest declaration or sample result alone cannot satisfy acceptance.

## Remaining acceptance by journey

- Explore and ANP. The iOS client reads the directory, scoped community feed and dated station references. Compare Android `HomeViewModel`, `WeekPickerViewModel`, and `HistoryViewModel` for weeks, downloads, city averages, trends, price alerts, manual and automatic sync, and stale or offline results. Exercise the station dossier, verified sources, quality data, station/charger suggestions and the owner paths that are enabled on Android. Replay the ANP, consensus, registry, quality and mobile exploration fixtures.
- Community and forum. Public first-page reads cover forum topics and comments, contributors, and station comments. Start with `CommunityFeedViewModel`, `ForumFeedViewModel`, and the Android capture and station feedback flows. Cover pagination, search and replies, real signed submissions, upload, 24-hour expiry, retries, private owner history, confirmation and dispute, ratings, revision-bound votes, reports and blocking. Validate the identity, media, feedback, helpfulness and consensus contracts. A local price draft remains private.
- Identity and privacy. Validate email and recovery keys against staging, including expiry and replay. Add a server-supported native provider exchange, signed private reads, account export and deletion, owner isolation, and training permission status and revoke. Validate Keychain behavior on a physical device and the shared account, privacy and identity failure cases.
- Map and EV. Compare `MapDiscoveryViewModel`, `SavedViewModel`, `GuidanceViewModel`, `MapDownloadsViewModel`, and `EvTripViewModel`. Verify filters, map search and pin tiers, offline packages, saved places and GPX, accessibility and guidance recovery, and EV connector/corridor planning against admitted charging sources. The online MapKit preview is available for native verification.
- Vehicles, ledger and Premium. Compare `VehicleViewModel`, `LedgerEntriesViewModel`, `MonthlyIndicatorsViewModel`, `LedgerSyncViewModel`, and `PremiumViewModel`. Verify editing and photos, alerts, exact quantity and cost units, eligible energy and cost intervals, monthly indicators, owner export, sync/restore and conflicts, entitlement and controls. Replay vehicle admission and financial, energy and wire contracts. The XCTest suite reads the shared EE-23 interval cases. Combined-cost vectors and real record integration remain open.
- Driver Hub and Empresas. Exercise shift lifecycle, GPS opt-in, accounting and recovery, and compare Android invitation, context, trip, checklist, expense, receipt and offline-sync flows. Match server scope/role/entitlement refusals with no effects on another organization or personal context. Replay driver-suite and enterprise contracts. Real company operations remain unavailable on iOS.
- Settings and accessibility. Match the eight Android language resources, appearance, terms, onboarding, permissions, notifications, privacy controls, manual sync and retention. Verify VoiceOver, Dynamic Type, reduced motion, locale switching and iPhone/iPad layouts. Match both Android Arcade games, input options, per-game records and app scene lifecycle.

## B-BR-IOS-02 / BUC-IOS-02 — Origin and training consent

- Switching origin requires a signed-out client with no retained account key. A session or recovery key issued for one origin must never be forwarded to a different origin.
- The training screen can show the current terms. Grant, status and revocation require the authenticated owner and observable server receipt under the existing privacy contract. The iOS release keeps data collection off and shows account consent management as unavailable until the signed transport and durable revocation are verified.

## B-BR-IOS-01 / BUC-IOS-01 — Driving routes and offline maps

- A driving route uses the selected station's routable coordinates and a current, user-requested location. Directions and distance come from the navigation provider. Missing coordinates, denied location, and a failed provider request produce an unavailable state. An external navigator can still open a routable destination.
- A regional package becomes available offline after download, manifest and checksum verification, and successful opening by the map engine. A saved identifier, display sample, or cached online tile cannot establish package availability. On iOS, show the unavailable state until the engine and package lifecycle meet this contract.

Android references: `NativeRouteViewModel`, the native map engine, and the map region download flow. Acceptance uses a valid station and a routable current location, then repeats with permission denied, a provider error, and no station pin. For offline acceptance, verify download, process restart in airplane mode, route creation, and deletion. The iOS route preview is one-shot. Foreground guidance, rerouting, and regional offline navigation require separate execution evidence.

## B-BR-IOS-VEHICLE / BUC-IOS-VEHICLE — Local vehicle and ledger records

- An unsigned local registration can add one vehicle. Additional vehicle admissions require the authority defined by Android's `VehicleAdmissionRule`. Previously saved vehicles remain readable and removable.
- A battery capacity is required for electric and plug-in hybrid vehicles. The decimal kWh input must resolve to a whole number of Wh between 1 and 1,000,000,000, inclusive. Comma and period are accepted decimal separators. Other propulsion types store no battery capacity.
- The local ledger requires the user to choose a registered vehicle. Refueling requires a supported fuel and charging requires compatible propulsion. Numeric fields use exact integer units on iOS. Invalid or out-of-bounds values prevent saving and keep the draft available for correction.
- An energy or cost-per-distance indicator requires qualified measurement intervals. The current iOS ledger record has no measurement completeness or segment boundary information, so the UI reports the indicators as unavailable.

Android references are `VehicleAdmissionRule`, `BatteryCapacity`, `LedgerDraft`, `PremiumBounds`, and `PremiumEnergy`. The corresponding shared fixtures are `contracts/testdata/premium/vehicle-registration-v2.json` and `contracts/testdata/premium/energy-v1.json`.

## B-BR-IOS-DRIVER / BUC-IOS-DRIVER — Manual local shifts

A new local shift requires the user's selection of a registered vehicle, a valid initial odometer reading, and no other active shift. Closing the selected shift records the actual completion timestamp and validates the final odometer, exact centavo amounts within the Android `DriverMoney` bound, and the manually declared duration. Malformed inputs leave the shift open for correction. This iOS flow stores manual local records. GPS, work accounting, and synchronization require their own integration and acceptance tests.
