# Abastevo for iPhone and iPad

The native application targets iOS 17 and later. Open `AnpFuel.xcodeproj` and select the shared `AnpFuel` scheme. SwiftUI owns presentation, and `AnpFuelCore` contains the portable business rules, URLSession client, and iOS adapters. The project preserves `com.anpfuel.app`.

The [parity contract](PARITY.md) tracks Android journeys and outstanding acceptance. The [public read contract](PUBLIC_READS.md) defines source attribution, city scope, missing data, offline cache and pin admission. Public forum, contributor and station-comment views read the backend. Official ANP survey weeks download from gov.br into verified local storage and remain usable offline. EV charging reads the admitted catalog. The route preview requests online directions from MapKit, and offline navigation hands routable stations to Apple Maps offline maps.

Accounts use Sign in with Apple, e-mail codes or recovery keys. Account calls run off the main actor. After any login, the device P-256 key registers a contributor and binds it to the account. Price observations, an optional evidence photo, station comments and votes, and forum topics and comments go through a persistent outbox with server receipts. Owner history reads the signed `GET /v1/contributors/me/observations`. Sign in with Apple depends on `POST /v1/accounts/providers/login`; a server without it returns `account.provider-unavailable`, and the app reports that state. Premium is sold through StoreKit 2 and verified by the server ([store billing](../docs/product/payments/store-billing.md)).

## Verification

Run the structural checks on Linux or macOS.

```sh
bash scripts/check-mobile.sh --static-only
python3 -m unittest -v scripts.tests.test_ios_app_integrity
```

Run the native build and XCTest on macOS with Xcode 26.4.1 and the iOS 26.4.1 simulator runtime. The GitHub iOS workflow uses the same Xcode command and bundles the shared Android municipality catalog and EE-23 fixture from their repository paths.

```sh
cd iosApp
xcodebuild test -project AnpFuel.xcodeproj -scheme AnpFuel \
  -destination 'platform=iOS Simulator,name=iPhone 17,OS=26.4.1' \
  CODE_SIGNING_ALLOWED=NO
```

Acceptance requires the matching Android journeys, staging API checks, and a physical iPhone and iPad run for Keychain, permissions, camera, VoiceOver, Dynamic Type and location. The simulator suite covers native compilation and XCTest, including the Go identity vectors from `contracts/testdata/identity` verified with CryptoKit. It cannot certify code signing, Sign in with Apple, App Store distribution, private media uploads, or device behavior.
