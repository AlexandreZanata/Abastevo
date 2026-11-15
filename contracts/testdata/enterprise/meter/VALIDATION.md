# Reading validation oracle v1

EMP-METER-003 / B-BR-EMP-280..285. These 104 synthetic cases extend the
accepted meter (261) and capture (40) oracles without changing them. Counts:
POLICY 8, CHECK 79, REEVALUATE 17. SHA-256
`65f0cc26b961c8f2cd661b11f89e8b242816a6d4426254efa8a942b4477bee95`.
Consumers: `backend/internal/modules/fleet/domain/meter` (Go) and
`com.anpfuel.domain.enterprise.meter` (Kotlin); both assert the digest, the
counts, unique case IDs, the known operations and fields, and complete
expected maps. No row is skipped or repaired.

The exact header is `id|operation|args|expected`; every other line has four
columns. Fields are `key=value` joined by `;` (the first `=` splits); an empty
field list is not allowed except `changes=` and `series=`, which are explicit
empty values. Unknown operations, unknown or duplicate keys and missing
expected keys are loader errors.

Identifiers: `R<i>` maps to `8c000000-0000-4000-8000-` plus the twelve-digit
number `200 + i` (R0 = …200). Any other identifier is literal input (the
tie-break cases use …150, which orders before …200). Instants are canonical
RFC 3339 UTC with an optional 1..9 digit fraction. Values are base units
(metres or seconds) in decimal and may be negative only to test refusal.

## Operations

- **POLICY** — `rate` (metres per hour) and `skew` (seconds) construct a
  plausibility configuration. Output `rate;skew`, or `error=INVALID` when the
  rate is outside 20 000..200 000 or the skew is negative.
- **CHECK** — `kind` (default `ODOMETER`), `rate` (default 150000), `skew`
  (default 300), `series` and `candidate`. `series` is a `~`-separated list of
  `id,value,occurred,received,state,flags` in series order (first = opening
  reading); `flags` is `+`-separated in canonical order or empty. `candidate`
  is `id,value,occurred,received`. Output
  `state;flags;retroactive;previous;next` (`?` when a neighbour is absent),
  or `error=INVALID`.
- **REEVALUATE** — `kind`, `rate`, `skew`, `series`. Output `changes`: the
  `~`-separated `id:STATE:flags:note` of every SUSPECT member whose final state
  or flags differ from the input, in series order (`note` is `RETROACTIVE` or
  empty), or `error=INVALID`.

## Shared rule

- Order: `occurred_at`, then `received_at`, then identifier (byte order).
  Neighbours are the closest `VALID` members before and after; `SUSPECT` and
  `VOIDED` members are ignored. A candidate ordered at or before the first
  member (the opening reading) is INVALID, whatever the opening's state.
- Flags, canonical order: `READING_BELOW_PREVIOUS` (value below the
  predecessor), `READING_ABOVE_NEXT` (above the successor),
  `IMPLAUSIBLE_RATE` (either non-decreasing pair fails, counted once),
  `FUTURE_INSTANT` (`occurred_at` strictly later than `received_at` plus the
  skew). Odometer pair: `Δv × 3600 × 10⁹ ≤ rate × Δt_ns + 3 600 000 × 10⁹`;
  hour-meter pair: `Δv × 10⁹ ≤ Δt_ns + 300 × 10⁹`; exact, no rounding. The
  hour meter ignores the distance rate.
- Any flag means SUSPECT; `retroactive` is true when a successor exists.
- Re-evaluation visits SUSPECT members in series order against the current
  VALID members, recomputes the neighbour flags, keeps an input
  `FUTURE_INSTANT`, and repeats full passes until one promotes nothing. VALID
  and VOIDED members never change.
- INVALID input: empty series, unordered or duplicate members, a candidate
  already in the series, empty identifiers, values outside
  0..10¹² m / 0..3 600 000 000 s, unknown kind or state, `VALID`/`VOIDED` with
  flags, `SUSPECT` without flags, unknown, duplicate or non-canonical flags,
  and an invalid configuration.
