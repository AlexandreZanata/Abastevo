# Enterprise exact-value vectors v1

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
