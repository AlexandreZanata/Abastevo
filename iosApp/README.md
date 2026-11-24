# Abastevo for iPhone and iPad

The native app runs on iOS 17 and later. Open `AnpFuel.xcodeproj` and select the shared `AnpFuel` scheme. `AnpFuelShell` owns SwiftUI presentation. `AnpFuelCore` contains business rules, wire codecs, storage and platform adapters. The app bundle identifier is `com.anpfuel.app`.

The parity contract describes supported journeys, backend boundaries, and staging and device acceptance. The public read contract covers source attribution, offline cache, missing data and map pin eligibility. [Android and iOS parity](PARITY.md) · [Public reads](PUBLIC_READS.md)

- **Explore and community.** Browse verified ANP weeks, the station dossier, registry and community prices, quality data and enforcement. Forum, observation and station-feedback journeys use their public and signed routes with private blocking and reporting.
- **Account and privacy.** Sign in with Apple or an account key, or create a free account with a public name and a verified recovery email. A lost key is recovered by email. The account sheet edits the public name and the recovery email. A live account session binds the device key for owner-signed operations. Export local data or a single server file with per-section coverage. Review the hash-bound Portuguese training terms and manage the optional preference. Training image collection is disabled.
- **Map and driving.** Search Abastevo stations, admitted chargers and Apple Maps places, or use owner-scoped saved places and GPX. MapKit previews routes and provides active guidance. Use Apple Maps downloads for offline directions. Driver shifts accept manual odometer readings signed out, or an explicitly recorded GPS estimate with incomplete coverage. Hosted earnings and reports use the selected cloud vehicle.
- **Vehicles, ledger and Premium.** Edit vehicles, keep sanitized photos on the device, and set per-vehicle price-drop alerts. The ledger offers exact local records, monthly analysis and CSV, opt-in signed sync, server reports and comparisons. Advanced alerts use existing rules and inbox routes. App Store products, purchases and restore use StoreKit with server verification. [Store billing](../docs/product/payments/store-billing.md)
- **Abastevo Empresas.** A company session and device enrollment scope missions, trip start and finish, pause/resume/cancel, fueling, expenses and departure/return checklists to the selected organization. The protected signed queue holds commands and receipts. Company tracking stays off. See the parity contract for fueling authorization and checklist photo limits.

The app follows the OS language, including iOS per-app language settings. English, Brazilian Portuguese, Spanish, German, French, Japanese, Russian and Simplified Chinese are available. The Shell and Core String Catalogs hold app copy, and the InfoPlist catalog holds permission prompts. Preserve the exact bytes of hash-bound legal documents.

## Verification

From the repository root on Linux or macOS, run the structural and localization checks. When adding Swift files or resources, update the checked-in project with `python3 scripts/ios_sync_xcodeproj.py` and confirm a clean `--check`.

```sh
python3 scripts/ios_sync_xcodeproj.py --check
python3 -m unittest -v scripts.tests.test_ios_app_integrity scripts.tests.test_ios_localization
bash scripts/check-mobile.sh --static-only
```

Run the app build and XCTest on macOS with Xcode 26.4.1 and an iOS 26.4.1 simulator runtime.

```sh
cd iosApp
xcodebuild test -project AnpFuel.xcodeproj -scheme AnpFuel \
  -destination 'platform=iOS Simulator,name=iPhone 17,OS=26.4.1' \
  CODE_SIGNING_ALLOWED=NO
```

The [iOS workflow](../.github/workflows/ios.yml) runs these checks for ready pull requests targeting `main` and pushes to `main` that affect iOS inputs. The project takes the Android municipality catalog and shared fixtures from their repository paths.

## Running on a simulator or device

1. In Xcode, choose the `AnpFuel` scheme and an iPhone or iPad simulator, then press Run. A fresh install uses `https://abastevo.com.br`. The app reopens the last selected environment. Change it in Settings while signed out with no retained account key.
2. For a physical build, configure your Apple Developer team in Signing & Capabilities. Device acceptance uses the provisioned Sign in with Apple capability, associated domains and the `com.anpfuel.app` identifier.
3. For Premium, configure the App Store channel on the backend and sign in with a Sandbox Apple Account on the device. The app displays the products in the server's `app_store` offer using StoreKit's current prices and periods. [Store billing integration](../docs/backend/payments/store-billing-integration.md)
4. Verify the owner routes and receipts against the staging configuration. Run the physical-device cases in the parity contract, especially Keychain reinstall, camera and media expiry, notifications, location permission, lock-screen guidance and driver distance, voice, Apple Maps offline routing, Apple sign-in, StoreKit and accessibility.
