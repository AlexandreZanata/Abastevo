# Capture provenance oracle v1

EMP-METER-002 / B-BR-EMP-152. These forty synthetic cases extend the accepted
261-case meter oracle without changing it. Counts: SOURCE17, READING23.
No locator proves a physical device, cookie, database row, authorization or
current session. The actual source boundary must provide those facts.

The exact header is `id|operation|args|expected`. Reject blank rows, duplicate
or empty case IDs, unknown operations/fields, duplicate/empty field keys,
malformed UTF-8, missing output fields and unexpected output fields. Expected
maps are compared completely. A semicolon separates fields and the first `=`
separates key/value; empty values remain explicit input, never a default.

Alias UUIDs use `8c000000-0000-4000-8000-` plus a twelve-digit suffix: A=1,
B=2, D=51, W=71, E=41, O=101. `?` is absence. Other strings are literal input.
SOURCE supplies org/kind/id and returns the exact retained kind/org/id.

READING defaults to organization A, asset11, meter21/ODOMETER, segment31,
reading201, member E/org A, operation O/org A, legacy device D/org A,
capture NONE, observed17/VALID, declared2026-11-02T00:00:00Z/UTC/offset0,
received2026-11-03T00:00:00Z, STANDALONE/TYPED and no photo. Overrides are
legacy/legacy_org, capture/capture_org/id, author/author_org,
operation/operation_org, value and state. `capture=NONE` omits the new source;
ZERO requests an invalid empty source. Every other capture value is the
actual source-kind input. SOURCE has exactly org/kind/id fields. Successful
READING returns canonical kind/org/id plus retained value/state.

The legacy Device input is enrolled shorthand only when explicit Capture is
absent. Both forms together are invalid even with matching IDs. Go snapshots
retain exactly one original form and clone its capture pointer. Reading's
canonical source accessor always has an explicit tag. Kotlin's retained
`.device` compatibility accessor is valid only for ENROLLED_DEVICE; on WEB_SESSION
it throws typed INVALID, while `deviceOrNull` is null. New adapters use the
canonical capture accessor. Author and operation remain required. FactInput,
classification, clocks, ordering and all other meter rules remain unchanged.
