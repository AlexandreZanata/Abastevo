# Enterprise domain vectors

## Exact-value vectors v1

EMP-PLAT-009 executes all 112 rows of exact-values-v1.psv in Go and Kotlin.
Each consumer checks the header, row count, unique IDs, known operations and
the complete expected output map. There are no language-specific skips.

The synthetic oracle follows BR-MONEY-001/003/004/005/006/007/010/011,
VAL-08 and VAL-09 and the existing financial-v1 bounds. Expected results
were recorded before implementation; they are not generated from either
implementation. Existing premium/driver fixtures remain unchanged.

The four columns are id, operation, args and expected, separated by a
literal vertical bar. Argument/result fields use semicolon-separated
key=value pairs; allocation lines use order:weight. The question mark is
fixture-only notation for absence, never a monetary zero. Spaces in parser
inputs are intentional. All identifiers and source text are synthetic.

The parser operations accept canonical unsigned integer text only.
Centavos allow 0..10^12; quantity allows 1..10^9 milli-units; recorded unit
price allows 1..10^6 milli-BRL. Signs, leading zeros, whitespace, exponent,
fraction, Unicode digits and overflowing values are refused.

RECONCILE uses canonical L/M3, existing integer representations and
nullable facts. Expected cents are half-up(quantity * price / 10000).
The explicitly supplied paid and discount facts produce a widened gross
sum (up to 2*10^12); a difference above one cent flags a warning while
preserving paid. Missing paid/quantity/discount/price stays unknown. A
missing price additionally yields a labelled exact rational milli-BRL
price from paid/quantity, rather than a manufactured recorded price.
Known expected/derived values remain calculable when an independent fact
is absent. Difference/warning remains absent until gross and expected are known.

Source-price text is preserved verbatim as metadata. The helper does not
interpret, truncate or round original decimal text, and does not claim
that source extraction/normalization has been implemented. DOC/FUEL owns
that conversion and record acceptance. This consistency check uses the
explicitly supplied milli-BRL value, as specified by BR-MONEY-005.

ALLOCATE uses largest fractional remainder, with ascending explicit line
order for ties. Output is canonical by order and sums exactly to source.
Weights and their sums use widened exact intermediates; a weight sum may
exceed int64. Missing basis is unknown; a known zero source with positive
basis produces known zeros. Duplicate/negative orders, negative weights,
over-bound source and more than the existing 1,000-item budget are refused.
The personal driver allocator retains its ID-based tie rule.

Run from the repository root:

```sh
(cd backend && go test -race ./internal/modules/orgvalue/domain -count=1)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.EnterpriseValuesTest' --offline --no-daemon
```

These are pure domain vectors. They activate no route, payment, fueling,
document persistence or client feature and require no database or provider.

## Operation-time vectors v1

EMP-PLAT-015 executes all 93 rows of time-v1.psv in Go orgtime/domain and
Kotlin EnterpriseTimeTest, checking the same complete output maps, header,
row count, unique IDs and known operations without language skips. The
synthetic oracle was recorded before implementation from BR-TIME-001/002/
003/004/006 and the clock case in VAL-23. The format is the same four-column
PSV; question mark means absent unit or default skew limit, not zero.

INSTANT accepts the shared bounded RFC3339 profile: years 0001–9999 both
before and after UTC normalization, seconds 00–59, at most nine fractional
digits, uppercase T/Z and Z or numeric minute offsets through ±18:00.
Unknown -00:00, leap seconds, calendar rollover, whitespace and excess
precision fail. Canonical output uses Z and trims trailing fractional zeros.

RECORD keeps the declared instant, trusted receipt, entry zone and declared
integral-second device offset (±18h) separately. Entry/reporting zones do
not reinterpret the instant or silently correct a mismatched device offset.
UTC and valid slash-separated IANA names/aliases are accepted; Local,
abbreviations, raw offsets, path/posix/right names and unknown zones fail.
The optional unit overrides organization reporting; a supplied invalid unit
does not fall back. Both context zones must be valid. Persist the resulting
day, zone and unit/organization source with the operation. The trip vector
uses its start instant. Historical snapshots survive later context changes.

PERIOD includes start and excludes end, rejecting empty/reversed ranges.
Authority uses server receipt, even when the declared time is earlier.
SKEW compares a sync device clock sample to its server receipt sample,
not a historical operation. Signed difference is seconds + nonnegative
nanos (−0.1s is −1s + 900000000ns). CLOCK_SKEW applies strictly above
the nonnegative seconds limit, default 300; zero is a valid explicit limit.
The estimate includes possible transit delay and changes neither time.
Extreme-year cases guard against Go Duration saturation/nanosecond overflow.

Run from the repository root (existing JDK17/Android SDK environment):

```sh
(cd backend && go test -race ./internal/modules/orgtime/domain -count=1)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.EnterpriseTimeTest' --offline --no-daemon
```

Vectors cover Brazil/local-midnight boundaries, New York gaps and repeated
hours, the Apia skipped day, historical Brazilian DST, quarter-hour zones,
invalid contexts, nanosecond edges and offset/clock bounds. This certifies
these vectors on the recorded Go/JVM providers, not every IANA database or
native Android device. No sync route, receipt retention or live feature is
activated; owners still provide trusted clock/context and review wiring.
