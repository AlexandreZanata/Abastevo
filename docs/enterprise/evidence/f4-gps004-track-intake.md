# EMP-GPS-004 — Track intake: batch endpoint, de-duplication and segment storage

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 task in worktree branch `codex/enterprise-f4-gps004`, built on
the F4 head `230c2960` (main `7ab0b477` + TRIP-009/013 + GPS-001) and, at the
coordinator's request, re-based as one commit on the F4 head `45c3baf3`
(+ EMP-OFFL-005 and EMP-TRIP-011) with the trip pause and cancellation
followed (see "Following EMP-TRIP-011" below), then on `4f9170fc`
(+ EMP-TRIP-007, migration 000237; shared documents only conflicted); the
coordinator integrates it into `codex/enterprise-f4` and owns PROGRESS, the
F4 execution ledger, pushes and the PR. Under the owner decision EDR-271
(legal review declared complete, tracking implemented for real, active per
organization only with its employee notice in force) this task persists
tracking sessions and accepts track batches on the server. Real-device
acceptance stays OWED; no emulator, device or physical phone was used (no
Android code changed). Synthetic data only; no real location.

Rules read: the EMP-GPS-004 block and its neighbours (GPS-001..006, GPS-012)
in [EP07](../backlog/ep07-gps-navigation.md); the whole
[tracking specification](../modules/gps-navigation.md) (BR-GPS-001..040,
§3 entities, §6.1, §8.1 offline/held sessions/concurrency/clock, §10.2
sampling and batch limit, §10.3 batch envelope, §10.10 codes, acceptance 6);
[privacy](../11-privacy-security.md) location rows (BR-PRIV-006/008/009/015);
[permissions](../07-permissions.md) §7.3 (`trip.trip.start` S for every
holder; `gps.track.read`/`gps.location.read_live` footnotes);
[offline sync](../10-offline-sync.md) §10.1/§10.2/§10.3 (BR-OFFL-018 limits,
intake table, tracking row of the offline matrix); 15-validation VAL-27/28;
EDR-015..018, EDR-197..202, EDR-229, EDR-271..276; the
[GPS-001 evidence](f4-gps001-tracking-session-domain.md) and its retained
list; the trip intake evidence and code (TRIP-002/003/005, OFFL-002 store,
`OPERATION_ID_REUSED`, dependency pending, held items), OFFL-009
(`store_late.go`: owners without a held path keep eligible late work
`RETRY`/`UNAVAILABLE`) and OFFL-012 (`store_state.go`, assessed before every
owner), and [PRIV-005](f3-priv005-sensitive-access-log.md). Precedents
copied: EMP-TRIP-003 (`000225` source/complete triggers), EMP-TRIP-008
(owner without held path), EMP-TRIP-013 (populated upgrade with injected
failure), EMP-TRIP-002 (barrier race test).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-428..434, BUC-EMP-217..219, EDR-277..280. All
are used; none remains reserved.

Rules B-BR-EMP-428 (policy and notice in force), B-BR-EMP-429 (session
transitions as signed operations), B-BR-EMP-430 (one session per trip,
insert-only, replayed), B-BR-EMP-431 (bounded batch operation),
B-BR-EMP-432 (only fixes inside the session), B-BR-EMP-433 (one accepted
batch, one effect), B-BR-EMP-434 (raw, scoped, private segments) and the use
cases BUC-EMP-217..219 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-GPS-004 — Track intake: batch endpoint, de-duplication and segment
storage (2026-11-15)", written before any behaviour.

Decisions in [16-decisions](../16-decisions.md), taken unattended as the
recommended routine choices:

- EDR-277: batches are the signed operation `gps.track.batch` through the
  OFFL-002 intake, not a dedicated route; six session operations
  (`gps.session.start|resume|suspend|interrupt|reduce|end`, the §8.1 list
  plus `reduce`) on the entity `tracking_session`; batch identifier =
  operation identifier; bounds 8 KiB and 200 fixes per operation, 64 KiB and
  50 operations per upload request; parametrized SQL in `tracking/adapters`
  (no shared sqlc stanza); server-only codec, Kotlin producer for GPS-003.
- EDR-278: outcomes stay in the closed OFFL-001 receipt vocabulary
  (`OFF` → `REJECTED` `FORBIDDEN`, other domain refusals and >200 fixes →
  `REJECTED` `MALFORMED`, another member → whole-request `FORBIDDEN`, trip or
  session not stored → `RETRY` `DEPENDENCY_PENDING`); counters on the
  segment, not the receipt; trip facts at the declared instant; receipts read
  under `trip.trip.read`; no held form, late holder or follow-up.
- EDR-279: migration 000240 (policy versions, sessions, transitions,
  segments), trip-row lock serialization, trigger re-proofs, deferred
  publication, whole-microsecond instants, rebuild by replay through the
  GPS-001 domain.
- EDR-280: fix instant = declared base + offset (device clock, same as the
  operations it is compared with); boot identity and base elapsed stored for
  GPS-005; source `PHONE`; device = enrollment; duplicate identity = instant.

## Shape frozen before behaviour

- `backend/internal/modules/tracking/application` (new): operation types,
  `EntityType`, bounds (`MaxFixes` 200, `MaxOffsetMillis`, `MaxSequence`,
  `MaxAccuracy`, `MaxSpeed`, `MaxDropped`, flags), errors `ErrMalformed`,
  `ErrPolicyOff`, `ErrUnavailable`, `ErrNotFound`; ports `Authority`
  (the trip intake authority), `Trips` (stored trips), `Store`; values
  `PolicyVersion`, `StoredTransition`, `SessionRecord`, `SessionChange`,
  `SessionInput`/`SessionCommand`, `BatchInput`/`BatchCommand`, `Fix`,
  `Dropped`, `StoredFix`, `Segment`, `SegmentRecord`, `TripBounds` (every
  private value prints redacted); functions `ParseSession`, `ParseBatch`,
  `TripFactsAt`, `Bounds`, `Rebuild`, `Decide`, `Refusal`, `Change`, `Admit`.
- `backend/internal/modules/tracking/adapters` (new): `NewSessionHandlers`
  (six owners), `NewBatchHandler`, `Store` (raw parametrized SQL).
- `backend/db/migrations/000240_organization_tracking_intake.sql` (new,
  additive): `org_tracking_policies`, `org_tracking_sessions`,
  `org_tracking_transitions`, `org_tracking_segments` (forced RLS,
  `REVOKE ALL FROM PUBLIC`, statement triggers refusing UPDATE/DELETE/
  TRUNCATE, composite organization keys to organizations, trips,
  memberships, device enrollments, sync operations and policy versions);
  functions `org_tracking_policy_in_force`, `org_tracking_operation_valid`,
  `org_tracking_precision_at`; BEFORE INSERT guards (scope, trip row lock,
  operation source, policy in force, contiguous ordered transitions with
  terminal `ENDED`, every fix inside a recording period with its precision,
  inside the trip, strictly increasing, new for the session — custom
  `23505` `org_tracking_segments_fix_key`), trigger-derived
  `geometry(MultiPoint,4326)`, and deferred publication triggers (a row
  commits only with its `ACCEPTED` receipt whose entity revision is the
  operation's last sequence or the batch sequence). No asset, unit,
  cost-centre or employee key, so no `org_*_current_usage` change.
- Shared files: `internal/platform/enterprisesuite/sync.go` (registers the
  seven owners and their no-follow-up plans), `contracts/openapi/v1.yaml`
  (seven owner entries, three payload schemas, intake description),
  `internal/platform/apicontract/meters_test.go` (owner count 8 → 15) and the
  new `apicontract/tracking_test.go`, `scripts/check-manifest.txt`,
  `docs/enterprise/10-offline-sync.md` §10.2 (one tracking paragraph).
- No HTTP route, authorization-route inventory, matrix or auditor-walk
  change (the batches use the existing `/sync/operations` route); no Kotlin,
  Android, job, producer or dependency.

## RED

All tests were written against the stubs (application functions returning
`errNotImplemented`, store methods returning a not-implemented error, the
migration as written, the handlers composed over them).

- Application (`go test -race -count=1 -json`), log
  `/data/dev/worktrees/f4-logs/gps004-app-red.jsonl`: 9 top-level, 8
  failing, 70/70 subtests failing (the lifecycle test stops before its
  corruption subtests). The passing top-level test is the redaction guard,
  true before behaviour because the value types already embed the redacted
  base; it is not claimed as RED.
- Contract (`apicontract -run 'Tracking|MeterStandalone'`), log
  `gps004-contract-red.jsonl`: both tests fail (owner count 8 ≠ 15; missing
  tracking owners and schemas).
- PostGIS (`-tags=integration -race -run TestTracking`), logs
  `gps004-db-red.jsonl`/`.stderr`: 7/7 top-level and 4/4 started subtests
  failing on `orgsync: unavailable`. The upgrade test already passed its
  migration rehearsal (injected failure, recovery, replay, previous image,
  ownership) before failing on the first tracking operation, which proves the
  migration as written before any Go behaviour.

## GREEN and REFACTOR

- Application + domain, log `gps004-app-green.jsonl`, coverage
  `gps004-app-green-coverage.out`: application 9/9 top-level, 84/84
  subtests; domain unchanged 7/7 and 359/359.
- PostGIS focused, log `gps004-db-green.jsonl`: 7/7 top-level, 19/19
  subtests under `-race` (before the re-base); on the merged tree
  `gps004-db-merged-focused.jsonl`: 8/8, 19/19.
- REFACTOR: the race proof first asserted a wait on the trip-row query; the
  run showed that the intake's organization operational-write lock
  (`LockSnapshotDBTX` in the device confirmation) already serializes two
  writes of one organization before the owner, so the proof now accepts any
  blocked lock wait of the runtime login (the TRIP-002 precedent); the
  trip-row lock and the segment trigger stay as the owner's own and the
  database's own serialization, the latter proved directly by the schema
  test. staticcheck found the unused stub error (removed).
- Contract: the `apicontract` package passes (union log).
- OpenAPI lint (`vacuum`), logs `gps004-vacuum.log` and
  `gps004-vacuum-base45c3.log`: 0 errors, the same 1 067 warnings and 665
  informs as the F4 head `45c3baf3`.

Named negatives mapped to tests (all real PostGIS through the real signed
intake store unless noted):

- Foreign or missing trip, foreign organization: `TestTrackingIntakeStoresNothingItMustRefuse/foreign-or-missing-trip-pending`
  (batch and start naming a trip not stored here → `RETRY`
  `DEPENDENCY_PENDING`, no claim); another organization's scope reads and
  writes nothing: `TestTrackingSchemaGuardsSessionsAndSegments/other-organization`
  (forced RLS hides every row; an insert for this organization under the
  other scope is `42501`), `TestTrackingStoreReadsBackTheSessionInScopeOnly`.
- Repeated batch: `TestTrackingIntakeRecordsTheSessionAndStoresEachBatchOnce`
  (two byte-exact replays of start and batch, `OPERATION_ID_REUSED` for the
  same identifier with another body, every accepted receipt replays at the
  end); a second batch with a used sequence → `MALFORMED`; an overlapping
  batch stores only its new instant (`duplicate_count` 2).
- Out-of-session fixes: the same test (half-open interruption boundary,
  fixes during `INTERRUPTED` counted `OUTSIDE_SESSION`), unit
  `TestAdmitKeepsOnlyFixesInsideTheSession` (suspension, reduction, mock,
  duplicates, before the trip start, a session that never recorded);
  database refusals in `TestTrackingSchemaGuardsSessionsAndSegments/guards`
  (before the session, wrong precision, repeated instant, an instant already
  stored → `org_tracking_segments_fix_key`).
- A fix after the trip ended: the lifecycle test (fixes at and after the
  finish counted outside; fixes collected before the end and uploaded after
  `ENDED` still stored), `.../after-finish` (the database refuses an instant
  at the finish).
- Personal trip and implicit start: `.../personal-origin-malformed`,
  `.../implicit-boot-start-malformed`; unit refusal table.
- Oversized batch: `.../oversized-batch-malformed` (201 fixes →
  `MALFORMED`), unit `TestParseBatchRefusesEveryMalformedOrOversizedForm`
  (201 fixes; an operation above 8 KiB is refused by the protocol seal with
  `ErrLimit` before any owner); OpenAPI `maxItems: 200`.
- Concurrent batches for one session, both orders:
  `TestTrackingIntakeConcurrentWritesConverge/batches-a-first|b-first`
  (barrier holds the first write under its locks until the second waits on
  a lock; each instant stored once, the later batch counts 2 duplicates,
  both receipts replay); concurrent starts of two sessions for one trip:
  `.../starts-a-first|b-first` (one session, the other `MALFORMED`).
- Policy and notice: `TestTrackingIntakeOpensOnlyUnderAPolicyWithItsNoticeInForce`
  (no version, `OFF`, notice in the future → `REJECTED` `FORBIDDEN` with no
  tracking row; `OPTIONAL` with notice in force opens and keeps revision 3;
  turning it off later keeps the session's batches and transitions working).
- Another member: `.../another-member-forbidden` (an assigned driver of the
  same vehicle → whole-request `FORBIDDEN`, nothing stored).
- Session of another trip and a start before the trip:
  `.../session-of-another-trip-malformed`, `.../start-before-the-trip-malformed`;
  second session for a trip `.../second-session-malformed`.
- Private coordinates: receipts are checked for positions in every accepted
  assertion; SQL refusals are checked for positions
  (`trackingSQLCode`); unit `TestTrackingValuesAreRedacted` (fmt, JSON,
  slog over every private value).
- Insert-only and RLS: `.../insert-only` (UPDATE/DELETE by the runtime login
  and TRUNCATE by the administrator → `55000` on all four tables), forced RLS
  asserted on all four tables; unpublished rows: `.../unpublished` (a row
  for a `REJECTED` batch claim fails at commit).
- Populated upgrade: `TestTrackingPopulatedUpgradeFailureRecoveryAndPreviousImage`
  (completed trip with receipts before 000240, ordinary non-superuser
  migrator, injected `1/0` leaves no table, function or row change, recovery
  applies once, replay no-op, previous image compatible, forced-RLS tables
  owned by the migrator, earlier receipts replay, the ended trip cannot gain
  a session, a new trip is tracked, the remaining embedded chain applies).

## Following EMP-TRIP-011 (pause, resume, cancellation)

The F4 head gained trip pause, resume and cancellation (migration 000233,
`org_trip_transitions`, trip states `PAUSED` and `CANCELLED`). 000240 redefines
no 000233 function. The intake now:

- derives the trip facts at a declared instant from the stored start, every
  lifecycle row and the finish, proving them consistent with the stored state
  (`TripFactsAt`, `timeline`; an inconsistent trip is unavailable);
- before a session command, makes the stored session follow (EMP-GPS-001
  `FOLLOW`) every stored pause, cancellation and finish from its last
  transition up to, not including, the command's instant; the followed rows
  (`TRIP_PAUSED`/`TRIP_COMPLETED`/`TRIP_CANCELLED`, actor `TRIP`) are stored
  with the command, so recording returns only through the driver's resume;
- judges a batch against the same followed periods and the trip's running
  intervals (`TripBounds.Running`): fixes while the trip is paused or at or
  after its cancellation or finish are `OUTSIDE_SESSION`;
- in the database, `org_tracking_trip_running_at` (start, latest lifecycle row,
  finish) bounds a session's opening and every stored fix, and
  `org_tracking_precision_at` applies the same follow rule (a trip pause at or
  after the session's last transition and not after the fix closes its period).

Tests: unit `TestTrackingFollowsTheTripPausesAndCancellation` (facts at each
instant, running intervals, open pause, inconsistent trips, follow at and
before the command instant, fixes of a paused and cancelled trip); PostGIS
`TestTrackingIntakeFollowsTripPauseResumeAndCancellation` (real `trip.pause`,
`trip.resume`, `trip.cancel` owners on the same intake: a device-reported
pause, an unreported pause followed by the server, an interruption refused on
the followed suspended session, the resume storing the followed row, fixes while
paused and after the cancellation outside, the session ended by
`TRIP_CANCELLED`) and `TestTrackingSchemaGuardsSessionsAndSegments/while-paused`
(the database refuses a fix while the trip is paused). Focused log on the
merged tree: `gps004-db-merged-focused.jsonl` (8/8 top-level, 19/19 subtests,
`-race`).

## Validation

All gates passed on the tree `45c3baf3` + this commit; after the last re-base
onto `4f9170fc` the union was rerun on the final EMP-GPS-012 head that
contains this commit (see that evidence). Commands from
`backend/` with toolchain go1.27.4, `GOCACHE=/data/dev/cache/f4-go-build`,
the shared PostGIS (isolated child databases) and a throwaway MinIO project
`f4-gps004-s3` for the storage suites (`down -v` afterwards):

```sh
gofmt -l ./internal ./cmd ./db                                       # empty
go build ./... && go vet ./... && go vet -tags=integration ./...     # clean
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/tracking/... ./internal/platform/enterprisesuite/... ./internal/platform/apicontract/...
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/tracking/... ./internal/modules/organization/adapters/... ./internal/platform/enterprisesuite/...
go test -race -count=1 -json -covermode=atomic -coverprofile=... ./internal/modules/tracking/...
go test -tags=integration -race -count=1 -p 2 -json -timeout=90m ./internal/modules/organization/adapters \
  ./internal/modules/tracking/... ./internal/modules/trip/... ./internal/modules/orgsync/... \
  ./internal/modules/orgaudit/... ./internal/platform/... ./cmd/api/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
python3 docs/enterprise/tools/check_plan.py   # 348 tasks, 545 rules, 167 use cases, 145 screens, problems 0
git diff --cached --check                      # clean
git diff --cached --binary | gitleaks stdin --redact   # no leaks found
```

- Static: `gps004-static-merged.log` (gofmt empty, vet both tags, staticcheck
  plain and integration clean).
- Race/integration union on the re-based tree, logs `gps004-db-union.jsonl`,
  `.stderr`, `gps004-db-union-merged.exit` (0): 34 packages (31 pass, 3 without
  tests), 1 087 top-level and 7 975 named subtests, 0 failures, including the
  `orgmedia`/document/export storage suites against MinIO. An earlier union on
  the pre-re-base tree was stopped when the coordinator asked for the re-base
  (`gps004-db-union-premerge-stopped.jsonl`, no failure before the stop).
- Untagged `go test -count=1 -timeout=45m ./...` ran once, on the EMP-GPS-012
  head that contains this commit (see that evidence).
- sqlc is not affected (no query file or `sqlc.yaml` change; the store uses
  parametrized SQL, EDR-277). The secret grep hits are benign (test variable
  names and the word "tokens" in prose).
- No Kotlin, Android or emulator run: the codec is server-only (EDR-277) and
  the session oracle is unchanged.

## Retained obligations

- Held and late tracking (§8.1): no held form or late holder exists; an
  eligible late tracking operation stays `RETRY`/`UNAVAILABLE` under
  OFFL-009 until a review owner exists, and a session or batch of a member
  whose access ended is not stored as held.
- The tracking policy writer: no route records `org_tracking_policies`
  versions (settings screen SCR-GPS-09 and its owner; no backlog task names
  it yet — a gap for the coordinator); production grants for the four
  tables (runtime `SELECT` on policies, `SELECT, INSERT` on the others) are
  not seeded, as for every earlier enterprise table.
- EMP-GPS-003: the Android producer and buffer, splitting uploads into
  operations of at most 200 fixes and 8 KiB, the per-fix filter; EMP-GPS-002
  binding the session start to the trip screen.
- EMP-GPS-005: monotonic correction from the stored boot identity and base
  elapsed milliseconds, `CLOCK_UNCERTAIN`, gaps, distance, summaries;
  EMP-GPS-011: retention deletes (the tables are insert-only today).
- EMP-TRIP-012/014: handover and supervisor closure (today the session owner
  is the trip's author).
- Eager follow: the followed rows are stored with the next session command,
  not when the trip changes (the trip packages never import tracking,
  EDR-273); batches apply the same rule meanwhile.
- Location reads and their access audit (EMP-GPS-012); live position
  (EMP-GPS-007).
- Real-device acceptance stays OWED (EDR-271). No M4 or F4 acceptance is
  claimed.
