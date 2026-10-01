# P16-T01 — Location risk contract

Status: LOCAL_DONE on `codex/phase-16-location-integrity` (phase PR
pending, first push with this commit). Issue: #40 (slice 1 of 4;
issue stays open until the phase PR merges). No native adapter,
backend intake or live-provider contact in this slice.

## Slice acceptance (frozen before coding)

- T01 (this commit) — frozen location risk contract: one verdict
  per device signal with stable reason codes, claim gate and
  persisted bands (never coordinates). Precedence: manual entry
  (no device fix, no permission needed) → permission denial →
  missing fix data (no fix/accuracy/age) → OS source info missing
  (a client-claimed `isMock=false` cannot upgrade) → OS-marked
  simulated → future fix/clock skew (|skew| > 300 s) → stale
  (age > 120 s) → coarse (> 100 m, DEGRADED, browse-only) →
  VERIFIED. Only VERIFIED has `allowsClaim`; unknown can never
  become verified proximity (acceptance). Bounds frozen:
  120 s fix age, ±300 s clock skew, 100 m claim accuracy
  (mirrors `MaxAcceptableAccuracyM`). Disclosure: reason codes
  are the vocabulary; UI wording stays P17. No universal
  spoof-proof promise (detects platform-marked simulation only).
  20 golden vectors in `contracts/testdata/location/risk-v1.json`
  replayed by Go; Kotlin + Swift parity transcribed.

## What changed

- `contracts/testdata/location/risk-v1.json` — 20 vectors
  (verified boundaries inclusive; simulated; denied; no-fix/
  accuracy-missing/fix-time-missing; forged client flag;
  stale/replayed; future fix; skew over; coarse; manual with and
  without permission; simulated-beats-coarse;
  denied-beats-stale; source-missing-beats-simulated).
- `backend/internal/modules/community/application/location.go` —
  `LocationV1`, verdict/reason/band constants, frozen bounds,
  `ClassifyFix`, `ClaimForFix` (nil unless VERIFIED), band
  helpers.
- `location_test.go` — fixture replay (bounds + 20 vectors),
  claim-gate matrix, coordinate-leak guard.
- `domain/.../portable/PortableLocation.kt` + test — 1:1 mirror
  (8 suites: bounds, boundaries, simulated, forged flag, denied/
  manual, unknown matrix, coarse, precedence).
- `iosApp/.../PortableLocation.swift` + XCTest — 1:1
  transcription, force-unwrap-free (check-mobile guard);
  implemented-unverified per P12 practice.
- `docs/security/LOCAL_MEDIA_LOCATION_POLICY.md` — status notes
  T01 freeze; native/backend stay NOT IMPLEMENTED (T02…T04).

## RED → GREEN

- RED proven by disabling the OS-source-info gate: the
  `source-missing` claim-gate case and the forged-client-flag
  vector flip to VERIFIED/claim allowed; GREEN on restore.
- `check-mobile` caught real force-unwraps in the Swift
  transcription pre-commit; replaced with optional binding.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet
gofmt -l internal/modules/community
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/community/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/community/...
./gradlew :domain:test :application:test --no-daemon
bash scripts/check-mobile.sh
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: build + `sqlc vet` clean (no schema change); Go
community suites green (3 new incl. 20/20 replays); Kotlin
`PortableLocationTest` 8/8; `check-mobile` full ok (Android
green; Swift SKIP recorded outstanding, never green); `vacuum`
0/0/27 info; `diff --check` clean; secrets scan clean.

## Limits (not claimed)

- No native adapter (T02), no backend intake distrust (T03), no
  device acceptance (T04); no release claim from this contract.
- Swift compiles/runs on macOS only (release horizon).
- Server-side replay/idempotency for reuse attacks lands with
  T03; here replay surfaces as stale/clock-anomaly vectors.

## Rollback

Pure addition (fixture, one Go file + test, two Kotlin files,
two Swift files, doc status line): deleting them restores the
tree. No migration, no flag, no behavior change to existing
intake (DeriveBands untouched).

## Next

P16-T02 native mock/simulation adapters (#41, same branch).
Issue #40 stays open until the phase PR merges.
