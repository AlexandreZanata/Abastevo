# EMP-DOC-002 — Document persistence, upload session and audited private access

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F3 API task (module DOC, cut C1, P0, size M) on
`codex/enterprise-f3-doc002`. It persists the EMP-DOC-001 container, opens a
member-bound upload session whose bytes pass through the EMP-PLAT-006
`orgmedia` verified upload, and serves private originals only through the
EMP-PRIV-005 audited capability. Synthetic C1 receipts only; real receipts
stay legally gated (OQ-04, LV-04, LV-11). No Kotlin/Android, panel, job,
link, allocation, conference or fiscal command is added.

Sources read: the EMP-DOC-002 task block and the DOC-003/004/008/014 blocks
of [EP09](../backlog/ep09-documents.md); the [documents module](../modules/documents.md);
[11-privacy-security](../11-privacy-security.md) §§4–5 (BR-PRIV-004/008/020,
UC-PRIV-05); [10-offline-sync](../10-offline-sync.md) §5 (UC-OFFL-03);
BR-TEN-006 and BR-AUD-003 ([04-business-rules](../04-business-rules.md));
OQ-04 and the [approved receipt scope](f0-m0-decisions.md#private-receipts);
the EMP-DOC-001 domain and [evidence](f3-doc001-document-domain.md); the
`orgmedia` module and [EMP-PLAT-006 evidence](f1-plat006-private-document-storage.md);
the `orgaudit` access log and [EMP-PRIV-005 evidence](f3-priv005-sensitive-access-log.md);
the authorization catalog v4 (`doc.document.attach`,
`doc.document.view_content`), the `orgaccess` guarded route and
`Authorization.Prepare`, and the fleet asset/meter route precedent.

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-219..222, BUC-EMP-112..113, EDR-152..153. All are
used; none remain reserved.

Proposed contract section title: "EMP-DOC-002 — Document persistence, upload
session and audited private access".

Rules B-BR-EMP-219, B-BR-EMP-220, B-BR-EMP-221, B-BR-EMP-222, BUC-EMP-112, BUC-EMP-113 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-DOC-002 — Document persistence, upload session and audited private access" (frozen before behaviour; text moved verbatim at integration).

### Decision rows

Decisions EDR-152, EDR-153 are recorded in [16-decisions](../16-decisions.md).

## Delivered surface

- Migration `backend/db/migrations/000205_org_documents.sql` (additive,
  append-only; compatibility floor stays `000132`; 000201/000203/000204 are
  reserved by parallel tasks). Table `org_documents` with organization root
  and creation-event (`org_audit_events`, deferrable) foreign keys, unique
  `(org_id, operation_id)`, closed CHECKs, the C1 5 MiB JPEG budget, forced
  RLS with USING/WITH CHECK, `REVOKE ALL FROM PUBLIC`, a row guard (scope,
  initial states, creation event `document.upload.created` targeting the row,
  immutable identity, derived reconciliation, newer fiscal consultation, one
  revision per changed dimension along `org_document_transition_allowed`) and
  a retention trigger refusing DELETE/TRUNCATE (55000). No runtime grant is
  seeded: runtime logins need `SELECT, INSERT, UPDATE` on `org_documents`.
- `backend/db/queries/document/documents.sql` (+ generated `documents.sql.go`,
  `models.go`, `db.go`) and the `document` stanza of `backend/sqlc.yaml`.
- `backend/internal/modules/document/application`: `Actor`, `Declaration`,
  `Create` (validated through EMP-DOC-001 `domain.New`, `orgmedia`
  `ByteLimit` and the C1 JPEG-only rule), `View`, `UploadAccess`, the
  `AttachPolicy` (`doc.document.attach`, operational write, not fresh) and
  `ContentPolicy` (`doc.document.view_content`, read) source policies, the
  `Authority`, `Repository`, `Media` and `UploadTokens` ports, and `Service`
  (default off; `Create`, `VerifyUpload`, `Upload`, `IssueContent`,
  `ReadContent`; the verified actor is bound to each orgmedia call through
  the context). Every private value type redacts fmt, slog and JSON.
- `backend/internal/modules/document/adapters`: `Store` (scoped transactions;
  creation through the audit runner, idempotent by operation; creator-bound
  `Pending`; `MarkStored` applies the domain's two storage commands under a
  row lock with one `document.upload.stored` event; `Facts` for the guarded
  resolver), `CurrentAuthority` (orgaccess `Authorization.Prepare` over a
  source borrowed from the document transaction), `MediaAuthority` (the
  `orgmedia` `Authority`: the object is projected from the row, ready only
  when `STORED`, upload creator-bound and pending only), `UploadTokens`
  (existing HMAC envelope, binding domain `document.upload.v1` over origin,
  organization and document, strict payload with a five-minute inner
  deadline) and `UnavailableObjects` for unconfigured storage.
- `backend/internal/modules/document/adapters/http`: guarded company routes
  `POST /v1/orgs/{org_id}/documents/uploads/create` (201),
  `.../{document_id}/content/access` and `.../{document_id}/content/read`
  (200), the `DocumentRecords` record gate and the `Ingress` handler for
  `.../{document_id}/content/upload`, which checks query, Origin and a single
  capability header, verifies the capability, the exact `image/jpeg` type and
  declared length before reading, sets a 15-second read deadline and limits
  the body.
- `backend/internal/platform/orgapi/document_ingress.go`
  (`DocumentIngressRule`, `DocumentIngressDescriptor`,
  `DocumentCapabilityHeader`); `apicontract.ValidateOrganizationRoutes` now
  accepts exactly that one boundary (path, rule, metadata, no cookie, required
  header, no query, one bounded binary JPEG body) and refuses its metadata on
  any other route.
- Composition `enterprisesuite.Register`: document and orgmedia audit
  vocabulary, a `doc.document` write admission class, the orgmedia service
  over `MediaAuthority`, the JPEG inspector, content tokens and
  `AccessAudit`, and the optional `Options.DocumentStorage` (production
  `cmd/api` passes none, so bytes and content answer `UNAVAILABLE`).
  `cmd/api/body_limit.go` allows exactly `5 MiB + 1` on the ingress path only.
- Contract and inventories: four operations and twelve schemas in
  `contracts/openapi/v1.yaml` (tag `enterprise-documents`);
  `authorization-routes-v1.json` routes `doc.document.attach` (1 route) and
  `doc.document.view_content` (2 routes) and adds the special route
  `DOCUMENT_INGRESS`; the inventory test counts (17 routed, 67 not yet
  routed, 85 normal routes, 20 special routes) and the full-router mount list
  follow; the authorization matrix gains both representatives (170 cells, 68
  allowed, 102 denied); `scripts/check-manifest.txt` registers the three new
  packages.

Relationship to `orgmedia` (EDR-152): `orgmedia.Object.DocumentID` is the
`org_documents.id`; digest, size, type and revision come only from the row,
so the storage key `org/<org>/doc/<document>/<sha256>` derives from the
document identity. There is no second media table or identity.

## RED → GREEN → REFACTOR

Stubs compiled with the exported API (`errNotImplemented` service and
adapter bodies, `Register` mounting nothing, `Ingress` answering 404) and the
migration held out of the chain.

- Unit RED (`/data/dev/worktrees/f3-logs/doc002-go-red.jsonl`,
  `-race`, document/..., apicontract, cmd/api, enterprisesuite): 113
  top-level tests, 36 fail and 77 pass; 768 subtests, 25 fail and 743 pass;
  0 skips; longest package 171 s. Failures: the six service tests, two
  upload-capability tests, two ingress tests, the four `cmd/api` route,
  inventory and body-limit tests, and 22 apicontract tests whose composed
  router lacked the newly contracted document operations. The redaction,
  record-gate and contract-example tests passed against the stubs (types,
  schemas and the ingress gate were written with the tests).
- Integration RED (`doc002-go-red-integration.jsonl`, `-race
  -tags=integration`, document/... plus the two composed HTTP tests in
  `organization/adapters`): 36 top-level, 20 fail and 16 pass; 280
  subtests, 24 fail and 256 pass; 0 skips. PostGIS tests failed on the
  missing table/migration, the S3 tests on not implemented, the HTTP and
  matrix tests on unmounted routes.
- Unit GREEN (`doc002-go-green.jsonl`, coverage
  `doc002-go-unit-cover.out`): 137/137 top-level and 918/918 subtests pass,
  0 skips. Coverage: `document/application` 93.4%, `document/domain` 99.5%,
  `document/adapters` 12.0% and `adapters/http` 42.8% (database and composed
  route paths are integration-only), `orgapi` 80.6%, `apicontract` 87.2%.
- Integration GREEN (`doc002-go-green-integration.jsonl`, `-race
  -tags=integration -coverpkg=./internal/modules/document/...`, document,
  orgmedia, orgaudit and orgapi): 113/113 top-level and 332/332 subtests
  pass, 0 skips; `document/adapters` 14.1 s. Coverage with PostGIS and S3
  (`doc002-go-integration-cover.out`): `document/application` 93.4%,
  `document/adapters` 75.1%, `document/domain` 99.5%; the HTTP adapter is
  exercised by the composed `organization/adapters` suites below.
- REFACTOR: the store error mapping was flattened (permission denials keep
  their key; 23505/23514/55000 conflict; 23503 not found) and the composed
  HTTP concurrency test now accepts the company session plane's own 401 for
  racing uses of one browser session while still requiring one row, one
  event and a sequential replay. OpenAPI response examples and parameter
  descriptions were added. Suites were re-run green afterwards. The
  B-BR-EMP-221 text was clarified during implementation: an unavailable
  access log answers `UNAVAILABLE` (503), not `NOT_FOUND`; nothing else in the
  frozen rules changed.
- Mutation sanity (sources restored byte-identical, checked by SHA-256):
  dropping the upload capability's five-minute inner deadline, marking every
  document ready, removing the stored-transition replay and removing the
  creation-event guard of the migration each fail at least one test
  (`TestUploadCapabilityRefusesLongInnerDeadlinesAndInvalidInputs`,
  `TestAuditedContentIsMemberBoundShortLivedAndFailsClosed`,
  `TestStoredTransitionIsCreatorBoundAuditedOnceAndReplaySafe`,
  `TestDocumentSchemaGuardsScopeIdentityRevisionsAndRetention`). The schema
  test gained the other-document creation-event probe from that check.

## What the tests prove

PostGIS and the disposable S3 bucket (`document/adapters`, role snapshots
from the real v4 catalog; BUC-EMP-112):

- `TestDocumentGraphInDatabaseEqualsTheDomainGraph`: for all 113 state pairs
  of the five commanded dimensions the SQL graph equals EMP-DOC-001
  `Allowed` (40 edges); reconciliation and non-canonical dimensions are never
  commanded.
- `TestDocumentSchemaGuardsScopeIdentityRevisionsAndRetention`: an unscoped
  ordinary login sees and updates nothing and its insert is refused (42501);
  another organization sees nothing; a foreign-organization row, a row
  without its creation event or under another document's event (23503), a
  non-initial row, identity/creator/operation/digest/size/type/creation-time
  changes, skipped or doubled revisions, silent revision bumps, REPLACED,
  reconciliation changes, fiscal changes without a newer consultation and
  out-of-graph conference moves (23514) are refused; DELETE/TRUNCATE are
  refused to the ordinary login (42501) and by the retention trigger even for
  the owner (55000); valid independent steps change exactly one dimension
  and revision; the schema holds no key, URL, name, token or content column.
- `TestDocumentMigrationPopulatedUpgradeFailureRecoveryAndPreviousCompatibility`:
  from the previous head with a populated audit event, a broken `000205`
  leaves no table, function or ledger row; the retry applies exactly the
  migration's version (derived from one file-name constant); the previous and
  current chains check compatible; a re-apply is a no-op; previous data and
  the `000132` floor are unchanged; the upgraded schema accepts a declaration.
- `TestDocumentCreateIsAuthorizedIdempotentAndScoped`: a DRIVER declares an
  own document at the initial container with one creation event; an
  identical retry is a replay with one row and one event; changed facts or
  another member under the same operation are `OPERATION_ID_REUSED`; another
  operation naming the document is `CONFLICT`; refusals leave no row or
  event; the same UUIDs coexist in organization B; an APPROVER (no attach)
  is refused with the permission key, a non-member and a suspended member
  are refused.
- `TestConcurrentDuplicateDeclarationsWriteOneRowAndOneEvent`: twelve
  concurrent identical declarations all succeed with exactly one first write;
  eight concurrent operations naming one document give one winner and seven
  `CONFLICT`.
- `TestStoredTransitionIsCreatorBoundAuditedOnceAndReplaySafe`: only the
  creator opens or completes the session; completion moves storage to
  `STORED` at revision 3 with one `document.upload.stored` event; a repeat
  writes nothing; losing the attach permission closes the session.
- `TestVerifiedUploadStoresOnlyTheDeclaredOriginalAndKeepsRefusalsPending`:
  the upload capability binds the creator and refuses another organization or
  document; digest-mismatched, oversized, truncated, interrupted and empty
  bodies, another member and another organization leave `UPLOADING` at
  revision 1; PDF-like bytes with a matching digest are refused by content
  inspection; the verified upload stores the exact bytes under
  `org/<org>/doc/<document>/`, invisible under organization B and to an
  unsigned client; a retry after `STORED` reads no byte and writes nothing;
  a declaration replay after `STORED` mints no capability.
- `TestAuditedContentIsMemberBoundShortLivedAndFailsClosed`: FLEET_MANAGER
  and the authoring DRIVER read the exact original with one `ISSUED` and one
  `SERVED` record each; a DRIVER on another member's document and a
  SUPERVISOR are `FORBIDDEN`; a foreign organization, non-member, pending or
  missing document is `NOT_FOUND`; another member's, organization's or
  document's use of a capability and the exact five-minute expiry serve
  nothing; with the access-log insert revoked no capability is minted and no
  byte is served, and restoring it restores access; a role change after
  issuance is `FORBIDDEN`; captured slog/log output, error strings and the
  audit rows contain no content marker, base64, digest, storage key,
  `/doc/`, `X-Amz`, media type or capability.

Composed company API (`organization/adapters`, real accounts, company
sessions, memberships, roles, capability flags, PostGIS and S3; BUC-EMP-113):

- `TestDocumentUploadSessionAndAuditedContentOverTheCompanyAPI`: the
  declaration (201) carries a capability expiring within five minutes;
  replay, `OPERATION_ID_REUSED` for changed facts or another member and 422
  for a PNG declaration; racing uses of one browser session produce one row
  and one event (the session plane may answer 401 to racers); ingress
  refusals (missing 401, forged 404, other organization 404, wrong type,
  digest mismatch, oversized, truncated 422, another document 404) leave
  `UPLOADING`; the verified upload stores once and replays; content access
  and read for FLEET_MANAGER and the authoring DRIVER, `FORBIDDEN` with the
  permission key for SUPERVISOR and for a DRIVER on the owner's document,
  `NOT_FOUND` across organizations, for missing and pending documents and for
  another member's or document's capability, 503 with no record while the
  access log is unavailable, `FORBIDDEN` after a role change, and exactly 4
  `ISSUED` / 3 `SERVED` records; request logs and metrics carry route
  templates only.
- `TestAuthorizationMatrixActualCompanyHTTP`: both document representatives
  in every ordinary role and every proof, origin, CSRF, organization,
  membership, assignment, freshness, capability, suspension, SELF-scope and
  ended-membership variant; refused requests leave documents, access records
  and audit unchanged.

Unit: the service validates the C1 declaration before any write, issues upload
capabilities only while pending, stores only after verified bytes, reads no
byte on replay or refusal and binds the verified actor to every orgmedia
call; the ingress verifies query, Origin, single capability, type and length
before reading any byte and maps orgmedia refusals; the record gate admits
only own declarations on the root and content on documents; tokens are bound
to origin, organization, document and member, expire within five minutes and
refuse other keys, binding domains, long inner deadlines, unknown fields and
tampering; the contract refuses caller-selected authority, storage locators
and unenabled formats, and the gate refuses every ingress drift.

## Validation

Go 1.27.4 toolchain, staticcheck under Go 1.27.1, sqlc v1.31.1, vacuum;
commands from `backend/` unless noted, with
`GOCACHE=/data/dev/cache/f3-go-build GOTMPDIR=TMPDIR=/data/dev/cache/f3-tmp
STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck`. PostGIS: the owned
disposable control database on `127.0.0.1:32775` (PostGIS 3.6) through the
`orgtest` harness, which created and dropped its own child databases and
ordinary runtime logins. S3: the coordinator-owned disposable MinIO
(`f3-s3-20261114`, bucket `validation-private`, CI test credentials), used
only through the explicit `ANPFUEL_TEST_STORAGE_*` variables.

```sh
gofmt -l ./internal ./cmd ./db                                   # empty
go build ./... && go vet ./... && go vet -tags=integration ./...  # clean
go test -count=1 ./...        # 199 packages ok, 73 without tests; 2493 top-level + 6778 subtests pass, 0 fail, 4 pre-existing skips
go test -race -count=1 -tags=integration ./internal/modules/document/... ./internal/modules/orgmedia/... ./internal/modules/orgaudit/... ./internal/platform/orgapi/   # 113/113 top-level, 332/332 subtests
go test -race -count=1 -tags=integration ./internal/modules/document/...   # final sources: 34/34 top-level, 280/280 subtests
go test -race -count=1 -tags=integration -timeout 45m ./internal/modules/organization/adapters/   # 341/341 top-level, 2647/2647 subtests, 685.4 s
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck [-tags=integration] <document, apicontract, orgapi, enterprisesuite, cmd/api, organization/adapters>   # no findings
/home/iiii/go/bin/sqlc generate && /home/iiii/go/bin/sqlc vet && /home/iiii/go/bin/sqlc diff   # clean
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check   # (repo root) 0 errors; only the house-style POST requestBody warning on the 4 new operations
python3 docs/enterprise/tools/check_plan.py      # (repo root) tasks 348; rules 545; use cases 167; screens 145; problems 0
git diff --cached --check                         # clean
git diff --cached | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'
git diff --cached --binary | gitleaks stdin --redact --no-banner   # no leaks found
```

Logs in `/data/dev/worktrees/f3-logs/`: `doc002-go-red.jsonl`,
`doc002-go-red-integration.jsonl`, `doc002-go-green.jsonl`,
`doc002-go-green-integration.jsonl`, `doc002-go-green-integration-final.jsonl`,
`doc002-go-green-http.jsonl` (both composed HTTP tests with
`-coverpkg=./internal/modules/document/...`: 2/2 top-level, 2394/2394
subtests), `doc002-go-integration-organization.jsonl`,
`doc002-go-backend-untagged.jsonl`, `doc002-go-staticcheck.log` (empty);
coverage profiles `doc002-go-unit-cover.out`, `doc002-go-integration-cover.out`,
`doc002-go-http-cover.out`. Union statement coverage of the three profiles:
`document/application` 93.4%, `document/adapters` 80.7%,
`document/adapters/http` 80.0%, `document/domain` 99.5%; document module
87.9%. The four skips are the pre-existing data/vector generators
(`identity/profile`, `enterpriseexport/portalgen` ×2, `quality/domain`).
The keyword scan matches only the capability/token vocabulary, the synthetic
HMAC fixture key (`synthetic-` ×5), the names of the disposable storage
variables, a fake `anonymous-synthetic` credential that must be refused and
the tests' `secrets` lists of values asserted absent; no credential, key or
personal data. All identifiers are synthetic `7b0e2a54-…` UUIDs or generated
UUIDv4s; the originals are the existing synthetic 4×4 JPEG with a synthetic
APP1 marker. Migration `000205` SHA-256
`ee138054c7156af5b0c1ad2ba63aa162f8c3287ffdab597f6f061da23f703287`.
No dependency, Kotlin, Android, panel, job or worker is added.

## Retained obligations

- Android producer: the declaration and content routes are company-cookie
  (panel) routes. The app capture/import path, its signed declaration and
  how the app obtains an upload capability belong to EMP-DOC-004 with
  EMP-OFFL-005; the ingress and `Actor` already accept the `android`
  channel shape but no Android route issues capabilities.
- Safety job and formats: PNG, PDF and XML stay refused at declaration and
  by the JPEG-only inspector; active-content inspectors, `QUARANTINED` and
  `FAILED` marking, interrupted-check recovery and cleanup of abandoned
  `UPLOADING` sessions belong to EMP-DOC-014 (and retention to EMP-DOC-012).
- Other dimensions: extraction, structure, conference and fiscal commands
  (EMP-DOC-009, EMP-DOC-006, EMP-DOC-005) write their own column and revision
  under the database guard; reconciliation stays `UNALLOCATED` until
  allocations exist (EMP-DOC-007/010); `REPLACED` needs successor columns
  (EMP-DOC-011); links and the link port are EMP-DOC-003; duplicate lookup
  indexes are EMP-DOC-008; a metadata read under `doc.document.read` (no role
  holds it in catalog v4) and the panel viewer are EMP-DOC-013/015.
- Storage provider: `cmd/api` composes no document bucket, so production
  answers `UNAVAILABLE` for bytes and content. Provider qualification, the
  private bucket configuration and its secrets, runtime grants (`SELECT,
  INSERT, UPDATE` on `org_documents`; `SELECT, INSERT` on the audit tables)
  and backup roles remain operator gates.
- Individual grant revocation: a capability stops working when the member
  loses coverage, the document changes or the access log refuses the read;
  there is no separate per-grant revocation store (EMP-PLAT-006 retained).
- The HTTP concurrency case proves one row and one event through one
  browser session; racing requests of one company session can receive 401
  from the session plane (existing behaviour, not changed here). The store
  suite proves twelve truly concurrent declarations.
- Real receipts stay legally gated (OQ-04, LV-04, LV-11). Integration:
  LOCAL_DONE only; no push, PR, CI, staging, F3, M3, C1 or release claim.

## Proposed shared-doc changes

1. `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`: append a section titled
   "EMP-DOC-002 — Document persistence, upload session and audited private
   access" holding the CONTRACT block above.
2. `docs/enterprise/16-decisions.md`: append the EDR block above (EDR-152,
   EDR-153) after the last existing decision row.
3. `docs/enterprise/backlog/ep09-documents.md`: set EMP-DOC-002 `state
   LOCAL_DONE` and add the BACKLOG block below under its task block.
4. `docs/enterprise/modules/documents.md`, paragraph after the EMP-PLAT-006
   link: replace "Document metadata/schema, upload-session persistence,
   feature routes and verification/lifecycle jobs retain EMP-DOC-002 and
   EMP-DOC-014 ownership." with "EMP-DOC-002 persists the document container
   (`org_documents`, migration 000205), the creator-bound upload session with
   its capability-bound ingress and the audited content routes
   ([evidence](../evidence/f3-doc002-document-persistence.md)); verification
   and lifecycle jobs retain EMP-DOC-014 ownership."
5. `docs/enterprise/09-architecture.md` module table: the `document` row
   gains persistence (`org_documents`), the company routes under
   `/v1/orgs/{org_id}/documents/` and the `DOCUMENT_INGRESS` boundary; links,
   jobs and the Android producer remain proposed.
6. `docs/enterprise/11-privacy-security.md` BR-PRIV-004 needs no change; the
   implementation matches it.
