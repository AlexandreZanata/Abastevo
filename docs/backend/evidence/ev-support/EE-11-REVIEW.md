# EE-11 independent exactness and boundary review

Status: LOCAL_DONE
Validation: PASS
Parent: d9b31fe45adeaa0ae114a8b59efc67c00b910b85. Date: 2026-10-21.

Before behavior: audit the submitted EE-11 engine under B-BR-EV-04/11/12/17–19
and BUC-EV-04/06/07/11 before dependent MEX-10. Reuse the frozen Tariff and
Calculation shapes and exact pricing semantics; no provider/source activation.

Actual read-only RED reproduction: five valid coprime denominators near1e9
with quantity1 and numerator1 should round once to KNOWN0 centavos. The Rust
oracle instead returns UNKNOWN/OVERFLOW because it multiplies denominators in
i128. `/tmp/bfp-ee11-audit-red.log` retains the failing assertion. A large
intermediate is not proof that the final amount exceeds the money ceiling.

Other review targets: RFC3339 fractional instants and impossible calendar
dates; schema UUID identity and Unicode maxLength semantics; every supplied
scenario quantity must satisfy its existing bounds even if a component is
absent. Keep malformed input refusal distinct from missing/unknown prices.

Acceptance: regressions must fail before fixes. Identical new extreme/Unicode/
time/quantity vectors execute in Go/Kotlin/Rust; invalid Tariff DTO identities
are refused by Go/Kotlin and impossible dates by the Rust decoder. Exact
positive arithmetic must cover all five distinct dimensions without a new
dependency. Real pure-module race/static, full Rust tests/fmt/clippy, affected
JVM/coverage and APK compile checks; source/diff/secrets review. Use actual
Go1.27.3 via GOTOOLCHAIN while the separate dev runtime-pin commit is pending
final history reconciliation. Recovery: refuse unsupported derived output and
retain raw inputs; no storage/schema/public API changes.

## Delivered and actual acceptance

- RED reproduced real defects in all three implementations. Go accepted
  malformed UUIDs, rejected valid128-scalar non-ASCII text and ignored unused
  invalid scenario quantities. Kotlin also ignored invalid unused quantities
  and accepted malformed UUIDs. Rust falsely overflowed small valid tariffs;
  its original decoder accepted February31 and did not support fractional
  instants. Original logs retained under `/tmp/bfp-ee11-*-red*.log` and
  `/tmp/bfp-ee11-calendar-red.log`; no failing assertion was removed.
- Go/Kotlin now enforce the existing frozen UUID and Unicode maxLength
  contract. Every supplied scenario quantity is validated before pricing
  completeness/currency/window decisions; absence remains distinct from zero.
  Existing membership, tax, grace, steps, half-up, money bound and UNKNOWN
  meanings are unchanged.
- Rust now uses checked four-limb positive arithmetic. Five unique dimensions
  with denominators<=1e9 yield a common denominator<2^150; numerators<=1e12
  and billed quantities<2e9 keep the sum numerator<2^193.256 bits cover the
  complete existing envelope. Exact monotone comparisons perform the single
  final half-up rounding without dividing/rounding intermediate components.
  Overflow checks remain; no arbitrary-precision dependency was added.
- Rust's shared decoder now validates real Gregorian dates and preserves
  RFC3339 instants to nanoseconds, including offsets. Its internal typed
  instants use i128 nanoseconds; this is an oracle helper, not a new runtime
  endpoint or persisted timestamp migration. Calendar/carry/money boundary
  tests cover both ordinary and overflow/refusal paths.
- All original42 rows are unchanged;11 independent cases extend the shared
  corpus to53. Extreme expected totals were derived independently with Python
  Fraction, including equal/coprime denominators, a large valid paid result,
  nanosecond windows, unused invalid quantities and Unicode bounds. The
  original failing standalone Rust reproduction now returns KNOWN0.
- `GOTOOLCHAIN=go1.27.3 go test -race -count=1` for charging, metrics and
  apicontract PASS; Go vet and pinned analysis-only staticcheck PASS.
  `/tmp/bfp-ee11-go-green.log` and `/tmp/bfp-ee11-contract-acceptance.log`.
  Saved source symbol scan `/tmp/bfp-ee11-govuln.json` confirms Go1.27.3,
  zero finding events and successful exit.
- Entire Rust crate tests and integration suites, fmt and all-target clippy
  with warnings denied PASS (`/tmp/bfp-ee11-rust-{green,clippy}.log`).53
  shared rows execute in Rust/Go/Kotlin, without changes to Cargo.lock.
- Full JVM/client acceptance PASS: domain632, application390, data450
  (two pre-existing opt-in skips), app375; no failures/errors. Debug app/test
  APKs and lintDebug PASS. `/tmp/bfp-domain-audit-complete.log` records the
  full run. The original global90% gate also passes through the meaningful
  [domain boundary audit](../mobile-expansion/DOMAIN_AUDIT_20261021.md);
  neither the threshold nor exclusions/build configuration were changed.
- Formatting, diff, local-link and changed-surface secret checks PASS.
  No SQL/schema/provider/dependency/source activation. Isolation and pending
  dev/main runtime-pin reconciliation remain explicit; public source and
  real commercial/native/production gates are not certified here.

Integration: INTEGRATION_PENDING. The audited checkpoint supersedes original
EE-11 exactness claims beyond its42 reviewed inputs; MEX-10 may now reuse the
repaired source. This review is additional prerequisite work, not one of the
four newly authorized construction microtasks.
