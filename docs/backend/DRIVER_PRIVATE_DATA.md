# Private individual-driver datasets — DRV-07/08

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
