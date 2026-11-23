# iosApp — Complete iPhone & iPad Application (v3.1.18)

Synchronized with `main`: full native Swift / SwiftUI iOS 17+ application (`AnpFuel.xcodeproj` + Swift Package `AnpFuel`), sharing exact portable contracts and HTTP/crypto protocols with Android and the Go backend.

## Architecture & Modules

- `AnpFuel.xcodeproj` & `project.yml` — Native Xcode iOS project (`com.anpfuel.app`, iOS 17+, iPhone & iPad) wiring `AnpFuelCore`, `AnpFuel`, `AnpFuelCoreTests`, `Assets.xcassets` (`AppIcon`), `App/Info.plist`, `App/PrivacyInfo.xcprivacy`, and `App/AnpFuel.entitlements`.
- `Package.swift` — Swift Package (`AnpFuel`, platforms `.iOS(.v17)` and `.macOS(.v14)`) for headless core tests.
- `Sources/AnpFuelCore/`:
  - Portable domain & application kernels matching Go/Kotlin 1:1: `PortableMoney`, `PortableText`, `PortableIdTime`, `PortableAuth`, `PortableAuthFlow`, `PortableAnonymousProof`, `PortableFeedback`, `PortableLocation`, `PortableLocationRecovery`, `PortablePhoto`, `PortablePhotoFlow`, `PortableOutboxAndSync`, `PortableOcrAndFraming`, `TankFillUseCase`, and `DomainModels` (all 8 fuel products, 27 Brazilian states, exact EE-23 integer rational energy calculator, RVA08 `training-terms-v1` with SHA-256 verification, EV trip planner, Enterprise & Driver Hub models, and Arcade games engine).
  - Native iOS platform adapters: `IosDeviceProofSigner` (`CryptoKit` ECDSA P-256 + Keychain), `IosKeychainSecretStore` (`Security` framework, non-synchronizable), `IosImageIoPhotoCodec` (EXIF/GPS-stripping JPEG encoder <= 256 KiB), `IosTransientPhotoCache` (24-hour TTL, `isExcludedFromBackup = true`), `IosLocationManager` (`CLLocationManager.requestLocation()` one-shot only, `CLLocationSourceInformation.isSimulatedBySoftware`), `IosVoiceGuidance` (`AVSpeechSynthesizer` across 8 locales), `AbastevoHttpClient` (`URLSession` + RFC 9421 signed requests), and `IosLocalStore` (offline seed catalog + backup-excluded persistence + owner purge).
- `Sources/AnpFuelShell/`:
  - Complete SwiftUI presentation layer matching Android's Energia Light/Dark design system:
    - `TermsGateView` & 5-step `OnboardingFlowView`
    - **Explore** (`ExploreRootView`, `StationDetailSheetView`, `LocationPickerSheetView`, `WeekPickerSheetView`, `PriceHistorySheetView`, `EvChargingExploreSheetView`, `QualityReportsSheetView`, `SuggestStationSheetView`, `MasterFaqSheetView`)
    - **Community** (`CommunityRootView`, `CapturePriceSheetView` with `PhotosPicker`/camera & `Vision` OCR + live zoom framing, `ForumFeedSheetView`, `ForumPostDetailView`, `ForumNewTopicSheetView`, `ContributorsSheetView`, `MyContributionsSheetView`)
    - **Map** (`MapRootView` with `MapKit` station/EV pins, `NativeRouteSheetView` with voice guidance & Apple Maps/Waze launch, `EvTripPlannerSheetView`, `SavedPlacesSheetView`, `MapDownloadsSheetView`)
    - **Profile** (`ProfileRootView`, `AuthSheetView` with Sign in with Apple / Email OTP / Key account, `AccountManagementSheetView` with JSON export/deletion/blocked users, `VehiclesAndLedgerSheetView`, `DriverHubSheetView`, `EnterpriseHubSheetView`, `ArcadeGamesSheetView`, `TrainingConsentSheetView`, `SettingsSheetView`, `HelpSheetView`)

## Verification

On Linux/CI (without Xcode):

```sh
bash scripts/check-mobile.sh --static-only
python3 -m unittest -v scripts/tests/test_ios_app_integrity.py
```

On macOS with Xcode (iOS Simulator / physical iPhone):

```sh
cd iosApp
swift build
swift test
xcodebuild test -project AnpFuel.xcodeproj -scheme AnpFuel \
  -destination 'platform=iOS Simulator,name=iPhone 16'
```
