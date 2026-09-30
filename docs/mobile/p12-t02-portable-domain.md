# P12-T02 — Portable domain and money contracts

Status: LOCAL_DONE on `codex/phase-12-kmp-foundation` (phase PR #21, draft).
Issue: #17. Dependency P12-T01 LOCAL_DONE satisfied. No behavior change to
existing screens; no KMP Gradle plugin added yet (deferred to P12-T03 by
design, per the frozen T01 rollback baseline).

## B-BR / BUC

Frozen contract, owned downstream: B-BR-002 exact money (Go kernel parity),
F03 280-scalar comments (P14), A04/B-BR-A04 identity shape (P13), M/L time and
risk inputs (P15/P16). This task introduces the portable kernel and golden
vectors; endpoint/schema/DB work stays in owning phases.

## What changed

Portable-ready pure Kotlin in `:domain` (package `com.anpfuel.domain.portable`,
zero `java.*` imports, guard-tested so the files move unchanged to
`commonMain` later). `com.anpfuel` packages and MIT notices preserved.

- `PortableMoney.kt` — integer milli-BRL, ANP comma grammar, range
  1..1000000, over-precision refusal (never rounding), canonical
  `"<int>,<3dp>"` format, capacity in milli-liters (1..200000) and
  overflow-checked half-up tank-fill multiply. Mirrors
  `backend/internal/modules/kernel/price.go` case-for-case, including the
  padded/negative-zero/zero branches.
- `PortableText.kt` — trim + CRLF normalization, surrogate-aware Unicode
  scalar counting (astral emoji = 1, combining sequence = base + mark, same
  as Go rune count), 280-scalar validity (F03).
- `PortableIdTime.kt` — lowercase canonical UUID and strict UTC
  `YYYY-MM-DDTHH:MM:SSZ` with leap-year calendar checks. No
  `java.util.UUID`/`java.time` in portable code.
- `contracts/testdata/compat/money-portable-v1.json` — canonical golden
  vectors (money valid/invalid + quarantine codes, tank-fill totals, text,
  UUID and instant cases) with provenance at base `6f4f048`.
- Tests: `PortableMoneyTest` (11), `PortableTextTest` (7),
  `PortableIdTimeTest` (5), `PortableParityTest` (7) — 30 new tests.

## Frozen drift table (legacy Android vs portable)

| Case | Legacy `:domain` | Portable (== Go kernel) |
|---|---|---|
| `5,499` / `5.499` | rounds to 5.50 | exact 5499 milli |
| `0,00` / `0.00` | accepted | `zero-price` refusal |
| `5,9999` | rounds | `over-precision` refusal |
| Tank 5.499 x 50 L | 275.00 | 274950 milli (274,950) |
| Exact 2dp e.g. 5.49 | 5.49 == 5490 milli | agreement (tested) |
| Comment length | UTF-16 units | Unicode scalars (tested) |

Existing calculators are untouched; they migrate to portable math in P12-T03.
The parity test asserts both the agreements and the divergences above, so no
drift can enter silently.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test --rerun-tasks --no-daemon
./gradlew :data:testDebugUnitTest :app:testDebugUnitTest --no-daemon
./gradlew :app:assembleDebug --no-daemon
cd backend && go test ./internal/modules/kernel/...
make quick-verify
git diff --check
```

Outcome:

- `:domain` + `:application`: all suites pass including 30 new portable
  tests, 0 failures (full counts recorded in commit evidence).
- `:data` + `:app` unit tests and `assembleDebug`: SUCCESS (no Android
  source touched; regression only).
- Backend `kernel` package: PASS (no Go source touched; parity reference).
- `make quick-verify`: ok; `git diff --check` clean; secrets scan clean.
- Delivery fix in this commit: `scripts/quick-verify.sh` selection now
  classifies P12 mobile areas (`domain/`, `application/`, `data/`, `app/`,
  `gradle/`, `shared/`, `iosApp/`, root Gradle manifests) into the full set
  instead of refusing them as unclassified; `test-gate` harness 10/10.
  Android behavioral evidence stays at task level + phase exit until P12-T05
  adds bounded KMP/iOS check selection.

## Limits (not claimed)

- No `commonMain` source set, no KMP Gradle plugin, no Swift framework yet;
  that assembly is P12-T03/P12-T04 on top of this contract.
- iOS/Swift parity against the JSON vectors is asserted by fixture presence,
  not by a Swift run; macOS evidence belongs to P12-T04/T05.
- Instrumentation (`connectedDebugAndroidTest`) and performance budgets stay
  deferred to P12-T05/P18.

## Next

P12-T03 application ports and offline state (#18): pure common use-case
state on top of this kernel, Room kept as Android adapter.
Issue #17 stays open until the P12 phase PR merges.
