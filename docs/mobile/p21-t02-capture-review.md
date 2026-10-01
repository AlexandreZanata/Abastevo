# P21-T02 — Lightweight camera and editable price review

Status: LOCAL_DONE on `codex/phase-21-photo-contribution`. Issue: #82. Binds B-BR-C01/C06 and BUC-C02/C03 to existing contracts. No backend change (sanitizer verified existing); no new permission.

## What this task adds

- Editable review on `CaptureScreen`: selectable OCR candidates (text order, never auto-minimum) with exact formatted prices and low-confidence legibility notes, manual price field for empty/low-confidence sets, explicit fuel chips and condition chips (STANDARD/CASH/DEBIT/CREDIT/APP/LOYALTY/OTHER wire codes), one explicit confirm tap enabled only with price + product + condition. The previous auto-first-candidate + hardcoded fuel path is gone.
- `ConfirmPriceCaptureUseCase`: `confirm` requires a contributor-picked non-blank condition (`Confirmed` carries it); new `confirmManual` parses human-typed exact milli-BRL (`PortableMoney`, never rounded), invalid input stays in choice preserving the caller's candidates, typed entries skip the OCR confidence gate via `OcrCandidate.manualEntry`.
- `CaptureOcrViewModel`: condition-aware confirm + manual path that keeps the candidate list on parse failure; `Confirmed` carries condition.
- Strings en + pt-BR (other locales fall back to en).

## Reused and verified (not rebuilt)

- `PortablePriceOcr` grammar/confidence (P10-T04), `CaptureOcrFlagProvider`/permission/cancel gates, `PhotoFlow` compression budgets, `CommunityPriceDisplay.formatMilliBrl`, `FuelProductI18n`.
- Backend sanitizer verified existing: `backend/internal/modules/evidence/adapters/media/forward.go` `SanitizeForward` (decode-only re-encode, EXIF/thumbnail/trailing payloads cannot survive, 256 KiB hard refuse) with frozen P15-T01 budgets (`budgets.go`: 150 KiB target, 256 KiB cap, 1600 edge, 2 MP, 3 attempts, 32 MiB hypothesis, 24 h deadline) and tests. No backend change in this task.

## Explicitly deferred (not waived)

- Low-end-device encode time/peak heap/file-size measurement: no supported device or emulator on this workstation. P15 budgets stay the frozen reference; real measurement belongs to P24-T03 device profiling. This keeps the task's device proof incomplete by the plan's own rule.
- CameraX preview stays out (system camera intent, per existing slice); condition display codes stay wire codes until translators cover them.

## Validation

- `PortablePriceOcrTest` 8/0-fail, `ConfirmPriceCaptureUseCaseTest` 8/0-fail, `CaptureOcrViewModelTest` 4/0-fail (RED→GREEN across domain/application/app).
- `./gradlew :domain:test :application:test :app:testDebugUnitTest :app:assembleDebug` (affected suites).
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).
