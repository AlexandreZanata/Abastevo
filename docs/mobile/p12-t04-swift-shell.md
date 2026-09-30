# P12-T04 — Swift framework and native shell

Status: IMPLEMENTED, NOT VERIFIED on `codex/phase-12-kmp-foundation`
(phase PR #21, draft). Issue: #19. Dependency P12-T03 LOCAL_DONE satisfied.
Per owner instruction: Swift sources plus XCTest vectors ship now with no
local verification; the real build/test runs on macOS when hardware is
available. No behavior change to Android or backend; no KMP Gradle plugin
added (wiring lands after the Mac run proves this shell, per ADR-015).

## B-BR / BUC

Same frozen contracts as P12-T02/P12-T03 (B-BR-002 money, F03 text, A04
identity shape). This task adds the third platform projection; owning
screens, accounts and persistence integration stay in P10/P13/P17.

## What changed (`iosApp/`, new SPM package `AnpFuel`)

- `Sources/AnpFuelCore/PortableMoney.swift` — integer milli-BRL port of the
  Kotlin/Go kernel: same grammar, same six quarantine codes, same range,
  canonical format, capacity bounds and overflow-checked half-up multiply
  via `multipliedReportingOverflow`. Zero force-unwraps; optionals resolve
  to typed codes.
- `Sources/AnpFuelCore/PortableText.swift` — F03 rules via
  `unicodeScalars.count` (never `String.count`, which counts grapheme
  clusters and would break parity).
- `Sources/AnpFuelCore/PortableIdTime.swift` — lowercase UUID and strict UTC
  instants with leap-year calendar math, no `UUID`/`ISO8601DateFormatter`.
- `Sources/AnpFuelCore/TankFillUseCase.swift` — cheapest-station selection
  (first-wins ties) plus exact total: the use case the shell executes.
- `Sources/AnpFuelShell/AnpFuelApp.swift` — thin SwiftUI host running the
  use case against the frozen golden vectors (Cheap / 5499 / 50 L).
- `Tests/AnpFuelCoreTests/` — 4 XCTest suites (money 9, text 7, id/time 5,
  use case 3 = 24 tests) transcribed 1:1 from
  `contracts/testdata/compat/money-portable-v1.json` and the Kotlin suites.
- `iosApp/README.md` — macOS runbook (`swift build`, `swift test`,
  `xcodebuild` simulator destination) and porting notes flagged for
  compiler review on Mac.

## Verification state (explicit, no green claims)

- `which swift swiftc xcodebuild` on this Linux host: all absent.
  NOTHING in `iosApp/` has been compiled, linted or run — not even a syntax
  check. XCTest counts above are file contents, not results.
- BLOCKED (environment, not code): `swift build`, `swift test`,
  `iosArm64`/`iosSimulatorArm64` compile and the shell fixture run require
  macOS + Xcode 26.4. Tracked as the P12-T04 macOS follow-up; it blocks
  P12-T05/G12, never bypassed with mocks.
- Android/backend evidence in this commit (unchanged areas): full baseline
  below, proving the iOS addition disturbed nothing.

## Validation on Linux (exact commands, this host)

```sh
./gradlew :domain:test :application:test --no-daemon
./gradlew :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
make quick-verify
git diff --check
```

Outcome: all Android suites pass with pre-T04 counts intact (full counts in
commit evidence), `assembleDebug` SUCCESS, `quick-verify` ok
(`iosApp/*` selection rule from T02 covers the new area), `git diff --check`
clean, secrets scan clean.

## Limits (not claimed)

- Swift correctness is author-reviewed only; syntax/type errors are
  possible and must surface on the Mac run, fixed there as explicit
  follow-up commits (never history rewrite).
- No KMP `shared` module, no Kotlin→Swift framework export (Alpha
  export deliberately avoided per ADR-015); interop wiring follows the Mac
  run.
- No App Store, TestFlight, signing or device provisioning touched.

## Next

P12-T05 foundation acceptance (#20): Android regression gate, dependency
drift review and the G12 evidence bundle — still gated on the P12-T04 macOS
run above. Issue #19 stays open until the P12 phase PR merges AND the Mac
run is recorded.
