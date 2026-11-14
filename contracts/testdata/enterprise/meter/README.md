# Pure meter oracle v1

EMP-METER-001 / B-BR-EMP-131..136. This synthetic oracle is independent of
the Go/Kotlin implementations. It activates no repository, intake handler,
device, meter, permission, document upload or plausibility policy.

## Format and strict loading

`meter-v1.psv` has the exact header `id|operation|args|expected`. Every remaining
line is one case with four columns. Do not skip blank or unknown rows. Assert
the exact total and per-operation counts recorded below, nonempty unique case
IDs, known operations, unique nonempty `key=value` field names and complete
expected maps. A semicolon separates fields; the first equals sign separates
key/value. Hexadecimal fields contain lowercase, even-length UTF-8 byte hex;
strictly reject malformed UTF-8 rather than replacing it. An explicit `?` is
fixture-only absence and never numeric zero. Unspecified fields take only the
explicit defaults below, not production defaults.

All aliases map to canonical nonzero UUIDs with prefix
`8c000000-0000-4000-8000-` and a twelve-digit suffix: A=1, B=2 (organizations),
X=11, Y=12 (assets), M=21, N=22 (meters), S=31, T=32 (segments), E=41, F=42
(memberships), D=51, Q=52 (devices), P=61 (photo). `Oi` maps to 100+i,
`Ri` to 200+i and `Fi` to 300+i. An unrecognized alias is literal input;
`?` maps to an absent identifier. These typed references validate shape and
supplied organization scope only; UUIDs reveal no table provenance.

## Shared pure contract

- Value kinds are ODOMETER/metres (unit `m`,0..1000000000000) and HOUR_METER/seconds
  (unit `s`,0..3600000000). Missing observation has a nonblank, well-formed, control-free
  reason of at most 280 Unicode scalars. It is distinct from observed zero.
- Optional modulus is positive canonical signed-int64-range integer text;
  every observed value is strictly below it. There is no invented modulus
  ceiling or automatic rollover. Decimal entry is canonical ASCII, no sign,
  exponent, whitespace, grouping or redundant leading integer zero. Fractional
  kilometre/hours parts allow respectively 1..3 / 1..2 digits, including zero
  trailing digits inside those limits. Hours/minutes are separate canonical
  whole hours and minutes in0..59. Conversion never rounds.
- Meter/segment/reading/author/device/operation/photo refs must match the
  supplied organization. Initial evidence is caller-classified VALID or
  SUSPECT; VOIDED is derived only from appended terminal facts. Time uses the
  accepted OrgTime bounded UTC/zone/offset rules. ReportingZones.UTC is used
  only for validation in this fixture, never an inferred organization zone.
  Explicit minimum declared
  instant is valid; minimum/unset server receipt is not. Offset disagreement
  with the IANA zone is retained, not repaired. No receipt-time query cutoff
  or future/installation/plausibility verdict is added here.
- One series owns one meter and one open segment with an observed opening
  reading. Append retains every equal observation with different operation
  IDs. Duplicate evidence IDs are INVALID; same-meter operation reuse as a
  new logical fact is OPERATION_ID_REUSED. A parent operation can be shared
  across different meters. Pure open-set validation permits both kinds on one
  asset but rejects two in-service meters of the same kind on that asset.
- VOID has a fresh fact ID and operation, author, device, server instant and
  nonblank reason. SUPERSEDE has those and one fresh replacement reading.
  The replacement and fact share ONE fresh correction operation, different
  from the original. Original observations never change. VOIDED targets are
  terminal and yield READING_ALREADY_SUPERSEDED on either repeated action.
  Every transition retains its exact org/asset/meter/segment/kind; no cycles,
  duplicate IDs, operation reuse or foreign links are allowed. No correction
  authority is granted by a pure constructor.
- Order is occurred_at, received_at, canonical reading UUID. Latest at t is
  the last VALID nonvoid/nonsuperseded reading with occurred_at<=t. Usage
  retains both actual latest-valid anchors and subtracts within the same
  meter/segment/kind only. Missing anchors return INSUFFICIENT_DATA with
  NO_READING_BEFORE; decreasing values or reversed endpoint instants are
  INVALID. Equal endpoints can return known zero with a real anchor.

## Fixture defaults and operations

Default owner tuple is A/X/M/S, kind ODOMETER, modulus absent. Membership E,
device D, operation O1 and absent photo all have organization A. Entry zone
and reporting organization zone are UTC, reporting unit absent, offset0.
Origin STANDALONE, method TYPED and supplied state VALID. Reading id R1,
value0, occurred_at2026-11-02T00:00:00Z, received_at2026-11-03T00:00:00Z.
An explicit empty/`?` override is invalid/absent, never a default restoration.

Series opening R0/O0 is observed0, VALID, at2026-11-01T00:00:00Z with receipt
2026-11-01T00:01:00Z. `opening_*` fields override it. Fact scope is the same
tuple, author E/device D and server instant2026-11-14T00:00:00Z; `fact_*`
fields override it. Every fact reason appears in the action itself. Default
HISTORY query is2026-11-30T00:00:00Z. These times are explicit synthetic input.

- VALUE: `kind`, `form=BASE|KM|HOURS|HM`, `text_hex`, optional `minutes_hex`
  and `modulus`. Output `kind;unit;value`. BASE is canonical integer base units.
- METER: defaults above plus `meter`, `segment` and `opening_*`; construct the
  scoped meter, its one segment and opening evidence. Output kind/unit/modulus,
  opening value and opening supplied state.
- OPEN_SET: `meters` is a `~`-separated list of `meter,asset,kind` tuples,
  with distinct per-meter segment/opening refs. `org`/`request_org` default A.
  Output count and sorted `asset:kind` pairs. Duplicate kind returns
  METER_ALREADY_IN_SERVICE; duplicate meter identity is INVALID.
- READING: defaults above; `org`, `asset_org`, `meter_org`, `segment_org`,
  `member_org`, `device_org`, `operation_org`, `photo_org` override reference
  scopes. `asset`, `meter`, `segment`, `id`, `member`, `device`, `operation`,
  `photo`, `kind`, `value_kind`, `value`, `state`, `occurred`, `received`,
  `entry`, `offset`, `origin`, `method` override evidence. Output all retained
  IDs/metadata, canonical UTC times and kind/unit/value. `value_kind` defaults
  to the meter kind; an incompatible value is INVALID.
- PRESENCE: `kind;value;reason_hex`. `value=?` constructs explicit missing;
  otherwise constructs observed base-unit value. Output kind/presence/value
  and retained reason hex (empty for observed). No fabricated reading is made.
- HISTORY: optional `readings` and `candidates` contain `~`-separated
  `id,operation,value,occurred,received,state` tuples. Base readings append in
  input order after the opening; candidates are constructed but appended only
  by a successful SUPERSEDE. Optional `reading_org`, `reading_asset`,
  `reading_meter`, `reading_segment`, `reading_kind` override appended refs.
  `actions` is a `~`-separated sequence of `VOID,factId,operation,target,reason_hex`
  or `SUPERSEDE,factId,operation,target,replacement,reason_hex`.
  `at` selects latest. Output order, `id:derivedState` pairs, original
  `id:value` pairs, latest/latest_value (`?` when absent), count and fact count.
- USAGE: same series/changes as HISTORY, plus required `start`/`end` instants.
  Output kind/unit/status, usage and from/to anchor IDs. Known status is
  OK with empty reason; insufficient status has absent usage and NO_READING_BEFORE,
  preserving any actually available endpoint anchor independently. No
  interpolation or cross-segment computation.

Errors have exact single-field maps: INVALID, SCOPE,
READING_ALREADY_SUPERSEDED, OPERATION_ID_REUSED or METER_ALREADY_IN_SERVICE.
Local invariants additionally prove redaction, copied inputs/snapshots,
unforgeable typed/unit combinations and no mutation on failure. The shared
corpus is not a replacement for those independent adversarial/property cases.

The frozen corpus has261 rows: VALUE99, METER14, OPEN_SET8, READING54,
PRESENCE11, HISTORY56, USAGE19. Initial SHA256:
`578b99a926e5591aea9bcf5ed98d6e7529301c672c51e6353d87c09f3f55147a`.
