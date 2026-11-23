# Abastevo for iPhone and iPad

The native application targets iOS 17 and later. Open `AnpFuel.xcodeproj` and select the shared `AnpFuel` scheme. SwiftUI owns presentation, and `AnpFuelCore` contains the portable business rules, URLSession client, and iOS adapters. The project preserves `com.anpfuel.app`.

The [parity contract](PARITY.md) tracks Android journeys and outstanding acceptance. The [public read contract](PUBLIC_READS.md) defines source attribution, city scope, missing data, and pin admission. Public forum, contributor and station-comment views read the backend. The app can hold a photo capture as a private draft. Apple sign-in, signed contribution submission, regional offline maps, and real company operations require further integration. The personal ledger and manual driver shifts are local. The route preview requests online directions from MapKit.

## Verification

Run the structural checks on Linux or macOS.

```sh
bash scripts/check-mobile.sh --static-only
python3 -m unittest -v scripts.tests.test_ios_app_integrity
```

Run the native build and XCTest on macOS with Xcode 26.4.1 and the iOS 26.4 simulator runtime. The GitHub iOS workflow uses the same Xcode command and bundles the shared Android municipality catalog and EE-23 fixture from their repository paths.

```sh
cd iosApp
xcodebuild test -project AnpFuel.xcodeproj -scheme AnpFuel \
  -destination 'platform=iOS Simulator,name=iPhone 17,OS=26.4' \
  CODE_SIGNING_ALLOWED=NO
```

Acceptance requires the matching Android journeys, staging API checks, and a physical iPhone and iPad run for Keychain, permissions, camera, VoiceOver, Dynamic Type and location. The simulator suite covers native compilation and XCTest. It cannot certify signing, App Store distribution, or device behavior.
