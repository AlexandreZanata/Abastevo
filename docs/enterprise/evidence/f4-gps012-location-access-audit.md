# EMP-GPS-012 — Location access permission and access audit

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 API task on `codex/enterprise-f4-gps004`, on top of the
EMP-GPS-004 commit, itself on the F4 head `4f9170fc` (with EMP-OFFL-005,
EMP-TRIP-011 and EMP-TRIP-007). The coordinator integrates it and owns PROGRESS, the F4 ledger,
pushes and the PR. Under EDR-271 the recorded track stored by EMP-GPS-004 is
now readable through one company route, only under `gps.track.read`, with
every read recorded in the PRIV-005 sensitive-access log before any byte is
served. No migration was needed (000241 unused). Synthetic data only; no
emulator, device or physical phone.

Rules read: the EMP-GPS-012 block ([EP07](../backlog/ep07-gps-navigation.md));
BR-GPS-030..035/039/040, UC-GPS-06/07, §7 roles and permissions, SCR-GPS-05/06
of the [tracking specification](../modules/gps-navigation.md);
[permissions](../07-permissions.md) §7.3 (`gps.track.read`
O/O/O/O/–/–/S²/–/O³ and `gps.location.read_live`, footnotes ² own trips only and
³ recorded tracks only, never live) and the v5 catalog requirements
`OWN_TRIPS_ONLY`/`RECORDED_TRACKS_ONLY` (`orgaccess/domain/coverage.go`,
facts `TripDriver`/`RecordedTrack`); BR-AUTH-004/005/011, BR-AUD-003/004,
BR-PRIV-004/008/021 ([privacy](../11-privacy-security.md) location and access
log sections); the [PRIV-005 evidence](f3-priv005-sensitive-access-log.md)
(`AccessRecorder`, `LOCATION_TRACK` issued/accessed actions, the `000202`
row guard that refuses a served record without an issuance); the TRIP-004
review route and `TripReviewAuthority` (structure through `TripStructure`),
the document content route (record before content) and the matrix/AUTH-011
precedents of FLEET-006 and DOC-003.

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-435..437, BUC-EMP-220..221, EDR-281..282. All
are used; migration 000241 is not used.

Rules B-BR-EMP-435 (recorded tracks only under `gps.track.read`),
B-BR-EMP-436 (every read recorded before any byte), B-BR-EMP-437 (the read
serves what was recorded, page by page) and BUC-EMP-220/221 are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-GPS-012 — Location access permission and access audit (2026-11-15)",
written before any behaviour. Decisions in [16-decisions](../16-decisions.md):

- EDR-281: one company route `POST /v1/orgs/{org_id}/trips/{trip_id}/track/read`
  in `tracking/adapters/http` under `gps.track.read` only; the resolver states
  the stored trip's author as own record and driver and the read as a recorded
  track; `TripStructure` also answers `gps.track.read`; a `TrackAuthority`
  re-proves the same facts in the read transaction through the company
  authority; pages of at most 20 segments behind a reader-bound cursor; no live
  route (the live projection is EMP-GPS-007, F5); no migration.
- EDR-282: two access rows per page (`LOCATION_TRACK` `ISSUED`, then `SERVED`
  referencing it), each with a fresh server identifier and a digest of the
  page's identities, after the in-transaction read and before the response;
  any record failure serves nothing.

## Shape frozen before behaviour

- `tracking/application/track.go`: `TrackPermission`, `MaxTrackPage` (20),
  `TrackPolicy` (read, capability `organizations`, not fresh),
  `ErrTrackDisabled`, `ErrTrackUnauthorized`, `ErrAccessUnrecorded`,
  `TrackActor`, `TrackRequest`, `TrackTransition`, `TrackSession`,
  `TrackFix`, `TrackSegment`, `TrackView` (all redacted), `TrackRepository`,
  `TrackService.Read` (validates actor, page and the completed decision before
  the repository).
- `tracking/adapters/track.go`: `AuditDefinitions`
  (`audit.AccessDefinitions(LocationTrack)`), the `TrackAuthority` port,
  `TrackStore` (`TripAuthor` for the resolver; `ReadTrack`: decision check,
  in-transaction authority, session and segment page, then the two access
  records, then the view).
- `tracking/adapters/http/track.go`: `TrackReadPath`, `TrackRecords` (only
  the track policy, only recorded-track facts), the resolver, the closed page
  request, the projection and `RegisterTrack`.
- `organization/adapters/track_authority.go` (new) and two minimal shared
  edits: `company_authority.go` (the track policy in the allow-list and a
  `TrackedTrip` mode that reads the trip's author under the retained locks and
  sets `AuthoredBy`, `TripDriver`, `RecordedTrack`) and
  `resource_structure.go` (`TripStructure` also for `gps.track.read`).
- Wiring: `internal/platform/enterprisesuite/routes.go` (route, authority,
  access log runner, audit definitions).
- Contracts: route inventory (`gps.track.read` ROUTED with its representative;
  the `gps.location.read_live` reason now names EMP-GPS-007), OpenAPI path and
  the schemas `EnterpriseTrackReadRequest`, `EnterpriseTrackView`,
  `EnterpriseTrackSession`, `EnterpriseTrackTransition`,
  `EnterpriseTrackSegment`, `EnterpriseTrackFix`.
- Counts on this base (F4 head `4f9170fc` + GPS-004): normal routes 150 → 151,
  routed 28 → 29, not routed 56 → 55, specials 26 (OFFL-005), signed operation
  owners 18 (8 + TRIP-011's 3 + GPS-004's 7, unchanged here); AUTH-011 auditor
  walk 150/30/120 → 151/31/120; matrix 280 cells 112/168 → 290 cells 117/173.
  (Developed on `45c3baf3`, where the same change read 147 → 148 and
  147/30/117 → 148/31/117; TRIP-007's three `trip.trip.review` routes were
  merged in before the final gates.)

## RED

The tests were written first and run against compiled stubs (policy, actor
and request validation returning zero values, `TrackService.Read`,
`TrackStore.ReadTrack`/`TripAuthor` and `TrackAuthority.CheckTrackTx`
returning errors, `RegisterTrack` mounting nothing, the company-authority and
structure edits not applied, the route not wired), with the inventory,
counts and OpenAPI already changed:

- Application (`-run Track`), log `/data/dev/worktrees/f4-logs/gps012-app-red.jsonl`:
  4 tests, 1 failing (`TestTrackReadPolicyRequestAndDecisionGate`); the three
  redaction tests pass before behaviour (the types embed the redacted base) and
  are not claimed as RED.
- Inventory (`cmd/api -run Authorization`), log `gps012-cmd-red.jsonl`:
  `TestAuthorizationInventoryCoversFullProductionRouter` fails (148 inventoried
  normal routes, 147 mounted, on the development base `45c3baf3`).
- Contract (`apicontract -run TrackRead`), log `gps012-contract-red.jsonl`:
  1/1 failing (the route is not mounted).
- PostGIS (`-run TestTrackRead`, without `-race` because the GPS-004 union was
  running), log `gps012-db-red.jsonl`: 2/2 failing (404 where 200 and 503 are
  expected). The extended matrix and AUTH-011 walk were not run at RED
  (≈15 minutes each); they ran at GREEN in the union.

## GREEN and REFACTOR

The stubs were replaced by the drafted behaviour and the route wired.

- Application + domain under `-race`, log `gps012-app-green.jsonl`: 19
  top-level, 443 subtests pass (tracking/application 12 top-level).
- Inventory and router (`cmd/api -run 'Authorization|Enterprise'`), log
  `gps012-cmd-green.jsonl`, and contract (`apicontract -run
  'Track|MeterStandalone'`), log `gps012-contract-green.jsonl`: all pass after
  adding the shared private-error responses 409/422 that the route validator
  requires on every organization route.
- PostGIS focused, log `gps012-db-green1.jsonl`: 2/2 top-level, 6/6 subtests.
- Union (race, real PostGIS, MinIO): see Validation.
- OpenAPI lint (`vacuum`), logs `gps012-vacuum.log`,
  `gps004-vacuum-final.log`, `gps004-vacuum-base4f91.log`: 0 errors; warnings
  1 074 on the F4 head `4f9170fc`, 1 074 with EMP-GPS-004, 1 080 with this
  task (the new schemas' missing per-property examples and the `trip_id`
  parameter description, the same classes the existing trip routes carry).

Named negatives mapped to tests (actual composed company router, real
PostGIS):

- Missing permission: `TestTrackReadServesOnlyUnderTrackPermissionAndRecordsEveryPage/trip-read-holder-without-track-permission-forbidden`
  (a FINANCE member, who holds `trip.trip.read`, is `FORBIDDEN` naming
  `gps.track.read`; nothing recorded); matrix: every `-` cell of
  `gps.track.read` is `FORBIDDEN` in every variant.
- Failed audit yet bytes served: `TestTrackReadServesNothingWhenTheAccessLogFails`
  (insert on `org_sensitive_access_events` revoked: `UNAVAILABLE`, no
  `segments`/`lat_e7` in the body, no access row; restored: 200 with one
  issued and one served row).
- Staff status alone: `.../staff-status-alone-grants-nothing` (an active
  `support_agent` staff assignment with a FINANCE membership is `FORBIDDEN`;
  the same account after its membership ended is `NOT_FOUND`).
- Foreign organization and unknown trip: `.../foreign-or-unknown-trip-not-found`;
  matrix `other-organization` variant; auditor walk unchanged foreign checks.
- Structural scope not covering the trip: `.../another-unit-supervisor-forbidden`;
  the covering case: the UNIT-scoped supervisor of the trip's unit reads (200),
  and the matrix `unit-scope` variant now expects 200 for the O cells of this
  representative only.
- Driver own trips only: `.../driver-reads-own-trips-only` (a DRIVER reads its
  own untracked trip as an empty page, recorded, and is `FORBIDDEN` on another
  member's trip); matrix: the DRIVER `S:OWN_TRIPS_ONLY` cell is refused on
  another member's trip in every variant.
- AUDITOR recorded tracks, never live: the AUTH-011 walk's positive read of a
  real recorded track (2 segments, one issued and one served row);
  `.../no-live-grant-for-auditor` and the walk assert the catalog denies
  `gps.location.read_live` to AUDITOR; no live route exists (inventory
  `NOT_YET_ROUTED`).
- Every page recorded: the owner's three pages give three issued and three
  served rows; cursors are bound to the reader (another reader's cursor is
  `MALFORMED`); a page above 20 or an unknown request key is `MALFORMED`.
- No coordinates in refusals: every refused response is checked for positions
  (`call` in the read test); access rows carry only identifiers and a digest
  (PRIV-005 contract).

## Validation

All gates pass on the final tree (`4f9170fc` + EMP-GPS-004 `2d53fc84` + this
commit). Commands from `backend/` with toolchain go1.27.4,
`GOCACHE=/data/dev/cache/f4-go-build`, the shared PostGIS (isolated child
databases) and a throwaway MinIO project `f4-gps004-s3` (`down -v`
afterwards):

```sh
gofmt -l ./internal ./cmd ./db                                       # empty
go build ./... && go vet ./... && go vet -tags=integration ./...     # clean
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/tracking/... ./internal/platform/enterprisesuite/... ./internal/platform/apicontract/... ./cmd/api/...
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/tracking/... ./internal/modules/organization/adapters/... ./internal/platform/enterprisesuite/... ./cmd/api/...
go test -tags=integration -race -count=1 -p 2 -json -timeout=90m ./internal/modules/organization/adapters \
  ./internal/modules/tracking/... ./internal/modules/trip/... ./internal/modules/orgsync/... \
  ./internal/modules/orgaudit/... ./internal/modules/orgaccess/... ./internal/platform/... ./cmd/api/...
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
python3 docs/enterprise/tools/check_plan.py   # 348 tasks, 545 rules, 167 use cases, 145 screens, problems 0
git diff --cached --check                      # clean
git diff --cached --binary | gitleaks stdin --redact   # no leaks found
```

- Static: `gps012-static-final.log` (gofmt empty, vet both tags, staticcheck
  plain and integration clean).
- Race/integration union on the final tree, logs `gps012-db-union.jsonl`,
  `.stderr`, `gps012-db-union.exit` (0): 39 packages (35 pass, 4 without
  tests), 1 204 top-level and 9 382 named subtests, 0 failures — including both
  commits' tracking tests, the extended `TestAuthorizationMatrixActualCompanyHTTP`
  (290 cells), `TestAuditorReadWriteMatrixActualCompanyHTTP` (151/31/120), the
  inventory and router tests and the storage suites against MinIO. A union on
  the `45c3baf3` development base also passed (`gps012-db-union-base45c3.jsonl`:
  35 packages, 1 187 top-level, 9 363 subtests, 0 failures) before TRIP-007 was
  merged in.
- Untagged: `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`, log
  `gps012-untagged-all.log` (exit 0): 225 packages ok, 83 without tests, 0
  failures; it covers the EMP-GPS-004 commit too.
- sqlc is not affected (no query file change; the store uses parametrized SQL).
  The secret-grep hits are benign (test fixture session tokens, the synthetic
  cursor secret bytes and redaction-test variable names).
- No Kotlin, Android or emulator run: no app or shared domain change.

## Retained obligations

- Live positions (`gps.location.read_live`, `LOCATION_LIVE`): EMP-GPS-007
  (projection and freshness) and EMP-GPS-008 (live map); the panel replay
  screen SCR-GPS-06 is EMP-GPS-009; the driver's own track read through the
  device channel and the transparency screens are EMP-GPS-015.
- Support-session access to locations (BR-PRIV-021) waits for support
  sessions (EMP-BACK-003); today no staff or support role reaches the route.
- Production grants: the runtime login needs `SELECT, INSERT` on
  `org_sensitive_access_events` and `SELECT` on the tracking tables; not
  seeded, as for every enterprise table.
- Raw-fix retention and expiry on read (EMP-GPS-011); derived summaries
  (EMP-GPS-005).
- The served session is the stored history: a trip pause or cancellation not
  yet followed by a later session command (GPS-004 stores followed rows with
  the next command) is not shown as a session row; stored fixes are already
  bounded by it.
- Real-device acceptance stays OWED (EDR-271); no M4 or F4 acceptance is
  claimed.
