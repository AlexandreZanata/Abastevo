# Segment oracle v1

EMP-METER-005 / B-BR-EMP-481..486 (EDR-311..314). These 143 synthetic cases
extend the meter (261), capture (40) and validation (104) oracles without
changing them. Counts: CHECK_WRAP 31, REEVALUATE_WRAP 13, CUMULATIVE 35,
USAGE 19, REPLACE 17, RESET 12, ROLLOVER 16. SHA-256
`296afeb474264ba4305004cbcc13c56761c192eca8c7ab9b9eede6e9cf15d21f`.
Consumers: `backend/internal/modules/fleet/domain/meter` (Go) and
`com.anpfuel.domain.enterprise.meter` (Kotlin); both assert the digest, the
counts, unique case IDs, the known operations and fields, complete expected
maps, and that every operation, `POSSIBLE_ROLLOVER`, both `INSUFFICIENT_DATA`
reasons and the error code have a vector. No row is skipped or repaired.
Expectations were produced by an independent arithmetic model written from
the contract rules (not from either implementation) and the worked examples
C and D, a reset and the unknown final value were checked by hand.

The header, row and field grammar are those of `VALIDATION.md` (`key=value`
joined by `;`, the first `=` splits; unknown operations or keys, duplicate
keys and missing expected keys are loader errors; `changes=`, `series=`,
`spans=` and `readings=` may be empty).

Identifiers: `R<i>` → `8c000000-0000-4000-8000-` + twelve digits of `200 + i`,
`S<i>` → `400 + i`, `M<i>` → `500 + i`; any other identifier is literal.
Outputs print them back the same way, `?` when absent. Instants are canonical
RFC 3339 UTC with an optional 1..9 digit fraction; values are base units.

## Operations

- **CHECK_WRAP** — as `CHECK` of `VALIDATION.md` (`kind`, `rate`, `skew`,
  `series`, `candidate`) plus optional `modulus` (absent: none). Output
  `state;flags;retroactive;previous;next`.
- **REEVALUATE_WRAP** — as `REEVALUATE` plus optional `modulus`. Output
  `changes`.
- **CUMULATIVE** — `kind`, `spans`, `readings`, `at`. Output
  `reason;usage;equivalent;anchor` (`reason` empty when known; `usage` and
  `equivalent` `?` when not).
- **USAGE** — `kind`, `spans`, `readings`, `start`, `end`. Output
  `reason;usage;from;to`.
- **REPLACE** — `kind`, `modulus` (old meter), `series` (the open segment,
  first = opening), `at`, `final` (value or `?` = unknown), optional
  `new_modulus`, `opening`. Output `previous;close;open;implied;reason`.
- **RESET** — `kind`, `modulus`, `series`, `at`, `before`, `after`. Same
  output.
- **ROLLOVER** — `kind`, `modulus`, `series`, `target`. Same output.

`spans` is a `~`-separated list of
`segment,meter,reason,opened,open,closed,close` in chain order (meters in
installation order); `closed` and `close` are `-` while open, `close` is `?`
when a removal closed it with an unknown value. `readings` is a `~`-separated
list of `VALID` anchors `id,segment,value,occurred,received`. `series` uses
the `VALIDATION.md` member form `id,value,occurred,received,state,flags`.

## Shared rule

- With a modulus `M` (positive; every value strictly below it), a candidate
  below its valid predecessor `v₁`, with no valid successor, whose wrap
  `(M − v₁) + v₂` passes the exact §10.2 pair check, is flagged
  `POSSIBLE_ROLLOVER` instead of `READING_BELOW_PREVIOUS`; the canonical flag
  order ends with `POSSIBLE_ROLLOVER`; without a modulus the flag is invalid
  input and never produced.
- A track is INVALID unless: spans are non-empty with unique identifiers; the
  first span of every meter is `INSTALLATION` and a meter's spans are
  contiguous (`RESET`/`ROLLOVER` opening exactly at the previous close, which
  is known; `ROLLOVER` opens at 0); a later meter opens at or after the
  previous meter's last close and never reappears; only the last span is
  open; a closed span closes strictly after it opens with `close ≥ open`;
  only a meter's last span may close unknown; values within the kind's
  bound; every reading names a span, lies in `[opened, closed]`, has
  `open ≤ value ≤ close` (when known), and the valid readings of a span are
  non-decreasing in BR-METER-008 order; reading identifiers are unique.
- The anchor at `t` is the latest reading with `occurred ≤ t` ordered by span,
  then occurred, received and identifier. `U(t)`, `E(t)` and `usage(t1, t2)`
  follow B-BR-EMP-482; a missing anchor is `NO_READING_BEFORE` (found anchors
  are still printed), an unknown close in the summed spans is `METER_GAP`,
  `start` after `end` is INVALID.
- Planners (B-BR-EMP-484..486) validate the series as `CHECK_WRAP` does and
  refuse with INVALID exactly as the contract states; `implied` is `?` with
  `reason=METER_GAP` for an unknown final value.
