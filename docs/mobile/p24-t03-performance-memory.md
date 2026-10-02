# P24-T03 — Performance and memory (budgets frozen, code-only slice)

Status: IN_PROGRESS on `codex/phase-24-android-acceptance`. Issue: #98 OPEN (not done).
Entry: P24-T02 code slice landed (`05b3fec`); novice/device proofs deferred by user
directive below. Binds B-BR-C01–C06 / BUC-C01–C05. No migration, no backend change,
no iOS work, no production code changed in this slice.

## User directive (2026-10-02, recorded in planning)

- No emulator or physical-device runs during the remaining phases. Validation on
  device/emulator happens manually only after ALL phases finish.
- Until then, run only code (JVM/unit) tests. This slice follows that rule: no
  `connected*`, no instrumented runs, no new device measurement claimed.

## Frozen budgets (predeclared, from integrated evidence)

| Area | Budget (frozen) | Source |
|---|---|---|
| Photo wire target / hard cap | 150 KiB / 256 KiB | `PortablePhoto` + backend `budgets.go` (P15-T01, P21-T02) |
| Decoded frame | ≤ 1600 px edge, ≤ 2 MP | `PortablePhoto.sampleSizeForBounds` (P15-T02) |
| Encode attempts / working-mem hypothesis | 3 / 32 MiB | `PortablePhoto` (device proof of hypothesis deferred, see below) |
| Transient cache TTL | 24 h | `PortablePhoto.TRANSIENT_TTL_MILLIS` |
| Discovery page size | 1..100, default 20 | `DiscoveryQuery` (P20-T01) |
| Cached-home regression reference | 382 ms | P18 local exit (regression only, NOT a cold-start result) |
| Genuine cold-process start | TBD by manual validation | No value invented here |

## Code-level guards (already in tree, re-run as evidence, no device needed)

- `PortablePhotoTest`: budgets mirror forward contract, sample-size keeps 12 MP /
  48 MP / panoramic frames inside 1600 px + 2 MP, wire-cap bounds, 24 h expiry.
- `DiscoveryQueryTest` + `DiscoveryStationsRuleTest`: pageSize 1..100 enforced, so
  list/scroll queries stay bounded by construction.
- `SourceTimeBadgeTest` + `StationPriceRowA11yTest` (P24-T02): presentation-only,
  no domain work in composables.

## Explicitly deferred (not waived, not green)

- Genuine cold-process start, list scrolling, peak heap / encode / OCR timing,
  background replay and battery on the frozen matrix: NO measurement in this slice.
  Real low-end hardware row stays open. Claiming acceptance now would be false.
- These items join the post-all-phases manual validation batch owned by the user.

## Validation (code tests only, per directive)

- `./gradlew :domain:test :application:test :data:testDebugUnitTest
  :app:testDebugUnitTest :app:assembleDebug --no-daemon` (record counts in commit
  evidence). No `connectedAndroidTest`, no emulator boot.
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).
