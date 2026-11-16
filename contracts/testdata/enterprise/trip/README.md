# Pure trip oracle v1

Superseded by `trip-v2.psv` (EMP-TRIP-011, EDR-252); the file is kept unchanged
and is no longer consumed.

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

# Trip logbook oracle v1

EMP-TRIP-003 / B-BR-EMP-320..324, BUC-EMP-161..163, EDR-207..209.
`logbook-v1.psv` uses the same header, field, hex and alias grammar as
`trip-v1.psv` and covers the closed logbook vocabulary, the validation of one
logbook event against its trip and the shared timeline order. It does not
cover persistence, intake, authority at receipt, held events, receipts, the
Android producer or any working-time result (EDR-028).

Additional aliases: E1=81, E2=82, E3=83 (logbook events).

## Fixed semantics

- The vocabulary is exactly NOTE, STOP, ARRIVAL and DEPARTURE (supported)
  and FUELING, TOLL, PARKING, INCIDENT, DAMAGE, HANDOVER, PAUSE, RESUME and
  CANCEL (reserved for their owners). Any other token — empty, padded,
  lowercase, mixed case, START, END, FINISH, a homoglyph, a NUL suffix — is
  `INVALID`.
- RECORD checks, first failure first: the scope and the trip's organization
  (`SCOPE`), the event identity and the author (absent `INVALID`, another
  organization `SCOPE`, a non-canonical or nil identity `INVALID`), the type
  (unknown `INVALID`, reserved `UNSUPPORTED_EVENT`), the text (required for
  NOTE; when present nonblank, control-free, at most 280 Unicode scalars,
  otherwise `INVALID`; padding is kept), the declaration (`INVALID`), the
  starting member (`FORBIDDEN`), then time: strictly before the start
  instant `EVENT_BEFORE_START`; on a COMPLETED trip strictly after the finish
  instant `EVENT_AFTER_FINISH`. Equal instants are inside the trip; instants
  compare across zones. Recording never changes the trip.
- ORDER sorts timeline items by original instant; at equal instants START,
  then EVENT by identity, then FINISH. The receipt is never an ordering key
  and the order does not depend on the input order.

## Operations and defaults

Errors are exactly `error=INVALID`, `SCOPE`, `FORBIDDEN`,
`UNSUPPORTED_EVENT`, `EVENT_BEFORE_START` or `EVENT_AFTER_FINISH`.

- TYPE (`value_hex`) returns `type` and `status` (`SUPPORTED` or
  `RESERVED`).
- RECORD builds the trip-v1 START with every default, and FINISH when
  `finished=true` (default `false`); only `at` and `fin_at` override the trip.
  Event defaults: scope A, identity E1 of organization A, author M of
  organization A, `ev_type_hex` "NOTE", `ev_text_hex` "Cliente ausente" (`?`
  is absent, an empty value is the empty text), at 2026-11-14T13:00:00Z,
  entry America/Cuiaba, offset -14400. Overrides: `ev_scope`, `ev_id`,
  `ev_id_org`, `ev_author`, `ev_author_org`, `ev_type_hex`, `ev_text_hex`,
  `ev_at`, `ev_entry`, `ev_offset`; `ev_omit` is a comma list from id,
  author, declaration. Output: `trip`, `id`, `type`, `text_hex`, `author`,
  `at` (canonical UTC), `entry`, `offset`, `trip_state`.
- ORDER (`items`, a comma list of `KIND/ALIAS/INSTANT` with KIND START, EVENT
  or FINISH) returns `order` as `KIND/ALIAS` in timeline order. Consumers also
  sort the reversed input and assert the same order.

The frozen corpus contains 103 cases: TYPE 27, RECORD 66, ORDER 10. SHA256:
`051b1a3a6947d8b7b620be5316ca6747b6f4fed86cde3ce7f7c9eb19171ef614`.

```sh
(cd backend && go test -race -count=1 -run 'Logbook' ./internal/modules/trip/domain)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.trip.EnterpriseTripLogbook*'
```

# Mission oracle v1

Superseded by `mission-v2.psv` (EMP-TRIP-009, EDR-246); the file is kept unchanged
and is no longer consumed.

EMP-TRIP-008 / B-BR-EMP-353..356, BUC-EMP-178..180, EDR-228. `mission-v1.psv`
covers the pure mission: the closed state, command and response vocabularies,
the complete 8×7 transition table, the new mission, the stored invariants of a
restored mission and the assign, respond and cancel commands on a restored
mission with their fixed failure order. It does not cover persistence, the
signed intake, authority, the eligible-assignee check, the availability
warning, structural scope or any runtime producer; those are proven on real
PostGIS.

## Format

Same grammar as `trip-v1.psv`: header `id|operation|args|expected`,
semicolon-separated `key=value` fields split at the first equals sign, text
inputs as lowercase even-length UTF-8 hex, the question mark as fixture-only
absence, no skipped rows. Aliases use the trip-v1 prefix: A=1, B=2
(organizations); V=21 (asset); D=31, D2=32 (employees); M=41, M2=42
(memberships); MI=91 (mission); U=95 (unit). An unknown alias is literal
input; a malformed or non-canonical organization is an empty scope the domain
refuses. Consumers assert the exact hash, the counts, unique IDs, allowed
argument names per operation, complete result maps, that all 56
state/command pairs appear in TRANSITION and that every operation and error
code has a vector.

## Fixed semantics

- States are exactly DRAFT, PLANNED, ASSIGNED, ACCEPTED, DECLINED,
  IN_PROGRESS, COMPLETED and CANCELLED (COMPLETED and CANCELLED terminal);
  commands exactly PLAN, ASSIGN, ACCEPT, DECLINE, START, COMPLETE and CANCEL;
  responses exactly ACCEPT and DECLINE. Any other token is `INVALID`.
- TRANSITION, first failure first: a terminal state `INVALID_TRANSITION`;
  PLAN, START, COMPLETE or an IN_PROGRESS state `UNSUPPORTED_COMMAND`; ACCEPT
  or DECLINE from a state other than ASSIGNED `INVALID_TRANSITION`; ACCEPT →
  ACCEPTED, DECLINE → DECLINED, ASSIGN → ASSIGNED, CANCEL → CANCELLED.
- Titles and reasons are nonblank, free of control characters and at most 280
  Unicode scalars. The window is `[starts, ends)` with whole-microsecond
  bounds and `ends` after `starts`; it has no maximum span.
- CREATE checks the scope (`SCOPE`), then the mission identity, the unit and
  the optional asset (absent `INVALID`, another organization `SCOPE`,
  non-canonical `INVALID`), then the title, then the window; the result is
  DRAFT at revision 1 without assignee or answer.
- RESTORE checks scope, identity, unit, asset, title, window, the state token,
  a positive revision, the assignee (both employee and membership, or
  neither), the answer, then the stored invariants: DRAFT only at revision 1
  and the other states at revision 2 or more; DRAFT and PLANNED without
  assignee or answer; ASSIGNED with an assignee and no answer; ACCEPTED,
  IN_PROGRESS and COMPLETED with the assignee's own ACCEPT without reason;
  DECLINED with the assignee's own DECLINE with a reason; CANCELLED with any
  consistent pair (an answer needs the assignee and must be theirs).
- ASSIGN checks the scope, the new employee and membership, a positive
  expected revision, then `CONFLICT` for a stale revision, then the table,
  then `INVALID_TRANSITION` for the current membership again while ASSIGNED
  or ACCEPTED; the result is ASSIGNED at revision + 1 with the new assignee
  and no answer; unit, asset, title and window are unchanged.
- RESPOND checks the scope, the responding membership, the response token,
  the reason (none for ACCEPT, required for DECLINE), a positive expected
  revision, then `FORBIDDEN` unless the membership is the current assignee's,
  then `CONFLICT` for a stale revision, then the table; the result is ACCEPTED
  or DECLINED at revision + 1 with the answer.
- CANCEL checks the scope, the reason, a positive expected revision, then
  `CONFLICT`, then the table; the result is CANCELLED at revision + 1 keeping
  the assignee and answer.

## Operations and defaults

Errors are exactly `error=INVALID`, `SCOPE`, `FORBIDDEN`, `CONFLICT`,
`INVALID_TRANSITION` or `UNSUPPORTED_COMMAND`.

- STATE, COMMAND and RESPONSE (`value_hex`) return `state` (and `terminal`),
  `command` or `response`.
- TRANSITION (`state;command`, literal tokens) returns the next `state`.
- CREATE defaults: scope A, identity MI, unit U, asset V (`?` none), all of
  organization A, title "Entrega no cliente", window
  2026-11-20T11:00:00Z → 2026-11-20T15:00:00Z. Overrides: `scope`, `id`,
  `id_org`, `unit`, `unit_org`, `asset`, `asset_org`, `title_hex`, `starts`,
  `ends`; `omit` is a comma list from id, unit.
- RESTORE, ASSIGN, RESPOND and CANCEL restore one mission with the CREATE
  content defaults and `state` ASSIGNED, `revision` 2, assignee employee D
  with membership M (`assignee`, `assignee_org`, `member`, `member_org`; `?`
  none), `answer` (`?` none, else ACCEPT or DECLINE), `answer_member`
  (default M), `answer_reason_hex` (`?` none). ASSIGN adds `to` (D2), `to_org`,
  `to_member` (M2), `to_member_org` and `expected` (2); RESPOND adds `by` (M),
  `by_org`, `kind_hex` ("ACCEPT"), `reason_hex` (`?`) and `expected` (2);
  CANCEL adds `reason_hex` ("Cliente cancelou") and `expected` (2).
- Mission output: `id`, `unit`, `asset`, `title_hex`, `starts`, `ends`
  (canonical UTC), `state`, `revision`, `assignee`, `member`, `answer`,
  `answer_member`, `answer_reason_hex` (`?` for absent values). Consumers
  assert that a command never changes its input mission.

The frozen corpus contains 238 cases: STATE 20, COMMAND 15, RESPONSE 8,
TRANSITION 62, CREATE 29, RESTORE 34, ASSIGN 23, RESPOND 32, CANCEL 15.
SHA256: `d591d9e496cb686e24658f59f91896892e323afaa35f1d5ba4bf2c752021c21f`.

```sh
(cd backend && go test -race -count=1 -run 'Mission' ./internal/modules/trip/domain)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.trip.EnterpriseTripMission*'
```

## Device trip operations (`trip-operations-v1.psv`)

EMP-TRIP-005 / B-BR-EMP-375, BUC-EMP-191, EDR-245. The exact operations the
Android producer builds for `trip.start`, `trip.event.record`, `trip.finish`
and `trip.submit` (schema 1). Header `id|operation|args|expected|bytes_hex`;
`args` are `key=value` fields split at the first equals sign (texts and
reasons as lowercase UTF-8 hex; an empty value is the empty text);
`expected` is `OK`, `MALFORMED`, `UNSUPPORTED` or `PROTOCOL`. An `OK` row
carries the exact canonical operation bytes with their payload digest; the
Kotlin builders must produce them and decode them back, and the Go protocol
plus `ParseStart`/`ParseEvent`/`ParseFinish`/`ParseSubmit` must accept them
with the same fields. A `MALFORMED`/`UNSUPPORTED` row carries the canonical
envelope without its digest that a lenient builder would emit: the Kotlin
builder and decoder and the Go intake parser refuse it with that class. A
`PROTOCOL` row is refused by the shared envelope profile itself (both
protocol seals). The fixed envelope is account `…9501`, origin
`https://api.example.invalid`, organization `…9502`, membership `…9503`,
enrollment `…9504` (prefix `7b0e2a54-0000-4000-8000-`), declared at
`2026-11-15T12:00:00.123Z` in `America/Cuiaba` (−14400 s) unless a row
overrides `at`, `zone` or `offset`. 54 rows (START 25, EVENT 17, FINISH 8,
SUBMIT 4), SHA-256
`6c12cb7970bf5184f10a0b77b10cbd78127a059a7d8dc84f432b985d2aaf3551`,
generated by an independent oracle
(`f4-logs/trip005-scripts/gen_trip_operation_vectors.py`).

# Mission oracle v2

EMP-TRIP-009 / B-BR-EMP-381..383, BUC-EMP-192..194, EDR-246, EDR-247.
`mission-v2.psv` supersedes v1 and is the oracle both consumers load. It
contains every v1 row byte for byte except the eight `PLAN` TRANSITION rows,
which follow the new table, and adds the LOAD, PLAN_VALUE and PLAN
operations. Persistence, authority, the unit and cost-centre existence and
lifecycle checks, the database refusals and the planning record stay proven on
real PostGIS. Same grammar and alias prefix as v1, plus U2=96 (destination
unit) and CC=97 (cost centre).

## Changed and added semantics

- TRANSITION: `PLAN` from DRAFT, PLANNED, ASSIGNED or DECLINED → PLANNED;
  from ACCEPTED `INVALID_TRANSITION`; from IN_PROGRESS `UNSUPPORTED_COMMAND`;
  from COMPLETED or CANCELLED `INVALID_TRANSITION`. Every other pair is the
  v1 table.
- LOAD (`value_hex`) returns `load` for exactly NONE, CARGO or PASSENGERS;
  anything else is `INVALID`.
- PLAN_VALUE checks the scope (`SCOPE`), then the destination (exactly one of
  a text or a unit; a unit of another organization `SCOPE`, a non-canonical
  one `INVALID`; the text profile), then the reason, then the optional cost
  centre, then the load token and its only field: NONE has neither
  description nor count, CARGO a description and no count, PASSENGERS a count
  1..999 and no description. Plan texts follow the title profile and refuse a
  standalone CPF-shaped token (11 digits or `NNN.NNN.NNN-NN`, not preceded or
  followed by an ASCII letter or digit) with `INVALID`.
- PLAN checks the mission scope, the plan as PLAN_VALUE, a positive expected
  revision, then `CONFLICT`, then the table; the result is PLANNED at
  revision + 1 without assignee or answer, unit, asset, title and window
  unchanged, plus the plan fields.

## Defaults

- PLAN_VALUE: scope A, `dest_text_hex` "Centro de distribuição Norte"
  (`?` none), `dest_unit` `?` (with `dest_unit_org`), `plan_reason_hex`
  "Entrega de peças de reposição", `project` CC (`?` none, with
  `project_org`), `load` CARGO (literal token), `cargo_hex` "Peças de
  reposição" (`?` none), `passengers` `?` (else a decimal literal).
- PLAN: the stored mission of RESTORE plus the PLAN_VALUE arguments and
  `expected` (2).
- Plan output: `dest_kind` (TEXT or UNIT), `dest_text_hex`, `dest_unit`,
  `plan_reason_hex`, `project`, `load`, `cargo_hex`, `passengers`; PLAN
  returns the mission output followed by the plan output.

The frozen corpus contains 317 cases: STATE 20, COMMAND 15, RESPONSE 8,
TRANSITION 62, CREATE 29, RESTORE 34, ASSIGN 23, RESPOND 32, CANCEL 15,
LOAD 8, PLAN_VALUE 50, PLAN 21. SHA256:
`caf313daaee52b842e374ca091636f79a17b39d14f8728b6efa7f36d6572b95c`.

```sh
(cd backend && go test -race -count=1 -run 'Mission' ./internal/modules/trip/domain)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.trip.EnterpriseTripMission*'
```

# Pure trip oracle v2

EMP-TRIP-011 / B-BR-EMP-390, BUC-EMP-197..198, EDR-252. `trip-v2.psv`
supersedes v1 and is the oracle both consumers load. It contains every v1 row
byte for byte except the eight TRANSITION rows of `IN_PROGRESS` and `PAUSED`,
which follow the new table, and adds the PAUSE, RESUME, CANCEL and HISTORY
operations. Same header, field, hex, alias and START/FINISH fixture grammar
and defaults as v1. Persistence, the signed intake, authority at receipt,
held facts and the database guards stay proven on real PostGIS. Generated by
an independent script (`f4-logs/trip011-scripts/gen_trip_v2.py`) from v1 and
the written rules.

## Changed and added semantics

- TRANSITION: PAUSE `IN_PROGRESS` → `PAUSED`; RESUME `PAUSED` → `IN_PROGRESS`;
  CANCEL from `IN_PROGRESS` or `PAUSED` → `CANCELLED`; FINISH from
  `IN_PROGRESS` or `PAUSED` → `COMPLETED`; START on either, PAUSE on `PAUSED`
  and RESUME on `IN_PROGRESS` `INVALID_TRANSITION`. `PLANNED`, `ASSIGNED`
  and `ACCEPTED` stay `UNSUPPORTED_COMMAND`; terminal states stay
  `INVALID_TRANSITION`.
- A trip has an operational revision: 1 after START, +1 per PAUSE, RESUME,
  CANCEL or FINISH.
- PAUSE, RESUME and CANCEL check, first failure first: the command scope
  (`SCOPE`), the author (absent or malformed `INVALID`, another organization
  `SCOPE`), a present reason (nonblank, control-free, at most 280 Unicode
  scalars, otherwise `INVALID`), a present declaration (`INVALID`), a
  non-positive expected revision (`INVALID`), then the absent parts as
  `error=INCOMPLETE;missing=` from REASON (CANCEL only) and DECLARATION, then
  `FORBIDDEN` for a member other than the one who started the trip, then a
  terminal trip `INVALID_TRANSITION`, then `CONFLICT` for an expected revision
  other than the trip's, then the table (`INVALID_TRANSITION`), then
  `BEFORE_START` for an instant strictly before the start instant (equal is
  accepted). There is no order check between lifecycle instants. A command
  never changes its input trip, identity, asset, driver, start event or
  reporting attribution.
- A CANCELLED trip reports usage `NOT_MEASURED` with the reason `CANCELLED`
  and no finish; a PAUSED trip keeps `NOT_FINISHED`.

## Operations and defaults

Errors are exactly `error=INVALID`, `SCOPE`, `INCOMPLETE` (with `missing`),
`FORBIDDEN`, `INVALID_TRANSITION`, `UNSUPPORTED_COMMAND`,
`FINISH_BEFORE_START`, `CONFLICT` or `BEFORE_START`.

- PAUSE, RESUME and CANCEL build the v1 START (every START argument
  allowed), then apply `history`, a comma list from PAUSE, RESUME, CANCEL and
  FINISH: step `i` (1-based) is declared at `2026-11-14T12:i0:00Z`, entry
  America/Cuiaba, offset -14400, by M of the trip organization at the current
  revision, CANCEL with the reason "Cliente cancelou" and the others without
  reason; a FINISH step uses the v1 FINISH defaults. A failing step is the
  result. The command defaults: `cmd_scope` (the start scope), `by` (M),
  `by_org` (the start organization), `expected` (the revision after the
  history), `reason_hex` ("Cliente cancelou" for CANCEL, `?` otherwise),
  `cmd_at` 2026-11-14T14:00:00Z, `cmd_entry` America/Cuiaba, `cmd_offset`
  -14400, `cmd_omit` (comma list from author, declaration), `again=true`
  applies the same input a second time to the result. Output: every v1
  START/FINISH field plus `revision`, `previous` (the state before the
  command), `command_author`, `command_at` (canonical UTC),
  `command_entry`, `command_offset` and `command_reason_hex` (`?` without
  reason).
- HISTORY (START arguments and `history`) applies the steps and returns every
  v1 field plus `revision` and `steps` (`COMMAND/revision` per applied PAUSE,
  RESUME, CANCEL or FINISH, `?` when none).

Additional consumer assertions: all 35 state/command pairs appear in
TRANSITION, every operation and every error code has a vector.

The frozen corpus contains 297 cases: ENUM 49, TRANSITION 40, STANDING 14,
START 57, FINISH 47, PAUSE 40, RESUME 16, CANCEL 22, HISTORY 12. SHA256:
`43aecf00903fffaff2ab17c3530595c3647e4040953fe2ff5e28b104b0bef41e`.

```sh
(cd backend && go test -race -count=1 -run 'SharedTripVectors' ./internal/modules/trip/domain)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.trip.EnterpriseTripVectorsTest'
```

# Trip handover oracle v1

EMP-TRIP-012 / B-BR-EMP-394/395, EDR-254. `handover-v1.psv` (87 cases:
HANDOVER 47, RESPONSIBLE 12, ACT 28; SHA-256
`5dd11b0510c63fc8081e6c7a4b743e2a5af580a03ec233092cc3573768bd456a`) covers
the pure handover of an open trip and the responsibility it moves. It is
consumed by Go (`trip/domain/handover_vectors_test.go`) and Kotlin
(`EnterpriseTripHandoverVectorsTest`); `trip-v2.psv` and `logbook-v1.psv` stay
unchanged and consumed (a trip without a handover keeps every earlier answer).
Generated by `f4-logs/trip012-scripts/gen_handover_v1.py` from the written
rules, never from either implementation. It does not cover persistence,
authority, receiver eligibility, the server clock or review separation.

## Grammar and defaults

Same four columns and field syntax as `trip-v2.psv`. Every case starts the
default trip-v2 trip (organization A, trip T, author M, asset V, driver D,
start 2026-11-14T12:00:00Z), then applies `history`: a comma list where step
i (1-based) happens at 12:i0Z — `PAUSE`, `RESUME`, `CANCEL` (reason "Cliente
cancelou") and `FINISH` (a stated missing final reading "Sem leitura final")
by the current responsible member, and `H:<member>` a handover to M, M2 or M3
with its employee D, D2 or D3, recorded by M3 at the current revision with the
reason "Troca de turno". Extra aliases: M3=43 and D3=33 (membership and
employee of the same 5e000000 prefix). A history step's failure is the
result.

- HANDOVER defaults: scope A, recorder M3, receiver M2 with employee D2 (each
  `*_org` A), expected the current revision, `reason_hex` of "Troca de
  turno", `at` 2026-11-14T13:00:00Z; `omit` from recorder, receiver,
  receiver_employee, reason, at; `again=true` applies the same input twice.
  Output: state, revision, responsible, start_author, driver, `handovers` as
  `revision:from>to/employee@instant` in order, and the last handover's
  recorder and reason hex.
- RESPONSIBLE: `at` absent is the current responsible member, present the
  member responsible at that instant (`responsible=`).
- ACT: `act` PAUSE, RESUME, CANCEL, FINISH or LOG (a NOTE "Parada") by `by`
  at `act_at` (default 2026-11-14T14:00:00Z) with `expected` (default the
  current revision). Output `state`/`revision` (and `finish_author`), or
  `logged=true;author=`.

## Fixed semantics

The order of the handover is scope and trip, recorder, receiver and
receiving employee (absent `INVALID`, another organization `SCOPE`), present
reason and instant (`INVALID`), expected revision ≥ 1 (`INVALID`), a missing
reason (`INCOMPLETE`, `missing=REASON`), a terminal trip
(`INVALID_TRANSITION`), a stale revision (`CONFLICT`), a recorder who is the
current responsible member or the receiver (`FORBIDDEN`), a receiver who
already is responsible (`INVALID`), then an instant before the previous
handover (`INVALID`; equal is accepted). No order is checked against the
start or lifecycle declarations. A handover keeps state, asset, start driver
and start author and adds one revision. Lifecycle commands and the finish
need the current responsible member; a logbook event needs the member
responsible at its declared instant (the receiver of the latest handover at
or before it). Errors are exactly INVALID, SCOPE, INCOMPLETE, FORBIDDEN,
INVALID_TRANSITION, CONFLICT, EVENT_BEFORE_START and EVENT_AFTER_FINISH;
consumers assert the hash, counts, unique IDs, allowed arguments, every error
code and that a handover never changes its input trip.
