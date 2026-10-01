# P10-T04 — Capture and local OCR

Status: LOCAL_DONE on `codex/phase-10-functional-integration` (phase PR
#54 pending, fourth task push). Issue: #48 (slice 4 of 8 local;
P10-T09 pilot stays deferred until G18/G09 and is not part of G10-LOCAL).
No ANP path touched, no legacy enum rename, no backend change.

## Slice acceptance (frozen before coding)

- T04 (this commit) — Capture/crop/compress + confirm extracted price:
  portable `PortablePriceOcr` (exact milli-BRL core via `PortableMoney`,
  tolerant `R$/RS/BRL/$` marker + `./,` 2..3 decimals, 0.90 marked /
  0.50 bare confidence, 0.60 manual bar, max 10 in text order, no `min()`,
  no product/condition fields) + `CaptureOcrConfig` default OFF;
  application `ConfirmPriceCaptureUseCase` (Disabled / PermissionDenied /
  Cancelled / NeedsHumanChoice, `confirm()` only with explicit approval +
  picked `FuelProduct`, no upload); data `CaptureOcrFlagStore` default OFF
  + `LocalRegexPriceOcr` (`OcrPort`) + `CaptureModule` with dependency
  rationale (system camera intent + `PhotoFlow` budgets, no CameraX bundled;
  Play-services ML Kit `text-recognition` plugs behind the same port at
  P10-T08 device pass); app `CameraPermissionHandler` +
  `CaptureOcrViewModel`/`CaptureScreen` + `Routes.CAPTURE`, `CAMERA`
  permission (`required=false`), ANP screens unchanged.

## What changed

- `domain/.../feature/CaptureOcrConfig.kt` — DISABLED default.
- `domain/.../portable/PortablePriceOcr.kt` — pure parse/order/confidence
  rules (no `java.*`).
- `application/.../port/CaptureOcrFlagProvider.kt` + `port/OcrPort.kt`.
- `application/.../usecase/capture/ConfirmPriceCaptureUseCase.kt` +
  outcomes (no HTTP dependency).
- `data/.../preferences/CaptureOcrFlagStore.kt` — default OFF.
- `data/.../local/ocr/LocalRegexPriceOcr.kt` — JVM-testable adapter.
- `data/.../di/CaptureModule.kt` — rationale, no base URL.
- `RepositoryModule`/`UseCaseModule` bindings.
- `app/.../capture/CameraPermissionHandler.kt` + `CaptureOcrViewModel.kt`
  + `CaptureScreen.kt`; `Routes.CAPTURE`; nav entry; manifest CAMERA.
- Tests: 6 portable (single/dot/multi-order/bare-low/skip/bound) + 6
  use-case (disabled-no-OCR/denied-no-OCR/cancel-no-partial/multi-no-pick/
  low-confirm-gate/condition-never-auto) + 3 data adapter + 2 permission +
  3 viewmodel + 1 androidTest device replay (CI).

## RED → GREEN

- RED proven by new symbols before implementation (same pattern as
  P10-T01/T02/T03): `PortablePriceOcr`, `CaptureOcrFlagProvider`,
  `OcrPort`, `ConfirmPriceCaptureUseCase`, `LocalRegexPriceOcr`,
  `CameraPermissionHandler` did not exist; new tests referenced them.
  GREEN after adding bounded adapters/screens only.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test --no-daemon --rerun-tasks
./gradlew :application:test --no-daemon --rerun-tasks
./gradlew :data:testDebugUnitTest --no-daemon --rerun-tasks
./gradlew :app:testDebugUnitTest :app:assembleDebug --no-daemon --rerun-tasks
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: `:domain:test` + `:application:test` + `:data:testDebugUnitTest`
green (incl. 15 new unit suites listed above); `:app:testDebugUnitTest`
+ `assembleDebug` BUILD SUCCESSFUL (ANP screens unchanged);
`check-mobile --static-only` ok; `diff --check`/secrets clean.
Limits: `:data:connectedDebugAndroidTest` + `:app:connectedDebugAndroidTest`
NOT run locally (no emulator); `CaptureOcrDeviceTest` added for CI/phase
exit. No backend change; no production URL invented. B-BR-010/011/015/016:
no public evidence, no unvalidated photo counted, no upload from capture.
