# EMP-AUTH-011 — Auditor read-only role and export permission gating

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 task (module AUTH, priority P1, type API, size S) in worktree
branch `codex/enterprise-f3-auth011`, opened from the integrated head with
migrations through `000208`. It proves the AUDITOR read/write matrix on every
currently routed organization route and adds the independent per-kind read
permission gate to the export pipeline of EMP-RPT-001, which unblocks
EMP-RPT-002. The coordinator integrates it into `codex/enterprise-f3`.

Sources read: the EMP-AUTH-011 task block (ep03) and the EMP-RPT-002 (ep13)
and EMP-BACK-003 (ep19) blocks; [07-permissions](../07-permissions.md) §§4,
7.1–7.4 (the AUDITOR row "reads everything in scope; changes nothing", the
`org-permissions-v4` registry, the matrix and footnote 3, BR-AUTH-001/003/
010/011/013/014); BR-AUD-003 ([04-business-rules](../04-business-rules.md)
§6); [intelligence](../modules/intelligence.md) §§4–5 (BR-RPT-002/003/012,
UC-RPT-01, "every action also needs the read permission of what it shows");
[11-privacy-security](../11-privacy-security.md) §4; the evidence of
EMP-AUTH-009 (conformance method), EMP-PRIV-005 (EDR-146/147), EMP-RPT-001
(EDR-156/157), EMP-DOC-002, EMP-APPR-006 (EDR-149) and EMP-ORG-010/EDR-048
(`org.ownership.respond` is an explicit consent key); source `orgaccess/domain`
(catalog, coverage), `orgaccess/application` (Prepare order),
`organization/adapters` (company authority, export authority, ownership
participant check), `orgexport/**`, `platform/orgapi` (gateway order and
`MissingPermission`), the route inventory and the 200-cell matrix.

IDs used: B-BR-EMP-247..250, BUC-EMP-126..127, EDR-166..167 (all of them; no
pre-allocated ID is left unused). No migration (`000211` stays unused).

## Rules frozen before behaviour

Observed before any change: the code-frozen catalog (`org-roles-v1`,
`org-permissions-v4`, 85 keys) already grants AUDITOR exactly the twelve keys
of the documented column — `idn.employee.read`, `org.settings.read`,
`org.ownership.respond` (all-role consent key, EDR-048), `fleet.asset.read`,
`trip.trip.read`, `gps.track.read` (`O3`, recorded tracks only),
`fuel.fueling.read`, `doc.document.view_content`, `exp.expense.read`,
`kpi.dashboard.read`, `rpt.export.run` and `audit.event.read` — and no
manage, record, review, assign, close or live-location key. Of these, six are
routed today (22 of the 103 normal routes); `trip.trip.read`,
`gps.track.read`, `fuel.fueling.read`, `exp.expense.read`,
`kpi.dashboard.read` and `audit.event.read` have no route yet. Period
list/read is gated by `fin.period.close` (EDR-149), which AUDITOR lacks.
The orgaccess `Prepare` refuses a missing permission before resolving any
record, so a refused route never reads its target. Export routes check only
`rpt.export.run` (EDR-157); the worker's `CheckRequesterTx` receives the kind
but checks no data permission, and no format declares one.

Proposed contract section title: "EMP-AUTH-011 — Auditor read-only role and
export permission gating".

Rules B-BR-EMP-247, B-BR-EMP-248, B-BR-EMP-249, B-BR-EMP-250, BUC-EMP-126, BUC-EMP-127 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-AUTH-011 — Auditor read-only role and export permission gating" (frozen before behaviour; text moved verbatim at integration).

### Decision rows

Decisions EDR-166, EDR-167 are recorded in [16-decisions](../16-decisions.md).

## Delivered surface

- Catalog: no change. `org-roles-v1`/`org-permissions-v4` (85 keys), the
  v1–v4 vectors, the route inventory (routed 20, signed 1, not routed 64,
  normal 103, special 24) and the 200-cell matrix (77 allowed / 123 denied
  plus the unit-scope variant) are unchanged; there is no Kotlin mirror of
  the catalog and no Gradle run was needed. New guard
  `backend/internal/modules/orgaccess/domain/auditor_matrix_test.go`
  compares the AUDITOR rules with an independent literal of the documented
  column (twelve keys, `AssignedScope`, `RecordedTracksOnly` on tracks, no
  live position, every other key denied).
- Export ports (`backend/internal/modules/orgexport/application`):
  `Format.ReadPermission()`; `ExportableRead` (registered
  `<module>.<resource>.read` key outside `gps.*` — sixteen of the 85 keys);
  `NewRegistry` refuses any other declaration; `Registry.ReadPermission(kind)`;
  `Command.ReadPermission()` (part of `Valid`); `Authority.CheckDataTx` and
  `CheckRequesterTx(…, kind, read)`; `Repository.Deliverable(…, Registry)`;
  `WorkerStore.CheckRequester(…, read)`. `Worker.Build` now looks the format
  up before the requester re-check (an unadmitted kind fails
  `EXPORT_FORMAT_UNAVAILABLE` without a requester check) and passes the
  declared permission.
- Store (`orgexport/adapters/store.go`): `authorizeRequest` (CheckExportTx then
  CheckDataTx) at the start of the request transaction, on replay and after
  the insert; `Deliverable` resolves the stored kind through the registry
  (`ErrFormatUnavailable` when retired) and runs CheckDataTx before the state
  is disclosed; `CheckRequester` forwards the permission; `storeError` keeps
  `ErrFormatUnavailable` (HTTP `EXPORT_FORMAT_UNAVAILABLE` 422 through the
  existing `Mapped`). A denial stays the bounded `PermissionDenial`, so the
  HTTP boundary answers `FORBIDDEN` with the data key (BR-AUTH-013).
- Authority (`organization/adapters/export_authority.go`): `CheckDataTx`
  (exportable key, valid actor and member, scoped transaction, the actor's
  ACTIVE membership equals the member, current assignments cover the key on
  the organization target under database time) and a shared `covers` helper;
  `CheckRequesterTx` re-checks `rpt.export.run` and the data key with it and
  maps either denial to `ErrRightsLost`. Structural and SELF grants never
  cover the organization target (fail closed).
- Contract: `contracts/openapi/v1.yaml` descriptions of the request, access
  and content operations state the data-permission gate (no schema, status
  or rule change); `apicontract` asserts it.
- No migration, SQL, route, DTO, dependency, catalog version, Android or
  runtime grant change. The worker and API composition are unchanged
  (`enterprisesuite` passes the same registry to the service and the worker).

## Design notes

- Prepare refuses a missing permission before resolving any record, so the
  auditor walk sends the contract's example bodies with synthetic IDs to the
  81 refused routes and still reaches the real refusal; the shared 32-part
  matrix snapshot and a 30-table digest of every other organization table a
  routed write could touch are compared on every refusal.
- Positive auditor routes use real records created by the owner through the
  same entry: a VEHICLE with an installed odometer, a document whose verified
  bytes went through the capability ingress to the disposable bucket, the
  auditor's own export processed by the production worker.
- `org.ownership.respond`: a non-participant auditor gets `NOT_FOUND` on read,
  accept and cancel; the nominated auditor reads its proposal and declines it
  (`DECLINED`); a later accept is `CONFLICT`; the auditor's role set stays
  `{AUDITOR}`. Accepting (becoming OWNER) follows the ORG-010 participant path
  and is not repeated here.
- Job metadata (list/read) keeps `rpt.export.run` only, so a requester who
  lost the data permission still sees why the file is refused; a `READY` file
  is never released without the data permission.

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

RED (signatures in place, stub bodies: `ExportableRead` false,
`Registry.ReadPermission`/`Command.ReadPermission` zero, `CheckDataTx` nil,
store without data checks, worker passing an empty permission):

- Unit: `go test -count=1 -race -json ./internal/modules/orgexport/...
  ./internal/modules/orgaccess/domain ./internal/modules/organization/adapters`
  — 79 top-level: 6 fail by assertion (five `gating_test.go` cases and
  `TestExportAuthorityCannotRunWithoutRetainedTransactionAndSource`), 73 pass
  (including the new AUDITOR column guard, see below); 1185 subtests pass; no
  panic, compile error or skip. Log
  `/data/dev/worktrees/f3-logs/auth011-go-red-unit.jsonl`.
- Integration: `go test -count=1 -race -tags=integration -json
  ./internal/modules/orgexport/adapters ./internal/modules/organization/adapters
  -run 'TestAuditorReadWriteMatrixActualCompanyHTTP|TestExportAndContentGatesActualCompanyHTTP|TestExportsThroughActualCompanyHTTPAndWorker|TestKindReadPermissionIsCheckedInEveryTransaction|TestExportLifecycleOnRealStorage|TestRightsLossAndCeilingsPublishNothing'`
  — 6 top-level: 4 fail (`TestKindReadPermissionIsCheckedInEveryTransaction`,
  `TestExportAndContentGatesActualCompanyHTTP`: the auditor's plan-kind
  request returned 202 instead of `FORBIDDEN subs.plan.read`; and the two
  pre-existing worker tests, whose harness authority now refuses the stub's
  empty data permission), 2 pass; 81 subtests pass. Log
  `/data/dev/worktrees/f3-logs/auth011-go-red-integration.jsonl`.
- Contract: `go test -count=1 -json ./internal/platform/apicontract -run
  TestReportExportContractExamplesAndExactShapes` — 1 fail
  (`export data permission gate undocumented`). Log
  `/data/dev/worktrees/f3-logs/auth011-go-red-contract.jsonl`.

Conformance found already enforced (no behaviour change needed, reported
honestly): `TestAuditorHoldsExactlyTheDocumentedReadOnlySet` and
`TestAuditorReadWriteMatrixActualCompanyHTTP` pass before and after GREEN —
the catalog and every routed policy already confine AUDITOR. A mutation
granting AUDITOR `fleet.asset.manage` in the catalog source makes both fail
(the HTTP walk reports 404/409/201 instead of 403 on the asset write
routes); the source was restored byte-identical. Log
`/data/dev/worktrees/f3-logs/auth011-mutation-auditor-write-grant.log`.

Test corrections before GREEN (no expectation weakened): the auditor asset
read asserts the projection's `id` field (the create acknowledgement uses
`asset_id`); the auditor router also grants `org_asset_lifecycle_events`
(lifecycle list returned 503 without it); the ownership read view is
unwrapped and a target's cancel is `DECLINED`, a later accept `CONFLICT`;
`TestRequestCarriesTheKindReadPermission` gives the fake repository a stored
view of the requested kind (the service rightly refuses an inconsistent
view).

GREEN:

- Unit, same selection with atomic coverage: 79 top-level / 1185 subtests
  pass, 0 failures, 0 skips. Log
  `/data/dev/worktrees/f3-logs/auth011-go-green-unit.jsonl`; profile
  `auth011-go-cover-unit.out`.
- Integration, same selection: 6 top-level / 81 subtests pass. Log
  `/data/dev/worktrees/f3-logs/auth011-go-green-integration-selected.jsonl`.
  The auditor walk logs: 103 routed normal routes, 22 documented positive (3
  write-shaped: export request and own consent), 81 `FORBIDDEN` (53 writes,
  28 reads) with unchanged private state.
- Contract: `./internal/platform/apicontract` 73 top-level / 703 subtests
  pass. Log `/data/dev/worktrees/f3-logs/auth011-go-green-contract.jsonl`.
- REFACTOR: `CheckRequesterTx` and `CheckDataTx` share one `covers` helper
  (organization target, database time, bounded denial); the store's three
  request checks share `authorizeRequest`. Added
  `TestExportDataAuthorityFailsClosed` (real authority on PostGIS: documented
  read admitted; undocumented key, UNIT grant, borrowed membership, missing
  actor, foreign-scope transaction, content/export/track/write/unknown keys,
  suspended membership all refused; requester re-check mirrors it). Re-run
  green (see Validation).

## What the tests prove

| Negative case | Test (real PostGIS/MinIO unless noted) |
|---|---|
| Auditor mutates a record | `TestAuditorReadWriteMatrixActualCompanyHTTP`: all 53 write routes outside the export request and the own consent are `FORBIDDEN` with the route key, exact safe envelope, the unchanged 32-part matrix snapshot and an unchanged 30-table digest; documented reads change nothing; unit `TestAuditorHoldsExactlyTheDocumentedReadOnlySet` |
| Auditor exports without `rpt.export.run` | `TestExportAndContentGatesActualCompanyHTTP`: UNIT-scoped AUDITOR and SUPERVISOR (broad read) `FORBIDDEN rpt.export.run`; revoked auditor `FORBIDDEN rpt.export.run` on read; matrix `rpt.export.run` cells unchanged |
| With `rpt.export.run` but without the kind's read (request) | same test: AUDITOR plan kind `FORBIDDEN subs.plan.read`, snapshot (jobs, audit and access counts) unchanged; `TestKindReadPermissionIsCheckedInEveryTransaction` (no job, event or queue row; replay refused too) |
| … and at the worker re-check | AUDITOR+FINANCE request, FINANCE ended → `FAILED EXPORT_RIGHTS_LOST`, no object; store-level and authority-level (`TestExportDataAuthorityFailsClosed`) equivalents; unit `TestWorkerRechecksTheKindReadPermission` |
| Data permission lost after `READY` | access and content `FORBIDDEN subs.plan.read`, no `EXPORT_FILE` record; restored grant serves the exact bytes once |
| Auditor reads a foreign record | other organization's list/settings, a foreign employee, unknown asset/document, a colleague's export (read/access) and the own export under another organization path: `NOT_FOUND`; non-participant ownership read/accept/cancel `NOT_FOUND` |
| Document bytes through general read authority | SUPERVISOR and UNIT-scoped AUDITOR `FORBIDDEN doc.document.view_content`, no access record; AUDITOR exact bytes with one `ISSUED` and one `SERVED`; export kinds cannot declare content or location (`TestExportableReadAdmitsOnlyRegisteredDataReads`, `TestRegistryRequiresAnExportableReadPermission`, authority test) |
| Role revoked between request and worker run | AUDITOR role ended → `FAILED EXPORT_RIGHTS_LOST`, no object (plus the existing FINANCE-ended and suspended-membership cases) |
| Stale proof | AUDITOR export request with a 20-minute-old session `FRESH_AUTH_REQUIRED` |
| Personal boundaries | own inbox list/preferences 200; another member's inbox, preferences, self-device list and leave read `NOT_FOUND` |

## Validation

Final race run after the last (test-only) edit,
`go test -count=1 -race -tags=integration -json -covermode=atomic
-coverpkg=./internal/modules/orgexport/...,./internal/modules/organization/adapters
./internal/modules/orgexport/... ./internal/modules/organization/adapters -run
'TestAuditorReadWriteMatrixActualCompanyHTTP|TestExportAndContentGatesActualCompanyHTTP|TestExportDataAuthorityFailsClosed|TestExportsThroughActualCompanyHTTPAndWorker|TestKindReadPermissionIsCheckedInEveryTransaction|TestExportLifecycleOnRealStorage|TestRightsLossAndCeilingsPublishNothing|TestExportAuthority|TestExportReadsAreRequesterBound|TestExportRequestIsAudited|TestAuthorizationMatrixActualCompanyHTTP'`:
11 top-level / 3072 subtests pass, 0 failures/skips (auditor walk 15.1 s,
200-cell matrix unchanged at 77 allowed / 123 denied, 53.7 s). Log
`/data/dev/worktrees/f3-logs/auth011-go-green-integration-final.jsonl`.

Coverage, unit and integration profiles merged
(`auth011-go-cover-unit.out`, `auth011-go-cover-integration.out`):
`orgexport/application` 86.1%, `orgexport/adapters` 73.9%,
`orgexport/adapters/http` 84.2%, `orgexport/domain` 93.8% (module 82.1%);
`ports.go` 90.0%, `ExportableRead`/`NewRegistry`/`Registry.ReadPermission`
100%; `export_authority.go` 65.6% with `CheckDataTx` 87.5% (uncovered lines
are the fail-closed provider-error branches of `covers` and the pre-existing
`CheckExportTx`/`CheckRequesterTx` source failures).

Affected race union, `go test -count=1 -race -tags=integration -timeout 45m
-p 3 -json ./internal/modules/orgaccess/... ./internal/modules/organization/adapters
./internal/modules/orgexport/... ./internal/modules/document/...
./internal/modules/orgmedia/... ./internal/modules/orgaudit/...
./internal/modules/review/... ./internal/platform/apicontract ./cmd/api` with
PostGIS and MinIO: 27 packages pass, 754 top-level / 6216 subtests pass,
0 failures, 0 skips (organization/adapters 1030.4 s, apicontract 752.7 s,
cmd/api 154.6 s; 22:28–22:46 UTC). Log
`/data/dev/worktrees/f3-logs/auth011-go-race-union.jsonl`. It started two
minutes before the last edit of the auditor walk (adding the 30-table digest,
test-only), which the final race run above covers.

Untagged whole module (CI): `go test -count=1 -timeout=45m -json ./...` —
211 packages pass, 77 without tests, 2626 top-level / 7399 subtests pass,
0 failures, 4 pre-existing skips (`identity/profile` TestGenerateVectors,
`enterpriseexport/portalgen` ×2, `quality/domain`). Log
`/data/dev/worktrees/f3-logs/auth011-go-untagged-all.jsonl`.

Gates, all passing:

```sh
go build ./... && go vet ./...                      # clean (auth011-build-vet.log)
go vet -tags=integration ./internal/modules/orgaccess/... ./internal/modules/organization/... ./internal/modules/orgexport/... ./internal/modules/document/... ./internal/modules/orgmedia/... ./internal/modules/orgaudit/... ./internal/modules/review/... ./internal/platform/... ./cmd/api ./cmd/worker   # clean
gofmt -l ./internal ./cmd ./db                      # empty
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/orgexport/... ./internal/modules/organization/adapters/... ./internal/modules/orgaccess/... ./internal/platform/enterprisesuite ./internal/platform/apicontract ./internal/platform/orgapi ./cmd/api ./cmd/worker   # no findings
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/orgexport/... ./internal/modules/organization/adapters ./internal/modules/orgaccess/... ./internal/platform/apicontract ./cmd/api ./cmd/worker ./internal/platform/enterprisesuite   # no findings (auth011-staticcheck.log)
/home/iiii/go/bin/sqlc diff && /home/iiii/go/bin/sqlc vet   # clean, no SQL change (auth011-sqlc.log)
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check   # repo root: 0 errors, 942 warnings / 611 info, identical to the base head (auth011-vacuum.log)
python3 docs/enterprise/tools/check_plan.py         # repo root: 348 tasks, 545 rules, 167 use cases, 145 screens, 0 problems
git diff --cached --check                           # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'   # see below
```

Secret/PII scan: the hits are the export capability vocabulary
(`capability.Token`, `tokens.issued`), storage configuration field names read
from the test environment (`SecretAccessKey`), the repeated-byte cursor
secret of the test router and the scan command in this file; no credential,
key, account or personal data is committed. Identifiers are generated UUIDv4s
or synthetic `7b0e2a54-…` values; the document fixture is the existing
synthetic 4×4 JPEG. No dependency was added.

## Retained obligations

- Unrouted documented auditor reads (`trip.trip.read`, `gps.track.read`
  recorded tracks only, `fuel.fueling.read`, `exp.expense.read`,
  `kpi.dashboard.read`, `audit.event.read`) are proven only as catalog
  descriptors; their owning route tasks must add auditor positive cases (the
  walk fails until a new route under a documented AUDITOR key gets one).
- Finance period read for AUDITOR needs a matrix revision and a new key
  (EDR-149/EDR-166); not added.
- EMP-RPT-002: real formats declare `ReadPermission()`; sensitive columns
  (BR-RPT-003: location needs `gps.track.read`, files need
  `doc.document.view_content`) need per-column checks, omission notes and
  their own audit; row scoping for UNIT/COST_CENTRE/TEAM/SELF readers is
  absent, so only organization-wide data grants export today.
- F6 full export (BR-RPT-012: `org.settings.manage` + `rpt.export.run`,
  owners notified, documents by month), schedules (BR-RPT-014) and the audit
  viewer are out of scope.
- EMP-BACK-003 support sessions are not a role and gain nothing here.
- Integration: DOC-003 and FLEET-006 add routes in parallel. The walk derives
  its routes from the inventory and its bodies from the contract, so a new
  route under a key AUDITOR lacks becomes a `FORBIDDEN` case automatically;
  the literal counts (103/22/81) and, for a route under a documented AUDITOR
  key, a positive case must be updated at integration. A contract example
  that does not decode for its DTO would surface as 400 there.
- No F3, M3 or C1 acceptance is claimed; LOCAL_DONE only, no push/PR/CI.

## Proposed shared-doc changes

1. `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`: append a section titled
   "EMP-AUTH-011 — Auditor read-only role and export permission gating
   (2026-11-14)" with a link to this evidence and the CONTRACT block above
   verbatim.
2. `docs/enterprise/16-decisions.md`: append the EDR block above (EDR-166,
   EDR-167) after the last decision row.
3. `docs/enterprise/backlog/ep03-identity-access.md`: EMP-AUTH-011 `state
   LOCAL_DONE` and the BACKLOG block below after its task block.
4. `docs/enterprise/07-permissions.md`: in the status line add "EMP-AUTH-011
   LOCAL_DONE" linked to `evidence/f3-auth011-auditor-export-gating.md`; in
   §7.2 change the `rpt.export.run` row to "Run exports and reports; each
   export kind also needs the read permission it declares (EMP-AUTH-011)";
   append to §7.4: "EMP-AUTH-011 proves the AUDITOR column on every routed
   organization route without a catalog change (EDR-166): documented reads
   succeed, the export request and the own ownership response are the only
   write-shaped routes it reaches, and every other route is `FORBIDDEN` with
   its key. `org.ownership.respond` stays a personal consent key; period
   reads stay under `fin.period.close` (EDR-149)."
5. `docs/enterprise/modules/intelligence.md` §5, after "Every action also
   needs the read permission of what it shows …": "For exports this is
   enforced by EMP-AUTH-011: every admitted kind declares one registered
   `<module>.<resource>.read` key outside location, checked with
   `rpt.export.run` in the request transaction, at each worker attempt and at
   each capability issue and download (EDR-167)."
6. `docs/enterprise/backlog/ep13-intelligence.md` EMP-RPT-002 "Risks and
   assumptions": add "Each format implements `Format.ReadPermission()` with
   the list's read key (EMP-AUTH-011); sensitive columns keep their own
   per-column permission and omission note."
7. `docs/enterprise/09-architecture.md` `orgexport` row: "formats declare the
   read permission of their data; request, worker and delivery re-check it
   (EMP-AUTH-011)".
8. `backend/README.md`: none — `CheckDataTx` reads only what the company
   pipeline already reads (`org_memberships`, `org_role_assignments`, the
   onboarding time function); no new runtime grant.
9. `PROGRESS.md`: EMP-AUTH-011 LOCAL_DONE / INTEGRATION_PENDING with this
   evidence link; EMP-RPT-002 dependency satisfied locally.
