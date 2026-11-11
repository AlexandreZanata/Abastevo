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
An unknown discount therefore skips the arithmetic check (vector
missing-discount). A fueling command whose receipt states no discount must
pass an explicit zero; BR-FUEL-002's "Paid R$ 221.00 with no discount" warning
depends on that mapping, which the EMP-FUEL command owns.

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

EMP-PLAT-015 executes all 99 rows of time-v1.psv in Go orgtime/domain and
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
abbreviations, single-segment names, raw offsets, path/posix/right/SystemV
names and unknown zones fail. Go applies this profile in kernel.LoadTimeZone,
which organizations share, so a stored organization zone always reports.
The optional unit overrides organization reporting; a supplied invalid unit
does not fall back. Both context zones must be valid. Persist the resulting
day, zone and unit/organization source with the operation. The trip vector
uses its start instant. Historical snapshots survive later context changes.

PERIOD includes start and excludes end, rejecting empty/reversed ranges.
Authority uses server receipt, even when the declared time is earlier.
A server receipt equal to the minimum instant 0001-01-01T00:00:00Z is
refused in RECORD and SKEW: it is Go's unset time value and must never
pass as an authority time. Device samples may still be that instant.
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
invalid contexts, nanosecond edges and offset/clock bounds. They map
instants to local days only; no operation converts a local time to an
instant, so gaps and repeated hours need no policy yet. A future builder of
local boundaries must define one with Go/JVM vectors first. This certifies
these vectors on the recorded Go/JVM providers, not every IANA database or
native Android device. No sync route, receipt retention or live feature is
activated; owners still provide trusted clock/context and review wiring.

## Organization lifecycle v1

EMP-ORG-001 executes all 111 rows of organization-lifecycle-v1.psv in Go
organization/domain. This is a backend domain oracle, not client parity or
runtime acceptance. Expected maps were recorded from BR-ORG-001/002/004/
025/027/037 and the organization state graph before implementation.
Consumers check the header/count/unique IDs, known operation and complete
output map; no case is skipped. The four-column PSV format is unchanged.

The 25 from/to pairs cover all five states. All 32 activation combinations
cover name, valid IANA zone, active unit, active owner and terms.
Other rows pin registered reasons, malformed kinds/zones/states, descriptive
tags/name, stale/exhausted revisions, archive blockers and unknown/pending/
cancelled/elapsed closure conditions. Kind tags are sorted/deduplicated as
a set, without manufacturing OTHER for absence. Name/zone text is retained;
spaces and invisible format characters are examined only for name presence.
The display name is free text bounded at 120 code points without control or
bidirectional override characters (BR-PRIV-007, BR-ORG-005). Zones follow
the shared operation-time profile: GMT, EST, Japan, Factory, localtime,
posixrules and SystemV/ names fail even when the host resolves them, and
Etc/ and slash-separated aliases are accepted.

All transition conditions are server-resolved facts, never client authority.
CLOSED cannot leave even through a self-edge. No clock, scheduler, owner/unit
write, grant, audit, repository or physical closure occurs. Rehydrate is a
trusted-storage constructor and cannot serve as a client command decoder.
Fresh-ID uniqueness and cross-record truth remain owning-command/DB work.

```sh
(cd backend && go test -race ./internal/modules/organization/domain -count=1)
```
