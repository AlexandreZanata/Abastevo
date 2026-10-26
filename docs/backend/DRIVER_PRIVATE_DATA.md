# Private individual-driver datasets — DRV-07/08/09/10/11/13

Status: private storage implemented; hosted HTTP/mobile/signed cloud admission
is disabled until DRV-10–16. No production rollout or import adapter is implied.

The driver module owns earnings, coverage/source identity, immutable typed
revision payloads, linked signed adjustments, request-digest receipts, erasure
tombstones and the per-owner deletion epoch. Migration 000133 is additive after
000132. No refuel/charge/expense record, public trust table or paid grant changes.

## Fields, access and data exit

Money is BRL centavos in constrained integer columns. Unknown gross/fees stay
NULL for inclusive-net summaries. Vehicle/account scope, UTC coverage, IANA
report timezone, platform label, DETAIL/SUMMARY and an opaque SHA256 source key
are retained. No provider credential, raw source identifier, passenger name,
pickup/dropoff address, free text, GPS trace or photo column is admitted.

`EarningsStore` is an internal port, not an authenticated public endpoint.
Every adapter call requires a configured transaction-bound ownership port;
cloudsync implements the account/vehicle query under its own SQL ownership.
The later application/HTTP boundary must additionally enforce signed owner
proof, live professional authority and dataset consent. The adapter is not
registered in the API, and no cloud allowlist or grant is expanded here.

Current records, signed adjustments and original/corrected earning revisions
have owner-scoped keyset pages (1..1000 requested rows, explicit next cursor).
Current-record pages batch adjustment counts/staleness instead of an N+1 query.
These ports provide retained-data read/export building blocks without a
subscription lookup; signed export UI/protocol acceptance remains later work.
Historical payload format is schema version 1 and unsupported versions fail
visibly. Original normalization is retained; private revision bodies are never
logged or exposed anonymously.

## Erasure and stale restore

Record erasure checks revision/epoch, cascades private revisions and adjustments,
and retains only its scoped ID/source/revision tombstone. The same record or
source cannot be silently recreated by an old operation. Owner erasure advances
the dataset epoch and clears private data/receipts/tombstones. Old epoch writes
are refused even if their operation receipt no longer exists. Soft account
deletion performs equivalent erasure; physical deletion cascades all new rows.
Only restricted lifecycle metadata remains after a soft deletion. There is no
raw-GPS inclusion, guest adoption or cross-account restore path.

Signed snapshots/import/export formats, cloud opt-in, restore/erasure epoch
exchange and global retention/compaction policy remain DRV-12/16/28/30. The
presence of these tables does not activate those datasets or certify backup.

## Migration, rollback and operations

000133 creates private tables/indexes and an account erasure trigger; it does
not rewrite old financial rows. Existing binaries ignore the added schema.
Roll back the application while retaining the additive migration/data; do not
run a destructive down migration. A failed migration rolls back its complete
transaction. A subsequent run applies only the missing version; applied
checksums remain immutable. Recovery/duplicate/concurrency/owner/cancel/partial
failure tests use an owned disposable PostGIS container, never staging data.

## DRV-08 shift and distance additions

Migration 000134 adds `driver_shifts`, `driver_shift_gaps`,
`driver_shift_commands`, `driver_distance_observations` and
`driver_distance_commands`. All rows are owner-scoped by account and vehicle;
account physical deletion cascades and the soft-delete trigger removes these
datasets. Signed command admission and cloud registration remain disabled.

`driver_shifts` retains the current state/version, origin, vehicle, IANA
timezone, monotonic boot/elapsed evidence, UTC clock, worked milliseconds and
reviewed milliseconds. `driver_shift_gaps` retains previous/next clock evidence,
a bounded reason code, an explicit reviewed flag and owner-entered work. The
last 50 command receipts per shift are retained; older commands must still pass
the version fence. No coordinate, route point, free text or passenger field is
present.

`driver_distance_observations` retains only an aggregate UTC window, vehicle
segment, basis, optional metres, coverage flags, quality kind/version/reason and
the selected odometer endpoints for ODOMETER rows. The same owner/vehicle/basis
cannot overlap; different bases may overlap solely for reconciliation. No raw
GPS trace, latitude/longitude, import payload or provider credential is stored.
`driver_distance_commands` binds one operation digest to a saved observation.

Read/list ports are private storage building blocks. Signed export, backup,
restore/epoch admission, report snapshots and cloud opt-in remain DRV-11–16/28
obligations; this registry does not claim them complete.

## DRV-09 cost and maintenance additions

Migration 000135 adds versioned `driver_cost_profiles`/items, versioned
`driver_maintenance_schedules`, operation receipts, owner epoch state and
tombstones. Actual cost items reference an existing ledger record/revision
through a transaction-bound ledger port; the driver module never writes
`ledger_records` and stores no second refuel/expense ledger. Zero centavos and
NULL remain distinct; the same holds for maintenance baseline/next-due values.
A maintenance schedule carries the explicit odometer segment/reset context.

Record erasure keeps `(vehicle, version)`/`(schedule, version)` tombstones and
blocks stale recreation. Owner erasure advances the dataset epoch and clears
private profiles/schedules/commands/tombstones. Soft account deletion erases
these rows; physical deletion cascades. Get/list ports are owner-scoped storage
building blocks for later export/report phases.

## Dataset ownership registry

| Dataset | Owner/retention | Record erase | Read/export boundary | Cloud backup |
| --- | --- | --- | --- | --- |
| Earnings | Account; private | Tombstone + owner epoch | Scoped pages/revisions | Excluded |
| Shifts | Account; private | Account soft/physical erase | Current/get only; full-history export remains DRV-11/15 | Excluded |
| Distance | Account; private | Account soft/physical erase | Get/list aggregate observations | Excluded |
| Cost profiles | Account/vehicle; private versions | Tombstone + owner epoch | Get/list versions | Excluded |
| Maintenance | Account/vehicle; private versions | Tombstone + owner epoch | Get/list versions | Excluded |
| Reconciliation | Account; private versions | Owner earnings epoch; deleted with target/owner | Get/list decisions | Excluded |
| Reports | Account/vehicle/window; private snapshots | Account soft/physical erase + 400-day cleanup | Signed build/get; stale refusal | Excluded |
| Local driver state | Owner/origin Room partition | Owner/origin delete; device storage lifecycle | Local-only drafts/records/revisions/commands | Excluded |

The current cloud manifest allowlist remains vehicles/favorites/preferences and
`backupExcluded` keeps professional datasets out. No reader claims complete
movement export or movement record-tombstone coverage in this phase.

## DRV-10 signed command boundary

Migration 000136 adds append-only reconciliation decisions, counterpart records
and exact operation receipts. A decision names the primary/counterpart earning
revisions, platform, window and—for payout reconciliation—an opaque payout
source hash. It shares the earnings owner epoch; owner erasure and soft account
deletion clear decisions/commands. No source identifier, passenger field or raw
payout payload is stored.

`CommandService` requires a verified signed principal (owner/origin/contributor/
key) before accessing storage. Hosted mutations additionally recheck the live
professional capability inside the same transaction through `Store.SetPaid`;
reads/export and erasure remain available after a lapse. The actual identity
verifier, subscription wiring and HTTP/OpenAPI admission remain DRV-11/12.

Decisions are persisted but not yet consumed by a report/import projection;
therefore this phase makes no non-duplicated-total claim. That consumption and
its UI are DRV-11/19.

## DRV-11 report snapshots

Migration 000137 adds owner/vehicle/window/timezone/platform/basis snapshots with
a SHA256 source revision, increasing version and private JSON payload. They are
erased on account soft/physical deletion and by the bounded worker cleanup.
Report generation is Premium (`professional.report`); retained reads remain free
after lapse but refuse a stale source revision. The driver routes are mounted
behind `ANPFUEL_DRIVER_ENABLED` (default false); disabled routes return 404
without storage access. No snapshot is included in cloud backup or public data.

## DRV-13 local Android state

Room schema 29 stores owner/origin-scoped shift, gap, earning, immutable earning
revision, distance, local command and draft rows. Reads always filter the exact
owner+origin; guest is a separate blank-owner partition. Drafts retain integer
metres/centavos/seconds and a unit version, and contain no GPS/location/start
fields. The local command table is not a cloud opt-in or dispatcher; DRV-14 owns
dispatch/replay. Existing ledger/map/vehicle/session tables are untouched by
migration 28→29. Legacy odometer repair remains the existing ledger-draft flow.
