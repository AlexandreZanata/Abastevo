# iosApp — thin iPhone host (P12-T04)

Status: IMPLEMENTED, NOT VERIFIED. No Swift/Xcode toolchain exists on the
Linux workstation, so nothing here has been compiled or run. The first real
build/test happens on macOS with Xcode 26.4 (see the BLOCKED checklist
below). Per the owner's instruction, this phase ships Swift sources plus
XCTest vectors now; the Mac run follows when hardware is available.

## Layout

- `Package.swift` — SPM package `AnpFuel` (iOS 17, macOS 14).
- `Sources/AnpFuelCore/` — portable kernel, Swift ports of P12-T02 with
  identical semantics and quarantine codes: `PortableMoney` (integer
  milli-BRL 1...1000000, ANP comma grammar, over-precision refusal,
  canonical `"<int>,<3dp>"` format, overflow-checked tank-fill multiply),
  `PortableText` (trim + CRLF normalization, `unicodeScalars` counting,
  280-scalar F03 rule), `PortableIdTime` (lowercase UUID, strict UTC
  instants with leap-year checks), `TankFillUseCase` (cheapest station plus
  exact total — the use case the shell executes).
- `Sources/AnpFuelShell/AnpFuelApp.swift` — thin SwiftUI host running the
  use case against the frozen golden vectors. Live reads, accounts and
  persistence arrive in P10/P13/P17.
- `Tests/AnpFuelCoreTests/` — XCTest vectors transcribed 1:1 from
  `contracts/testdata/compat/money-portable-v1.json` and the Kotlin
  `PortableMoneyTest`/`PortableTextTest`/`PortableIdTimeTest` suites
  (money valid/invalid + boundaries, format, tank-fill totals incl. half-up,
  capacity bounds, 280/281 comments, emoji/combining scalars, UUID and
  instant vectors, cheapest/tie/empty use-case behavior).

## Run on macOS (BLOCKED until Mac access)

```sh
cd iosApp
swift build                                  # core + shell compile
swift test                                   # 4 XCTest suites, must be green
xcodebuild test -scheme AnpFuel \
  -destination 'platform=iOS Simulator,name=iPhone 16'   # simulator run
```

Expected: `AnpFuelCore` builds for `iosArm64`/`iosSimulatorArm64`, all
XCTest suites pass, and the shell renders the cheapest tank fill
(Cheap / 5499 / R$ 274,950 for 50 L). Record device/Xcode versions and the
full log in `docs/mobile/p12-t05` acceptance; any failure stays open as a
P12-T04 follow-up, never a silent skip.

## Porting notes (reviewed without a compiler — verify on Mac)

- `String.unicodeScalars.count` is the scalar count; `String.count` would
  count grapheme clusters and break F03 parity — never substitute it.
- `trimmingCharacters(in: .whitespaces)` covers ANP fixture spacing; full
  Unicode trim parity with Kotlin `trim()` is fixture-equivalent, not
  character-exact — do not feed exotic whitespace as identity proof.
- No force-unwraps (`!`) in `AnpFuelCore`; every optional resolves to a
  typed `PortableMoneyError` code or a `false` validator.
- Swift export of the Kotlin framework (Alpha) is intentionally NOT used:
  the first app ships framework interop plus this Swift wrapper per ADR-015.
  KMP `shared` module wiring lands after the Mac run proves this shell.
