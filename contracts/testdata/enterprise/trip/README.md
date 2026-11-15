# Pure trip oracle v1

EMP-TRIP-001 / B-BR-EMP-268..275, BUC-EMP-136..138, EDR-178..181. This
synthetic oracle covers the pure enterprise trip: the closed operational,
command and review vocabularies, the C1 transition table, the
operational/review standing, the driver-initiated start with its eligible
context and start reporting day, and the operational completion with honest
meter evidence. It does not cover persistence, intake, authority,
assignments, asset availability, the one-active-trip rule, review decisions,
missions, pause, resume, cancellation, handover, supervisor closure, logbook
events, tracking or any runtime producer.

## Format

`trip-v1.psv` uses the exact four-column header `id|operation|args|expected`.
Fields are semicolon-separated `key=value`, split at the first equals sign.
ENUM inputs and reason texts use lowercase even-length UTF-8 hex; other text
inputs are literal. The question mark is fixture-only absence. No blank or
unknown rows are skipped. Consumers assert the exact hash, count, operation
counts, nonempty unique case IDs, distinct field names, allowed argument
names, that all 35 state/command pairs appear in TRANSITION, coverage of
every operation and every error code, and complete result maps. Unknown
syntax fails closed.

Alias UUIDs use prefix `5e000000-0000-4000-8000-` plus a twelve-digit
suffix: A=1, B=2 (organizations); T=11, T2=12 (trips); V=21, V2=22 (assets);
D=31, D2=32 (employees); M=41, M2=42 (memberships); MT=51, MT2=52 (meters);
SG=61, SG2=62 (segments); R1=71, R2=72, R3=73 (readings). An unknown alias
is literal input. A malformed or non-canonical organization becomes an empty
scope that the domain itself refuses. Raw UUIDs prove no existence,
acceptance or authority.

## Fixed semantics

- Operational states are exactly PLANNED, ASSIGNED, ACCEPTED, IN_PROGRESS,
  PAUSED, COMPLETED and CANCELLED; COMPLETED and CANCELLED are terminal.
  Commands are exactly START, FINISH, PAUSE, RESUME and CANCEL. Review
  states are exactly DRAFT, SUBMITTED, IN_REVIEW, APPROVED, RETURNED,
  REJECTED, CANCELLED and VOIDED. Any other token (empty, padded, lowercase,
  APPROVED as an operational state, COMPLETED as a review state, sync words)
  is `INVALID`.
- TRANSITION answers, first failure first: an invalid token `INVALID`; a
  terminal state `INVALID_TRANSITION`; PAUSE, RESUME or CANCEL, or a state
  among PLANNED, ASSIGNED, ACCEPTED and PAUSED, `UNSUPPORTED_COMMAND`;
  IN_PROGRESS + START `INVALID_TRANSITION`; IN_PROGRESS + FINISH `COMPLETED`.
- A standing combines any operational and review state; `completed` is true
  only for COMPLETED and `approved` only for APPROVED.
- START checks the scope (`SCOPE`), the trip identity and the author
  membership (absent `INVALID`, another organization `SCOPE`), then the
  present parts of the eligible context in the order asset, driver,
  departure evidence, declaration (another organization or a reading of
  another asset `SCOPE`; a malformed declaration `INVALID`), then reports
  every absent part as `error=INCOMPLETE;missing=<ordered list>` from ASSET,
  DRIVER, DEPARTURE_EVIDENCE, DECLARATION, then attributes the reporting day
  (unusable zone `INVALID`). The day is the local day of the start instant
  in the unit zone when supplied, else the organization zone; the entry
  zone never decides it and an invalid unit zone never falls back.
- Departure and final evidence are exactly one of a reading (reading
  reference plus an exact value of the meter's kind, below the meter's
  modulus) or a missing reason (nonblank, control-free, at most 280 Unicode
  scalars). The fixture builds the value without a modulus so that the
  domain checks it against the reference.
- FINISH checks the scope (`SCOPE`; an unconstructed trip `INVALID`), its
  actor (absent `INVALID`, another organization `SCOPE`), the present final
  evidence (another organization or asset `SCOPE`; the departure reading
  identity reused `INVALID`) and declaration (`INVALID`), then absent parts
  `error=INCOMPLETE;missing=` from FINAL_EVIDENCE, DECLARATION, then
  `FORBIDDEN` for a member other than the starting member, then the
  transition (`INVALID_TRANSITION` on a completed trip), then
  `FINISH_BEFORE_START` when the finish instant is strictly before the start
  instant (equal is accepted). It never changes identity, organization,
  asset, driver, start event or reporting attribution.
- Usage is `MEASURED` with the meter kind and exact difference only for a
  COMPLETED trip whose two readings share one meter and segment reference
  (meter identity, kind and modulus included) and do not decrease. Otherwise
  it is `NOT_MEASURED` with NOT_FINISHED, READING_ABSENT, DIFFERENT_SEGMENT
  or DECREASING; a decreasing or rolled-over final reading still completes.

## Operations and defaults

Errors are exactly `error=INVALID`, `SCOPE`, `INCOMPLETE` (with `missing`),
`FORBIDDEN`, `INVALID_TRANSITION`, `UNSUPPORTED_COMMAND` or
`FINISH_BEFORE_START`. A rejected constructor has no successful output; a
fixture-built meter or fleet reference keeps its own INVALID/SCOPE meaning.

- ENUM (`kind=STATE|COMMAND|REVIEW;value_hex`) returns `value`; STATE also
  returns `terminal`. An unknown fixture kind fails the loader.
- TRANSITION (`state;command`, literal tokens) returns the next `state`.
- STANDING (`operational;review`) returns `operational`, `review`,
  `completed` and `approved`.
- START builds one trip. Defaults: org A (scope, identity, author, asset,
  driver and departure organizations default to it), id T, author M, asset V,
  driver D, `dep=READING` on meter MT (ODOMETER, no modulus), segment SG,
  reading R1, value 184320000; `dep=ABSENT` uses `dep_reason_hex` (default
  "Odometro ilegivel"); at 2026-11-14T12:00:00Z, entry America/Cuiaba,
  offset -14400, organization zone America/Cuiaba, no unit zone. These are
  explicit synthetic fixture values, not domain defaults. Overrides: `scope`,
  `org`, `id`, `id_org`, `author`, `author_org`, `asset`, `asset_org`,
  `driver`, `driver_org`, `dep`, `dep_org`, `dep_asset` (default asset),
  `dep_meter`, `dep_segment`, `dep_reading`, `dep_kind`, `dep_value_kind`
  (default `dep_kind`), `dep_value`, `dep_modulus`, `dep_reason_hex`, `at`,
  `entry`, `offset`, `zone`, `unit_zone`. `omit` is a comma list from id,
  author, asset, driver, departure, declaration, zone. The fixture builds
  identity, author, asset, driver, departure and declaration in that order;
  the first construction error is the result.
- FINISH builds the START trip, then finishes it. Finish defaults:
  `fin_scope` (the start scope), author M of the start organization,
  `fin=READING` in the start organization on the start asset, meter, segment,
  kind and modulus, reading R2, value 184610000; `fin=ABSENT` uses
  `fin_reason_hex` (default "Sem leitura final"); at 2026-11-14T15:00:00Z,
  entry America/Cuiaba, offset -14400. Overrides: `fin_scope`, `fin_author`,
  `fin_author_org`, `fin`, `fin_org`, `fin_asset`, `fin_meter`,
  `fin_segment`, `fin_reading`, `fin_kind`, `fin_value_kind`, `fin_value`,
  `fin_modulus`, `fin_reason_hex`, `fin_at`, `fin_entry`, `fin_offset`;
  `fin_omit` is a comma list from author, final, declaration; `again=true`
  finishes the completed trip a second time with the same input. Consumers
  assert that a finish never changes its input trip.
- START and FINISH output: `ready`, `org`, `id`, `asset`, `driver`, `state`,
  `start_author`, `start_at` (canonical UTC), `start_entry`, `start_offset`,
  `departure`, `day`, `zone`, `reporting_source` (`organization` or `unit`),
  `finish_author`, `finish_at`, `finish_entry`, `finish_offset`, `final`
  (`?` before finish), `usage`, `usage_reason`, `usage_kind` and
  `usage_delta` (`?` when not applicable). Evidence prints as
  `<reading>:<kind>:<value>` or `ABSENT:<reason hex>`.

The frozen corpus contains 207 cases: ENUM 49, TRANSITION 40, STANDING 14,
START 57, FINISH 47. SHA256:
`068f9dbb8a7a994fdb1c424365d2c45b06655ec01a84b29b874253ea86c6d295`.

```sh
(cd backend && go test -race -count=1 ./internal/modules/trip/domain)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.trip.*'
```

# Trip time-crossing oracle v1

EMP-TRIP-017 / B-BR-EMP-276..279, BUC-EMP-139..141, EDR-182..183.
`trip-time-v1.psv` uses the same header, field, hex, alias and START/FINISH
fixture grammar as `trip-v1.psv` (its defaults included) and covers the
device run clock, the midnight and zone crossing view and elapsed time. It
does not cover persistence, the Android clock source, period locks, paid
hours or any labour result.

## Fixed semantics

- A run clock is a boot identity matching `^[A-Za-z0-9._-]{1,64}$` and a
  non-negative monotonic count in milliseconds; anything else is `INVALID`.
  A run clock is validated after the declaration of its event.
- The trip's day, zone and source are always those of its start. FINISH
  takes no zone. The end day is the local day of the finish instant in the
  stored reporting zone; `crosses_midnight` is true when it differs from the
  start day. `entry_zone_changed` compares the two entry zones and
  `offset_changed` the two device offsets. A finish whose local day in the
  stored zone is outside 0001..9999 is `INVALID` (after
  `FINISH_BEFORE_START`). An unfinished trip prints `?` for every finish and
  crossing field.
- Elapsed time is finish instant minus start instant as `seconds` and
  non-negative `nanos`. Without a run clock on both events it is
  `DECLARED`. With both on one boot, a non-negative monotonic difference
  whose distance from the instant difference is at most the tolerance is
  `CONFIRMED` (the value stays the instant difference); a different boot is
  `GAP` with `BOOT_CHANGED`; a negative monotonic difference or one beyond the
  tolerance is `GAP` with `CLOCK_DISCONTINUITY`. A gap and `NOT_FINISHED`
  carry no value (`?`). A negative tolerance is `INVALID`, checked first.

## Operations and defaults

Errors are exactly `error=INVALID` or `FINISH_BEFORE_START`.

- CLOCK (`boot_hex;elapsed`) returns `boot_hex` and `elapsed`.
- CROSSING (`at`, `entry`, `offset`, `zone`, `unit_zone`, `fin_at`,
  `fin_entry`, `fin_offset`, `finished`) builds the trip-v1 START, and
  FINISH unless `finished=false`, with every other trip-v1 default. Output:
  `id`, `state`, `day`, `zone`, `reporting_source`, `start_at`,
  `start_entry`, `start_offset`, `finish_at`, `finish_entry`,
  `finish_offset`, `end_day`, `crosses_midnight`, `entry_zone_changed`,
  `offset_changed`.
- ELAPSED (`at`, `entry`, `offset`, `zone`, `fin_at`, `fin_entry`,
  `fin_offset`, `boot_hex` with `mono` for the start clock, `fin_boot_hex`
  with `fin_mono` for the finish clock, `tolerance` in milliseconds with the
  fixture value 2000 when absent, `finished`) builds the trip the same way;
  a fixture run clock is built after the zones (start) or the declaration
  (finish). Output: `state`, `status`, `seconds`, `nanos`, `gap`,
  `start_clock`, `finish_clock` (`<boot hex>:<count>` or `?`).

The frozen corpus contains 66 cases: CLOCK 12, CROSSING 17, ELAPSED 37.
SHA256:
`c5054633152e33112d7345b5302bd264ee6a5235f399617397f3971ae45c6a22`.

```sh
(cd backend && go test -race -count=1 -run 'Time|Clock|Elapsed' ./internal/modules/trip/domain)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.trip.EnterpriseTripTime*'
```
