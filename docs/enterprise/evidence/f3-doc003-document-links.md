# EMP-DOC-003 — Links between documents and operations

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F3 API task (module DOC, cut C1, P0, size S) on
`codex/enterprise-f3-doc003`, opened from the integrated head with migrations
through `000207` (EMP-DOC-002 `org_documents` is `000205`). It persists
same-organization associations between an EMP-DOC-002 document and the target
records that exist in the schema today, exposes them through three guarded
company routes and offers a Go link port that operation consumers (FUEL-002
receipts, meter evidence) call inside their own transaction. Synthetic C1 data
only. No Kotlin/Android, panel, job, allocation, conference or fiscal command
is added; `orgaccess` internals and `document/domain/fiscal` are not touched.

Sources read: the EMP-DOC-003 task block and the DOC-004/005/007/015 blocks of
[EP09](../backlog/ep09-documents.md); FUEL-002/011 in
[EP08](../backlog/ep08-fuelings.md); [06-domain-model](../06-domain-model.md)
§§3–5; [10-offline-sync](../10-offline-sync.md) §§3–5 (BR-OFFL-007,
BR-OFFL-015, UC-OFFL-03); BR-TEN-003/005/006/007, BR-AUD-001
([04-business-rules](../04-business-rules.md)); BR-PRIV-004
([11-privacy-security](../11-privacy-security.md)); the
[documents module](../modules/documents.md) (BR-DOC-002/003, §7 "Capture or
association requires the owning record's permission and a scoped target");
[fleet](../modules/fleet.md) (BR-FLEET-009/036, BR-METER-025, permission table:
`fleet.asset.manage` covers "document links"); [fuelings](../modules/fuelings.md)
(receipt through `doc.document.attach`, content through
`doc.document.view_content`); the [approved receipt scope](f0-m0-decisions.md#private-receipts);
the EMP-DOC-001 domain ([evidence](f3-doc001-document-domain.md), EDR-134..136)
and EMP-DOC-002 persistence ([evidence](f3-doc002-document-persistence.md),
EDR-152/153); migrations `000170` (employee identities), `000197`/`000201`/
`000206` (assets and the asset usage source), `000198`/`000199` (stored and held
meter readings) and `000205`; the authorization catalog v4.

IDs used: B-BR-EMP-239..242, BUC-EMP-122..123, EDR-162..163 (all of them; no
pre-allocated ID is left unused). Migration `000209_*` (append-only; `000208`
is reserved by the parallel EMP-AUTH-006 task and the coordinator may renumber
this file at integration — every test derives the version from one file-name
constant and builds the previous chain as `name < thisMigration`).

## Rules frozen before behaviour

Observed before any change: `org_documents` (000205) is the only document
table; no link, allocation, expense, fueling, trip or work-order table exists.
Of the seven EMP-DOC-001 target kinds only three have tables: `ASSET`
(`org_assets`, whose DRAFT rows may be physically discarded when the asset
usage source `org_asset_current_usage` reports no use and which refuses any
foreign key it does not list), `EMPLOYEE` (`org_employee_identities`, history
never deleted) and `METER_READING` (insert-only `org_meter_readings` and the
held `org_meter_held_readings`, which share one reading identity per
organization). The catalog grants `doc.document.attach` ("upload and link
documents to own records") S to OWNER, ADMIN, FLEET_MANAGER, SUPERVISOR,
DRIVER and OPERATIONS_STAFF and O to FINANCE; no role holds
`doc.document.read`.

Proposed contract section title: "EMP-DOC-003 — Links between documents and
operations".

Rules B-BR-EMP-239, B-BR-EMP-240, B-BR-EMP-241, B-BR-EMP-242, BUC-EMP-122, BUC-EMP-123 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-DOC-003 — Links between documents and operations" (frozen before behaviour; text moved verbatim at integration).

### Decision rows

Decisions EDR-162, EDR-163 are recorded in [16-decisions](../16-decisions.md).

## Delivered surface

- Migration `backend/db/migrations/000209_org_document_links.sql` (additive,
  append-only, transactional; compatibility floor stays `000132`). Table
  `org_document_links` keyed by `(org_id, document_id, target_kind,
  target_id)`; unique `(org_id, operation_id)` with a deferred foreign key to
  its audit event; composite same-organization foreign keys to
  `org_documents`, `org_assets`, `org_employee_identities`,
  `org_meter_readings` and `org_meter_held_readings(org_id, reading_id)`; the
  `org_document_links_binding` CHECK ties each kind to exactly its reference
  column; index `(org_id, target_kind, target_id, document_id)`; forced RLS
  with USING/WITH CHECK and `REVOKE ALL FROM PUBLIC`; the row guard
  `org_document_link_guard()` (organization setting, the
  `document.link.created` event targeting the document, documents in
  `FAILED`/`REPLACED`/`DELETED` and `DRAFT` assets refused, database
  `created_at`, UPDATE refused 55000) and the retention trigger
  `org_document_link_keep()` (DELETE/TRUNCATE 55000). It replaces the 206
  `org_asset_current_usage` source only to list
  `org_document_links_asset_fkey` (an association, not use); every other
  foreign key to `org_assets` still answers P7904. No runtime grant is
  seeded.
- `backend/db/queries/document/links.sql` (+ generated `links.sql.go`) and the
  `000209` schema entry of the `document` stanza in `backend/sqlc.yaml`.
- `backend/internal/modules/document/application/links.go`: `Binding`
  (`ASSET`, `EMPLOYEE`, `METER_READING`, `HELD_METER_READING`, each owned by one
  kind), `ResolvedTarget`, the `TargetResolver` port, the closed `Targets`
  registry (`Resolver`, `Policy`: owner permission, `organizations`,
  `Read`/`OperationalWrite`, never fresh, never a `doc.*` permission),
  `TargetRef`, `Link` (validated through EMP-DOC-001 `NewTarget`; `Digest`),
  `LinkView`, `Linked`, `LinkPage`, the page filters, the `Linker` port
  (`LinkTx`), `LinkRepository`, `LinkReadPolicy` (`doc.document.attach`,
  read) and the `Service.Link`, `DocumentLinks`, `TargetLinks` methods
  (default off; validation before any repository call). `Service.Links` and
  `Authority.CheckTargetTx` were added; every private value redacts fmt, slog
  and JSON.
- `backend/internal/modules/document/adapters/links.go`: `Store.LinkTx` (the
  port in the caller's transaction: document row lock `FOR UPDATE`, document
  side `doc.document.attach` over the document's facts, the EMP-DOC-001
  `Document.Link` decision, storage-state refusal, target side through the
  owner policy with lazy resolution, replay by operation, existing pair, one
  audited insert), `Store.Link` (own scoped transaction), `DocumentLinks` and
  `TargetLinks` (keyset pages, both endpoints rechecked in the read
  transaction); `CurrentAuthority` accepts `LinkReadPolicy` and gains
  `CheckTargetTx` (orgaccess `Authorization.Prepare` over the borrowed
  transaction with a lazy resolver); `AuditDefinitions` registers
  `document.link.created` (target type `document`).
- Owner resolvers: `backend/internal/modules/fleet/adapters/document_targets.go`
  (`DocumentTargets`: `ASSET` → `fleet.asset.manage`, `DRAFT` refused;
  `METER_READING` → `meter.reading.record`, stored reading author or held
  operation member) and
  `backend/internal/modules/membership/adapters/document_targets.go`
  (`EmployeeDocumentTargets`: `EMPLOYEE` → `idn.employee.manage`).
- `backend/internal/modules/document/adapters/http/links.go`: `LinkRecords`
  (record gate: link creation and the document list on an existing
  document, the target list on the subject's own root) and the three guarded
  company routes, mounted only when `Service.Links` is configured, with sealed
  cursors bound to route, document or target and limit.
- Composition `enterprisesuite.registerDocuments`: the registry
  `{ASSET, METER_READING: fleet, EMPLOYEE: membership}` on the document store,
  which is both `Repository` and `Links`.
- Contract and inventories: three operations and seven schemas in
  `contracts/openapi/v1.yaml` (target kinds enumerated as the three registered
  ones); `authorization-routes-v1.json` routes the three paths under
  `doc.document.attach` (representative unchanged); the inventory test counts
  (routed 20, signed 1, not routed 64, normal 103 → 106, special 24); the
  full-router mount list gains the three paths; the authorization matrix keeps
  its 200 cells (77 allowed / 123 denied: no representative changed) and its
  private-state snapshot now includes `org_document_links`.

## RED → GREEN → REFACTOR

Stubs compiled with the exported API (`errNotImplemented` application
bodies, store, resolver and authority methods answering `UNAVAILABLE`/
`INVALID`, `registerLinks` mounting nothing); the contract, inventory and
router expectations were written first.

- Unit RED (`/data/dev/worktrees/f3-logs/doc003-go-red.jsonl`, `-race`,
  `document/...`, `apicontract`, `cmd/api`, `enterprisesuite`): 131 top-level
  tests, 34 fail and 97 pass; 804 subtests, 1 fails and 803 pass; 0 skips;
  longest package `apicontract` 269 s. Failures: the six link application
  tests, three `cmd/api` tests (the authorization inventory and two full
  router/contract tests) and 25 `apicontract` tests whose composed router
  lacked the newly contracted link operations. The
  redaction, link record-gate refusal and link contract tests passed against
  the stubs (types, schemas and the gate's refusals were written with them).
- Integration RED, migration held out of the chain
  (`doc003-go-red-integration.jsonl`, `-race -tags=integration`): 6
  top-level, 6 fail (missing migration in the embedded chain; missing
  `org_document_links` for the five composed tests); 0 skips.
- Integration RED, migration and generated queries present, behaviour still
  stubbed (`doc003-go-red2-integration.jsonl`): 6 top-level, 6 fail — the
  upgrade test passed every migration, failure-recovery and compatibility
  assertion and failed only at the post-upgrade link (`unavailable`); the
  store, port, concurrency and schema tests failed on the stubbed store and
  the HTTP test on the unmounted route (404).
- GREEN, first implementation: every link test passed except one fixture
  call that used the superuser pool with `WithOrgTx` (refused by the harness
  as an unsafe runtime role); it now uses the ordinary runtime pool.
- Unit GREEN (`doc003-go-green.jsonl`, `-race`, coverage
  `doc003-go-unit-cover.out`; `document/...`, `cmd/api`, `enterprisesuite`,
  `fleet/...`, `membership/...`): 221/221 top-level and 1099/1099 subtests,
  0 skips. In that run `apicontract` hit the default 10-minute timeout under
  host load (load average 14–24 from parallel agents) with coverage
  instrumentation; rerun with `-timeout 45m`
  (`doc003-go-green-apicontract.jsonl`): 73/73 top-level, 710/710 subtests,
  718.8 s.
- Integration GREEN, affected union (`doc003-go-green-integration.jsonl`,
  `-race -tags=integration -p 3`; document, orgmedia, orgaudit, fleet,
  membership, `cmd/api`, `migrate`, `enterprisesuite`): 323 top-level pass and
  1 fail, 1154 subtests pass. The failure was the EMP-DOC-002 schema test's
  owner `TRUNCATE org_documents` (now 0A000 because links reference
  documents); the same reference also affected the held-review constraints
  test. Both were adjusted as described above and rerun:
  `document/...` (`doc003-go-green-integration-document.jsonl`, coverage
  `doc003-go-integration-cover.out`) 43/43 top-level, 280/280 subtests; the
  rest of the union 281/281 top-level, 874/874 subtests.
- Composed link suite with coverage (`doc003-go-green-links.jsonl`, coverage
  `doc003-go-links-cover.out`; the five link tests, the EMP-DOC-002 company
  API test and the held-review constraints test): 7/7 top-level, 5/5
  subtests.
- `organization/adapters` full (`doc003-go-integration-organization.jsonl`,
  `-race -tags=integration -timeout 45m`): 380/380 top-level, 3097/3097
  subtests, 807.6 s; the authorization matrix keeps its 200 cells.
- Coverage (union of the three profiles, statements): `document/application`
  95.6% (`links.go` 98.7%), `document/adapters` 81.8% (`links.go` 83.6%,
  `authority.go` 72.7%), `document/adapters/http` 79.3% (`links.go` 78.8%),
  `document/domain` 99.5%; resolvers: fleet `document_targets.go` 77.4%,
  membership `document_targets.go` 63.2% (uncovered lines are the
  unavailable-transaction and wrong-kind guards).
- REFACTOR: mutation sanity with sources restored byte-identical (SHA-256
  checked): removing the target-side authority, taking a shared instead of an
  exclusive document lock, skipping the existing-pair lookup and dropping the
  binding CHECK each fail at least one test
  (`TestDocumentLinksResolveRealTargetsAndCheckBothEndpoints` —
  "expected fleet.asset.manage denial", `TestConcurrentDuplicateDocumentLinksWriteOneRowAndOneEvent`
  — eleven errors, the same test — "duplicate link under another operation
  wrote", `TestDocumentLinkSchemaGuardsScopeReferencesAndRetention` —
  "binding-mismatch"). The port test then gained the signed-device actor case
  and was re-run green; the DRAFT-asset refusal is enforced twice (owner
  resolver and database guard).

## What the tests prove

Store, schema and port on real PostGIS with real targets
(`organization/adapters`, real accounts, memberships, current role
assignments through orgaccess `Prepare`, real ACTIVE and DRAFT assets, a meter
opening reading, a device-recorded stored reading, a held reading produced by
the real intake after a permission change, employees and a second real
organization; BUC-EMP-122):

- `TestDocumentLinksResolveRealTargetsAndCheckBothEndpoints`: a DRIVER links
  an own still-`UPLOADING` receipt to an own stored reading (one row bound to
  `reading_id`, one `document.link.created` event targeting the document under
  the creating operation and account) and to an own held reading (bound to
  `held_reading_id`); the identical retry is a replay, the same operation with
  another target is `OPERATION_ID_REUSED`, another operation naming the pair
  returns the first link with no row or event. Each endpoint's permission is
  enforced with its own key: the DRIVER lacks `fleet.asset.manage`,
  `idn.employee.manage` and `meter.reading.record` over another member's
  reading; a FLEET_MANAGER links an own receipt to an ACTIVE asset and to the
  DRIVER's reading but not to an employee, and is refused
  `doc.document.attach` on the DRIVER's receipt; a SUPERVISOR is refused on the
  document side and FINANCE on the target side; the OWNER links an employee. A
  DRAFT asset and a `FAILED` document are `CONFLICT`; forged kinds (an
  asset kind with a reading or employee UUID, a reading kind with an asset
  UUID), a missing reading, another organization's employee and document and a
  missing document are `NOT_FOUND`; `FUELING`, `EXPENSE`, `WORK_ORDER`,
  `TRIP`, `BOGUS` and a lower-case kind are `VALIDATION_FAILED`; a member of A
  inside B has no membership. Refusals write no row or event; the linked
  document's whole row (statuses, revisions, timestamps, `UNALLOCATED`) is
  byte-identical after linking and the link table has exactly its eleven
  reference/identity columns (no amount, content or locator). Lists page by
  kind and target, refuse another member's SELF document, show the SUPERVISOR
  both links of the reading with their pending storage, refuse the asset list
  without `fleet.asset.manage`, and hide foreign or unregistered targets.
- `TestDocumentLinkPortCommitsAndRollsBackWithTheConsumerTransaction`: inside a
  consumer transaction the port's link and event roll back with the
  consumer's refusal and commit with it; the same port links a held reading
  for a signed-device (`android`) actor, the FUEL-002 intake shape; a
  transaction scoped to another organization and a missing transaction are
  refused without writing.
- `TestConcurrentDuplicateDocumentLinksWriteOneRowAndOneEvent`: twelve
  concurrent identical commands give one first write and eleven replays; eight
  concurrent operations naming one new pair give one row and one event, all
  without error.
- `TestDocumentLinkSchemaGuardsScopeReferencesAndRetention`: forced RLS (no
  setting sees nothing and an unscoped insert is 42501; organization B sees
  nothing); as the ordinary login, a binding mismatch, an unregistered kind,
  a target differing from its reference and two reading references are 23514;
  an employee UUID under the asset reference, an asset UUID under the reading
  reference, another organization's document or employee, a missing event, an
  event of another action or document are 23503; a DRAFT asset and a `FAILED`
  document are 23514; a row of another organization is 42501; the baseline row
  passes with its deferred event key. UPDATE, DELETE and TRUNCATE are refused
  by the triggers even for the owner (55000) and the ordinary login cannot
  update (42501). The asset usage source still serves an asset with a link.
- `TestDocumentLinkMigrationPopulatedUpgradeFailureRecoveryAndPreviousCompatibility`
  (`document/adapters`): from the previous head with a populated pending
  document and its audit event, a broken `000209` leaves no table, function,
  ledger row, usage-source change or data change; the retry applies exactly
  the migration's version (derived from one file-name constant); previous and
  current chains check compatible; a re-apply is a no-op; previous rows and
  the `000132` floor are unchanged; the usage source lists exactly the new
  reference; the upgraded schema links the populated document to an employee
  through the ordinary login (only the employee parent is seeded below its
  register's guards, because this harness's organization root has no
  snapshot).

Composed company API (`organization/adapters`, real sessions, CSRF, roles,
capability flags, PostGIS and the disposable S3; BUC-EMP-123):

- `TestDocumentLinksOverTheCompanyAPI`: a FLEET_MANAGER links an own pending
  receipt to an ACTIVE asset (201, four response fields only) with replay,
  `OPERATION_ID_REUSED` and duplicate-pair answers, links it to the creator's
  meter opening reading and is refused a DRAFT asset (409); the DRIVER is
  refused with `meter.reading.record`, `fleet.asset.manage` and
  `doc.document.attach` keys; unregistered and unknown kinds are 422; a forged
  kind, a foreign employee, another organization's path and a missing
  document are 404; the OWNER links an employee. Document and target lists
  page through sealed cursors that refuse another limit or route (400); the
  DRIVER cannot list the manager's links. The SUPERVISOR sees the reading's
  link but is `FORBIDDEN` (`doc.document.view_content`) from the receipt, as is
  the DRIVER; the manager's pending receipt has no content (404) until the
  verified upload, after which the list shows `STORED`, only the manager
  obtains a capability and exactly one `ISSUED` record exists. Every refusal
  leaves the private-state snapshot (now including `org_document_links`)
  unchanged; linking left the document row unchanged; request logs carry no
  document UUID or capability.

Unit: bindings belong to exactly one kind; the registry is closed (unregistered
kinds `ErrInvalid`) and only builds non-fresh `Read`/`OperationalWrite` owner
policies, never a `doc.*` or unknown permission; link commands, digests, pages
and cursors validate before any repository call; the service is default off;
private values redact; the link record gate refuses every other policy and
shape and the upload gate refuses the link read policy; the OpenAPI schemas
refuse caller authority, amounts, allocations, content, digests and
unregistered kinds, and the three routes are non-fresh `doc.document.attach`
company routes.

Existing suites adjusted for the new references (no weaker assertion): the
EMP-DOC-002 schema test and the EMP-OFFL-008 held-review constraints test
probed a plain `TRUNCATE` of `org_documents` / `org_meter_held_readings`,
which PostgreSQL now refuses earlier (0A000) because links reference them;
both now also assert that refusal and probe `TRUNCATE ... CASCADE`, which
reaches the link table too and must still meet the retention triggers
(55000).

## Validation

Go 1.27.4 toolchain, staticcheck under Go 1.27.1, sqlc v1.31.1, vacuum;
commands from `backend/` unless noted, with
`GOCACHE=/data/dev/cache/f3-go-build GOTMPDIR=TMPDIR=/data/dev/cache/f3-tmp
STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck`. PostGIS: the owned
disposable control database on `127.0.0.1:32775` (PostGIS 3.6) through the
`orgtest` harness, which created and dropped its own child databases and
ordinary runtime logins. S3: the coordinator-owned disposable MinIO (bucket
`validation-private`) through the explicit `ANPFUEL_TEST_STORAGE_*` variables.

```sh
gofmt -l ./internal ./cmd ./db                                    # empty
go build ./... && go vet ./... && go vet -tags=integration ./...   # clean
go test -count=1 ./...        # 210 packages ok, 77 without tests; 2605 top-level + 7066 subtests pass, 0 fail, 4 pre-existing skips
go test -race -count=1 ./internal/modules/document/... ./cmd/api/ ./internal/platform/enterprisesuite/ ./internal/modules/fleet/... ./internal/modules/membership/...   # 221/221, 1099/1099
go test -race -count=1 -timeout 45m ./internal/platform/apicontract/   # 73/73, 710/710, 718.8 s
go test -race -count=1 -tags=integration -p 3 -timeout 45m ./internal/modules/document/... ./internal/modules/orgmedia/... ./internal/modules/orgaudit/... ./internal/modules/fleet/... ./internal/modules/membership/... ./cmd/api/ ./internal/platform/migrate/ ./internal/platform/enterprisesuite/   # see RED → GREEN (document rerun 43/43, 280/280)
go test -race -count=1 -tags=integration -timeout 45m ./internal/modules/organization/adapters/   # 380/380, 3097/3097, 807.6 s
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck [-tags=integration] <document, fleet/adapters, membership/adapters, enterprisesuite, apicontract, cmd/api, organization/adapters>   # no findings
/home/iiii/go/bin/sqlc generate && /home/iiii/go/bin/sqlc vet && /home/iiii/go/bin/sqlc diff   # clean
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check   # (repo root) 0 errors; 945 warnings vs 942 at the base: only the house-style POST requestBody warning on the 3 new operations
python3 docs/enterprise/tools/check_plan.py      # (repo root) tasks 348; rules 545; use cases 167; screens 145; problems 0
git diff --cached --check                         # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'
git diff --cached --binary | gitleaks stdin --redact --no-banner   # no leaks found
```

Logs in `/data/dev/worktrees/f3-logs/`: `doc003-go-red.jsonl`,
`doc003-go-red-integration.jsonl`, `doc003-go-red2-integration.jsonl`,
`doc003-dev1.jsonl` (first GREEN attempt), `doc003-go-green.jsonl`,
`doc003-go-green-apicontract.jsonl`, `doc003-go-green-integration.jsonl`,
`doc003-go-green-integration-document.jsonl`, `doc003-go-green-links.jsonl`,
`doc003-go-integration-organization.jsonl`,
`doc003-go-backend-untagged.jsonl`, `doc003-go-staticcheck.log` (empty);
coverage profiles `doc003-go-unit-cover.out`,
`doc003-go-integration-cover.out`, `doc003-go-links-cover.out`. The four
skips are the pre-existing data/vector generators (`identity/profile`,
`enterpriseexport/portalgen` ×2, `quality/domain`). The keyword scan matches
only the test loop variable `secret` over identifiers asserted absent, the
existing `Tokens` service field in context lines and the word "token" in rule
text; no credential, key or personal data. All identifiers are synthetic
`7b0e2a54-…` UUIDs or generated UUIDv4s; documents are synthetic receipts.
Migration `000209` SHA-256
`ca287dae5112e1cee6f2739b24076f9b756a72b6ea1d69e012cef57557cd2b15`. The
production migration owner must own `org_asset_current_usage(uuid,uuid)` (it
is replaced) and hold REFERENCES on the five parent tables. No dependency,
Kotlin, Android, panel, job or worker is added; `orgaccess` internals and
`document/domain/fiscal` are untouched.

## Retained obligations

- Kinds without tables: `FUELING` (EMP-FUEL-002), `EXPENSE` (EMP-EXP),
  `WORK_ORDER` (EMP-MAINT) and `TRIP` (EMP-TRIP) are refused until each owning
  task adds its reference column and foreign key in a new migration (widening
  the kind and binding CHECKs) and registers a resolver with its own
  permission; FUEL-002 then calls `Linker.LinkTx` inside the fueling
  transaction (and the meter evidence path inside the reading transaction).
- Owner read surfaces: the asset detail (SCR-FLEET-02), meter history
  (SCR-METER-02), fueling review (SCR-FUEL-05) and the DOC-015 receipt viewer
  show links under their own read permissions; members without
  `doc.document.attach` (APPROVER, AUDITOR) have no link list in C1. A
  metadata read under `doc.document.read` stays unrouted (no role holds it).
- Unlink, correction and replacement successors (EMP-DOC-011), retention and
  erasure (EMP-DOC-012): links are insert-only here.
- Android producer: the routes are company-cookie routes; the signed device
  link path (offline attachment references, `ORPHAN_REFERENCE` handling for a
  refused target) belongs to EMP-DOC-004 with EMP-FUEL-011/EMP-OFFL-005. The
  port already accepts the `android` actor shape.
- Allocation and reconciliation (EMP-DOC-007/010): a link is not allocatable
  and no allocation table exists.
- Runtime grants are operator gates: `SELECT, INSERT` on `org_document_links`
  and the target columns the resolvers read (see the proposed README text).
- Real receipts stay legally gated (OQ-04, LV-04, LV-11). Integration:
  LOCAL_DONE only; no push, PR, CI, staging, F3, M3, C1 or release claim.

## Proposed shared-doc changes

1. `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`: append a section titled
   "EMP-DOC-003 — Links between documents and operations" holding the
   CONTRACT block above.
2. `docs/enterprise/16-decisions.md`: append the EDR block above (EDR-162,
   EDR-163) after the last existing decision row.
3. `docs/enterprise/backlog/ep09-documents.md`: set EMP-DOC-003 `state
   LOCAL_DONE` and add the BACKLOG block below under its task block.
4. `docs/enterprise/modules/documents.md`, after the EMP-DOC-002 sentence of
   the status paragraph, add: "EMP-DOC-003 persists owned same-organization
   links (`org_document_links`, migration 000209) to assets, employees and
   stored or held meter readings through a closed registry of owner
   resolvers, with a link port for operation consumers and three company
   routes ([evidence](../evidence/f3-doc003-document-links.md)); fueling,
   expense, work-order and trip links retain their owning tasks."
5. `docs/enterprise/09-architecture.md` module table, `document` row: status
   "(pure domain, persistence and links LOCAL_DONE; jobs and the Android
   producer proposed)"; owned data adds "`org_document_links` (migration
   000209), the `Linker` port and the `TargetResolver` registry"; dependencies
   add "owner resolvers in `fleet` and `membership`".
6. `backend/README.md`, new subsection after "Organization export jobs
   (EMP-RPT-001)":

   ```markdown
   ### Organization document links (EMP-DOC-003)

   Migration209 adds the forced-RLS, insert-only `org_document_links` table and
   lists its asset reference in `org_asset_current_usage`. The API runtime role
   needs `GRANT SELECT, INSERT ON org_document_links`, the existing document
   grants (`SELECT, INSERT, UPDATE ON org_documents` — UPDATE only for the
   `FOR UPDATE` row lock that serializes link writes — and `SELECT, INSERT ON
   org_audit_events`) and the target columns the owner resolvers read:
   `GRANT SELECT(org_id,id,state) ON org_assets`, `GRANT
   SELECT(org_id,id,author_membership_id) ON org_meter_readings`, `GRANT
   SELECT(org_id,reading_id,operation_id) ON org_meter_held_readings`, `GRANT
   SELECT(org_id,operation_id,membership_id) ON org_sync_operations` and `GRANT
   SELECT(org_id,id) ON org_employee_identities`. Links are never updated or
   deleted; only ASSET, EMPLOYEE and METER_READING targets are registered.
   ```
