# MEX-09 — Go/Kotlin financial and typed-wire parity

Status: LOCAL_DONE
Validation: PASS
Parent: b71f2732. Date: 2026-10-21.

## Before behavior

B-BR-MEX-03/04; BUC-MEX-02/03; PRM financial-v1 and accepted EE-02.
MEX-08 remains blocked by EE-19; MEX-09 is the next ready MEX task after
MEX-07. Reuse metrics, PremiumConsumption, ledger values and owner-scoped
transport types. Do not introduce billing, ledger persistence or EV math.

Actual paid centavos and integer milli-units/metres remain authoritative;
configured tank/route/efficiency estimates never become measured intervals.
Require initial and final full anchors, homogeneous supported liquid fuel and
matching units including the excluded anchor. GNV/mixed methods stay UNKNOWN;
raw entries remain valid. Preserve discounts, gaps/reset/missing-odometer and
weighted sums. Checked additions and arbitrary-precision ratio intermediates
must prevent valid bounded inputs from wrapping. Out-of-range final integer
sums fail with existing input-out-of-bounds, never a plausible partial metric.
Explicit matched-window inputs must satisfy the existing nonnegative money/
odometer bounds before subtraction, and preserve half-open time/dedup rules.

Acceptance: real RED/GREEN in both existing engines; identical input/result
CSV vectors consumed by Go and Kotlin, retaining original financial-v1 JSON;
full/partial/full, discount, fractions/weighted values, missing anchors, mixed
anchor/unit, GNV/flex refusal, sum/scaled overflow, matched period/gap/reset and
existing comparison arithmetic. Shared enum/value/wire compatibility plus
existing owner/outbox DTO tests; full affected JVM, Go consumers, build/static/
format/diff/secrets. No migration, new dependency or public API required.

## Delivered and actual acceptance

- Reused the existing PRM06/PRM09 engines. Both now require the first and
  last full anchors, include the excluded anchor in fuel/unit compatibility,
  reject invalid raw bounds and preserve authoritative discounted totals.
  Configured km/L/tank/route estimates remain separate from measured intervals.
- Checked additions protect quantity, cost and distance totals. Arbitrary-
  precision ratio intermediates preserve valid large results instead of
  wrapping: 10,000 bounded fills produce cost/km `10000000000000000000`,
  previously `-8446744073709551616`. Final sums outside signed64 return
  `input-out-of-bounds`, with no partial totals. Matched windows reject negative
  base costs and invalid odometer endpoints before subtraction; half-open
  timestamps, duplicate expense exclusion and existing method IDs survive.
- Extracted the existing Go comparison arithmetic into its pure module and
  reused it in the same adapter. Kotlin now has the equivalent pure value
  calculation; adapter method/unit/timezone/period checks stay in place.
  No new report endpoint or runtime billing/EV method is implied.
- [36 executable financial vectors](../../../../contracts/testdata/premium/financial-execution-v1.csv)
  run the same inputs and exact output strings/integers in Go and Kotlin.
  Coverage includes all fifteen unchanged financial-v1 examples, both missing
  anchors, anchor mismatch, invalid unit, large scaling, checked sums, gap/reset,
  GNV UNKNOWN, unmatched month, dedup/window boundaries and positive/negative/
  incomplete/zero-base comparison. The Go frozen-vector guard verifies every
  original expected value; month/comparison are no longer silently omitted.
- [22 shared raw-value vectors](../../../../contracts/testdata/premium/financial-wire-v1.csv)
  exercise actual Go Refuel JSON round trips/normalization and Kotlin's frozen
  enums/bounds. Kotlin fuel values now carry their existing L/M3 pairing and
  the engine reuses it. CNG/M3 remains accepted raw data with an unsupported
  consumption method; FLEX/unknown/electric aliases and invalid bounds refuse.
  Existing owner/origin command/mapper/outbox tests pass; an added ledger
  payload round trip preserves exact integer text and scope. Client payloads
  remain opaque; new typed ledger persistence/transport belongs to MEX-11/12.
- RED actually executed against the original engines: six Go subcases and
  three Kotlin tests failed (missing anchors, overflow and negative base money;
  Go also reproduced the excluded-anchor fuel/unit defects). GREEN after
  corrections. Logs: `/tmp/bfp-mex09-{go,kotlin}-red.log`,
  `/tmp/bfp-mex09-{go,kotlin}-green.log`.
- Go race acceptance PASS for metrics/domain, adapters/HTTP, ledger/domain,
  full OpenAPI/API contract and API composition:

  ```sh
  cd backend
  go test -race -count=1 ./internal/modules/metrics/... ./internal/modules/ledger/domain ./internal/platform/apicontract ./cmd/api
  ```

  `/tmp/bfp-mex09-acceptance-go.log` records the full scope; final added
  failure vectors were re-executed with the metrics race suite in
  `/tmp/bfp-mex09-final-vectors-go.log`. `go vet`, pinned
  `GOTOOLCHAIN=go1.27.1 staticcheck` for those same packages and API build
  (`-o /tmp/bfp-mex09-api`) PASS. No SQL/migration changes: the unchanged
  MEX-07 real PostGIS transaction/race acceptance remains applicable.
- Full Android/JVM consumer matrix PASS:

  ```sh
  ./gradlew --offline --no-daemon --max-workers=2 :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug
  ```

  Final XML totals: domain608, application390, data450 (two pre-existing
  skips), app375; zero failures/errors. Final six financial failure vectors
  and source formatting were revalidated by `:domain:test :app:assembleDebug
  :app:assembleDebugAndroidTest`; final domain608 includes all36+22 shared
  cases. Logs: `/tmp/bfp-mex09-acceptance-android.log` and
  `/tmp/bfp-mex09-final-domain.log`. APK compilation is not device execution.
  Lint: zero fatal/errors,207 warnings/two informational reports;
  no claim of a warning-free project or overall coverage threshold.
- gofmt, `git diff --check`, tracked scanner and all changed/untracked-surface
  secret review PASS; changed Markdown local-link check:306 links, zero missing.
  No dependencies, schema migrations or public wire
  changes. Same isolated branch as MEX-07; primary checkout/drafts untouched.

Fixture encoding: CSV uses exact decimal integers and canonical reduced
rational strings. Financial rows name INTERVAL/SUM/MATCHED/COST/COMPARE;
colon tuples encode fill `quantity:cost:full:fuel:unit`, result
`distance:quantity:cost:unknown`, or expense `id:amount:millis:odometer`.
Semicolons separate tuples, `_` means absent, `N*tuple` repeats at most20,000
items and `#` expands an expense index. Empty expected integers mean zero;
empty expected strings mean absent, never skipped assertions. All fixtures
are synthetic, using exact manually derived expected results.

Recovery: refuse the unsupported/invalid derived result, retain raw records
and free read/export/delete. No data reset or down migration is required.
Integration: INTEGRATION_PENDING; no push/PR/merge/wiki/deploy in these units.
The separate global Go1.27.2 vulnerability CI gate remains unresolved; this
record does not claim a clean vulnerability scan. Native journeys/provider/
production acceptance remains OWED under the existing task owners.
Next ready MEX unit is MEX-10 on new authorization; MEX-08 still needs EE-19.
