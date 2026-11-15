# EMP-DOC-014 — Upload safety checks: type, size and active content

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F3 job task (module DOC, cut C1, P0, size S) on
`codex/enterprise-f3-doc014`, opened from the integrated head with migrations
through `000209` (EMP-DOC-002 `org_documents` is `000205`, EMP-DOC-003
`org_document_links` is `000209`). It adds a retry-safe safety check to the
private document upload flow: verified bytes are inspected and checked over
the stored object before storage may reach `STORED`; refused content gets a
truthful terminal state with one closed reason; interrupted checks and
abandoned upload sessions converge through one organization-scoped job.
Synthetic C1 receipts only. No Kotlin/Android, panel, route, fleet or orgaccess
change.

Sources read: the EMP-DOC-014 task block and the DOC-006/DOC-012 blocks of
[EP09](../backlog/ep09-documents.md); [11-privacy-security](../11-privacy-security.md)
§4 (BR-PRIV-001/003/004/008/009/020); [10-offline-sync](../10-offline-sync.md)
§§4–6 (BR-OFFL-015, UC-OFFL-03, attachment states); the
[approved receipt scope](f0-m0-decisions.md#private-receipts); the evidence of
EMP-DOC-001 ([storage graph](f3-doc001-document-domain.md)), EMP-DOC-002
([persistence, EDR-152/153](f3-doc002-document-persistence.md)), EMP-DOC-003
(links), EMP-DOC-006 ([fiscal structure, EDR-158/159](f3-doc006-fiscal-structure.md))
and EMP-PLAT-006 ([private storage](f1-plat006-private-document-storage.md));
the `orgmedia` service, S3 adapter and JPEG inspector; the community
`evidence/adapters/media` strict JPEG policy; the platform job queue and
dispatcher; the EMP-RPT-001 `org-export-build`/`org-export-cleanup` worker
wiring (EDR-156) as the latest job precedent.

IDs used: B-BR-EMP-251..254, BUC-EMP-128..129, EDR-169..170 (all of them; no
pre-allocated ID is left unused). Migration `000212_*` (append-only; `000210`
and `000211` are reserved by the parallel EMP-FLEET-006 and EMP-AUTH-011
tasks; every test derives the version from one file-name constant and builds
the previous chain as `name < thisMigration`). The guard function
`org_document_guard()` is redefined from its latest body, which is the
`000205` body (`000209` does not redefine it).

## Rules frozen before behaviour

Observed before any change: the ingress runs the EMP-PLAT-006 `orgmedia`
upload synchronously (declared size plus one byte, exact digest, JPEG
inspection, create-only write with read-back) and one transaction then writes
`UPLOADING → RECEIVED → STORED` with one `document.upload.stored` event. Every
refusal, including content that matches the declared digest but is not a
JPEG, leaves `UPLOADING` with no event and no expiry; nothing ever writes
`QUARANTINED` or `FAILED`, the row has no reason column and no job exists for
documents. The JPEG inspector already refuses non-JPEG magic, malformed marker
structure, bytes after the end-of-image marker, pixel caps and undecodable
images, all as one "unsupported" error; it does not look for markup, script,
document, archive or executable signatures inside metadata segments. Content
is delivered only base64-encoded in no-store JSON and orgmedia re-inspects the
original on every read.

Proposed contract section title: "EMP-DOC-014 — Upload safety checks: type,
size and active content".

Rules B-BR-EMP-220, B-BR-EMP-251, B-BR-EMP-252, B-BR-EMP-253, B-BR-EMP-254, BUC-EMP-128, BUC-EMP-129 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-DOC-014 — Upload safety checks: type, size and active content" (frozen before behaviour; text moved verbatim at integration).

### Decision rows

Decisions EDR-169, EDR-170 are recorded in [16-decisions](../16-decisions.md).

### Backlog note


## Delivered surface

- Migration `backend/db/migrations/000212_org_document_upload_safety.sql`
  (additive, transactional; compatibility floor stays `000132`):
  `org_documents.storage_reason` (closed CHECK of five reasons, and a reason
  only in `QUARANTINED`, `FAILED` or `DELETED`); `org_document_reason_allowed`
  (the reason of each terminal transition, compared with the Go table by a
  test); `org_document_upload_abandoned` (`created_at + 24 h 5 min` on the
  database clock); `CREATE OR REPLACE org_document_guard()` from its latest
  (`000205`) body, adding: no reason at insert; a reason is written once, only
  by its own transition, never changed or cleared; `UPLOAD_ABANDONED` only
  after the window; entering `QUARANTINED`/`FAILED` without a reason is
  refused (23514). Pending (`UPLOADING`/`RECEIVED`) rows of the previous
  binary get their `org-document-safety` closure job (forced RLS lifted only
  inside the atomic migration). No grant is seeded.
- `backend/db/queries/document/documents.sql` (+ generated code; `000212` in
  the `document` stanza of `backend/sqlc.yaml`): `SettleStorage`
  (compare-and-set step with reason; replaces the unused `AdvanceStorage`),
  `DocumentSafety`/`DocumentSafetyForUpdate`, `DocumentCreatorAccount`.
- `orgmedia`: `domain.ErrUnsafe`; `adapters/active.go` (active-content
  signatures; metadata segments located by a marker walk) and
  `JPEGInspector` returning `ErrUnsafe` for bytes after the end-of-image
  marker or a signature and `ErrUnsupported` otherwise; `S3.Get` maps an
  object larger than its declaration to `ErrIntegrity` (was an outage);
  `S3.Delete` removes under the organization key and confirms absence; the
  service propagates `ErrUnsafe` (decided only after integrity, nothing
  stored) and refuses to serve unsafe bytes on read.
- `document/application/safety.go`: `Reason` (closed set, `Transition`,
  audit `Code`), `Outcome`, `Subject` (redacted), the `SafetyRepository`,
  `SafetyObjects` and `Safety` ports, the `Checker` (`Converge`: check of
  `RECEIVED`, closure of an abandoned `UPLOADING`, removal for `FAILED`), the
  window constants (`UploadWindow` 24 h, `UploadGrace` 5 min, `CheckDelay`
  1 min) and `ErrRetry`/`ErrNotDue`.
  `service.go`: `Repository` replaces `MarkStored` with `MarkReceived` and
  `Refuse`; `Service.Safety`; `Upload` (integrity refusals pass through;
  content verdicts close the session; verified bytes are received and checked
  inline; `RECEIVED` retries run the check without reading); `Create` mints a
  capability only if it expires by the session close.
- `document/adapters`: `safety.go` (`Store.MarkReceived`, `Refuse`,
  `Subject`, `Settle`; audit vocabulary `document.upload.received`,
  `document.upload.quarantined`, `document.upload.failed` with required closed
  reason codes `content.unsafe`, `content.unsupported`,
  `integrity.mismatch`, `check.exhausted`, `upload.abandoned`); `jobs.go`
  (`SafetyKind` `org-document-safety`, 10 attempts, version-1
  reference-only payload, per-purpose de-duplication `check`/`expiry`,
  strict envelope, static queue errors, `SafetyHandler`); `Store.Create`
  queues the closure job in the creating transaction; `UnavailableObjects`
  gains `Delete`. `adapters/http`: `ErrUnsafe` maps to `VALIDATION_FAILED`.
- Composition: `enterprisesuite.registerDocuments` composes the same
  `Checker` (bucket and JPEG inspector) into the ingress;
  `enterprisesuite.DocumentHandlers` composes the job handler only with a
  configured document bucket; `cmd/worker/documents.go` and `main.go` compose
  it with the enterprise switch and no bucket (as `cmd/api`), so production
  composes none; `org-document-safety` joins the fixed-excluded and serial
  native kinds of `cmd/worker/dispatchers.go`.
- Contract: `contracts/openapi/v1.yaml` descriptions only (declaration
  operation, upload operation and its 200 response, created-document schema):
  the session window, `RECEIVED`, the terminal refusals and retries. No
  schema, route, permission or inventory count changes (routed 20, signed 1,
  not routed 64, normal 106, special 24 unchanged).
- Tests adapted to the amended B-BR-EMP-220 (no assertion weakened): the
  DOC-002 store test now proves the creator-bound, audited, replay-safe
  `RECEIVED` step plus the system `STORED` settlement; the DOC-002 media test
  expects the digest-pinned PDF-like upload to close `FAILED` (revision 3)
  instead of staying pending; fixtures grant `SELECT, INSERT ON job_queue`;
  two DOC-003 link fixtures reach `FAILED` through `RECEIVED` with
  `INTEGRITY_MISMATCH` (a fresh session can no longer be abandoned); the
  worker dispatcher counts include the new excluded kind.

## RED → GREEN → REFACTOR

The exported API was compiled first with stub bodies (`Checker.Converge`,
`Reason.Transition`/`Code`, the store's `MarkReceived`/`Refuse`/`Subject`/
`Settle`, the job envelope and handler, `S3.Delete` answering unavailable,
`Service.Upload` returning not-implemented after the transfer, `Create`
without the session cap, the JPEG inspector without the unsafe verdict) and
migration `000212` held out of the chain.

- Unit RED (`/data/dev/worktrees/f3-logs/doc014-go-red.jsonl`, `-race`,
  document/..., orgmedia/..., cmd/worker, enterprisesuite): 95 top-level, 15
  fail and 80 pass; 616 subtests, 14 fail and 602 pass; 0 skips; longest
  package 2.5 s. The failures are exactly the new checker, service, job,
  inspector, storage and ingress tests. The same log includes `apicontract`,
  which only re-reads the edited OpenAPI descriptions: under the shared
  machine's load (parallel agents' suites, load average about 20) it hit the
  default 10-minute `go test` timeout before finishing (153 of its tests had
  passed); it is proven GREEN in the union below with `-timeout 45m`.
- Integration RED (`doc014-go-red-integration.jsonl`, `-race
  -tags=integration`, document/... and orgmedia/...): 105 top-level, 25 fail
  and 80 pass; 616 subtests, 14 fail and 602 pass; 0 skips. PostGIS tests
  failed on the missing column, function and guard, the S3 tests on the stubs,
  the DOC-002 media test on the now-terminal PDF-like upload. Composed RED
  (`doc014-go-red-integration-http.jsonl`, `organization/adapters -run
  Document`): 6 top-level, 4 fail (the upload/content and three link suites,
  whose fixtures now need the reason column) and 2 pass.
- Unit GREEN (`doc014-go-green.jsonl`, same packages, `-race`): 95/95
  top-level and 616/616 subtests pass, 0 skips.
- Integration GREEN: `doc014-go-green-integration.jsonl` (`-race`, document,
  orgmedia, orgaudit) 126 of 128 top-level and 639/639 subtests pass; the two
  failures were the pre-existing `orgmedia` S3 tests
  `TestPrivateS3OriginalAndMemberBoundAuditedAccess` and
  `TestConcurrentPrivatePUTNeverOverwritesAndDetectsSubstitution`, which
  received `orgmedia: unavailable` from their first private upload while the
  shared machine was at load average 20. They pass alone, in a full
  `orgmedia` rerun (`doc014-go-orgmedia-rerun.jsonl`, `-race`: 33/33
  top-level, 15/15 subtests), and in four further runs concurrent with the
  document suites; a 90-second probe of 54 320 conditional PUT/DELETE/HEAD
  cycles against the bucket while the document safety suites ran saw no
  provider error. The refusal path returns no provider detail by design
  (BR-PRIV-008), so the failing transport call could not be named; the
  document code never addresses another document's key (cross-organization
  and key tests above). Non-race coverage run
  (`doc014-go-integration-cover.jsonl`): 105/105 top-level, 616/616 subtests.
- REFACTOR: the schema test's committed steps were separated from the
  always-rolled-back probe helper (`scoped`), which had silently rolled back
  the valid steps; no production change. Suites re-run green afterwards.
- Mutation sanity (`doc014-mutations.log`; each source restored
  byte-identical, checked by SHA-256): disabling the active-content scan,
  classifying trailing payloads as unsupported, skipping the stored-object
  verdict, removing the capability window, settling without the
  compare-and-set, removing the database window check and keeping failed
  objects are each killed by at least one test.

## Validation

Go 1.27.4 toolchain, staticcheck under Go 1.27.1, sqlc v1.31.1, vacuum;
commands from `backend/` unless noted, with
`GOCACHE=/data/dev/cache/f3-go-build GOTMPDIR=TMPDIR=/data/dev/cache/f3-tmp
STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck`. PostGIS: the owned control
database on `127.0.0.1:32775` through the `orgtest` harness (own child
databases and ordinary logins, dropped afterwards). S3: the coordinator-owned
disposable MinIO (bucket `validation-private`) through the explicit
`ANPFUEL_TEST_STORAGE_*` variables only.

```sh
gofmt -l ./internal ./cmd ./db                                   # empty
go build ./... && go vet ./... && go vet -tags=integration ./...  # clean
go test -count=1 -timeout=45m ./...      # 211 packages ok, 77 without tests; 2642 top-level + 7420 subtests pass, 0 fail, 4 pre-existing skips
go test -race -count=1 -tags=integration -timeout 45m -p 3 -skip '<5 Rust-preparer tests>' ./internal/modules/document/... ./internal/modules/orgmedia/... ./internal/modules/orgaudit/... ./cmd/worker/ ./internal/platform/apicontract/ ./internal/platform/migrate/ ./internal/platform/enterprisesuite/
    # 229 pass, 1 fail (the transient orgmedia S3 test above), 1 pre-existing skip; 1364/1364 subtests; apicontract 656 s
go test -race -count=1 -tags=integration ./internal/modules/orgmedia/...   # rerun: 33/33 top-level, 15/15 subtests
go test -race -count=1 -tags=integration -timeout 45m -run 'Document|AuthorizationMatrix' ./internal/modules/organization/adapters/   # 8/8 top-level, 3005/3005 subtests
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck [-tags=integration] <document, orgmedia, cmd/worker, enterprisesuite, organization/adapters>   # no findings
/home/iiii/go/bin/sqlc generate && /home/iiii/go/bin/sqlc vet && /home/iiii/go/bin/sqlc diff   # clean
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check   # (repo root) 0 errors; 945 warnings / 611 informs, identical to the base spec
python3 docs/enterprise/tools/check_plan.py      # (repo root) tasks 348; rules 545; use cases 167; screens 145; problems 0
git diff --cached --check                         # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'
git diff --cached --binary | gitleaks stdin --redact --no-banner   # no leaks found
```

The five `cmd/worker` tests skipped in the union need the pinned Rust
quality, sanctions and enforcement preparer CLIs that this machine lacks
(`TestEnforcementWorkerNativeDiscoveryCorrectionAckLossCleanupAndDeadBudget`,
`TestNativeFineWorkerDiscoveryDispatchRustAndCorrection`,
`TestFineWorkerCompanyRetirementCleanupAndAcknowledgmentRecovery`,
`TestNativeWorkerRustDirectoryPublicationAndRetirement`,
`TestNativeWorkerDirectoryRevalidation`); no export or document code runs
in them. The full `organization/adapters` package was not re-run: no
authorization wiring changed; its document, link and authorization-matrix
suites (the ones that compose documents) were. The four untagged skips are
the pre-existing generators (`identity/profile`, `enterpriseexport/portalgen`
×2, `quality/domain`).

Coverage (`doc014-go-unit-cover.out` ∪ `doc014-go-integration-cover.out`,
PostGIS and S3): `document/application` 96.0% (`safety.go` 98.5%,
`service.go` 94.1%), `document/adapters/safety.go` 80.6%,
`document/adapters/jobs.go` 89.2%, `orgmedia/adapters` 91.5% (`active.go`
77.8% — the uncovered lines are the conservative fallbacks for layouts the
structure walk has already refused; `jpeg.go` 90.5%, `storage.go` 93.1%),
`orgmedia/application` 93.2%, `orgmedia/domain` 100%, `document/domain`
99.5%. The composed HTTP paths are exercised by the `organization/adapters`
suites above.

The keyword scan matches only the capability/token vocabulary and the
existing synthetic HMAC fixture key (`synthetic-` ×5); no credential, key or
personal data. All identifiers are synthetic `7b0e2a54-…` UUIDs or generated
UUIDv4s; originals are the existing synthetic 4×4 JPEG with synthetic
markers, payloads and signatures. Migration `000212` SHA-256
`96be2b52842e5f410cbc1ea45a68d0db0c9432c424762562d7dfe86c8d211370`. No
dependency, Kotlin, Android, panel, route or permission is added.

## What the tests prove

PostGIS (owned control database, `orgtest` isolated child databases and
ordinary NOBYPASSRLS logins) and the disposable MinIO bucket
(`document/adapters`, `orgmedia/adapters`):

- `TestUploadSafetyClosesRefusedContentAndStoresOnlyCheckedOriginals`
  (BUC-EMP-128, ingress): a JPEG with a trailing archive or an embedded
  script is `QUARANTINED UNSAFE_CONTENT`, PNG and HTML bytes declared as JPEG
  are `FAILED UNSUPPORTED_CONTENT`, each at revision 3 with exactly one
  terminal event (reason code, creating account) and no received or stored
  event; no object exists under the key; a retry is `CONFLICT` and reads no
  byte; a declaration replay mints no capability; content is `NOT_FOUND`.
  Digest-mismatched, oversized and partial bodies leave `UPLOADING` at
  revision 1 with no reason and no object. The clean original becomes
  `STORED` at revision 3 with one received and one stored event (creating
  account), its object exists and is served byte-for-byte.
- `TestSafetyCheckOverStoredObjectsKeepsUnverifiedBytesInaccessible`
  (BUC-EMP-128, job): under `RECEIVED`, partial, substituted and oversized
  stored objects become `FAILED INTEGRITY_MISMATCH` and are removed, a
  digest-matching PDF becomes `FAILED UNSUPPORTED_CONTENT` and is removed, an
  unsafe JPEG becomes `QUARANTINED` and is kept, a clean one `STORED`; none is
  served before or after a refusal; a repeated final job writes nothing; a
  missing object leaves `RECEIVED` with no event and the final attempt
  settles `CHECK_ATTEMPTS_EXHAUSTED` (`check.exhausted`). Organization B's job
  for A's document completes as a no-op, B's scope cannot load, read or delete
  A's key, and A's check ignores a copy planted under B's prefix.
- `TestConcurrentSafetyChecksCommitOneOutcome` (BUC-EMP-129): eight
  concurrent checks (final and non-final mixed) of one received document
  agree and commit exactly one `STORED`, `QUARANTINED` or `FAILED` outcome at
  revision 3 with three events in total per document.
- `TestStorageFailureMidCheckAndRestartServeOnlyVerifiedBytes`
  (BUC-EMP-129): a fault-injecting object store makes the upload answer
  `RECEIVED` (revision 2, object present, no stored event); a fresh process
  (new pools, store, bucket client, orgmedia service and tokens over the same
  database and bucket) serves nothing; the queued check claimed by the real
  `jobs.Dispatcher` fails with the static `ErrRetry` text (job `queued`,
  attempt 1, document unchanged), then the retried claim completes it
  (`done`, attempt 2, `STORED`) and the fresh process serves the exact
  original; an upload retry of another `RECEIVED` document converges it
  without reading bytes; a final attempt over an unavailable store settles
  `CHECK_ATTEMPTS_EXHAUSTED` and removes the object; a failed removal keeps
  `FAILED INTEGRITY_MISMATCH` with the object present and `ErrRetry`, and the
  next attempt removes it with still exactly one failed event.
- `TestAbandonedUploadSessionsCloseOnlyAfterTheirWindow` (B-BR-EMP-254): the
  declaration queues one reference-only closure job due at
  `created_at + 24 h 5 min` (±1 min); an open session is `ErrNotDue` and the
  database refuses a direct abandonment (23514) also one minute before the
  grace ends, when a declaration replay already mints no capability; after
  the grace the real dispatcher closes it `FAILED UPLOAD_ABANDONED` at
  revision 2 (`upload.abandoned`, creating account) and removes the object a
  crashed transfer left; a repeated closure writes nothing and an upload is
  `CONFLICT`.
- `TestSafetySchemaGuardsReasonsAndScope`: the SQL reason table equals the Go
  `Reason.Transition` for all 7×7×6 combinations; entering `FAILED` or
  `QUARANTINED` without a reason, with another transition's reason, an unknown
  or lower-case reason, a reason without a step, on `RECEIVED` or `STORED`,
  changing or clearing a fixed reason, and inserting with a reason are
  refused (23514); `QUARANTINED → DELETED` keeps its reason; every failure
  reason is admitted from `RECEIVED`; an unscoped login updates and sees
  nothing; organization B cannot load or settle A's document; the store does
  not abandon an open session; forced RLS stays on.
- `TestSafetyMigrationPopulatedUpgradeFailureRecoveryAndPreviousCompatibility`:
  from the previous head with pending documents in both organizations, a
  stored one and a legacy `FAILED` one (no reason), a broken `000212` leaves no
  column, function, guard change, job or ledger row and keeps forced RLS; the
  retry applies exactly the version derived from the file-name constant; the
  previous and current chains check compatible; a re-apply is a no-op; all
  previous document and audit data are unchanged and no reason is invented;
  exactly the two pending documents get closure jobs (exact payload, 10
  attempts, due at `created_at + 24 h 5 min`); the floor stays `000132`; the
  upgraded schema runs `MarkReceived` and a reasoned settlement, still admits
  the previous binary's `RECEIVED → STORED` steps without a reason and reads
  the legacy `FAILED` row without one.
- `TestReceivedTransitionIsCreatorBoundAuditedOnceAndReplaySafe`: only the
  creator receives or refuses; one received event; exactly one check job
  (reference-only payload, 10 attempts, due one minute later); replay writes
  nothing; a received document cannot be refused at the ingress; only the
  system settlement stores it (creating account), and a stale settlement
  writes nothing.
- Existing suites (unchanged assertions unless listed under "Tests adapted"):
  DOC-002 upload, content and access-log tests, the composed
  `organization/adapters` document upload/content and link suites, and the
  orgmedia S3 suites.

Unit: `orgmedia/adapters` `TestJPEGInspectorSeparatesUnsafeFromUnsupportedContent`
(18 unsafe variants — trailing archive/document/text, script, upper-case
script, markup, SVG, PHP, entity, `javascript:`, PDF, ZIP, ELF, PE, RAR, 7z
in metadata, `<iframe` and `javascript:` in scan data — are `ErrUnsafe`; 8
inadmissible inputs including a 20000×20000 header are `ErrUnsupported`;
XMP, encoder comments and ICC-like binary metadata pass; bytes never change;
a short signature in scan data is not quarantined) and
`TestS3OversizedReadsAreIntegrityFailuresAndDeleteConfirmsAbsence`;
`orgmedia/application` `TestUnsafeVerdictFollowsIntegrityAndNeverStoresOrServes`;
`document/application` checker tests (verified store, ten refusal
classifications with removal only for `FAILED`, four outage kinds retrying
then exhausting, removal retry, settlement and operation failures, state-only
convergence, lost compare-and-set, closed reasons inside the EMP-DOC-001
graph) and service tests (received-then-checked, content verdicts closing
with their reason, outcome mapping, replays reading no bytes, the capability
window and constants); `document/adapters` job envelope, de-duplication,
final-attempt and static-error tests; `cmd/worker`
`TestDocumentSafetyJobsRunOnTheirOwnSerialSlot`.

## Retained obligations

- Formats: C1 stays JPEG-only. The XML upload inspector (wrapping the
  EMP-DOC-006 server-only parser) and XML admission, PNG and PDF inspectors
  need their producer, declaration type, budget and retention review.
- Quarantined objects are retained isolated; their reviewed deletion
  (`QUARANTINED → DELETED`), and retention of `FAILED` rows, belong to
  EMP-DOC-012. An object written by a transfer that lands after its session's
  closure job has removed the key is never referenced (the key is reachable
  only through a `STORED` row) and is left to that retention workflow.
- `org_document_link_guard` (000209) still admits links to `QUARANTINED`
  documents; the link lists show that storage state. Whether a quarantined
  receipt may stay linked is for EMP-DOC-003/011 to decide.
- The closed reason is persisted and audited but not yet projected in the
  creator's view; the metadata read and panel viewer are EMP-DOC-013/015.
- Android: the capability path and its own session window (offline uploads
  long after the declaration) belong to EMP-DOC-004 with EMP-OFFL-005.
- Production composes no document bucket (API and worker), so bytes stay
  `UNAVAILABLE` and safety jobs wait without charging attempts until the
  provider qualification and private bucket configuration (operator gate).
  Runtime grants: see the README proposal below.
- Real receipts stay legally gated (OQ-04, LV-04, LV-11). Integration:
  LOCAL_DONE only; no push, PR, CI, staging, F3, M3, C1 or release claim.

## Proposed shared-doc changes

1. `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`: move the bullets between
   `CONTRACT:BEGIN`/`CONTRACT:END` verbatim under a new section after
   "EMP-DOC-003 — Links between documents and operations", preceded by
   `## EMP-DOC-014 — Upload safety checks: type, size and active content (2026-11-14)`
   and the evidence link. In B-BR-EMP-220 replace "Only after that does one
   transaction move storage `UPLOADING → RECEIVED → STORED` and record
   `document.upload.stored`. A refused, interrupted, oversized, mismatched or
   wrong-type transfer leaves the document `UPLOADING` with no event; a retry
   after `STORED` returns the stored record and writes nothing." with "Only
   after that does storage move `UPLOADING → RECEIVED`, and only the
   EMP-DOC-014 safety check makes it `STORED` (B-BR-EMP-251..253). A refused,
   interrupted, oversized or mismatched transfer leaves the document
   `UPLOADING` with no event; content refused for bytes that match the
   declaration closes it (B-BR-EMP-252); a retry after `STORED` returns the
   stored record and writes nothing."
2. `docs/enterprise/16-decisions.md`: append the rows between
   `EDR:BEGIN`/`EDR:END` in EDR number order; in EDR-153's Rejected column the
   phrase "marking `FAILED` or `QUARANTINED` without the reviewed safety job"
   stays true (EDR-169 is that reviewed job).
3. `docs/enterprise/backlog/ep09-documents.md` EMP-DOC-014: state `TODO` →
   `LOCAL_DONE` with the note between `BACKLOG:BEGIN`/`BACKLOG:END`.
4. `docs/enterprise/modules/documents.md`, the EMP-DOC-002 paragraph: replace
   "verification and lifecycle jobs retain EMP-DOC-014 ownership." with
   "EMP-DOC-014 adds the upload safety check: verified bytes are `RECEIVED`,
   only the check over the stored object makes them `STORED`, refused content
   is `QUARANTINED`/`FAILED` with a closed reason, and the
   `org-document-safety` job converges interrupted checks and closes sessions
   24 hours after the declaration ([evidence](../evidence/f3-doc014-upload-safety.md))."
5. `docs/enterprise/09-architecture.md` module table, `document` row: add the
   `org-document-safety` job (serial worker slot, composed only with a
   document bucket) and migration `000212`.
6. `backend/README.md`: in "Private original transfers (EMP-PLAT-006)"
   replace "Only the synthetic JPEG inspector is supplied; other format
   inspectors and durable ready/quarantine state stay with
   EMP-DOC-014/EMP-DOC-002." with "Only the synthetic JPEG inspector is
   supplied; it refuses trailing payloads and active content as unsafe
   (EMP-DOC-014). Other format inspectors stay unimplemented." and add after
   "Organization document links (EMP-DOC-003)":

   ```
   ### Document upload safety (EMP-DOC-014)

   Migration212 adds `org_documents.storage_reason` (closed reasons fixed by
   their transition), the 24-hour upload session window and the
   `org-document-safety` closure job of pending rows. The API runtime role
   needs `GRANT SELECT, INSERT ON job_queue` for declarations and received
   uploads, besides the document grants. The worker role needs `GRANT SELECT,
   INSERT, UPDATE ON job_queue`, `GRANT SELECT, UPDATE ON org_documents`
   (UPDATE also for the `FOR UPDATE` row lock) and `GRANT SELECT, INSERT ON
   org_audit_events`, connects as an ordinary NOSUPERUSER/NOBYPASSRLS login
   that owns no RLS table and uses the same private document bucket as the
   API. Neither `cmd/api` nor `cmd/worker` configures that bucket yet, so the
   job is not composed and queued jobs wait without charging attempts.
   ```
7. `docs/enterprise/10-offline-sync.md` §6 needs no change (the attachment
   states already list `QUARANTINED`/`FAILED`); `11-privacy-security.md`
   BR-PRIV-020 needs no change.
