# Tracking session oracle v1

EMP-GPS-001 / B-BR-EMP-421..427, BUC-EMP-214..216, EDR-273..276. This
synthetic oracle covers the pure organization tracking session: the closed
state, permission, policy, origin, cause, actor and quality vocabularies; the
opening of a session for an `IN_PROGRESS` organization trip of its driver;
the start, resume, suspend, interrupt, reduce and end commands with their
typed causes; the trip-follow rule; collection; recorded history and
recording periods; and the track quality classification. It does not cover
persistence, intake, authority, the Android service, buffers, sampling, fixes,
coordinates, gaps, distance, divergence, live positions or any runtime
producer.

## Format

`session-v1.psv` uses the exact four-column header `id|operation|args|expected`.
Fields are semicolon-separated `key=value`, split at the first equals sign.
ENUM inputs use lowercase even-length UTF-8 hex; other inputs are literal. The
question mark is output-only absence. No blank or unknown rows are skipped.
Consumers assert the exact hash, count, operation counts, nonempty unique case
IDs, distinct field names, allowed argument names, coverage of every error
code (and no other), complete result maps, and that a command never changes
its input session. Unknown syntax fails closed.

Alias UUIDs use prefix `9b000000-0000-4000-8000-` plus a twelve-digit suffix:
A=1, B=2 (organizations); S=11, S2=12 (sessions); T=21, T2=22 (trips); M=31,
M2=32 (memberships). An unknown alias is literal input. A malformed or
non-canonical organization becomes a scope that the domain itself refuses.
Instants are whole-minute offsets from `2026-11-15T10:00:00Z` (output uses
the same offsets); `zero` is the unset instant `0001-01-01T00:00:00Z`. Raw
UUIDs prove no existence, acceptance or authority.

## Fixed semantics

- States are exactly NOT_STARTED, RECORDING, SUSPENDED, INTERRUPTED and ENDED
  (ENDED terminal). Permissions: PRECISE, APPROXIMATE, NONE. Policies: OFF,
  OPTIONAL, EXPECTED. Origins: TRIP_SCREEN, NOTIFICATION, PERSONAL, BOOT,
  WORKER, PUSH, GEOFENCE, SERVER. Causes: NOT_STARTED, PERMISSION_DENIED,
  NOTIFICATION_BLOCKED, TRIP_PAUSED, LONG_STATIONARY, PERMISSION_REVOKED,
  PROCESS_STOPPED, DEVICE_RESTARTED, STOPPED_BY_DRIVER, OWNER_CHANGED,
  PRECISION_REDUCED, TRIP_COMPLETED, TRIP_CANCELLED, MEMBERSHIP_ENDED,
  DEVICE_REVOKED. Actors: DRIVER, DEVICE, TRIP, SERVER. Qualities: PRECISE,
  APPROXIMATE, INSUFFICIENT. Any other token (empty, padded, lowercase, trip
  states, Android permission names, fix-level gap reasons) is `INVALID`.
- OPEN, first failure first: scope (`SCOPE`); session identity, trip facts
  (trip, driver, operational state), author, origin, policy and instant
  (absent or malformed `INVALID`, another organization `SCOPE`); the existing
  session (another organization or another trip `SCOPE`); a PERSONAL origin
  `PERSONAL_TRIP`; any origin other than TRIP_SCREEN `IMPLICIT_START`; an
  author who is not the trip's driver `FORBIDDEN`; policy OFF `TRACKING_OFF`;
  a trip not IN_PROGRESS `TRIP_STATE`; an existing session in any state
  `SESSION_EXISTS`. The session opens NOT_STARTED with cause NOT_STARTED and
  owner = author.
- START (from NOT_STARTED) and RESUME (from SUSPENDED or INTERRUPTED): scope,
  the receiver, the trip facts and that they name the session's trip
  (`SCOPE`), author, origin, permission and instant (`INVALID`), the origin as
  above, `FORBIDDEN` when the author or the trip's current driver is not the
  owner, `INVALID_TRANSITION` from any other state, `TRIP_STATE` when the trip
  is not IN_PROGRESS, `OUT_OF_ORDER` for an instant before the last entry
  (equal accepted). Permission NONE keeps the state with PERMISSION_DENIED; a
  hidden notification keeps the state with NOTIFICATION_BLOCKED; otherwise
  RECORDING with the permission as precision.
- SUSPEND, INTERRUPT, REDUCE and END: scope, receiver, trip facts, cause and
  instant; a cause of another command `INVALID`; for STOPPED_BY_DRIVER only,
  the author (absent `INVALID`, another organization `SCOPE`, not the owner
  `FORBIDDEN`), otherwise the author is not consulted; then the from-state
  (SUSPEND and INTERRUPT from RECORDING, REDUCE from RECORDING with PRECISE
  precision, END from any non-ENDED state; else `INVALID_TRANSITION`); then
  the trip state of the cause (TRIP_PAUSED needs PAUSED; LONG_STATIONARY,
  interruption causes and PRECISION_REDUCED need IN_PROGRESS; TRIP_COMPLETED
  needs COMPLETED; TRIP_CANCELLED needs CANCELLED; MEMBERSHIP_ENDED and
  DEVICE_REVOKED any started state; else `TRIP_STATE`); then `OUT_OF_ORDER`.
  Actors: TRIP_PAUSED, TRIP_COMPLETED, TRIP_CANCELLED → TRIP; LONG_STATIONARY,
  PRECISION_REDUCED and the device interruption causes → DEVICE;
  STOPPED_BY_DRIVER → DRIVER; MEMBERSHIP_ENDED, DEVICE_REVOKED → SERVER.
- FOLLOW never answers `INVALID_TRANSITION`: IN_PROGRESS changes nothing;
  PAUSED suspends a RECORDING session (TRIP_PAUSED) and otherwise changes
  nothing; COMPLETED or CANCELLED end any non-ENDED session; PLANNED, ASSIGNED
  and ACCEPTED are `TRIP_STATE`. The instant must be set even when nothing
  changes; its order is checked only when an entry is recorded.
- COLLECT is true only while the bound trip is IN_PROGRESS, still driven by
  the owner, and the session is RECORDING; it returns the current precision.
- QUALITY: coverage = floor((expected − gap) × 1000 / expected); INSUFFICIENT
  with zero expected time (coverage `?`), fewer than two fixes or coverage
  below 500; APPROXIMATE with any coarse period or coverage below 900;
  otherwise PRECISE. Negative values, gap above expected or expected above
  10¹² are `INVALID`.

## Operations and defaults

Errors are exactly `error=INVALID`, `SCOPE`, `FORBIDDEN`, `TRACKING_OFF`,
`PERSONAL_TRIP`, `IMPLICIT_START`, `SESSION_EXISTS`, `INVALID_TRANSITION`,
`TRIP_STATE` or `OUT_OF_ORDER`. Fixture-built trip references keep their own
INVALID/SCOPE meaning.

- ENUM (`kind=STATE|PERMISSION|POLICY|ORIGIN|CAUSE|ACTOR|QUALITY;value_hex`)
  returns `value`; STATE also returns `terminal`.
- OPEN defaults: org A (scope, session, trip, driver and author organizations
  default to it), session S, trip T, driver M, trip state IN_PROGRESS, author
  M, origin TRIP_SCREEN, policy EXPECTED, at 0, `existing=none`. `existing`
  is `none`, `open` (S2 opened for trip T), `ended` (S2 opened then ended with
  MEMBERSHIP_ENDED at 0), `other_trip` (S2 for T2) or `foreign` (S2 in org
  B). `omit` is a comma list from session, trip, driver, state, author,
  origin, policy, at. The fixture builds the existing session, session
  identity, trip, driver, trip state, author, origin, policy and instant in
  that order; the first construction error is the result.
- Command operations (START, RESUME, SUSPEND, INTERRUPT, REDUCE, END, FOLLOW,
  COLLECT) first open the default session at minute 0 and apply `prior`
  steps (`CMD` or `CMD:ARG`, at minutes 10, 20, ...; START/RESUME take
  PRECISE, APPROXIMATE, NONE or BLOCKED (precise, notification hidden);
  SUSPEND/INTERRUPT/END take a cause; FOLLOW takes a trip state; REDUCE takes
  none); a failing prior step fails the loader. The final command defaults:
  scope A, trip T of A, driver M of A, author M of A, origin TRIP_SCREEN,
  permission PRECISE, notification true, at 100, cause TRIP_PAUSED
  (SUSPEND), PROCESS_STOPPED (INTERRUPT), PRECISION_REDUCED (REDUCE),
  TRIP_COMPLETED (END); the trip state defaults to PAUSED for SUSPEND with
  TRIP_PAUSED, COMPLETED or CANCELLED for END with TRIP_COMPLETED or
  TRIP_CANCELLED, otherwise IN_PROGRESS (prior steps use the same rule).
  `omit` lists absent parts. Inputs are built trip, driver, trip state,
  author, origin, permission (actions) or cause (events), then instant.
- Session output: `org`, `id`, `trip`, `owner`, `policy`, `state`, `cause`,
  `precision` (`?` unless RECORDING), `coarse`, `history` (comma list
  `STATE:CAUSE:ACTOR:PERMISSION@minute`, `?` for absent cause or permission)
  and `periods` (comma list `start~end~PRECISION`, `?` for an open end or no
  period). COLLECT outputs `collecting` and `precision`. QUALITY (`expected`
  20400, `gap` 1020, `fixes` 2160, `coarse` false) outputs `quality` and
  `coverage`.

The frozen corpus contains 359 cases: ENUM 64, OPEN 55, START 53, RESUME 30,
SUSPEND 23, INTERRUPT 28, REDUCE 13, END 23, FOLLOW 22, COLLECT 22, QUALITY 26.
SHA256: `4ddac666121fe1183e067dd65589a57c3f97a82e230e3091454ce9b889f9d39e`.

```sh
(cd backend && go test -race -count=1 ./internal/modules/tracking/domain)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.tracking.*'
```
