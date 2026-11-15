# EMP-RPT-001 — Asynchronous export infrastructure: job, private file and expiring link

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 task (module RPT, priority P1, type JOB, size M) in worktree
branch `codex/enterprise-f3-rpt001`; the coordinator integrates it into
`codex/enterprise-f3`. It delivers the export job lifecycle (request,
worker, private file, expiry and audited requester-bound download) that the
export formats of EMP-RPT-002 and the export permission matrix of
EMP-AUTH-011 plug into. It defines no real export format.

Sources read: the EMP-RPT-001, EMP-RPT-002 and EMP-AUTH-011 task blocks;
[intelligence](../modules/intelligence.md) §§4–6, 8 and 10 (BR-RPT-001/002/
004/005/009/010, UC-RPT-01, the export job lifecycle, §10.10 formats and
§10.11 error codes); the Export job row of [08-state-model](../08-state-model.md);
[09-architecture](../09-architecture.md) §§7, 7.1, 9 and 9.1 (jobs, queue,
storage layout, EMP-PLAT-006); [11-privacy-security](../11-privacy-security.md)
§4 (BR-PRIV-001/002/003/004/008/009); the `rpt.export.run` row of the v4
authorization catalog and the fresh-proof list of `orgaccess`; the job queue
and dispatchers (`platform/jobs`, `cmd/worker`); `orgevent`; `orgmedia`
(S3 adapter and capability tokens); `orgaudit` `AccessRecorder` and migration
000202 (EMP-PRIV-005); the EMP-APPR-006 route, authority and inventory
precedent.

Out of scope: real export formats and their scoped data readers (EMP-RPT-002),
the auditor/export matrix (EMP-AUTH-011), inline small CSV in the request,
cancellation, PDF, schedules, the full organization export, notifications,
organization-configurable retention, panel screens and Android.

## Rules frozen before behaviour

Pre-allocated identifiers: B-BR-EMP-227..230, BUC-EMP-116..117,
EDR-156..157. All are used; none remains reserved.

Proposed contract section title: "EMP-RPT-001 — Asynchronous export
infrastructure: job, private file and expiring link".

Rules B-BR-EMP-227, B-BR-EMP-228, B-BR-EMP-229, B-BR-EMP-230, BUC-EMP-116, BUC-EMP-117 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-RPT-001 — Asynchronous export infrastructure: job, private file and expiring link" (frozen before behaviour; text moved verbatim at integration).

### Decision rows

Decisions EDR-156, EDR-157 are recorded in [16-decisions](../16-decisions.md).

## Delivered surface

- Pure domain `backend/internal/modules/orgexport/domain/export.go`: closed
  `State` lifecycle and `CanTransition` (`RUNNING`→`QUEUED` only as a lease
  release), closed `FailureCode`, ceilings and defaults (500 000 rows,
  16 MiB, 1–120 s, retention 1–30 days with default 7, three attempts, three
  open jobs, 4 096 parameter bytes, 5-minute capability), `ValidKind`,
  `ValidMediaType`, `Limits.Validate`, lossless `CanonicalParameters` (sorted
  lower-snake keys, exact number text, no null, duplicate key, lone surrogate,
  invalid UTF-8 or trailing input, depth ≤ 16) and the organization- and
  kind-bound `ParametersDigest`; the bounded `Sink` (rows and bytes counted,
  poisoned after any refusal, `Result` never truncated); `Object` with
  `Key`/`Prefix` (`org/<org>/export/<export>/<sha256>`) and exact `Check`.
  No I/O, clock or authority.
- Application `backend/internal/modules/orgexport/application`: `Policy`
  (fresh `rpt.export.run`, request `ConfigurationWrite`, reads `Read`), the
  `Authority` port (company `CheckExportTx` and session-free
  `CheckRequesterTx` with the kind), the closed `Registry` of `Format` ports,
  `Objects`, `Tokens`, validated `Command`/`View`/`Filter`/`Key`/`Page`, the
  `Repository` and `WorkerStore` ports, `Service` (request, read, list,
  access, content; every call binds the completed request decision before
  and after the repository; access records `ISSUED` before minting; content
  verifies, reads, verifies the bytes, records `SERVED`, re-checks) and
  `Worker` (`Build`: claim → requester re-check → format under the time
  ceiling → provisional → create-only upload with read-back → publish; lease
  release and bounded retry; `Cleanup`: expire at expiry, delete with
  confirmed absence, then record).
- Persistence: migration `backend/db/migrations/000207_org_export_jobs.sql`
  (`org_export_jobs`, forced RLS with USING/WITH CHECK, `REVOKE ALL FROM
  PUBLIC`, retained rows, deferred FK to the request audit event, state-shape
  CHECKs, insert guard requiring the requester's ACTIVE membership, transition
  guard with immutable identity/ceilings, server-owned instants, claim only
  after lease expiry, publish/provisional only under a live lease and only of
  the recorded object, expiry only when due, and a single deletion mark);
  sqlc package `backend/db/queries/orgexport` (new `sqlc.yaml` stanza).
- Adapters `backend/internal/modules/orgexport/adapters`: `Store`
  (repository and worker store; a per-requester transaction advisory lock
  serializes replay and the open-job limit; claim `FOR UPDATE SKIP LOCKED`;
  every worker write fenced by the lease; build/cleanup enqueued in the same
  transaction), `S3` (existing SigV4 core, `PresignServerPUT`,
  `PutIfAbsent`, bounded `Get`, `DeleteConfirmed`; HTTPS except loopback
  fixtures; redacted formatting) and `Unconfigured`, `Tokens`
  (`httpapi.Seal/Open`, binding domain `orgexport.access.v1`, strict
  5-minute inner deadline), `BuildHandler`/`CleanupHandler` (exact version-1
  reference payload, static queue errors), `AuditDefinitions`
  (`rpt.export.requested` plus the EXPORT_FILE vocabulary).
- HTTP `backend/internal/modules/orgexport/adapters/http/exports.go`: five
  guarded CompanyOnly routes under `/v1/orgs/{org_id}/reports/exports`
  (`request` 202, `list`, `{export_id}/read`, `{export_id}/access`,
  `{export_id}/content`), strict DTOs, keyset cursor bound to filter and
  caller, `Mapped` refusals, projections without requester identity, lease,
  key or URL.
- Authority: `backend/internal/modules/organization/adapters/export_authority.go`
  (`ExportAuthority`) and the two export policies in the fixed whitelist of
  `company_authority.go`.
- Composition: `internal/platform/enterprisesuite/{exports.go,routes.go}`
  (routes always mounted with the suite; storage from `Options.ExportStorage`
  or `Unconfigured`; formats from `Options.ExportFormats`, none in
  production; capability key derived from the cursor secret by HMAC; quota
  class `rpt.export`; audit vocabulary), `ExportHandlers`; `cmd/api/main.go`
  passes the existing private bucket; `cmd/worker` composes the handlers when
  the enterprise switch is on and storage is configured, on two dedicated
  serial slots, and always excludes both kinds from the general dispatcher.
- Registry and contract: `orgapi` codes `EXPORT_QUEUE_FULL` 429,
  `EXPORT_NOT_READY` 409, `EXPORT_EXPIRED` 410, `EXPORT_FORMAT_UNAVAILABLE`
  422 (declared with the stubs, mapped in GREEN); five operations, tag
  `enterprise-reports`, eleven schemas and the four codes in
  `contracts/openapi/v1.yaml`; `authorization-routes-v1.json` routes
  `rpt.export.run` (18 routed, 66 not yet routed, 98 normal routes); the
  inventory counts, full-router mounted list, worker slot tests, the
  authorization matrix (new representative `rpt.export.run` list; private
  state snapshot extended to `org_export_jobs` and access records) and
  `scripts/check-manifest.txt` (four quick packages).
- No Kotlin mirror, no shared vector corpus: export jobs are server-side;
  the oracles are the Go tables and the real PostGIS/S3 tests. No `orgmedia`,
  `document`, fleet or notification change.

## Design notes

- Module name `orgexport`, distinct from the public-portal `enterpriseexport`.
  `orgmedia` is not changed: its object and token ports are bound to the
  document identity, so `orgexport` reuses the same exported primitives
  (`evidence/adapters/storage` SigV4 core and `httpapi.Seal/Open`) with its
  own key namespace and binding domain.
- Fencing. The queue already gives one SKIP LOCKED holder per queue lease;
  the export row adds its own lease so a superseded queue holder cannot
  publish. Claim increments `attempts` and takes a 5-minute lease (longer
  than the 120 s ceiling plus two 20 s transfers); a re-claim is allowed only
  after expiry; provisional, publish, release and fail match the lease token.
  A concurrent claimer gets `ErrBusy` and the queue retries; a READY or
  terminal job is `ErrSettled` and the queue job completes.
- Provisional object protocol. Content is deterministic for the same
  parameters and "as of" instant, so the digest-named key converges across
  re-runs. Before recording a new provisional object, a different earlier one
  is deleted (confirmed absence). A create-only PUT that meets an existing
  key is accepted only after the read-back matches; a mismatching stored copy
  is deleted before the retry, so a partial upload can never block or be
  served. `READY` requires the recorded object under a live lease (guard).
- Retries. The export row owns the budget (three attempts); the queue
  allows six build and ten cleanup attempts so the row can always settle
  first. A transient failure releases the lease (`RUNNING`→`QUEUED`) unless
  it was the last attempt, which ends `FAILED`/`EXPORT_ATTEMPTS_EXHAUSTED`. A
  lease that expired on the last attempt is settled the same way at the next
  claim. Worker shutdown counts as a transient failure of that attempt.
- Time ceiling. Only the ceiling's own deadline is `EXPORT_TOO_LARGE`;
  cancellation of the worker context is retryable.
- Expiry and deletion rule. Publishing enqueues `org-export-cleanup` with
  `not_before` = expiry in the same transaction. Cleanup locks the row: a
  READY job past expiry becomes `EXPIRED`; a FAILED or EXPIRED job with an
  undeleted object has it deleted with confirmed absence, and only then is
  `object_deleted_at` recorded. Read-time checks refuse an expired READY
  job before the cleanup runs. A dead cleanup job leaves the object private
  under its organization prefix until an operator replays it (retained).
- Requests serialize per requester with a transaction advisory lock (no
  table privilege needed), so concurrent duplicates converge on one job and
  concurrent different requests cannot pass the open-job limit; the company
  pipeline's account lock adds the same ordering in production.
- Delivery stays inside the guarded JSON gateway (route gate: POST, JSON,
  no-store), so content is base64 and buffered; this is why the byte ceiling
  is 16 MiB (EDR-156/157).
- Clarifications made while implementing (no rule was weakened): request and
  worker re-check require an `ACTIVE` organization (the frozen text said
  "accepts writes/work"); the media type is fixed at request, not recorded
  provisionally; a mismatching stored copy is deleted before the retry; a
  lease expired on the last attempt also ends `EXPORT_ATTEMPTS_EXHAUSTED`;
  both queue kinds have dedicated slots.

## Negative cases → tests

| Negative case | Test (real PostGIS 3.6 / disposable MinIO unless noted) |
|---|---|
| Wrong requester (another member / organization) | `TestExportReadsAreRequesterBoundAndIsolated` (read, deliverable, list, borrowed membership, foreign organization: `NOT_FOUND`); `TestExportLifecycleOnRealStorage` (colleague access/content with the requester's capability and the other organization: `NOT_FOUND`, nothing recorded); `TestExportsThroughActualCompanyHTTPAndWorker` (HTTP 404 for a colleague and the other organization, DRIVER 403 `rpt.export.run`); unit `TestCapabilityIsRequesterAndFileBoundAndShort` |
| Permission revoked before processing | `TestExportsThroughActualCompanyHTTPAndWorker` (FINANCE role ended, then membership suspended, before the dispatcher runs: `FAILED`/`EXPORT_RIGHTS_LOST`, no object); `TestRightsLossAndCeilingsPublishNothing`; unit `TestBuildFailsLoudlyWithoutPublishing` |
| Incomplete file served (storage failure mid-publish) | `TestIncompleteUploadIsNeverServed` (a truncated body lands at the exact key: not READY, corrupt object deleted, access `EXPORT_NOT_READY`, queue error static; outage retried; healthy retry publishes the complete bytes; three partial attempts end `EXPORT_ATTEMPTS_EXHAUSTED` and cleanup deletes the provisional object) |
| Expired link / capability | unit `TestCapabilityIsRequesterAndFileBoundAndShort` (5-minute boundary, future issuance, other member/account/org/job/file, tampered, oversized, other origin); HTTP tampered capability 404 |
| Expired job not downloadable | `TestExportLifecycleOnRealStorage` (expiry reached: access and content `EXPORT_EXPIRED` before cleanup; cleanup → `EXPIRED`, object absent, metadata kept, still refused, nothing recorded) |
| Unbounded work truncates | `TestRightsLossAndCeilingsPublishNothing` (51 rows over a 50-row ceiling: `FAILED`/`EXPORT_TOO_LARGE`, no object, no cleanup); unit sink and time-ceiling tests |
| Duplicate request (same key) | `TestExportRequestIsAuditedEnqueuedAndIdempotent` (replay returns the job; changed command or another member `OPERATION_ID_REUSED`; 8 concurrent identical requests → 1 job); HTTP replay |
| Concurrent workers on one job | `TestConcurrentWorkersProcessOnce` (6 workers with separate pools: 1 produce, 5 `ErrBusy`, one READY at attempt 1; two real dispatchers: exactly one claims) |
| Audit write failure | `TestAuditWriteFailureServesNothing` (INSERT on the detail table revoked: no capability, no bytes, no generic event; restored: one SERVED record) |
| RLS without org setting | `TestExportSchemaGuardsHoldForDirectSQL` (nothing visible unscoped; unscoped and cross-organization inserts 42501; guard refusals; DELETE/TRUNCATE refused; terminal immutability) |
| Populated upgrade with failure/recovery | `TestExportMigrationPopulatedUpgradeFailureAndRecovery` (previous head = every entry `< 000207`; broken migration leaves no table/function/ledger row and identical rows; retry applies exactly `000207`; previous chain compatible; re-apply no-op; floor `000132`; forced RLS; upgraded store works) |
| No content/URL/token in logs or audit | `TestExportLifecycleOnRealStorage` dump of jobs, audit, access and queue rows; HTTP response scan; redaction unit tests |
| Open-job limit under concurrency | `TestOpenExportLimitIsEnforcedUnderConcurrency` (6 concurrent → 3 accepted, 3 `EXPORT_QUEUE_FULL`; independent per member; a settled job frees a slot) |

## Validation

Toolchain Go 1.27.4 (`export PATH=/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin:$PATH GOCACHE=/data/dev/cache/f3-go-build GOTMPDIR=/data/dev/cache/f3-tmp TMPDIR=/data/dev/cache/f3-tmp STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck`),
staticcheck under Go 1.27.1, sqlc v1.31.1, vacuum; commands from `backend/`
unless noted. `ANPFUEL_TEST_DATABASE_URL` selects the owned disposable
control database on 127.0.0.1:32775 (PostGIS 3.6); `orgtest` creates an
isolated child database and an ordinary NOSUPERUSER/NOBYPASSRLS runtime login
per test. `ANPFUEL_TEST_STORAGE_*` select the coordinator-owned disposable
MinIO on 127.0.0.1:32789 (bucket `validation-private`, tmpfs, CI test
credentials; values are not recorded here).

RED (exported API compiled with stub bodies, `orgapi` export codes declared
but not mapped, no migration or SQL):

- Unit: `go test -count=1 -race -json ./internal/modules/orgexport/...` plus
  `-run TestExportAuthority` (organization/adapters), `-run 'TestReportExport|TestOrganization'`
  (apicontract), `-run 'TestAuthorizationInventory|TestFullProductionRouter'`
  (cmd/api), `-run 'TestOrganizationExport|TestWorker|TestEnterprise'`
  (cmd/worker) and `./internal/platform/enterprisesuite`: 46 top-level tests,
  37 fail by assertion (6 domain, 15 application, 4 adapters, 4 HTTP,
  1 authority, 1 contract, 2 full-router/inventory, 3 worker-slot, 1 suite),
  9 pre-existing pass; 36 subtests (6 fail, 30 pass); no panic, no compile
  error, no skip. Log `/data/dev/worktrees/f3-logs/rpt001-go-red-unit.jsonl`.
- Integration: `go test -count=1 -race -tags=integration -json ./internal/modules/orgexport/adapters -run 'Integration|Export|Incomplete|Rights|Concurrent|Audit'`
  and `./internal/modules/organization/adapters -run 'TestExportsThroughActualCompanyHTTPAndWorker|TestAuthorizationMatrixActualCompanyHTTP'`:
  13 top-level fail (12 integration tests reach `relation "org_export_jobs"
  does not exist` on real PostGIS or the missing `000207` file; the unit
  vocabulary test selected by the pattern fails on the stub). Log
  `/data/dev/worktrees/f3-logs/rpt001-go-red-integration.jsonl`.

Test corrections before GREEN (no expectation weakened): the "invalid
deliverable object" case changed a structurally valid size to an invalid
digest; the RLS case created the other organization's own job it then
expects to see; the expiry helper uses one transaction instant instead of
per-column `clock_timestamp()` (which broke the `as_of = requested_at`
CHECK); the module fixture provisions members of the bare `orgtest` roots
(no onboarding snapshot) as migration owner with triggers suspended, while
the export insert guard still requires the ACTIVE membership and the real
company pipeline is proven in `organization/adapters`. REFACTOR added
`TestCanonicalParametersKeepEscapesLossless` and a storage-untouched
assertion for busy/settled jobs (staticcheck SA4006), re-run green.

GREEN:

- Unit, same selection with atomic coverage: 47 top-level / 53 subtests
  pass, 0 failures, 0 skips. Log
  `/data/dev/worktrees/f3-logs/rpt001-go-green-unit.jsonl`, profile
  `/data/dev/worktrees/f3-logs/rpt001-go-cover-unit.out`: `orgexport/domain`
  **93.8%**, `orgexport/application` 85.7%, `orgexport/adapters/http` 40.4%,
  `orgexport/adapters` 23.2% (store and storage paths are integration-only).
- Integration, `go test -count=1 -race -tags=integration -covermode=atomic -coverpkg=./internal/modules/orgexport/... -json ./internal/modules/orgexport/...`
  plus the organization/adapters export HTTP and authorization matrix tests:
  42 top-level / 2538 subtests pass, 0 failures/skips (orgexport/adapters
  15.0 s, organization/adapters 26.7 s; matrix now 180 runtime cells,
  64 allowed / 116 denied). Log
  `/data/dev/worktrees/f3-logs/rpt001-go-green-integration.jsonl`; profiles
  `rpt001-go-cover-integration.out` and `rpt001-go-cover-integration-org.out`.
  Combined unit+integration coverage: `orgexport/domain` 93.8%,
  `orgexport/application` 86.0%, `orgexport/adapters/http` 84.2%,
  `orgexport/adapters` 78.6% (module 83.9%; uncovered lines are fail-closed
  branches for corrupt stored rows and provider errors).
- Affected integration sweep, `go test -count=1 -race -tags=integration -json -p 3 ./internal/modules/orgaudit/... ./internal/modules/orgmedia/... ./cmd/api ./cmd/worker ./internal/platform/apicontract ./internal/platform/migrate ./internal/platform/enterprisesuite ./internal/platform/orgapi ./internal/platform/jobs`
  (with PostGIS and MinIO): 12 packages pass, including the S3-backed
  `orgmedia` storage suites and the full migration chain with `000207`; 222
  top-level / 726 subtests pass, 1 pre-existing skip. `cmd/worker` has 5
  pre-existing environment failures that stop before any export code: they
  require the pinned Rust quality, sanctions and enforcement CLIs
  (`ANPFUEL_TEST_QUALITY_PREPARER_PATH`, `ANPFUEL_TEST_SANCTIONS_PREPARER_PATH`,
  trusted enforcement CLI), which this environment does not provide. Re-run
  without those five, `cmd/worker` passes 15/15 top-level (1 skip). Logs
  `/data/dev/worktrees/f3-logs/rpt001-go-integration-affected.jsonl` and
  `/data/dev/worktrees/f3-logs/rpt001-go-integration-worker-no-cli.jsonl`.
- `organization/adapters` alone (company authority whitelist and matrix
  changed), `go test -count=1 -race -tags=integration -timeout 45m -json ./internal/modules/organization/adapters`:
  367/367 top-level and 2819/2819 subtests pass in 900.6 s. Log
  `/data/dev/worktrees/f3-logs/rpt001-go-integration-organization.jsonl`.
- Untagged whole module (CI): `go test -count=1 -json ./...` — 204 packages
  pass, 74 without tests, 2554 top-level / 6881 subtests pass, 0 failures,
  4 pre-existing skips (`enterpriseexport/portalgen` ×2, `identity/profile`,
  `quality/domain`); `/data/dev/worktrees/f3-logs/rpt001-go-untagged-all.jsonl`.

Gates, all passing:

```sh
go build ./... && go vet ./...                      # clean
go vet -tags=integration ./internal/modules/orgexport/... ./internal/modules/organization/adapters ./internal/modules/orgaudit/... ./internal/modules/orgmedia/... ./cmd/api ./cmd/worker ./internal/platform/...   # clean
gofmt -l ./internal ./cmd ./db                      # empty
/home/iiii/go/bin/sqlc generate && /home/iiii/go/bin/sqlc vet && /home/iiii/go/bin/sqlc diff   # clean
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/orgexport/... ./internal/modules/organization/adapters/... ./internal/platform/enterprisesuite ./internal/platform/apicontract ./internal/platform/orgapi ./cmd/api ./cmd/worker   # no findings
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/orgexport/... ./internal/modules/organization/adapters ./cmd/api ./cmd/worker ./internal/platform/enterprisesuite   # no findings
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check   # repo root: 0 errors; 924 warnings / 594 info vs 919 / 588 at base (5 request-body-description style warnings on the new operations, as on the period operations; 6 info)
python3 docs/enterprise/tools/check_plan.py         # repo root: 348 tasks, 545 rules, 167 use cases, 145 screens, 0 problems
git diff --cached --check                           # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'   # see below
git diff --cached --binary | gitleaks stdin --redact --no-banner   # no leaks found
```

Secret/PII scan: the keyword hits are the capability/lease `token`
vocabulary, configuration field names (`SecretAccessKey` read from config or
test environment variables, never a value), the synthetic token secret
`synthetic-…` and the repeated-byte cursor secret of the test router, and the
scan command in this file; no credential, key, account or personal data is
committed. All identifiers are synthetic `7b0e2a54-…` UUIDs or generated
UUIDv4s. Logs: `/data/dev/worktrees/f3-logs/rpt001-staticcheck.log` (empty),
`/data/dev/worktrees/f3-logs/rpt001-vacuum.log`; source manifest
`/data/dev/worktrees/f3-logs/rpt001-source.sha256`; migration `000207`
SHA-256 `d3e65fceb30582dbeb82e5ac7702da3275492c4e584e2fc4951345eac014b51a`.
No dependency was added.

## Retained obligations

- Formats (EMP-RPT-002): no real kind is admitted, so every production request
  is `EXPORT_FORMAT_UNAVAILABLE`. Real formats own their scoped, deterministic
  data readers under the requester's current scope, sensitive-column rules,
  manifests and the spreadsheet dependency review; the synthetic kind exists
  only in tests.
- Export permission matrix (EMP-AUTH-011): per-kind read permissions plug into
  `Authority` (the worker check already receives the kind); AUDITOR export
  cases beyond the `rpt.export.run` matrix row belong there.
- Not delivered: inline small CSV in the request (BR-RPT-001), cancel while
  queued (`EXPORT_CANCELLED`), schedules, PDF, the full organization export,
  an organization-configurable retention (fixed 7 days per job), streaming
  delivery above 16 MiB, `rpt.export.*` events and the ready/failed
  notifications (EMP-NOTIF owners), the panel screens SCR-RPT-01/02.
- Operations: a dead cleanup job leaves its object private under the
  organization prefix until an operator replays it; a dead build job of a
  never-claimed export stays `QUEUED` and counts toward the requester's open
  limit until replayed (dead-job alerting is the existing platform path).
- Runtime: production grants (see proposed README text), the worker's
  ordinary runtime login (it must not own RLS tables), storage lifecycle
  rules and quota measurement are not provisioned here.
- Integration renumbering of `000207` changes only the migration file name,
  the `sqlc.yaml` schema entry and `exportMigration` in
  `orgexport/adapters/harness_integration_test.go`.
- No F3, M3 or C1 acceptance is claimed; LOCAL_DONE only, no push/PR/CI.

## Proposed shared-doc changes

1. Product contract: new section "EMP-RPT-001 — Asynchronous export
   infrastructure: job, private file and expiring link (2026-11-14)" with the
   evidence link and the CONTRACT block above verbatim.
2. `16-decisions.md`: append the EDR-156 and EDR-157 rows above.
3. `backlog/ep13-intelligence.md`: EMP-RPT-001 `state LOCAL_DONE` and the
   BACKLOG note below after its block.
4. `backend/README.md` runtime rollout grants (no live provisioning):
   "EMP-RPT-001 (migration 000207): the API and worker runtime roles need
   `GRANT SELECT,INSERT,UPDATE ON org_export_jobs`, `GRANT SELECT,INSERT ON
   org_audit_events,org_sensitive_access_events` and `GRANT SELECT,INSERT,UPDATE
   ON job_queue`; the export insert guard reads `org_memberships`; the worker's
   requester re-check needs what the company pipeline already uses: `SELECT`
   plus the row-lock privilege for `FOR SHARE` on `accounts` and
   `organizations`, and `SELECT` on `org_memberships`, `org_role_assignments`,
   `org_onboarding_commands` and `payment_billing_exits`. The worker must
   connect with an ordinary
   NOSUPERUSER/NOBYPASSRLS login that owns no RLS table. Rows are never
   deleted; export objects live under `org/<org_id>/export/` in the existing
   private bucket and are deleted by the `org-export-cleanup` job."
5. `09-architecture.md` §9, after the storage table: "EMP-RPT-001 implements
   the export prefix as `org/<org_id>/export/<export_id>/<sha256>` in module
   `orgexport` (migration 000207): create-only upload, read-back before
   `READY`, a requester-bound 5-minute API capability (`orgexport.access.v1`)
   recorded as `EXPORT_FILE` before minting and serving, and deletion with
   confirmed absence at expiry by the `org-export-cleanup` job." followed by
   a link labelled "Evidence" to `evidence/f3-rpt001-export-infrastructure.md`.
   §7: "Export jobs run on dedicated serial slots `org-export-build` and
   `org-export-cleanup`, excluded from the general dispatcher."
6. `modules/intelligence.md` §10.11: add `EXPORT_RIGHTS_LOST` and
   `EXPORT_ATTEMPTS_EXHAUSTED` as recorded job failure codes (no HTTP status;
   read on the job), and note that F3 delivers files through a base64 JSON
   capability exchange with a 16 MiB ceiling.
7. `08-state-model.md` Export job row: "`RUNNING`→`QUEUED` is a fenced lease
   release; the last attempt or a lease expired on it ends `FAILED`
   (`EXPORT_ATTEMPTS_EXHAUSTED`)."
8. `PROGRESS.md`: EMP-RPT-001 LOCAL_DONE / INTEGRATION_PENDING with this
   evidence link.
