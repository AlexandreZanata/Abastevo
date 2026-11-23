# Abastevo for iPhone and iPad

The native application targets iOS 17 and later. Open `AnpFuel.xcodeproj` and select the shared `AnpFuel` scheme. SwiftUI owns presentation, and `AnpFuelCore` contains the portable business rules, URLSession client, and iOS adapters. The project preserves `com.anpfuel.app`.

The [parity contract](PARITY.md) tracks Android journeys and outstanding acceptance. The [public read contract](PUBLIC_READS.md) defines source attribution, city scope, missing data, offline cache and pin admission. Public forum, contributor and station-comment views read the backend. Official ANP survey weeks download from gov.br into verified local storage and remain usable offline. EV charging reads the admitted catalog. The vehicle ledger stores exact records on the device, computes monthly indicators from qualified intervals, and syncs through the signed `/v1/ledger/*` routes after explicit opt-in (B-BR-IOS-VEHICLE). Manual driver shifts are local. The route preview requests online directions from MapKit, and offline navigation hands routable stations to Apple Maps offline maps.

Abastevo Empresas covers invitations, company selection with device enrollment, assigned assets, trip start and finish, the departure checklist, and expenses with an optional receipt photo. Each organization has its own protected, device-signed queue, which is purged on logout. Tracking stays off on iOS. [PARITY.md](PARITY.md) (B-BR-IOS-EMP) lists the routes and what still needs staging and device evidence.

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

## Testing on a Mac

1. Install Xcode 26.4.1 or later with the iOS 26.4 simulator runtime, then open `iosApp/AnpFuel.xcodeproj` and pick the `AnpFuel` scheme.
2. Simulator: choose an iPhone 17 or iPad simulator and press Run. The app starts on the `abastevo.com.br` origin; Settings → network environment switches origins only while no account is signed in.
3. Device: in the `AnpFuel` target, Signing & Capabilities, select your team. Sign in with Apple and associated domains need a paid Apple Developer team; with a personal team, remove those two capabilities locally (do not commit the change). Keep the bundle ID `com.anpfuel.app` for App Store Connect products.
4. Tests: `⌘U` in Xcode, or the `xcodebuild test` command above. CI runs the same command on every push to this branch.
5. Premium purchases need the App Store channel configured on the server you use (see [store billing integration](../docs/backend/payments/store-billing-integration.md)) and a Sandbox Apple Account signed in on the device (Settings → App Store → Sandbox Account; recent iOS versions also list it under Settings → Developer). Without a server `app_store` offer the Premium sheet shows status only. An Xcode `.storekit` file is not useful end to end: the server refuses locally signed transactions.
6. Things only a device proves: Keychain persistence across reinstall, camera evidence capture, location prompts, Sign in with Apple, notifications, VoiceOver and Dynamic Type, and Apple Maps offline navigation.
