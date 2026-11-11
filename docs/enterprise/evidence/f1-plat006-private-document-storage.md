# F1 EMP-PLAT-006 private document storage

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Delivery state INTEGRATION_PENDING; F1/M1 remain open.
The owner requested implementation and delivery into existing
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202), head `dev`,
base `main`. Construction starts at published `5b692fb5` in the isolated
`codex/enterprise-f0-f1` worktree. Fetched dev matches that head; main is
`ef96cd13`. Other worktrees, including occupied local dev, are preserved.
EMP-GOV-002's accepted synthetic C1 decision and EMP-PLAT-002's scope/harness
are present. Real-receipt intake/retention remains legally gated.

## Implemented boundary

B-BR-EMP-15, B-BR-EMP-16 and BUC-EMP-09 were recorded before implementation.
`orgmedia` supplies pure identity/budget rules, explicit application ports and
private S3/token/audit/inspection/delivery adapters. There is no document
migration, new dependency, new service or business route in this task.

Current AUTH/DOC authority resolves the verified account/member and the
same-organization synthetic declaration. Individual access revocation is an
explicit mandatory port, independent of token validity. Metadata, a link,
account tier or token alone never grants permission. The service defaults off
and rejects missing ports. Configuration uses the existing enterprise switch.

Upload reads at most declared bytes plus one and verifies the exact snapshot,
digest and detected content type. Inspection cannot transform the original.
It repeats current declaration/authority checks around storage. Accepted
bytes use a derived `org/<org_id>/doc/<document_id>/<sha256>` key. A conditional
PUT refuses replacement; a bounded verified GET confirms every created/reused
object. Failed or partial transfer claims no ready document state.

The concrete synthetic inspector reuses existing strict JPEG structure and
dimension checks plus full decode. Inspection runs on intake and delivery.
Default budgets are image 5 MiB, PDF 10 MiB, XML 1 MiB; a budget does not enable
a format. PNG/PDF/XML are refused by this inspector, including active/encrypted
PDF-like bytes and external-entity XML. Their reviewed inspectors and safety
job belong to EMP-DOC-014. Original bytes are never sanitized or re-encoded.
Community client MIME/size/24-hour rules are unchanged.

The token adapter reuses `httpapi.Seal/Open` with an organization-document
binding and a strict five-minute signed inner deadline. It binds the canonical
HTTPS origin, organization, verified account/member and document revision,
digest, type and length. Expiry is inclusive and duration arithmetic does not
overflow. The shared cursor envelope's longer TTL does not extend access.

Issuance and every actual redemption commit a bounded organization audit event
before the token or bytes leave the service. Server-generated UUIDs distinguish
repeated reads; an audit collision/replay fails closed. Events hold references,
registered codes and a digest, never content, token or storage URL.
Authority, individual revocation, identity and expiry are rechecked after
storage/audit; cancellation also refuses delivery. Private result/configuration
types redact fmt and structured slog output.

The delivery helper sets no-store, no ETag, nosniff, exact MIME/length and a
fixed attachment filename. Future authenticated POST controllers carry tokens
in the body and use the existing private error envelope. The service limits
transfers to four and applies a 20-second context. The HTTP owner must also
set its request read deadline: context alone cannot interrupt an arbitrary
Reader. That follows the existing community ingress pattern.

Tested source blobs: domain `6e8bb8fee04a0868b2909accdd202edf47bdac7b`,
application `ec7ac4d91461ce874ab39164e40e1047f07b47ab`,
S3 adapter `30ae86d863d62bdbbc4f6022a1bb45a2a6c13362`,
token adapter `40b90bb25fe902ad2aff8618e1d809b88579098e`,
audit adapter `4840f96fc021958066cda59acfcacfa264ddfb76` and shared server
PUT helper `185bf3ae2809c070a1410c54ebcecdd172da65fb`.
Source and evidence travel in the same task commit.

## Executed checks

Linux amd64, backend Go 1.27.4 and pinned staticcheck v0.8.1. Go commands used
`GOCACHE=/tmp/enterprise-go-build`. The unique tmpfs Compose project
`enterprise-plat006-20261111` supplied pinned PostGIS 18/3.6 and MinIO with
the existing `storage-init` private bucket. Ephemeral loopback ports were
32778 for PostgreSQL and 32777 for S3. Only disposable databases, ordinary
runtime logins and synthetic unique document keys were used.

- RED reproduced missing domain/application and server PUT/token adapters.
  Further RED tests caught structured-log exposure, temporary inspector
  failures mislabeled as permanent refusal, canceled content release,
  unconfigured formats on reads and overflowing signed deadline arithmetic.
  All passed after correction. The concurrency fixture's counter was also
  made race-safe without disabling its test.
- `go test -race -tags=integration -coverprofile=/tmp/enterprise-plat006-coverage.out ./internal/modules/orgmedia/... -count=1`
  passed on the final source with explicit disposable
  `ANPFUEL_TEST_DATABASE_URL` and all
  `ANPFUEL_TEST_STORAGE_{ENDPOINT,BUCKET,REGION,ACCESS_KEY_ID,SECRET_ACCESS_KEY}`
  values. Coverage: domain 100%, application 93.0%, adapters 94.4%; total 94.2%.
- Real S3/PostGIS tests proved separate organization keys for the same
  document/hash, denied anonymous object GET, exact original bytes including
  synthetic APP1 metadata, and one issuance plus one audit per actual read.
  Foreign organization/member, individual grant revocation, exact five-minute
  expiry, changed revision and unverified state returned no content.
- Eight concurrent conditional PUTs produced one create and seven
  precondition failures, all verifying the same snapshot. A deliberately
  substituted stored object was refused and a retry did not replace it.
- An exact 5 MiB JPEG traversed inspection, conditional S3 intake and audited
  retrieval with unchanged bytes, using the server-only transfer policy.
- Real audit INSERT denial yielded no token or content; restoration recovered
  access. Unit tests covered unavailable storage/inspection, interrupted
  readers, metadata changes and revocation during processing, cancellation,
  digest/type/length mismatch, inspector transformation, missing ports and
  the four-transfer concurrency limit. Structured/default formatting and
  private content/error-header checks passed.
- Signed adversarial payloads proved that the shared 15-minute envelope,
  extended/overflowing inner deadlines, malformed payloads and changed
  origin/member/account/document/revision/type/size cannot extend or rebind
  document access. Cursor and token signing retain separate binding domains.
- Scoped `go test -race -tags=integration` passed for existing evidence S3
  storage (including its real-S3 protocol matrix), orgaudit and orgapi.
  Existing evidence application, HTTP ingress and media regression suites
  also passed under race. Community policy remains unchanged.
- `go vet -tags=integration` and pinned staticcheck with `GOTOOLCHAIN=go1.27.1`
  passed for orgmedia and shared storage. The documented staticcheck compiler
  workaround changes no runtime build; backend builds use Go 1.27.4.
  `go build ./cmd/api ./cmd/worker ./cmd/ops` passed.
- Explicit gofmt, enterprise checker and checker regressions, changed/new-file
  whitespace and secret-surface review passed. The quick manifest registers
  the three new behavioral packages.

## Retained ownership and delivery gates

The test policy port supplies future current AUTH/DOC metadata and grant
revocation decisions; it is not a production membership or document store.
EMP-DOC-002 owns durable declarations, upload sessions, feature routes and
document state. EMP-DOC-014 owns further format inspectors and safety jobs.
This checkpoint does not enable those formats or real receipts.

Real provider/R2 qualification, runtime-role provisioning, retention/cleanup,
legal review, app/panel and production release remain with their owning gates.
There is no provider/staging mutation or live-readiness claim. Integration,
remote checks, wiki publication and deployment retain their existing gates.
Next bounded task: EMP-PLAT-007.
