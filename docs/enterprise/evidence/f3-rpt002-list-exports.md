# EMP-RPT-002 — CSV exports for operational lists (spreadsheet retained)

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 task (module RPT, priority P1, type API, size M) in worktree
branch `codex/enterprise-f3-rpt002`, opened from the integrated head with the
export infrastructure (EMP-RPT-001, `orgexport`, migration 000207) and the
per-kind data permission (EMP-AUTH-011). It admits the first real export
kinds on that infrastructure. The coordinator integrates it into
`codex/enterprise-f3`.

Sources read: the EMP-RPT-002 task block (ep13);
[intelligence](../modules/intelligence.md) §§4–5 (BR-RPT-002/003/006/007/008/
010/011, BR-KPI-002/015, UC-RPT-01) and §10.10–10.11;
[07-permissions](../07-permissions.md) §4 and the `org-permissions-v4`
catalog source; [11-privacy-security](../11-privacy-security.md) §4
(BR-PRIV-001/002/005/007/008) and [identity](../modules/identity.md) "Employee
fields and their purposes"; the evidence of EMP-RPT-001 (EDR-156/157) and
EMP-AUTH-011 (EDR-166/167); the public-portal CSV writer precedent
(`enterpriseexport/portalgen/downloads.go`: `encoding/csv`, apostrophe prefix,
separate JSON manifest with name/rows/bytes/SHA-256); `orgexport/**`, the
export authority, the asset, meter, held-reading, allocation-period and
employee-register schemas (migrations 000170/000176/000197/000198/000199/
000201/000206) and their list queries and HTTP projections.

Observed before any change: production composes no export format, so every
kind is `EXPORT_FORMAT_UNAVAILABLE`; the worker calls `Format.Produce` without
a transaction after its own requester re-check; a produce error other than a
ceiling is retried. There is no `meter.reading.read` key: meter reads are
served under `fleet.asset.read`. `doc.document.read` is registered but granted
to no role, so a document-metadata list cannot pass `NewRegistry` today. No
reviewed spreadsheet dependency exists in `go.mod`.

IDs used: B-BR-EMP-255..258, BUC-EMP-130..131, EDR-171..172. No migration
(`000213` stays unused; 000210–000212 are reserved by parallel tasks).

## Rules frozen before behaviour

Proposed contract section title: "EMP-RPT-002 — CSV exports for operational
lists".

Rules B-BR-EMP-229, B-BR-EMP-255, B-BR-EMP-256, B-BR-EMP-257, B-BR-EMP-258, BUC-EMP-130, BUC-EMP-131 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-RPT-002 — CSV exports for operational lists" (frozen before behaviour; text moved verbatim at integration).

### Decisions (EDR rows, frozen before behaviour)

Decisions EDR-171, EDR-172 are recorded in [16-decisions](../16-decisions.md).

### Backlog note


## Delivered surface

- Pure domain `backend/internal/modules/orgexport/domain/list.go`:
  `ListSpec`/`ListBasis` validation, typed cells (`Text`, `Integer`,
  `Instant`, `Empty`), `InertText`/`RecoverText`, the streaming
  `ListWriter` (ZIP through the sink: `<file>.csv` then `manifest.json`,
  deflate, entries timed at "as of"; BOM, `encoding/csv` with LF, header,
  per-column type checks, UTF-8 check, CSV SHA-256/bytes/rows) and the
  manifest types (`ListManifest`, `ManifestFile`, `Column`, `Omission`,
  `Encoding`, `ReviewBasis`). `export.go` gains `Sink.CountRow` (row ceiling
  for packaged rows). No I/O beyond the in-memory sink.
- Application `orgexport/application/worker.go`: `ErrRightsLost` returned by a
  format ends the job `FAILED`/`EXPORT_RIGHTS_LOST` at once (no retry, no
  object).
- Adapters `backend/internal/modules/orgexport/adapters/lists/{lists.go,kinds.go}`:
  `Format` (kind, declared read permission, `application/zip`, default
  ceilings, closed canonical filters), `Produce` (`database.WithOrgTx` →
  `CheckRequesterTx` → `SET TRANSACTION READ ONLY` → `transaction_timestamp()`
  with the organization's name and time zone → one parametrized statement with
  `LIMIT` ceiling + 1, streamed row by row into the writer), the three lists
  (spec, filter vocabulary mirrored from the stored CHECKs, statement, row
  decoder with exact sub-microsecond instants), `New`, `Spec` and the
  `Requesters` port.
- Composition: `internal/platform/enterprisesuite/exports.go` always admits
  `lists.New(o.Pool, authority)` plus any test-only kinds, for the API (company
  export authority) and the worker (session-free export authority);
  `Options.ExportFormats` comment updated. `cmd/api` and `cmd/worker` are
  unchanged.
- Contract: `contracts/openapi/v1.yaml` — `EnterpriseExportKind` names the
  admitted kinds and the unavailable `*.xlsx` kinds (pattern kept, no enum so an
  unadmitted kind still reaches `EXPORT_FORMAT_UNAVAILABLE`); request and
  content descriptions state the closed filters, `VALIDATION_FAILED`, the
  package, inert text and manifest; the list/read/access descriptions drop
  "admits no real export format"; new schemas
  `EnterpriseAssetListExportParameters`,
  `EnterpriseMeterReadingListExportParameters`,
  `EnterpriseEmployeeListExportParameters`. No path, status, response schema or
  error code changes.
- Inventories: `scripts/check-manifest.txt` adds the quick package
  `backend/internal/modules/orgexport/adapters/lists`; new contract guard
  `internal/platform/apicontract/list_exports_test.go`.
- No migration, sqlc query, route, DTO, catalog, dependency, Kotlin or Android
  change; fleet, membership and document modules are read, not modified.

## Design notes

- Kinds end in `.csv` because the table format is CSV; the stored and
  delivered file is the list package (`application/zip`). The spreadsheet
  kinds `fleet.asset_list.xlsx`, `fleet.meter_reading_list.xlsx` and
  `idn.employee_list.xlsx` are not registered (BR-RPT-011, CX-10 review not
  done here).
- Least privilege: the readers need only column `SELECT`s (proposed README
  text below); work e-mail/phone and the capture locator columns are not
  granted, so the reader cannot read them even by mistake
  (`TestListExportsFailWithoutPartialOutput` proves the export role cannot
  select `work_email`).
- Isolation is layered: explicit `org_id = $1` in every statement, forced RLS
  under the job's scope, and the requester re-check in the same transaction.
- Instants: `generated_at` is the reading transaction's
  `transaction_timestamp()`; "as of" is the job's. Asset unit and cost-centre
  columns are the allocation periods valid at `generated_at`; readings are
  bounded by `received_at` ≤ "as of"; the reading period filter compares
  `(occurred_at, occurred_submicro)` with the split bound, so boundaries are
  exact to the nanosecond.
- Identifiers, codes and instants are text cells too; the inert rule is a
  no-op for them, so one rule covers every non-integer cell.
- Determinism holds for the CSV bytes across runs of the same job and data;
  compressed package bytes also depend on the Go deflate implementation and the
  manifest's `generated_at`, which is why the manifest carries the CSV's own
  SHA-256.
- Clarifications made while implementing (no rule weakened): BR-RPT-006 says a
  neutralised cell is "quoted"; `encoding/csv` quotes a field only when it needs
  quoting (separator, quote, CR/LF, leading space) — the apostrophe is what makes
  the cell inert, and quotes are doubled (proposed wording below). LF and the
  apostrophe itself were added to the neutralised starts so the scheme stays
  reversible. The review-basis vocabulary adds `NOT_APPLICABLE` for registers
  without a review flow; held meter readings are excluded and not counted (their
  count is review-queue data under `meter.reading.correct`).

## Negative cases → tests

| Negative case | Test (real PostGIS 3.6 / disposable MinIO unless noted) |
|---|---|
| Formula interpretation | unit `TestInertTextNeutralisesFormulaStartsReversibly`, `TestListWriterProducesExactInertCSVAndItsManifest` (round-trip through `encoding/csv`, no cell starts with `= + - @` TAB CR LF, negative integers stay numeric); `TestListExportsReadExactScopedInertRows` (stored `=HYPERLINK(...)`, `+5511`, `@SUM(A1)`, `-Synthetic tractor`, `'Sold, "as is"`, `=CMD|' /C calc'!A0`); `TestListExportsThroughActualCompanyHTTPAndWorker` (formula-shaped asset, category and employee text created through the company routes) |
| Foreign rows | `TestListExportsReadExactScopedInertRows`/`TestListExportFiltersAndAsOf` (organization B's assets, readings, employees, IDs and contact never in CSV or manifest; filtering by B's asset ID returns no row); RLS check in `TestListExportsFailWithoutPartialOutput`; HTTP test (foreign asset of an activated second organization absent) |
| Stale requester scope | HTTP test (FLEET_MANAGER role ended after request → `FAILED`/`EXPORT_RIGHTS_LOST`, no object); `TestListExportsFailWithoutPartialOutput` (loss inside the reading transaction and a suspended membership → `ErrRightsLost`, sink empty); unit `TestBuildFailsRightsLostFoundWhileReading` (worker settles it without retry or storage) |
| Format not admitted | HTTP test (`fleet.asset_list.xlsx`, `idn.employee_list.xlsx` → 422 `EXPORT_FORMAT_UNAVAILABLE`, nothing written); unit `TestAdmittedListKindsAndPermissions`; contract `TestListExportKindsAreDocumentedExactly` (no enum) |
| Invalid filters | unit `TestCanonicalFiltersAreClosedAndBounded`; HTTP test (`state` BOGUS, reversed period, `work_email` key → 422 `VALIDATION_FAILED`, nothing written) |
| Incomplete output never READY | unit `TestListWriterCeilingsFailWholeAndEmptyIsValid`, `TestListWriterRefusesInvalidRows`; `TestListExportsFailWithoutPartialOutput` (row and byte ceilings → `ErrTooLarge`; privilege outage → retryable error); HTTP test (reading privilege revoked → three attempts → `FAILED`/`EXPORT_ATTEMPTS_EXHAUSTED`, no object, access `EXPORT_NOT_READY`) |
| Exact values round-trip | `TestListExportsReadExactScopedInertRows` (every cell re-parsed and compared with the stored value: 999 999 999 999 m, 3 599 999 999 s, −10 800 s, sub-microsecond instants, empty absent cells); `TestListExportFiltersAndAsOf` (exact period boundaries, late receipt bounded by "as of"); HTTP test (opening reading 123 456 789 m and −10 800 s through the real install route) |
| Deterministic output | unit `TestListWriterIsDeterministic`; `TestListExportsReadExactScopedInertRows` (second production: byte-identical CSV, manifest equal but for `generated_at`) |
| Omitted sensitive columns reported without values | unit `TestSpecsOmitSensitiveColumnsWithReasons`; integration and HTTP tests (contact e-mail/phone and capture session/enrolment IDs absent from CSV and manifest; `PURPOSE_LIMITED` ×2 and `CAPTURE_LOCATOR` reported) |
| Wrong data permission | unchanged EMP-AUTH-011 coverage (`TestExportAndContentGatesActualCompanyHTTP`, re-run green) |

## RED → GREEN → REFACTOR

Environment: Go 1.27.4 toolchain (`export
PATH=/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin:$PATH
GOCACHE=/data/dev/cache/f3-go-build GOTMPDIR=/data/dev/cache/f3-tmp
TMPDIR=/data/dev/cache/f3-tmp STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck`),
commands from `backend/`. Integration runs use the owned control PostGIS 3.6
(`ANPFUEL_TEST_DATABASE_URL` on 127.0.0.1:32775; isolated `orgtest` child
databases with ordinary NOSUPERUSER/NOBYPASSRLS logins) and the
coordinator-owned disposable MinIO (`ANPFUEL_TEST_STORAGE_*` on
127.0.0.1:32789, bucket `validation-private`; values not recorded here).

RED (exported API compiled with stub bodies: `Sink.CountRow` nil,
`ListWriter`/`Validate` returning a not-implemented error, cells and
`InertText` identity, `lists.New` empty, worker unchanged, contract
unchanged):

- Unit: `go test -count=1 -race -json ./internal/modules/orgexport/...` plus
  `go test -count=1 -json ./internal/platform/apicontract -run
  'TestListExportKindsAreDocumentedExactly|TestReportExportContractExamplesAndExactShapes'`
  — 49 top-level: 13 fail by assertion (7 domain, 4 lists, 1 worker, 1
  contract), 36 pre-existing pass; 51 subtests (37 fail, 14 pass); no panic,
  compile error or skip. Log `/data/dev/worktrees/f3-logs/rpt002-go-red-unit.jsonl`.
- Integration: `go test -count=1 -race -tags=integration -json
  ./internal/modules/orgexport/adapters/lists -run 'TestListExport'` and
  `./internal/modules/organization/adapters -run
  TestListExportsThroughActualCompanyHTTPAndWorker` — 4 top-level fail (the
  three reader tests reach "list kind not composed"/"composed list formats 0";
  the HTTP test gets `EXPORT_FORMAT_UNAVAILABLE` where the admitted asset kind
  must answer `VALIDATION_FAILED`), 15 subtests fail; no panic. Log
  `/data/dev/worktrees/f3-logs/rpt002-go-red-integration.jsonl`.

Test corrections before GREEN (no expectation weakened):
`TestUncomposedFormatProducesNothing` gained a count guard (it passed
vacuously on the empty stub) and `TestCanonicalFiltersAreClosedAndBounded` a
missing-kind guard (a nil format panicked the binary); the reader fixture
writes a full organization snapshot (the root's shape CHECK), a SUSPENDED
membership with its origin and account, and guards uncomposed kinds. During
GREEN two test faults were fixed: the foreign-value scan now skips the
manifest's echoed `filters` (a requested foreign asset ID is the requester's own
input; the CSV is still scanned), and the HTTP read-failure case revokes the
table privilege (the router's runtime role holds table-level `SELECT`, so a
column revoke did not fail the read).

GREEN:

- Unit, same selection with atomic coverage: 49 top-level / 54 subtests pass,
  0 failures, 0 skips. Log
  `/data/dev/worktrees/f3-logs/rpt002-go-green-unit.jsonl`; profile
  `rpt002-go-cover-unit.out` (`orgexport/domain` 92.2%, `adapters/lists` 38.1%:
  the readers are integration-only).
- Integration, `go test -count=1 -race -tags=integration -json
  -covermode=atomic -coverpkg=./internal/modules/orgexport/...
  ./internal/modules/orgexport/...` plus `./internal/modules/organization/adapters
  -run 'TestListExportsThroughActualCompanyHTTPAndWorker|TestExportsThroughActualCompanyHTTPAndWorker|TestExportAndContentGatesActualCompanyHTTP|TestAuditorReadWriteMatrixActualCompanyHTTP|TestExportDataAuthorityFailsClosed|TestExportAuthority'`:
  67 top-level / 141 subtests pass, 0 failures/skips (lists 5.2 s,
  orgexport/adapters 15.5 s, organization/adapters 14.6 s). Log
  `/data/dev/worktrees/f3-logs/rpt002-go-green-integration.jsonl`; profiles
  `rpt002-go-cover-integration.out`, `rpt002-go-cover-integration-org.out`.
- Merged unit + integration coverage: `orgexport/adapters/lists` **86.9%**
  (`kinds.go` 88.2%, `lists.go` 85.3%), `orgexport/domain` 92.6% (`list.go`
  **90.9%**), `orgexport/application` 86.4%, `orgexport/adapters` 79.0%,
  `orgexport/adapters/http` 84.2%. Uncovered lines are fail-closed branches for
  corrupt stored instants/kinds and writer failures inside the ZIP library.
- REFACTOR: the readers' `statement` lost its always-nil error; staticcheck
  SA4000 (identical `CountRow() != nil` operands) in
  `TestSinkCountRowAppliesTheRowCeiling` rewritten as a loop. Re-run green.

## Validation

Affected race union with PostGIS and MinIO, `go test -count=1 -race
-tags=integration -timeout 45m -p 3 -json -skip
'^(TestEnforcementWorkerNativeDiscoveryCorrectionAckLossCleanupAndDeadBudget|TestFineWorkerCompanyRetirementCleanupAndAcknowledgmentRecovery|TestNativeFineWorkerDiscoveryDispatchRustAndCorrection|TestNativeWorkerDirectoryRevalidation|TestNativeWorkerRustDirectoryPublicationAndRetirement)$'
./internal/modules/orgexport/... ./internal/modules/organization/adapters
./internal/modules/fleet/... ./internal/modules/membership/...
./internal/platform/apicontract ./internal/platform/enterprisesuite ./cmd/api
./cmd/worker` (23:24–23:36 UTC): 19 packages pass, 758 top-level / 4959
subtests pass, 0 failures, 1 pre-existing environment skip
(`cmd/worker` `TestEnforcementPinnedWholeWorkerReplayCorrectionAndFullTraversal`);
organization/adapters 707.8 s (export authority, auditor walk, 200-cell
matrix and the new list HTTP test), apicontract 593.4 s, cmd/api 97.6 s. The
five skipped `cmd/worker` tests need the pinned Rust quality, sanctions and
enforcement preparer CLIs (`ANPFUEL_TEST_QUALITY_PREPARER_PATH`,
`ANPFUEL_TEST_SANCTIONS_PREPARER_PATH`, trusted enforcement CLI) that this
machine lacks; they stop before any export code (as recorded by EMP-RPT-001).
Log `/data/dev/worktrees/f3-logs/rpt002-go-race-union.jsonl`.

Untagged whole module (CI), `go test -count=1 -timeout=45m -json ./...`
(23:36–23:38 UTC): 212 packages pass, 77 without tests, 2648 top-level / 7446
subtests pass, 0 failures, 4 pre-existing skips (`enterpriseexport/portalgen`
×2, `identity/profile` TestGenerateVectors, `quality/domain`). Log
`/data/dev/worktrees/f3-logs/rpt002-go-untagged-all.jsonl`.

Gates, all passing:

```sh
go build ./... && go vet ./...                      # clean (rpt002-build-vet.log)
go vet -tags=integration ./internal/modules/orgexport/... ./internal/modules/organization/... ./internal/modules/fleet/... ./internal/modules/membership/... ./internal/platform/... ./cmd/api ./cmd/worker   # clean (rpt002-vet-integration.log)
gofmt -l ./internal ./cmd ./db                      # empty
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/orgexport/... ./internal/modules/organization/adapters/... ./internal/platform/enterprisesuite ./internal/platform/apicontract ./cmd/api ./cmd/worker   # no findings
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/orgexport/... ./internal/modules/organization/adapters ./internal/platform/apicontract ./internal/platform/enterprisesuite ./cmd/api ./cmd/worker   # no findings (rpt002-staticcheck.log)
/home/iiii/go/bin/sqlc diff && /home/iiii/go/bin/sqlc vet   # clean, no SQL change (rpt002-sqlc.log)
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check   # repo root: 0 errors, 945 warnings / 611 info, identical to the base head (rpt002-vacuum.log)
python3 docs/enterprise/tools/check_plan.py         # repo root: 348 tasks, 545 rules, 167 use cases, 145 screens, 0 problems
git diff --cached --check                           # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'   # see below
```

Secret/PII scan: the only keyword hits are a diff context line of the
existing `exportCapabilityKey(secret []byte)` signature and the scan command in
this file; `git diff --cached --binary | gitleaks stdin --redact --no-banner`
reports no leaks. Contact values are synthetic (`*@example.invalid`,
`+55659999900xx`), names are "Synthetic …"/"FOREIGN …" fixtures, identifiers
are synthetic `7b0e2a54-…` UUIDs or generated UUIDv4s; storage credentials are
read from the test environment and not recorded.

No dependency was added; `go.mod`/`go.sum` are unchanged. The reader SQL is
plain parametrized text in `kinds.go`, exercised statement by statement on real
PostGIS by the integration tests (not a sqlc package; EDR-172).

## Retained obligations

- Spreadsheet (XLSX): not admitted. The §10.10 proposal (minimal in-repository
  writer on `archive/zip` and `encoding/xml`, strings and numbers only, a
  "Sobre este relatório" sheet) needs its own recorded CX-10 review and
  vectors; until then every `*.xlsx` kind is `EXPORT_FORMAT_UNAVAILABLE`.
- Row scoping: only organization-wide grants export (EMP-AUTH-011). UNIT,
  COST_CENTRE, TEAM, ASSET and SELF readers need scoped list filters labelled
  with their scope (BR-KPI-011) before they may export.
- Sensitive per-column permissions (BR-RPT-003): no admitted list has a
  location or document-content column; the first list that does (trips, tracks,
  documents) must add the per-column check, omission note and audit category.
- Documents: no document-metadata list (its `doc.document.read` key is granted
  to no role; a catalog decision is needed). Fuelings, expenses, trips and
  indicator tables arrive with their owning modules.
- Not delivered: inline small CSV in the request (BR-RPT-001), cancellation,
  schedules, PDF, the full organization export, panel screens SCR-RPT-01/02,
  "inclui pendentes" variants (no admitted list has a pending part to show).
- Runtime: the export worker login needs the column grants proposed below; no
  live provisioning here.
- Integration: the parallel EMP-FLEET-006 (driver assignments) and EMP-DOC-014
  change their own modules only; if FLEET-006 adds asset columns the asset list
  is unchanged until a task decides to export them.
- No F3, M3 or C1 acceptance is claimed; LOCAL_DONE only, no push/PR/CI.

## Proposed shared-doc changes

1. `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`: append a section
   `## EMP-RPT-002 — CSV exports for operational lists (2026-11-14)` with a
   link to this evidence and the bullets between `CONTRACT:BEGIN`/`CONTRACT:END`
   verbatim.
2. `docs/enterprise/16-decisions.md`: append the rows between
   `EDR:BEGIN`/`EDR:END` (EDR-171, EDR-172).
3. `docs/enterprise/backlog/ep13-intelligence.md` EMP-RPT-002: `state TODO` →
   `LOCAL_DONE` and the note between `BACKLOG:BEGIN`/`BACKLOG:END` after its
   block.
4. `docs/enterprise/modules/intelligence.md`:
   - §4 BR-RPT-006, replace "is prefixed with an apostrophe and quoted, and
     embedded quotes are doubled" with "is prefixed with an apostrophe (as is a
     cell that already begins with one, so the scheme is reversible), the field
     is quoted when it holds a separator, a quote, a line break or a leading
     space, and embedded quotes are doubled".
   - §10.10 CSV row, Notes: append "; delivered with its manifest in a ZIP
     package (`application/zip`, `<list>.csv` + `manifest.json`,
     EMP-RPT-002)". Spreadsheet row, Writer: append "EMP-RPT-002 did not admit
     it (no reviewed writer); the kinds stay `EXPORT_FORMAT_UNAVAILABLE`".
   - §5 UC-RPT-01, after "choose CSV or spreadsheet and the columns": add
     "(EMP-RPT-002 admits CSV for assets, meter readings and the employee
     register with fixed columns)".
5. `docs/enterprise/09-architecture.md` `orgexport` row: append "admits the
   assets, meter-reading and employee list formats (`orgexport/adapters/lists`,
   EMP-RPT-002): requester re-check and read-only scoped read in one
   transaction, ZIP package with CSV and manifest".
6. `backend/README.md`, under the EMP-RPT-001 export grants: "EMP-RPT-002 list
   readers (worker login only; column privileges, no work contact or capture
   locator): `GRANT SELECT(org_id,id,kind,category_id,name,fleet_number,plate,serial_number,usage_basis,state,revision,acquired_at,acquired_submicro,disposal_kind,disposed_at,disposed_submicro,disposal_reason) ON org_assets`,
   `GRANT SELECT(org_id,id,label) ON org_asset_categories`,
   `GRANT SELECT(org_id,asset_id,unit_id,valid_from,valid_to) ON org_asset_unit_periods`,
   `GRANT SELECT(org_id,asset_id,cost_centre_id,valid_from,valid_to) ON org_asset_cost_centre_periods`,
   `GRANT SELECT(org_id,id,asset_id,kind) ON org_meters`,
   `GRANT SELECT(org_id,id,meter_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,received_at,origin,method,state,author_membership_id,capture_kind) ON org_meter_readings`,
   `GRANT SELECT(org_id,id,state,revision,merged_into) ON org_employee_identities`,
   `GRANT SELECT(org_id,employee_id,display_name,registration_number,job_title,home_unit_id,revision) ON org_employee_profiles`,
   `GRANT SELECT(org_id,employee_id,state) ON org_memberships` and
   `GRANT SELECT(org_id,display_name,time_zone) ON organizations` (the last two
   are already covered where the requester re-check holds table `SELECT`). The
   reading transaction is switched to read-only after the requester re-check."
7. `PROGRESS.md`: EMP-RPT-002 LOCAL_DONE / INTEGRATION_PENDING with this
   evidence link; EMP-RPT-003 (PDF) remains blocked on its own writer decision.
