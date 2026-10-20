# FQ-07R5C — Compatible historical fine storage

Status: LOCAL_DONE
Validation: PASS

Parent a59c61d1, maintained dev. B-BR-FQ-01/02/06/07/14/16;
BUC-FQ-03/04/06. One additive storage prerequisite; no runtime publication.

Before behavior: append migration108. Preserve every legacy and2025 value,
publication key, provenance and current pointer. Keep the canonical sanction event
ledger, add explicit historical reference/transit/unlabelled fields and a nullable
conditional historical-context FK; old rows keep every new field NULL. Historical
revision context freezes the exact six-profile period/name/header/DAY-or-MONTH
clock and metadata count even for empty snapshots, separate from the2025 context.
Public source-note annotations are an exact allowlist linked to that context and
original locator, never additional fines. No invented report year/extraction month
for the2016–2019 group, no AI→DUF relabel, no unknown-paid→zero or due→decision.

SQL independently checks exact money against raw plain/currency text, nullable
paid only for the verified2023 dash form, calendar/range/profile/reference/clock
and field budgets. Staging operations lock/recheck the revision, require the
sanctions stream and matching source period/context, cannot write published or
foreign revisions, and converge unique content without moving current pointers.
Immediate real PostGIS107→108/replay, representative legacy+2025 preservation,
negative/failure/concurrency/observed lock-barrier tests are mandatory, followed
by affected quality/migration/contracts/sqlc/build/static acceptance. Applied
migrations remain unchanged. No dependency, source activation, D or deployment.

Atomic historical publisher/lease/HTTP, native runner/discovery/image and safe
Directory association follow this checkpoint; full runtime history is still owed.


## Delivered and verified, 2026-10-19

Append-only108 keeps every applied migration intact. Historical context freezes
one exact source profile/period/file/header, source DAY or MONTH, original record
count and metadata count. The2025 revision clock table remains unchanged. The
canonical sanction ledger gains nullable historical-context/reference/transit/
unlabelled columns; legacy/2025 shape requires every new field NULL. Historical
rows instead require the context FK, correct original reference label, finite
actual calendar, empty-or-known department and raw source money semantics.
Legacy exact as_of/decision and2025 report-year/month fields remain NULL for
historical rows. No value or current pointer is backfilled/reinterpreted.

SQL money helpers independently normalize the same explicit Unicode whitespace
and optional R$ prefix, then validate decimal grouping and compare exact numeric
minor units to BIGINT. Unknown paid is permitted only for the2023 source's R$-
form with paid_minor NULL; neither zero nor a bare dash is substituted. Paid may
exceed the fine: no balance or legal interpretation is introduced. Source transit
is optional only in the profiles that actually publish that field. Source notes
are an exact context-bound public allowlist; arbitrary/private note content fails.

All three staging ports hold/recheck a shared revision lock, require the sanctions
stream and matching edition/source context, and reject publication/withdrawal or
foreign streams. Original evidence locators are bounded against the exact source
filename/header/record count before event/note writes. Separate source context
retains precision even with no observations. Unique inserts never update existing
facts or move the current pointer. SQLC outputs and model/read scan expansion are
generated from the actual schema, not hand-edited.

## Validation and corrected failure

Real PostGIS source/storage matrix PASS:

- 107→108 and replay preserve byte-equivalent logical legacy+2025 event snapshots,
  revision records, current pointers and native clocks; both old publishers replay
  to the same IDs. All historical columns stay NULL and no history context is
  invented by upgrade. Prior105/98/99/100 upgrade expectations append108 and pass
  their original replay/frozen-history obligations.
- All six source profiles stage successfully, retaining empty department,
  original AI/DUF label, exact NBSP currency text, group DAY versus annual MONTH,
  NULL unknown paid, no invented legacy clock/decision/DUF fields and source notes.
- Nine bad context/profile/count/month cases, five invalid group-day cases,
  twenty-one money/identity/reference/calendar/field/control cases and malformed
  money forms are refused. Tests include infinity/year10000, overflow, unknown
  paid→zero, known paid erased, unsupported transit/unlabelled data, source notes
  with arbitrary/private content, malformed/foreign/header/out-of-range locators.
- Eight concurrent context, observation and note writers each create exactly one
  row. For each of the three write ports, pg_blocking_pids confirms an actual
  revision-row lock wait; withdrawing that revision then releases the waiter,
  which returns no row and writes no facts. Foreign source writes are refused.
- A source context and valid observation followed by an invalid note roll back
  together, leaving zero partial rows/context/notes.

The original concurrent observation test exposed a real two-unique-index race:
arbitrating only (revision_id,native_key) could still raise a primary-key conflict
when simultaneous retries used the same generated ID. Historical and existing
native2025 event inserts now refuse collisions through ON CONFLICT DO NOTHING
without modifying either row. Only a created row returns an ID; no row remains a
refusal/error to the publisher. A separate generated-ID collision against an
existing2025 event proves the publisher rolls back its new revision/context and
preserves the old current pointer. The concurrency test was retained, not weakened.

Final affected critical matrix PASS (`/tmp/fq07r5c-final-critical.log`):
adapters34.864s, HTTP15.448s, jobs15.045s, native1.163s, prepared1.128s,
read9.047s, application1.015s, domain1.018s, migrate5.073s,
apicontract5.892s. Existing actual native file-job/parser/publication/read/HTTP/
privacy/reports/clock/concurrency regressions remain green.

```sh
# backend/, documented trusted CLI env paths and disposable local PostGIS
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/absolute/trusted/prepare_quality ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/absolute/trusted/prepare_sanctions ANPFUEL_TEST_PREPARER_PATH=/absolute/trusted/prepare_registry go test -race -tags=integration ./internal/modules/quality/... ./internal/platform/migrate ./internal/platform/apicontract -count=1
sqlc vet
sqlc generate
```

SQLC vet and whole generated-tree regeneration stability PASS; affected vet,
API/worker compilation and staticcheck PASS (`/tmp/fq07r5c-static.log`).
gofmt/diff/tracked-secret and owned new-file/link review PASS. No dependency,
private source fixture, API contract, native device execution or deployment.
All databases were disposable local PostGIS; migration108 was not applied to VPS.

## Recovery and next

Do not edit or reverse an applied migration. Source imports/schedules remain off;
a prior binary may intentionally refuse readiness against a newer schema. This is
an additive local source checkpoint, not a deployment rollback certification.
No historical publisher/worker currently creates these rows in runtime.

Next FQ-07R5D: atomic typed historical publisher, frozen publication key/source
context and correction/empty/rollback/replay guards, then source-accurate lease/
HTTP/OpenAPI reads and history runner/discovery/image. Safe Directory/station
links, live FQ-06 XLSX, linked Android, FQ-16/17 acceptance and final protected
dev→main delivery/overview remain owed. D stays held. No source activation,
production/native-device certification or full plan closure is claimed.
