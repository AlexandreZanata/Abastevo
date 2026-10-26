# DRV-16 — Mobile driver hub and Premium states

Status: LOCAL_DONE
Validation: PASS (local JVM + owned API35 emulator; no device-matrix claim)
Integration: INTEGRATION_PENDING
Parent: efdcce26. Behavior: d1db75ab. Contract: [DRV-16](../../../product/DRV_16_CONTRACT.md).

T01: `Routes.DRIVER_HUB = "driver-hub"` is reachable from a Perfil entry and a
Veículos action, and is mapped to the Perfil bottom tab. The four-tab shell and
existing map child/deep-link tests remain green. The hub routes only to real
vehicle, ledger-entry and monthly-indicator/export screens.

T02: `DriverHubViewModel` combines the signed session, server-derived Premium
status and local vehicle list into VERIFIED/OFFLINE/LAPSED/UNAVAILABLE/
SIGNED_OUT. Offline/unavailable show retry; lapse keeps local vehicle/ledger/
monthly actions and opens the existing Premium screen; signed-out offers the
existing sign-in. No local grant, checkout, price, trial or reputation change is
fabricated. Monthly export is the existing real export path.

T03: first use (no vehicle) opens the real vehicle form/list; with a vehicle the
hub opens real ledger entries and monthly indicators/export. Loading uses the
shared skeleton/loading state; failure and offline states are actionable.
Shift control is explicitly deferred to DRV-18 and is not rendered as a fake
button.

## Actual validation

- JVM: `DriverHubViewModelTest`, `RoutesTest` and `MapNavigationTest` PASS.
- `:app:testDebugUnitTest` and `:app:compileDebugKotlin` PASS.
- Owned API35/16KB emulator: **10/10 instrumentation PASS** across
  `DriverHubScreenTest` (first use, verified actions, lapse/offline/signed-out)
  and `BottomTabNavigationDeviceTest` (four tabs/back behavior).
- `scripts/check-mobile.sh --static-only` PASS.

## Deliberate limits

No shift form/control, GPS, driver command UI or field/device-matrix acceptance;
DRV-18 owns shift control. Strings are added to the default locale; full
eight-locale/accessibility proof remains DRV-29. No push/PR/merge in this
checkpoint.
