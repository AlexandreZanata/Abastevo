# EMP-OFFL-005 — Attachment upload: offline queue, retry and resume, linkage to operations

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F3/C1 Android task (module OFFL, P0, type APP, size M; prerequisite of
EMP-OFFL-013 and of EMP-DOC-004 → EMP-FUEL-011/EMP-EXP-004 → EMP-TRIP-020) on
`codex/enterprise-f4-offl005` (worktree `/data/dev/worktrees/f4-offl005`),
based on the F4 head `fe7ec66d` (main `93f3a88d` + EMP-TRIP-009/013 + the
origin/dev Play-store work; Android compileSdk/targetSdk 36 under AGP 8.7.3).
The coordinator integrates it into `codex/enterprise-f4` and owns PROGRESS, the
F4 ledger, pushes and the PR. Synthetic data only: device-encoded synthetic
JPEGs and the existing synthetic 4×4 JPEG; no camera photo, real receipt,
person, plate or place. Real receipts stay legally gated (OQ-04, LV-04, LV-11)
and the feature stays behind its capability flag. No migration, no SQL, no
permission or role, no job. The only Android device used is the owned emulator
`emulator-5588` (AVD `enterprise-f2-api36`, API 36); the physical phone was
never addressed.

Sources read: the EMP-OFFL-005 block and the OFFL-003/004/008/013, DOC-002/003/
004/014 blocks ([EP04](../backlog/ep04-sync.md), [EP09](../backlog/ep09-documents.md));
[10-offline-sync](../10-offline-sync.md) §§3–6, 8, 10.1–10.4 and 12
(BR-OFFL-002/003/013/015, UC-OFFL-03, AC 8, the attachment states, the
"Receipt capture and attachment" matrix row);
[11-privacy-security](../11-privacy-security.md) §4 (BR-PRIV-003/004/020);
OQ-04 and the [approved receipt scope](f0-m0-decisions.md#private-receipts);
EDR-076..080, 122, 152/153, 162/163, 169/170, 177, 216..218, 239, 242..245 in
[16-decisions](../16-decisions.md); the evidence of DOC-002 (upload session,
≤5-minute capability, `DOCUMENT_INGRESS`, JPEG only, RECEIVED/STORED only after
verified bytes), DOC-014 (safety check, 24-hour session window), DOC-003
(links, owner resolvers), PRIV-005, PLAT-006, OFFL-003 (protected queue),
OFFL-004/008 (presentation, review results), FLEET-013, TRIP-005 (sealed
protected-store entries, signed producers, native acceptance with a real local
backend) and CHECK-003 (reports accepted independently of photos, the
`INCIDENT` link kind); the F4 ledger.

IDs used: B-BR-EMP-415..420, BUC-EMP-211..213, EDR-267..270 (all of them; none
left unused). No migration number was needed.

## Observed before any change

- The Android side had no attachment producer: `ProtectedEnterprisePersistence`
  already seals `ATTACHMENT` entries in the PRIV-003 generation, but nothing
  wrote them except a generic test helper; the envelope's `attachments` field
  is answered `RETRY`/`UNAVAILABLE` by every owner ("for EMP-OFFL-005").
- No device path could declare a document or obtain an upload capability: the
  DOC-002 declaration (`documents/uploads/create`) and the DOC-003 link routes
  are company-cookie routes; the `content/upload` ingress takes only a
  capability; the `Actor` and `Authorization.Prepare` already accept the
  `android` channel (DOC-002 retained "Android producer … how the app obtains
  an upload capability" for EMP-DOC-004 with EMP-OFFL-005).
- The ingress is a single bounded request (≤5 MiB + 1) without byte offsets;
  `UPLOADING` retries, `RECEIVED` checks and `STORED` replays are already
  idempotent (DOC-002/014); a document closes `FAILED UPLOAD_ABANDONED` 24 h 5 min
  after its declaration.
- The DOC-003 registry links `ASSET`, `EMPLOYEE`, `METER_READING` (stored and
  held readings) and `INCIDENT`; a reading recorded by a trip start is an
  `org_meter_readings` row authored by the trip's member.

## Rules frozen before behaviour

B-BR-EMP-415..420 and BUC-EMP-211..213 are the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
section "EMP-OFFL-005 — Attachment upload: offline queue, retry and resume,
linkage to operations (2026-11-15)"; EDR-267..270 are appended to the EDR table
of [16-decisions](../16-decisions.md). Both were written before any code. The
recommended routine decisions, taken unattended:

- **EDR-267 — local storage.** Sealed `ATTACHMENT` entries plus one sealed
  `attachment-book` v1 `RECORD` in the exact partition's protected generation,
  written together; capture only beside an operation already queued in the
  same snapshot; ≤16 MiB of unsettled bytes and ≤500 index entries per
  partition, refused beyond, nothing evicted; verified bytes removed in the
  write that records `STORED`; build flag `ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED`
  (data module, default off, on in enterprise validation builds). The
  encrypted per-file blob store with its key in the vault is rejected for C1
  (a new ciphertext channel outside the PRIV-003 erasure and lost-head rules)
  and retained for real receipts; the measured write cost is below.
- **EDR-268 — retry and resume.** Whole-body safe retry with an explicit
  state machine (`WAITING_FOR_RECORD` → `DECLARED` → `RECEIVED` → `STORED`, or
  `REFUSED` with a closed code, or `FILE_MISSING`); only server answers move
  an attachment; transport failures, timeouts, 5xx, 401/404 and integrity 422
  keep it pending under the OFFL-003 `QueueRetryPolicy` (per-attachment retry
  state in the index); the operation worker delivers attachments after each
  partition's operations and review lookups; capability and "sending" are
  memory only.
- **EDR-269 — linkage.** The owning operation identifier gates the
  declaration (its final receipt), and an explicit DOC-003 target captured with
  the attachment is linked by the server at declaration time through
  `Service.Link` with the real device actor; `INCIDENT` (CHECK-003 reports) and
  `METER_READING` (including trip departure/final readings) are the C1
  producers' targets; the envelope `attachments` field stays unused.
- **EDR-270 — backend change.** One special signed route
  `POST /v1/orgs/{org_id}/sync/attachments` (`SIGNED_ATTACHMENT_SESSION`)
  declaring, linking and granting the upload; no SQL, migration, permission,
  role, job or orgaccess change.

## Delivered surface

Backend (Go):

- `internal/platform/orgapi/signed_attachment_session.go`:
  `SignedAttachmentSessionRule`/`Descriptor` (account proof, signature, current
  enrollment, creator-bound, owner-linked, whole-body preflight, 200).
- `internal/modules/orgsync/application/attachments.go`: the
  `AttachmentSessions` port with `AttachmentDeclaration`/`AttachmentSession`
  (redacted values).
- `internal/modules/orgsync/adapters/http/attachments.go`: the endpoint with
  the reference feed's transport and device-proof sequence (Origin/host,
  alternate authority, JSON, no query, 4 KiB body; account session, identity-v1
  signature, binding owner, enrolled device; body membership/enrollment equal
  to the enrolled ones; re-confirmation of the device before any capability
  leaves), the strict flat request parser (fifteen keys once each, exact JSON
  types, closed DOC-001 target vocabulary, distinct identifiers, `RECEIPT`,
  `image/jpeg`, 1..5 MiB, lowercase SHA-256, graphic ASCII token) and the fixed
  response wire bound to request, membership, enrollment and document (a
  capability only for `UPLOADING`).
- `internal/modules/document/application/device.go`: `Service.OpenDevice`
  (android actor only; whole command validated first; `Create`, then — unless
  the document is `FAILED`/`REPLACED`/`DELETED` — `Link`; the capability only
  after the link) and `internal/modules/document/adapters/http/device.go`:
  `DeviceSessions`, the document owner of the port (permission refusals keep
  their key).
- Composition: `enterprisesuite.registerDocuments` mounts the signed route with
  the device store and the same document service; `apicontract` validates the
  new boundary both ways; OpenAPI path `enterpriseAttachmentSession` and the two
  schemas; route inventory special route (26 special, boundary count 1);
  router mount list; sync component list.
- Shared vectors `contracts/testdata/enterprise/sync/attachment-session-v1.psv`
  (89 rows: REQUEST 53 = OK 15 / INVALID 37 / LIMIT 1, RESPONSE 36 = OK 11 /
  INVALID 20 / LIMIT 1 / BINDING 4; SHA-256 `1996b45f…170b9c`) with
  `ATTACHMENT.md`, generated by an independent Python oracle
  (`f4-logs/offl005-scripts/gen_attachment_session_vectors.py`).

Android (Kotlin):

- Domain `domain/.../enterprise/attachment/`: `AttachmentSession.kt` (request
  builder/strict parser, bound response parser, `AttachmentStorage`,
  `AttachmentUploadGrant`) and `AttachmentBook.kt` (`EnterpriseAttachment`,
  the strict canonical `attachment-book` v1 codec, capture/budget/CAS rules,
  `AttachmentPolicy.checkJpeg`, `AttachmentDelivery.ownerGate/settle`,
  `AttachmentPresentation` labels independent of the operation's sync state).
- Application `application/.../enterprise/attachment/Attachments.kt`: store and
  transport ports and `EnterpriseAttachmentService` (`capture`, `attachments`,
  `deliver`).
- Data: `ProtectedAttachmentStore` (one `queueAccess` snapshot per call; foreign
  or unreadable index unavailable and never replaced),
  `EnterpriseAttachmentHttpClient` (signed session; capability-only ingress
  without cookie, signature or Origin; only 403/409/422 with their closed codes
  are final), `EnterpriseOperationWorkRunner` attachment hook with separate
  counts, DI and the `ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED` flag.
- No UI: the capture/import screens belong to EMP-DOC-004 and the owning
  producers' screens (EMP-FUEL-011, CHECK-003 Android), which consume
  `EnterpriseAttachmentService` and `AttachmentLabel` (retained).

## RED → GREEN → REFACTOR

Logs in `/data/dev/worktrees/f4-logs/offl005-*`. Go with the go1.27.4 toolchain,
`GOCACHE=/data/dev/cache/f4-go-build`, PostGIS through `orgtest` child
databases of `127.0.0.1:32775`, a throwaway MinIO project `f4-offl005-s3`
(`infra/compose.validation.yml`, removed with `down -v` at the end). Gradle
with JDK 17, `--offline --no-daemon --no-parallel --max-workers=2` under
`flock /data/dev/worktrees/.f4-gradle.lock`.

Shape first: the orgapi rule, the port, the endpoint (mounting nothing,
answering 404, parser and writer `errNotImplemented`), `Service.OpenDevice`
and `DeviceSessions` returning unavailable, the OpenAPI contract, the route
inventory, the vector file; Kotlin types with `TODO()` bodies and the runner
hook not called.

- **Go unit RED** (`offl005-go-red-unit.jsonl`, `-race`, orgsync http,
  document application, apicontract filtered to the new and affected tests):
  16 top-level, 14 fail / 2 pass (the redaction test and the sync component
  list, which read no behaviour). The three other failing apicontract tests
  failed on "unregistered organization contract operation POST
  /v1/orgs/{org_id}/sync/attachments" — the contract written before the route.
- **cmd/api RED** (`offl005-go-red-cmdapi.jsonl`): 2 of 4 top-level fail
  (inventory and full router: the special route and mount missing).
- **PostGIS + MinIO RED** (`offl005-go-red-integration.jsonl`): 3/3 fail (the
  route is absent).
- **Kotlin RED** (`offl005-kotlin-domain-app-red.log`,
  `offl005-kotlin-data-red.log`, XML in `offl005-kotlin-red-xml/`): domain
  97 tests, 96 fail (the loader-grammar test reads no codec); application 8/8;
  data 11/11 (HTTP client 4, runner 2, store 4, process death 1).
- **GREEN**: Kotlin application 8/8 and data 11/11 on the first implementation
  run; domain 96/97, the remaining failure a fixture mutation that did not
  change the stored spelling (whitespace was inserted before a key the
  canonical form never puts first) — the mutation now targets the first brace
  and the test asserts every mutation changes the text; no expectation
  changed. Go unit: two test-fixture corrections (a response-vector helper that
  failed instead of classifying an unparseable expiry as not producible, and
  the last transport case served without chi routing, so the organization path
  parameter was empty) → orgsync http and document 73/73 top-level, 731
  subtests (`offl005-go-green-unit-2.jsonl`); apicontract green in
  `offl005-go-green-unit-1.jsonl`; cmd/api 4/4 (`offl005-go-green-cmdapi.jsonl`);
  PostGIS + MinIO 3/3 on the first run (`offl005-go-green-integration-1.jsonl`).
- **REFACTOR**: the race test signs every request on the test goroutine and
  only sends them concurrently (no fatal call off the test goroutine); a host
  budget test with timings was added; suites re-run green.

## Validation

```sh
# backend/, go1.27.4 toolchain, PostGIS 127.0.0.1:32775 through orgtest, MinIO f4-offl005-s3
gofmt -l ./internal ./cmd ./db                                     # empty (toolchain gofmt)
go build ./... && go vet ./... && go vet -tags=integration ./...    # clean
go vet -tags='integration nativefixture' ./internal/modules/organization/adapters/   # clean
GOTOOLCHAIN=go1.27.1 staticcheck [-tags=integration] orgsync/... document/... orgapi apicontract enterprisesuite cmd/api organization/adapters
GOTOOLCHAIN=go1.27.1 staticcheck -tags='integration nativefixture' organization/adapters   # no findings (offl005-go-static.log)
go test -race -count=1 -json -tags=integration -timeout 60m -p 2 ./internal/modules/organization/adapters/ \
  ./internal/modules/document/... ./internal/modules/orgsync/... ./internal/modules/orgmedia/... \
  ./internal/platform/apicontract/ ./internal/platform/enterprisesuite/ ./internal/platform/orgapi/ ./cmd/api/
    # offl005-race-union.jsonl: 17 packages pass, 884 top-level / 7026 named, 0 failures, 0 skips, 45:02
    # (organization/adapters 2700.6 s, apicontract 2094.0 s, cmd/api 141.2 s)
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...                    # offl005-untagged-all.log: 223 packages ok, exit 0
python3 docs/enterprise/tools/check_plan.py                         # tasks 348; rules 545; use cases 167; screens 145; problems 0
git diff --cached --check                                           # clean
git diff --cached --binary | gitleaks stdin --redact --no-banner    # no leaks found
```

- No SQL, migration, sqlc input, permission, role or job changed, so `sqlc`,
  migration tests, the authorization matrix and the AUTH-011 auditor walk were
  not touched; they ran unchanged inside the `organization/adapters` union
  (the authorization inventory and full-router tests in `cmd/api` cover the
  new special route: 26 special routes, `SIGNED_ATTACHMENT_SESSION` 1; normal
  routes unchanged). No new Go package, so `scripts/check-manifest.txt` is
  unchanged.
- Kotlin host suites (`offl005-kotlin-enterprise-suites-1.log`): domain
  enterprise 3638, application enterprise 128, data enterprise 194, app
  enterprise 23 tests — 0 failures, errors or skips. The real-generation budget
  test added afterwards passed with the other attachment data tests (12/12) in
  `offl005-lint-normal-build.log`.
- Android lint and builds: `:app:assembleDebug :app:lintDebug :data:lintDebug`
  (`offl005-lint-normal-build.log`): 0 errors; 352 app and 17 data warnings,
  as before the task, none in touched sources; enterprise validation builds
  `-PenterpriseDeviceValidation=true :app:assembleDebug
  :app:assembleDebugAndroidTest :data:assembleDebugAndroidTest`
  (`offl005-validation-build.log`): pass.
- Measured write cost (host JVM, file-backed test keys, load average about
  12; EDR-267): sealed writes of 261, 501 and 668 ms for 5 MiB captures with 0,
  5 and 10 MiB already held, 760 ms for a retry-state transition and 709 ms for
  the verification write with 15 MiB held — every queue write re-seals the
  generation, which is why unsettled bytes are capped at 16 MiB and verified
  bytes leave the device. On the emulator the synthetic photos are a few KiB,
  so their writes are within the stage times below.
- Secret/PII review of the staged diff: synthetic `7b0e2a54-…`/`7a000000-…`
  identifiers, synthetic tokens (`synthetic-attachment-access`,
  `v1.synthetic-capability`), device-encoded synthetic pictures and the existing
  synthetic 4×4 JPEG; the only credential names are the disposable
  `ANPFUEL_TEST_STORAGE_*` variable names. The native fixture's proof, CA and
  session stayed in a mode-0700 `/data/dev/cache` directory and the test
  package's private storage, both removed afterwards; fixture and driver logs
  carry status codes only, never a request, capability or byte. The
  throwaway MinIO project was removed with `down -v`.

## Native acceptance on `emulator-5588`

Acceptance, not RED: written after the services. Real signed backend
`TestAttachmentNativeSourceFixture` (tags `integration nativefixture`,
`organization/adapters/orgsync_attachment_native_fixture_test.go`): the actual
default enterprise runtime over an `orgtest` child database of the owned
PostGIS (`127.0.0.1:32775`), the throwaway MinIO bucket as document storage,
signed HTTPS with a disposable CA, two organizations of one synthetic account,
a meter-equipped vehicle assigned to the member in A. Its only extra behaviour
is an ingress fault the driver arms through files in the private fixture
directory (`truncate`: half of the next body with its full declared length;
`flip`: one byte altered; `stall`: part of the body read and the connection
held). Device side `EnterpriseAttachmentNativeDeviceTest` with the product's
protected store, queue, trip producers, `EnterpriseAttachmentService` (the
test's backoff base 2 s), signer and HTTP clients; driver
`f4-logs/offl005-scripts/native-driver.sh` (`-s emulator-5588` only,
force-stop of the test package between stages). Logs:
`offl005-native-source.log`, `offl005-native-device.log`,
`offl005-native-driver.out`.

- **Stage 1** (route present): custody of both organizations through the real
  contact route and the signed reference pull of A. Route removed.
- **Stage 2** (new process, no route): a trip started from the protected cache
  with the 184 320 km departure reading (production trip producer), then two
  device-encoded synthetic JPEG photos of that reading captured through the
  product service (`METER_READING`, owner = the start operation): both
  "Salvo no aparelho", the start "Salvo no aparelho"; operation and attachment
  delivery attempts sent nothing (0 intake, 0 session, 0 upload requests).
- **Stage 3** (new process, no route): both photos survived the process death
  with their exact sealed bytes and digests; organization B has none; no
  identifier, digest, target, `attachment-book`, `METER_READING` or `JFIF`
  marker is readable in the partition's generation files; nothing sent.
- **Stage 4** (route restored, plan `truncate,flip`): the start was accepted
  (one intake 200). The first delivery declared and linked both photos (two
  sessions 200) and sent them; the fixture truncated the first and altered the
  second, the ingress answered 422 twice and the device kept both `DECLARED`
  ("Enviando"), never verified, while the record showed "Aceito pela
  empresa". After the backoff both were sent again with fresh sessions and
  answered 200 `STORED`; their sealed bytes left the device; a further
  delivery sent nothing. A third photo of the same reading was captured.
- **Stage 5** (plan `stall`): the driver started the delivery, the fixture
  read part of the body and held the transfer, and the driver force-stopped the
  process (`Process crashed`, as intended).
- **Stage 6** (new process): the third photo was `DECLARED`, its bytes still
  sealed; the next delivery asked the session again (a new capability) and
  completed the same document `STORED`; three photos verified; a further
  delivery sent nothing. Cleanup removed both partitions, the identity/session
  keys, the private fixture and the route.
- **Server** (fixture at stop): answers — intake [200], session ×6 [200],
  ingress truncate [422], flip [422], stalled 1 (never forwarded), clean ×3
  [200]; organization B received nothing. Organization A: 3 documents created
  by this member, 3 `STORED` at storage revision 3, 3 links to the trip-start
  reading authored by this member, exactly 3 `document.upload.received` and 3
  `document.upload.stored` events; B holds no document.
- Run 1 passed all six device stages but its fixture check failed: the held
  transfer was counted only when its handler returned, and through `adb
  reverse` the killed client's connection never closed, so the handler was
  still waiting at the stop check (it ended at its 3-minute bound). The fixture
  now counts the held transfer when it is held and releases it after 20 s; run
  2 passed (logs `-run1` kept). No product code changed between the runs.
- Instrumentation times: 5.0 s, 2.5 s, 1.3 s, 23.8 s (two backoff rounds),
  killed, 3.0 s. APK SHA-256 (`offl005-validation-apks.sha256`): data test
  `767cb711…8943b`.
- Not run natively: other existing device suites (no shared screen or
  storage code changed beyond the additive runner hook); an `INCIDENT` photo (no
  Android incident producer exists); a two-physical-device run (OWED, phone
  excluded); a 24-hour session closure (host- and DOC-014-proven only).

## Named negatives → tests

| Negative / acceptance | Test(s) |
|---|---|
| Operation and attachment outcomes are independent (record accepted, photo pending or refused) | `EnterpriseAttachmentServiceTest` independence test (receipt bytes unchanged, label `SENDING`); `AttachmentBookTest` label test; native stage 4 (start "Aceito" while both photos are refused transfers) |
| Never reported ready before server verification | `AttachmentDelivery.settle` and presentation tests (`VERIFIED` only from `STORED`); service `RECEIVED` test; composed test (session answers `STORED` only after the verified upload); native stages 4 and 6 |
| Only matching declared size and SHA-256 reach `STORED` through private ingress | composed `TestSignedAttachmentSessionDeclaresLinksAndStoresOnlyVerifiedBytes` (mismatched, truncated, interrupted bodies leave `UPLOADING` rev 1; stored digest and size equal the declaration); native fixture counts (3 `STORED`, 3 received events) |
| Wrong organization | composed refusal test (route of another organization `NOT_FOUND`, foreign enrollment in the body `NOT_FOUND`, nothing declared); composed upload test (capability on another organization's path refused, state unchanged); store test (organization B reads nothing); native stage 3 and fixture (organization B has no document and received nothing) |
| Wrong record | composed refusal test (missing reading `NOT_FOUND`, unregistered kind `VALIDATION_FAILED`, another member's reading under SELF `FORBIDDEN` with no link); `TestDeviceSessionWithholdsTheCapabilityWhenTheLinkIsRefused`; client and service refusal tests (`FORBIDDEN`, `CONFLICT`, `OPERATION_ID_REUSED`, `VALIDATION_FAILED` final; 404 retried) |
| Expired authority or capability (re-request a session) | composed: session renewal writes nothing and returns a fresh capability, a forged capability is refused, `AUDITOR` without `doc.document.attach` gets `FORBIDDEN` and no capability; service: a refused transfer is retried with a new session and grant; native stage 6 (a new process re-requests the session) |
| Mismatched or truncated bytes | composed mismatched/truncated/interrupted cases; service retry test; native stage 4 (fixture truncates one transfer and alters another: 422, then both converge) |
| Duplicate file completion (idempotent) | composed (repeat upload after `STORED` is `replayed`, no new received/stored event); `TestSignedAttachmentSessionRacesConvergeOnOneDocumentAndLink` (six concurrent sessions: one document, one link, two audit events); service (a verified photo is never sent again); native stages 4 and 6 (a further delivery sends nothing) |
| Missing local file (explicit failed state, never silent) | service refusal test (missing and altered sealed bytes → `FILE_MISSING`, nothing sent); presentation test (`FILE_MISSING` label) |
| Reporting success before verification | as above; process-death test (a `STORED` write or nothing); native stage 6 (the cut transfer left `DECLARED`, never verified) |
| Process death mid-upload (resume or safe retry) | `ProtectedAttachmentProcessTest` (child JVM halted before the durable replace and after the trusted head, for capture and verification: index and bytes together or neither); native stages 5→6 (process killed while the fixture holds the transfer open; the next process completes the same document) |
| Owner refused | service refusal test (`OWNER_REJECTED`, never declared, bytes kept) |
| Capture outside C1 or without its operation, budget | `AttachmentBookTest` (JPEG markers, size, duplicates, full index, 16 MiB budget, nothing evicted); store tests (unknown operation, bytes differing from the declaration, real-generation budget) |
| Privacy | store plaintext test and native stage 3 (no marker, digest, identifier, `JFIF` or format name outside encryption); value redaction tests (Go and Kotlin); composed test (no proof material or content in answers) |
| Shared wire | `attachment-session-v1.psv` consumed by `AttachmentSessionVectorsTest` and the Go request/response vector tests |

## Retained (with reasons; not part of this task's acceptance)

- **UI.** Capture and import screens, the attachment rows on the pending-sync
  screen and the record timelines (SCR-OFFL-01/02 attachment progress, "comprovante
  não encontrado no aparelho" copy, retry and dismissal actions): EMP-DOC-004 and
  the producers' screens (EMP-FUEL-011, CHECK-003 Android, EMP-TRIP-020);
  `AttachmentLabel` is the contract they render. No string was added.
- **Producers.** No production producer captures attachments yet; the native run
  uses the product's trip producer and the attachment service directly. A
  capture in the same protected write as its operation (EDR-267) and the
  owners' own target choice belong to EMP-DOC-004/EMP-FUEL-011.
- **Real receipts and storage.** OQ-04/LV-04/LV-11; the flag stays off in normal
  builds; production composes no document bucket (sessions answer, bytes stay
  `UNAVAILABLE`) until provider qualification and the private bucket (operator
  gate); no new runtime grant beyond DOC-002/003/014's.
- **Encrypted blob store** for large or numerous real receipts and its erasure
  channel (EDR-267); retention of the attachment index and dismissal of
  refused or missing attachments; re-declaration with a new document after a
  closed (`FAILED UPLOAD_ABANDONED`) session (shown refused today).
- **Link kinds** `TRIP`, `FUELING`, `EXPENSE`, `WORK_ORDER` (their owners), held
  incident reports, unlink/replacement (EMP-DOC-011); an `INCIDENT` photo through
  the device path is proven by the shared code path and DOC-003/CHECK-003 tests,
  not natively (no Android incident producer exists).
- **Scheduling.** Wi-Fi-only/battery-tuned uploads and byte-level progress
  (10-offline-sync §2 "later"); a route rate budget (none, like the reference
  feed).
- **EMP-OFFL-013** resilience suite, two physical devices and visual/TalkBack
  review (no screen in this task).
- Merged-tree verification, production grants and any staging/release.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. `docs/enterprise/10-offline-sync.md` §10.3, after the TRIP-005 sentence:
   "Attachments are sealed beside their operation in the same partition and
   delivered after it: a signed session declares, links and grants the upload,
   the bytes go to the capability-bound ingress, and only a `STORED` answer
   verifies them (EMP-OFFL-005, EDR-267..270)."
2. `data/src/main/kotlin/com/anpfuel/data/enterprise/README.md`, after the
   EMP-TRIP-005 paragraph: "EMP-OFFL-005 seals attachment bytes as `ATTACHMENT`
   entries and their index as one `attachment-book` v1 `RECORD` per partition,
   written together beside an operation already queued; verified bytes are
   removed in the write that records `STORED`; the operation worker delivers
   them after the queue behind `ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED` (default
   off)."
3. `docs/enterprise/modules/documents.md`, after the EMP-DOC-014 sentence:
   "EMP-OFFL-005 adds the signed device path `POST /v1/orgs/{org_id}/sync/attachments`
   (declaration, link and upload capability for the enrolled member's own
   document)."
4. `docs/enterprise/09-architecture.md` module table, `document` row: the
   `SIGNED_ATTACHMENT_SESSION` boundary beside `DOCUMENT_INGRESS`.
5. `backend/README.md` "Organization document links (EMP-DOC-003)": add "The
   signed device attachment session (EMP-OFFL-005) reuses the DOC-002 and
   DOC-003 grants; no new grant."
