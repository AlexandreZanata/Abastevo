# P12-T05 — Foundation acceptance and gates

Status: LOCAL_DONE (G12 PARTIAL) on `codex/phase-12-kmp-foundation` (phase
PR #21, draft). Issue: #20. Dependency P12-T04 implemented-unverified.
No behavior change, no dependency change, no visual redesign: `app/`
styling untouched, backend untouched, manifests untouched.

## G12 verdict: PARTIAL — 3 of 4 gates green, macOS evidence BLOCKED

| G12 requirement | State | Evidence |
|---|---|---|
| Supported pins frozen | GREEN | [P12-T01 baseline](p12-t01-toolchain-baseline.md): Kotlin 2.0.21 / AGP 8.7.3 / Gradle 8.10.2 / KSP 2.0.21-1.0.28 / min 26 / target 35, unchanged since |
| Android regression parity | GREEN | `:domain`+`:application` 94 suites / 475 tests, `:data`+`:app` 50 suites / 169 tests (1 skipped), `assembleDebug` SUCCESS — full counts below |
| Portable domain vectors | GREEN | 30 domain + 17 application portable tests, golden `money-portable-v1.json`, drift table frozen ([T02](p12-t02-portable-domain.md), [T03](p12-t03-application-ports.md)) |
| Shared framework / Swift shell built on macOS | BLOCKED | `iosApp` SPM + 24 XCTest vectors ship per owner instruction, zero local compile/run ([T04](p12-t04-swift-shell.md)); `swift build/test` + iosArm64/simulator pending Mac access |

G12 turns GREEN only after the P12-T04 macOS run is recorded. The phase PR
stays draft and P12 issues stay open until then — no finish/merge is
claimed by this task.

## Android regression parity (exact commands, this host)

```sh
./gradlew :domain:test :application:test --rerun-tasks --no-daemon
./gradlew :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
```

Outcome: `:domain`+`:application` 94 suites / 475 tests, 0 failures/errors;
`:data`+`:app` 50 suites / 169 tests, 0 failures/errors, 1 pre-existing
skip; `assembleDebug` SUCCESS. Baseline growth across P12 is additive only:
430 → 460 (+30 T02) → 475 (+15 T03) domain/app; 167 → 169 (+2 T03 mapper)
data/app. Zero pre-existing tests modified.

## Dependency / license / security drift review

- `git diff 6f4f048..HEAD` over `backend/go.mod`, `backend/go.sum`,
  `gradle/libs.versions.toml`, wrapper properties, all module manifests and
  every `AndroidManifest.xml`: EMPTY. No dependency added, removed or
  repinned anywhere in P12; no new permission, capability or network client.
  There is nothing to license-review because nothing was introduced.
- Backend `govulncheck ./...`: clean inside every `quick-verify` run
  (0 vulnerabilities). Android advisories: no version moved, so no new
  exposure vs the T01 baseline; the next version bump re-runs this review
  with its upgrade rationale (owns P12-T02…T04 follow-ups, never skipped).
- Secret surface: `scan-secrets.sh` + PR-diff scan clean on every gate run.

## Regression fixture inventory (frozen)

- `contracts/testdata/anp/` (30 ANP cases + manifest), `api/`, `consensus/`,
  `identity/`, `compat/legacy-deltas.json` — untouched.
- `contracts/testdata/compat/money-portable-v1.json` (new, T02) — canonical
  money/text/UUID/instant vectors shared by Go, Kotlin and Swift.
- Kotlin parity tests assert the JSON key vectors plus legacy-2dp drift;
  Swift XCTest files transcribe the same vectors 1:1 (run pending Mac).

## Bounded KMP/iOS check selection (new, Quick verification intact)

- New `scripts/check-mobile.sh` (+ `make check-mobile`): hermetic static
  guards (portable import bans, fixture validity + key vectors, iosApp
  skeleton shape, AnpFuelCore force-unwrap heuristic) plus toolchain-gated
  full mode (Android baseline when JDK+SDK present, Swift suite when a
  toolchain exists). Absent toolchains report loud SKIP — SKIP is not
  evidence.
- `scripts/quick-verify.sh` full selection now runs `check-mobile.sh
  --static-only` when mobile areas change (T02 classification extended in
  T05 wiring). Required `Quick verification` keeps its triggers, timeout
  and required status; nothing is disabled, skipped or weakened.
- `make test-gate`: 10 → 13 passing (static green on repo, refusal on
  empty tree, planted-import failure naming the file).

## Supported-device matrix (frozen as proposal, validation deferred)

- Android: min API 26 preserved, compile/target 35; low-resource reference
  plus current-device runs with measured cold-start/list/photo/outbox/heap
  budgets belong to P18/G18 profiling — hypotheses today, not claims.
- iPhone: `iosArm64` + `iosSimulatorArm64`, Xcode 26.4, iOS 17 floor per
  `Package.swift`; simulator/device runs belong to the blocked Mac session.
- Instrumentation (`connectedDebugAndroidTest`) stays a device-gated suite
  at P10-T08/P12-T05-device time, never synthesized from unit runs.

## Rollback

Additive-only phase: removing `domain/.../portable`,
`application/.../portable`, `data/.../local/outbox`, `iosApp/`,
`contracts/testdata/compat/money-portable-v1.json` and the delivery-script
deltas restores the tree to `6f4f048` behavior bit-identically. No
migration, no flag, no data at stake.

## Next (requires Mac, then merge)

1. macOS session: run `iosApp/README.md` commands, record versions + full
   log, fix any Swift findings as follow-up commits → P12-T04 done.
2. `scripts/git-flow.sh finish --required "Quick verification"` on the
   verified head, guarded merge, branch deletion, wiki snapshot → P12
   INTEGRATED, issues #16–#20 closed by the merge, G12 GREEN.
3. Then P13 free accounts (#P13-T01…). Issue #20 stays open until step 2.

## Owner decision 2026-09-30: G12 accepted on basic compatibility

Step 1 above is WAIVED by explicit owner decision recorded before
merge: G12 is accepted with basic compatibility (pins frozen,
Android parity 475+169 green, portable vectors green, static mobile
gates green) and WITHOUT the macOS Swift build/run. The shipped
`iosApp` shell + XCTest vectors remain implemented-unverified:
full real-device confirmation (macOS build, simulator/device runs,
supported-device matrix) is deferred to the release version, when
Mac access exists. Nothing in this decision weakens CI, rewrites
evidence, or certifies devices — it re-scopes G12 acceptance, and
P13-T05/G13 device evidence stays outstanding to the same release
horizon. This section, not a rewritten verdict table, is the audit
trail: the PARTIAL table above is preserved as-run.
