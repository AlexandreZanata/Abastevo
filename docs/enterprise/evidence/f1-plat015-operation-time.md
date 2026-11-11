# F1 EMP-PLAT-015 operation-time kernel

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Scoped source acceptance, INTEGRATION_PENDING through
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1 remain open. Aggregate domain coverage and native/activation
obligations remain owed; this record does not certify release readiness.

## Entry and contract

The owner requested the next eligible enterprise task and GitHub publication.
The isolated enterprise worktree was clean on codex/enterprise-f0-f1 at
a709ad5c, matching PR202's open dev head. Main ef96cd13 remains an ancestor.
Occupied local dev and unrelated primary-checkout edits are preserved.
EMP-PLAT-012 still awaits the external-channel decision EMP-GOV-014.

Read the selected task, current progress, BR-TIME-001/002/003/004/006 and
VAL-23. B-BR-EMP-31/32 and BUC-EMP-17 were documented before behavior. The
93-row synthetic [oracle](../../../contracts/testdata/enterprise/time-v1.psv)
was recorded independently of the implementations. Both consumers validate
the header/count/unique IDs, known operations and complete expected maps.
No existing driver/premium time conventions or vectors are changed.

## Implemented source

Go orgtime/domain and Kotlin EnterpriseTime in the existing domain module
use standard time/instant/zone types without a replacement time hierarchy,
third-party dependency, migration, route or service. Go includes its
standard-library IANA fallback. Time is supplied explicitly; no system clock
is acquired by this kernel.

- Parse the bounded RFC3339 profile, normalize to UTC and retain nanoseconds.
  Years 0001–9999 must hold before and after normalization. Leap seconds,
  rollover dates, unknown -00:00, padding, malformed or over-±18:00 numeric
  offsets and excess precision fail. Format uses Z without trailing zeros.
- Capture declared occurred_at, trusted server received_at, entry zone and
  device offset separately. The integral-second device offset can differ
  from zone rules; neither the instant nor metadata is silently corrected.
- Resolve reporting in the supplied operating-unit zone, otherwise the
  organization zone. A supplied invalid unit cannot fall back. Both context
  zones must be valid. Retain day, selected zone and unit/organization
  provenance as one historical snapshot. A trip caller supplies its start.
- AuthorityAt selects receipt, irrespective of an earlier/later declaration.
  Periods include their start and exclude their end; empty/reversed periods
  and unsupported native instant inputs fail. Go comparisons remove
  process-local monotonic readings.
- Assess a separate sync device sample against its server receipt, retaining
  signed seconds plus nonnegative nanos. CLOCK_SKEW means absolute delta
  strictly exceeds the nonnegative organization limit (default 300 seconds).
  An explicit zero limit is valid. Inputs are unchanged. Full-range cases
  avoid Go Duration saturation and nanosecond multiplication overflow.

The one-way clock estimate can include transit delay; it is not an external
time service or proof of membership. Owning server flows still acquire
received_at from the trusted clock, load organization/unit context under
authority, resolve membership/plan/period locks and apply skew flags/review
to the batch. An old operation's occurred_at must never be used as a sync
clock sample. No active sync/auth/trip/report flow consumes this kernel yet.

Persistence owners must store all snapshot fields together, preserving
historical attribution when organization/unit zones or tzdb rules change.
These vectors establish scoped Go/JVM parity on the recorded providers,
not equivalence of every deployed IANA database. Native Android tzdb and
composed consumers remain future acceptance.

## Executed checks

Go 1.27.4 (module toolchain; host Go 1.22.2), staticcheck v0.8.1 under its
Go 1.27.1 toolchain, Gradle 8.10.2, existing Kotlin 2.0.21/JDK17
Temurin 17.0.19. GOCACHE=/tmp/enterprise-go-build;
XDG_CACHE_HOME=/tmp/enterprise-staticcheck for staticcheck. Gradle used
JAVA_HOME=/home/iiii/.gradle/jdks/eclipse_adoptium-17-amd64-linux.2,
ANDROID_HOME=ANDROID_SDK_ROOT=/data/dev/android/sdk/Sdk, offline/no-daemon.

1. RED: the UTC Go vector failed with invalid-time before parser behavior.
   Kotlin's initial parser scaffold ran 86 vectors with 44 expected failures.
   The initial 86-vector GREEN passed; seven full-range/provenance regressions
   and two invariants per language were then added before final checks.
2. From backend:
   `go test -race -coverprofile=/tmp/enterprise-plat015-go.cover ./internal/modules/orgtime/domain ./internal/modules/orgvalue/domain ./internal/modules/kernel -count=1`
   passed. Time and exact-value packages each have 100% statement coverage;
   kernel 94.4%. The time suite executes 93 shared rows plus two invariant
   tests, including direct typed-input rejection.
3. `go vet` and pinned `staticcheck` on those three packages passed.
   `go build ./cmd/api ./cmd/worker ./cmd/ops` passed. The quick behavioral
   manifest now includes orgtime/domain.
4. Final `./gradlew :domain:test --offline --no-daemon` passed:
   **1,155 tests, zero failures/errors/skips**, including 93 shared time
   vectors and two time invariants. The new time classes cover
   **592/594 instructions (99.66%) and all source lines**. Boundary midnight,
   New York gap/fold, Apia skipped day, historical Brazil DST, quarter-hour
   zones, aliases, offset mismatch, nanoseconds and full-year limits pass
   identical expectations. Changing device time cannot extend the receipt
   period; changing context cannot rewrite the historical snapshot.
5. Final enterprise verifier/regression, added-link checks, fail-on-output
   Go formatting, diff whitespace and tracked/staged secret scans passed.
   Verifier: 348 tasks, 545 rules, 167 use cases, 145 screens, 0 problems.
   These remain specification counts, not completed product features.

The existing aggregate Kotlin domain coverage gate remains below 90%:
**56,621/70,558 instructions = 80.2475%** with the configured exclusions.
The earlier deficit was independently reproduced before EMP-PLAT-009;
[this baseline evidence](f1-plat009-exact-values.md) retains it. The current
XML was calculated with the same exclusions; no threshold, test, source
exclusion or dependency was weakened. This scoped task passes; aggregate
coverage is not represented as PASS or merge-ready. Verification was not
repeated solely to reproduce that known numerical failure.

SHA-256 of final behavior inputs:

- Go implementation: e17abb317390ab0f7f9dd11b5a3afff2a031341b28dcef0ccd8000480f91ea76.
- Go vectors test: 8e2d8395ca57c5495240bd0be45d588f867277b35d6890039d8d042ca15d40b2.
- Go invariants: 42ebffd93f851cb3ea1a6425cd233df13bbdfc82f51a4aaee4ea7708080c6095.
- Kotlin implementation: 63d03e784acdc42e649fbbba978314a6445867092135eea2996690c728a48313.
- Kotlin tests: 8ec1d57d2deeb2068350d08be365fd945444cbae6266a8e392f30e9eb302895f.
- Shared oracle: d15fb83a5143a616f250b2837747156261d8af163cfaaa8b421dcd2bb75a646b.

## Rollback and remaining gates

The pure helpers have no persistent or external effect. A source rollback
removes their callers; future consumers preserve original instants and
stored reporting snapshots rather than recomputing or destroying history.
No production data, photos, credentials or location traces were used.

EMP-PRIV-003 is the next eligible F1 foundation; EMP-PLAT-016 is F3 and is
not advanced by this task. Keep the global product surface off and the
existing Android protected-persistence port unavailable. Native device
execution, protected-store proof, current AUTH/membership providers,
single-family revocation, ordinary runtime login, feature limits/cleanup,
private storage lifecycle/backup authority, legal real-receipt policy and
the other [review activation gates](f1-pr202-review.md) remain owed.
This publication neither merges PR202 nor deploys/certifies production.
