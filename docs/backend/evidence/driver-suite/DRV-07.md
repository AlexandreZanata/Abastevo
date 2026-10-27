# DRV-07 — Private earnings, source identity and revision persistence

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Tested behavior: `7d830547`. Contract: [DRV-03–07](../../../product/DRIVER_03_07_CONTRACT.md).

T01: append-only migration 000133 after actual 000132 adds exact constrained
BRL component columns, coverage/source identity, typed revision history,
adjustments, immutable request-digest receipts, scoped tombstones and owner
erasure epochs. Unknown gross/fees stay NULL. No old ledger rewrite.
T02: concrete private pgx/sqlc adapter and transaction-bound cloudsync ownership
port preserve canonical microsecond UTC/IANA periods, DETAIL/SUMMARY and source
SHA256 identities. No raw provider/passenger/GPS/free-text field. Matching source
retries with different proposed IDs converge to one accepted ID. Signed linked
adjustments survive restart and expose obsolete target revisions for review.
Read/export pages include current facts, adjustments and original revisions;
current-record adjustment stats use one batch query. No HTTP/Premium/cloud
admission is invented; see [private data registry](../../../backend/DRIVER_PRIVATE_DATA.md).
T03: real owned disposable PostGIS, eleven tests under -race, PASS. Empty schema,
132→133 upgrade, actual SQL failure after DDL and transactional recovery, two
migrators once-only, legacy ledger preservation, twelve concurrent source
imports, coverage races, adjacent periods, foreign account/vehicle/revision FK,
body/revision drift, restart, malformed SQL money, rollback, cancellation before
and after writes, live authorization revocation, pagination, erasure epoch and
physical/soft account lifecycle all exercised. Record/source tombstones and old
epoch checks reject stale recreation. Account erasure clears private payloads.

RED `/tmp/drv07-schema-red.log`: actual migrated DB lacked driver schema.
Review RED `/tmp/drv07-money-snapshot-red.log`: caller-owned mutable money changed
across ownership I/O. Normalization now captures owned monetary values before
hashing/awaiting I/O. UTC supported-year validation happens after normalization;
regression covers local year 1 crossing into UTC year 0.
GREEN final exact source: `/tmp/drv07-final-exact.log` (eleven PostGIS tests plus
actual domain vectors); all backend unit tests, vet/staticcheck, SQLC vet/diff,
format, secrets and mobile/security static guards PASS. No new dependency.

Migration recovery is an application rollback retaining additive schema/data,
not a destructive down migration. Private schema admission remains disabled in
HTTP/cloud until DRV-10–16; storage completion does not pass DRIVER_BACKEND_GATE.
The separately measured global Android coverage gate fails on pure dev too;
it is recorded in [batch acceptance](DRV-03-07_ACCEPTANCE.md), not suppressed
or counted as PASS. The new driver package has 93.48% instruction coverage.


No deployment, provider activation or suite/UI release claim. Next DRV-08 (not authorized).
