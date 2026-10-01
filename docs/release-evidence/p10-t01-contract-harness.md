# P10-T01 — Kotlin contract harness and compatibility adapters

Status: LOCAL_DONE on `codex/phase-10-functional-integration` (phase PR
pending, first push with this commit). Issue: #45 (slice 1 of 8 local;
P10-T09 pilot stays deferred until G18/G09 and is not part of G10-LOCAL).
No package moves, no enum renames, no Room migration, no wire change.

## Slice acceptance (frozen before coding)

- T01 (this commit) — consume shared fixtures in the existing
  architecture with bounded adapters only: `WireFuelMapper` owns the
  A05 bridge (legacy `GASOLINE_PREMIUM` ↔ wire `GASOLINE_ADDITIVED`;
  the legacy name never appears on the wire; unknown enums refused
  without coercion), `BackendCnpjMapper` preserves alphanumeric CNPJ
  and leading zeroes with the backend check-digit rule (letters never
  coerced to digits), and `P10ContractHarnessTest` replays
  `legacy-deltas.json` / `money-portable-v1.json` / ANP manifest
  compatibility classes. Legacy `FuelProduct` (keeps
  `GASOLINE_PREMIUM`), `Cnpj` (numeric-only, refuses alphanumeric)
  and `PriceAmount` (2-decimal) stay untouched.

## What changed

- `data/.../mapper/WireFuelMapper.kt` — 7-value wire map, `toWire` /
  `fromWire` (only wire values accepted), `wireValues` for round-trip.
- `data/.../mapper/BackendCnpjMapper.kt` — format-strip + uppercase,
  14-char ASCII validation, Receita check digits (A=17…Z=42, mirrors
  `backend/internal/modules/kernel/cnpj.go`).
- `data/.../mapper/WireFuelMapperTest.kt` — 6 suites (premium bridge,
  round-trip, legacy-name refusal, unknown/case refusal).
- `data/.../mapper/BackendCnpjMapperTest.kt` — 6 suites (leading
  zeroes, alphanumeric, lowercase, no-coercion, checksum/malformed
  quarantine incl. `12ABC345/01DE-30`).
- `domain/.../contract/P10ContractHarnessTest.kt` — 7 suites replaying
  the frozen fixtures and documenting the three legacy divergences.

## RED → GREEN

- RED proven by the new mapper tests before implementation:
  `:data:compileDebugUnitTestKotlin` failed on the missing
  `WireFuelMapper`/`BackendCnpjMapper`; GREEN after adding them.
- Behavioral RED: the harness first assumed legacy `Cnpj` strips
  letters; `:domain:test` failed (`DomainException`) because legacy
  `Cnpj` refuses alphanumeric input outright — the test now asserts
  that refusal explicitly, with the backend adapter owning alpha.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :data:testDebugUnitTest --no-daemon
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: `:domain:test` + `:data:testDebugUnitTest` green (incl. 7 new
harness + 12 new mapper suites); full Android baseline BUILD
SUCCESSFUL; `check-mobile` full ok (Android green; Swift SKIP
recorded, never green); `diff --check`/secrets clean. No backend
change; Go kernel vectors (`04218406000104`, `12ABC34501DE35`)
replayed on the Kotlin side only.
