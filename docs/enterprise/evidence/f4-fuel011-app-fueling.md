# EMP-FUEL-011 — App: add a fueling offline with receipt and meter reading

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F3/C1 Android task (module FUEL, P0, type APP, size M; prerequisite of
EMP-TRIP-020) on `codex/enterprise-f4-doc004` after EMP-DOC-004 (`ab3ff194`)
and a normal merge of the F4 head `2f031276` (`975ef4b2`, EMP-EXP-002: every
accepted `fueling.submit` now creates its FUEL expense in the same
transaction, migration 000244). Synthetic data only; real receipts stay
legally gated (OQ-04). One minimal backend change (EDR-297): the `FUELING`
document link kind, migration **000245**. The only Android device used is the
owned emulator `emulator-5588`, every native run under
`flock /data/dev/worktrees/.f4-emulator.lock`; the physical phone was never
addressed.

Sources read: the EMP-FUEL-011 block and the FUEL-002/003/012, DOC-004,
OFFL-005, FLEET-013 and TRIP-020 blocks; [fuelings](../modules/fuelings.md)
§§4–12 (BR-FUEL-002/006/007/008/009/016/017/024, UC-FUEL-01, SCR-FUEL-01/02,
§10.1/10.2); [10-offline-sync](../10-offline-sync.md) §§4–5 and the offline
matrix ("Fueling", "Fueling with an agreement voucher", "Receipt capture");
[12-ux-app](../12-ux-app.md) §4.9; OQ-04 and the
[approved receipt scope](f0-m0-decisions.md#private-receipts); EDR-175..177,
EDR-216, EDR-244, EDR-267..270, EDR-291..293 and the latest EDR rows; the
evidence of FUEL-001 (Kotlin candidate, shared `fueling-v1.psv`), FUEL-002
(the exact 17-key payload, reading or missing reason, held forms store
nothing), EXP-002 (the coordinator's head note: usage functions latest bodies
in 000244), OFFL-005 (owner gate, link at declaration, retained `FUELING`
kind), DOC-004 (capture path), TRIP-005 (producer registry, flows, native
pattern, `trip-operations-v1.psv`), FLEET-013 (picker) and METER-003
(validator warns, never blocks); the DOC-003 registry, link store and the
CHECK-003 `INCIDENT` precedent (migration 000230).

IDs used: B-BR-EMP-455..460, BUC-EMP-231..233, EDR-294..297 (all of them;
none left unused). Migration number used: **000245** (the FUELING link
resolver needs a typed reference column).

## Observed before any change

- No Kotlin builder or decoder for `fueling.submit` existed; the Go intake
  (`fueling/application.ParseOperation`) refuses envelope attachments,
  dependencies and soft references and every retained form as `UNSUPPORTED`
  (`RETRY`/`UNAVAILABLE`), and stores a held fueling nowhere (EDR-177).
- The DOC-003 registry had no `FUELING` owner: the signed attachment session
  answered it `VALIDATION_FAILED` (asserted by the OFFL-005 refusal test), and
  `org_document_links` had no fueling reference column.
- After EXP-002 an accepted fueling also inserts its expense (deferred
  completeness trigger `org_fueling_expense_complete`), and
  `org_expenses.fueling_id` references `org_fuelings(org_id,id)`; neither
  touches document links.

## Rules frozen before behaviour

B-BR-EMP-455..460 and BUC-EMP-231..233 are the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
section "EMP-FUEL-011 — App: add a fueling offline with receipt and meter
reading (2026-11-15)"; EDR-294..297 are appended to the EDR table of
[16-decisions](../16-decisions.md). Both were written before any code.
Recommended routine decisions, taken unattended:

- **EDR-294 — producer bytes.** Pure Kotlin builder and strict decoder over the
  FUEL-001 candidate rules, refusing in the Go parser's order and class; one
  new shared oracle `fueling/fueling-submit-v1.psv` (59 rows) from an
  independent Python generator, consumed by Kotlin and by the real Go
  `protocol.ParseOperation` + `application.ParseOperation`.
- **EDR-295 — no separate index.** The registered producer re-reads its
  operation inside the protected write and projects nothing; list and detail
  come from the partition's retained `fueling.submit` entries; sync from each
  entry's receipt; flag `ENTERPRISE_FUELING_UI_ENABLED`.
- **EDR-296 — exact offline entry.** Picker asset (implements excluded), the
  asset's cached products, exact decimal conversion (3/2/3 decimals, no
  rounding), explicit fill/payer, offline payment methods only, responsible
  = own employee record, METER-003 against this device's own fuelings and the
  BR-MONEY-005 arithmetic warning, never blocking.
- **EDR-297 — FUELING link kind.** Migration 000245 (column, composite
  same-organization foreign key, widened kind and binding checks; nothing
  else), `fueling/adapters.FuelingDocumentTargets` under
  `fuel.fueling.record` with the author as SELF subject, registered in the
  enterprise suite; the receipt is a second protected write after the
  fueling, declared only after the fueling's final receipt.

## Delivered surface

- **Domain** (`domain/.../enterprise/fueling/`, pure): `FuelingOperations.kt`
  (`FuelingEnvelope`, `FuelingSubmitInput`, `FuelingReadingInput`,
  `FuelingFact`, `FuelingEvidence`, `FuelingOperationException` with the
  `MALFORMED`/`UNSUPPORTED` classes; `submit` builds the exact sealed operation,
  `read` mirrors `ParseOperation`); `FuelingPresentation.kt` (`FuelingView`
  with sync from the entry's own receipt and the review line
  `NOT_SENT`/`AWAITING_APPROVAL`/`NEEDS_REVIEW`/`REFUSED`, newest first, other
  operation types ignored; `FuelingEntry` exact decimal conversion).
- **Application** `application/.../enterprise/fueling/Fuelings.kt`: the
  `EnterpriseFuelingStore` port, `EnterpriseFuelingProducer` (re-reads its
  operation inside the protected write, projects nothing), `FuelingForm`,
  `EnterpriseFuelingService` (record in the context's own partition with the
  member's employee as responsible; arithmetic and METER-003 warnings),
  `FuelingFlow` (the TRIP-005 owner/epoch/lease fencing; `record` saves the
  fueling, then its optional receipt with target `FUELING`; a failed or
  refused receipt write sets `receiptNotSaved` and keeps the fueling; `attach`
  adds a receipt later; `sendNow` is one dispatch).
- **Data** `ProtectedFuelingStore` (the partition's `QUEUE` entries of type
  `fueling.submit` in one `queueAccess` snapshot); DI registers the producer
  beside the trip producers and provides store, service and flow.
- **App** `app/.../enterprise/fueling/`: `FuelingViewModel` (re-reads after each
  delivery), `FuelingPanel`/`FuelingsHomeCard` (SCR-FUEL-01/02: list with sync
  chip and receipt rows, picker, form with product, quantity, paid, unit
  price, discount, fill, payer, offline payment methods, reading or reason,
  place name, inline warnings and the embedded DOC-004 receipt sheet; detail
  with facts, sync and review lines, receipts and "Adicionar comprovante");
  `EnterpriseContextScreen` wiring with the same partition/member/epoch
  guards and withdrawals as the trips; `ReceiptCaptureCard` save/cancel
  optional for the embedded use; flag `ENTERPRISE_FUELING_UI_ENABLED`; 86
  strings × 8 locales (`enterprise_fuelings.xml`, generated by
  `f4-logs/fuel011-scripts/gen_fueling_strings.py`).
- **Backend** (EDR-297): migration `000245_organization_fueling_document_links.sql`;
  `db/queries/document/links.sql` (`fueling_id` on the insert) and
  `db/queries/fueling/fuelings.sql` (`FuelingDocumentAuthor`), sqlc
  regenerated (`sqlc.yaml` adds 000245 to the document schema);
  `document/application.BindFueling` and the link store binding;
  `fueling/adapters/document_targets.go` (`FuelingDocumentTargets`);
  `enterprisesuite` registers it. No route, OpenAPI, permission, role, job,
  usage-source or Kotlin protocol change; the route inventory, authorization
  matrix and AUDITOR walk counts are unchanged.
- **Shared vectors** `contracts/testdata/enterprise/fueling/fueling-submit-v1.psv`
  (59 rows: OK 15 / MALFORMED 39 / UNSUPPORTED 5; SHA-256
  `67068cfb…f5f0b8`) with the README section, generated by
  `f4-logs/fuel011-scripts/gen_fueling_submit_vectors.py`; consumers
  `EnterpriseFuelingSubmitVectorsTest` and `fueling/application/submit_vectors_test.go`.
- **Test fixtures**: the DOC-004 native fixture grants the fueling owner
  (`fuelingGrants`, including the EXP-002 expense grant); the OFFL-005
  attachment refusal test now names `EXPENSE` as the unregistered kind (FUELING
  gained an owner); the DOC-003 column-list assertion includes `fueling_id`.

## RED → GREEN → REFACTOR

Logs in `/data/dev/worktrees/f4-logs/fuel011-*` (XML in `fuel011-kotlin-red-xml/`).
Shape first: domain `submit`/`read`/presentation/entry as `TODO()`, the
application service, producer and flow writes as `TODO()`, the data store as
`TODO()`, the Go resolver answering unavailable, the migration as written, the
vector file and both consumers.

- **Go shared vectors** (`fuel011-go-vectors-1.jsonl`): the independent oracle
  against the real `protocol.ParseOperation` + `ParseOperation` — 1 top-level /
  59 subtests pass on the first run (a consumer of existing parsers has no
  behavioural RED, as in TRIP-005).
- **Kotlin domain RED** (`fuel011-kotlin-red.log`): 63 tests, 62 failing on
  `NotImplementedError` (59 vectors, 3 presentation/entry); the corpus
  completeness test, which reads no builder, passed.
- **Kotlin application/data RED** (`fuel011-kotlin-red-2.log`, after fixing two
  test compile errors — a `fail()` type inference and suspend calls inside a
  non-suspend lambda): `EnterpriseFuelingFlowTest` 8/8 and
  `ProtectedFuelingStoreTest` 1/1 failing on `NotImplementedError`.
- **Go unit RED** (`fuel011-go-unit-red.jsonl`): `TestFuelingDocumentTargetsPermissionAndKinds` failing.
- **Go PostGIS RED**: the first run (`fuel011-go-db-red.jsonl`) failed for the
  wrong reason — the fixture declared the fueling a minute before the vehicle
  became active, so the intake held it (`orgsync: unavailable`); the test now
  declares at `x.declaredNow` like every fueling test (no expectation changed)
  and the rerun (`fuel011-go-db-red-2.jsonl`) failed genuinely: the composed
  company link answered 503 from the stub resolver. The upgrade rehearsal
  passed already (it exercises the migration as written, not behaviour).
- **GREEN**: Go unit pass; PostGIS (`fuel011-go-db-green-1.jsonl`) 13
  top-level pass — the new link and upgrade tests plus the `TestFuelingIntake*`
  and `TestDocumentLink*` suites — and `TestDocumentLinksOverTheCompanyAPI`
  failed only for want of the MinIO environment in that run (it passed in the
  union below). Kotlin (`fuel011-kotlin-green-2.log`): fueling domain 277
  (submit vectors 60, presentation 3, the unchanged FUEL-001 suites),
  application flow 8/8, data store 1/1 after two fixture corrections in the
  first GREEN run (a held fueling is an entry without a receipt — the queue
  never stores a `RETRY` receipt — and `SyncBinding` has no value equality, so
  its fields are compared); no expectation changed.
- **REFACTOR**: the German warning string was reworded after lint flagged a
  false "seit dem" typo; `FuelingViewModelTest` (current gesture only, saving
  offline, sending online) was written with the wiring and is not claimed as
  RED.

## Validation

```sh
# Kotlin (fuel011-kotlin-suites-lint-1.log, fuel011-lint-2.log, fuel011-validation-build-1.log)
gradle.sh --continue :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test --tests 'com.anpfuel.application.enterprise.*' \
  :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.enterprise.*'
    # domain enterprise 4211, application enterprise 151, data enterprise 196, app enterprise 25 tests: 0 failures, errors or skips
gradle.sh --continue :app:assembleDebug :app:lintDebug :data:lintDebug
    # pass; lint 0 errors; app 354 and data 17 warnings, as at the DOC-004 head; none in a touched source
    # (the first run flagged one German string as a "seit dem" typo; it was reworded)
gradle.sh -PenterpriseDeviceValidation=true :app:assembleDebug :app:assembleDebugAndroidTest :data:assembleDebugAndroidTest   # pass
# Go (backend/, go1.27.4 toolchain, PostGIS 127.0.0.1:32775 through orgtest, MinIO f4-doc004-s3)
gofmt -l ./internal ./cmd ./db                              # empty
go build ./... && go vet ./... && go vet -tags=integration ./... \
  && go vet -tags='integration nativefixture' ./internal/modules/organization/adapters/   # fuel011-go-vet.log: clean
GOTOOLCHAIN=go1.27.1 staticcheck [-tags=integration] fueling/... document/... enterprisesuite; -tags='integration nativefixture' organization/adapters   # no findings
sqlc vet && sqlc generate && sqlc diff                      # clean; the generated diff is exactly the two edited queries
go test -race -count=1 -json -tags=integration -timeout 90m -p 2 ./internal/modules/organization/adapters/ ./internal/modules/document/... \
  ./internal/modules/fueling/... ./internal/modules/expense/... ./internal/modules/orgsync/... ./internal/modules/orgmedia/... \
  ./internal/platform/apicontract/ ./internal/platform/enterprisesuite/ ./internal/platform/orgapi/ ./cmd/api/
    # fuel011-race-union.jsonl (56.7 min, organization/adapters 3398 s, apicontract 2790 s): 24 packages, 1005 top-level /
    # 9345 named; 4 failures, each an expectation that FUELING had no owner or a missing grant, all corrected below
go test -race ... -run 'TestBindings|TestDocument|TestFueling|TestSignedAttachmentSession|TestIncidentReport|TestTargetRegistry|TestLink' \
  ./internal/modules/document/... ./internal/platform/apicontract/ ./internal/modules/organization/adapters/
    # fuel011-race-rerun-2.jsonl: 7 packages, 54 top-level / 146 named, 0 failures (the first rerun found the link
    # fixture missing the resolver's column grant on org_fuelings: 503 instead of 404)
GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...           # fuel011-untagged-all.log: 229 packages ok, exit 0
python3 docs/enterprise/tools/check_plan.py                 # tasks 348; rules 545; use cases 167; screens 145; problems 0
git diff --cached --check                                   # clean
```

- **Union failures and their corrections.** `TestBindingsBelongToExactlyOneRegisteredKind`,
  the apicontract link-target test and `TestDocumentLinksOverTheCompanyAPI`
  still encoded FUELING as unowned; `TestFuelingSchemaIsForcedRLSAndInsertOnly`
  failed `42501` because `TRUNCATE org_fuelings CASCADE` now also reaches
  `org_document_links` (the 000245 foreign key) — the test grants it like the
  EXP-002 `org_expenses` precedent and the link table's own retained guard
  still refuses (`55000`). The DOC-003 `linkGrants` fixture gains the
  resolver's production grant `SELECT (org_id,id,author_membership_id) ON
  org_fuelings`.
- **Tests updated because FUELING gained an owner** (none weakened; each now
  names a kind that still has no owner and adds the FUELING positive or
  not-found case): `document/application.TestBindingsBelongToExactlyOneRegisteredKind`
  (FUELING binds to `Fueling`; `TRIP`/`WORK_ORDER` added to the unknown
  list), `apicontract` link target (FUELING admitted, `fueling` lowercase still
  refused), `TestSignedAttachmentSessionRefusesWrongRecordsAuthorityAndReuse`
  (`EXPENSE` as the unregistered kind), the DOC-003 company route test (an
  asset identifier named as a FUELING target is `NOT_FOUND`, `EXPENSE` stays
  `VALIDATION_FAILED`), the DOC-003 raw-insert guard (`EXPENSE` as the
  unregistered kind) and the link column list (`fueling_id`). OpenAPI: the two
  link-kind enums (`EnterpriseDocumentLinkTarget.kind` and the attachment
  session `target_kind`) add `FUELING`; no path, route, inventory, matrix or
  AUDITOR-walk change (counts as on the merged head).
- Secret/PII review of the staged diff: synthetic `7b0e2a54-…`/`7f000000-…`
  identifiers, synthetic amounts and place names, device-encoded synthetic
  pictures; the only credential names are the disposable
  `ANPFUEL_TEST_STORAGE_*` variables. The native fixture's proof, CA and
  session stayed in the mode-0700 `/data/dev/cache/f4-doc004-native`
  directory (empty afterwards) and the test package's private storage, erased
  by stage 4; logs carry status codes and counts only.

## Native acceptance on `emulator-5588`

Acceptance, not RED: written after the services. Every run under
`flock /data/dev/worktrees/.f4-emulator.lock`; the physical phone was never
addressed.

- **Real signed backend** (`fuel011-native-source.log`,
  `fuel011-native-device.log`, `fuel011-native-driver.out`; driver
  `fuel011-scripts/native-driver.sh` over the DOC-004 driver and source
  scripts): `TestReceiptNativeSourceFixture` on the merged head (EXP-002
  included: every accepted fueling commits with its expense) over an
  `orgtest` child database of the owned PostGIS `127.0.0.1:32775` and the
  throwaway MinIO project `f4-doc004-s3` (removed with `down -v` at the end),
  with `EnterpriseFuelingNativeDeviceTest` (data test APK `41fae8de…6912`) in
  four processes. Stage 1 (route present, 6.5 s): custody of both
  organizations and the signed reference pull of A. Route removed. Stage 2
  (new process, 9.1 s) through the product's `FuelingFlow`: 150 L of Diesel
  S10 at R$ 6,490 paid R$ 970,00 (arithmetic warning shown, paid total kept),
  odometer 184 600 km, place "Posto sintético", with a device-encoded
  synthetic receipt through the DOC-004 path in the same save ("salvo com o
  comprovante"); a second fueling of 40,5 L paid R$ 262,85 by the employee
  with Pix and the stated absence "Hodômetro apagado", its receipt added
  afterwards from the detail; both "Salvo no aparelho", review "ainda não
  enviado", both receipts "Salvo no aparelho" with target `FUELING` = their
  fueling; organization B's flow lists nothing and A's fueling cannot own a
  receipt in B (`UNKNOWN_OPERATION`); nothing sent. Stage 3 (new process,
  2.9 s): both fuelings with their exact quantities, paid totals and unit
  price, both sealed receipts with their digests; B empty; no fueling or
  operation identifier, document identifier, digest, `fueling.submit`, place
  name, reason, `FUELING` or `JFIF` readable in the partition's generation
  files; nothing sent. Route restored, plan `clean,truncate`. Stage 4 (new
  process, 18.9 s): both fuelings "Aceito pela empresa" with "Aguardando
  aprovação" (one intake request, 200); the first receipt stored, the second
  transfer truncated by the fixture and refused 422 while both fuelings stayed
  accepted and that receipt showed "Enviando"; after the backoff it was stored;
  sealed bytes left the device; a further dispatch and delivery sent nothing;
  cleanup erased both partitions and the keys. Server at stop: intake [200],
  session ×3 [200], ingress clean ×2 [200], truncate [422]; organization A: 2
  fuelings, 2 documents created by the member, 2 `STORED`, 2
  `document.upload.received` and 2 `document.upload.stored` events, 2
  `FUELING` links, 0 trips; organization B: 0 documents. Fixture PASS
  (686.1 s); the whole run passed the first time.
- **Presentation** `EnterpriseFuelingPresentationDeviceTest` 3/3
  (`fuel011-native-app.log`, pt-BR; validation app `5080e3d0…ab5b`, test APK
  `5e1b7e5c…78cd`): an offline fueling is "Salvo neste aparelho", never
  "Aceito", "Revisão: ainda não enviado", the entered total "R$ 970" and "Enviar
  agora" disabled offline; an accepted fueling shows "Aceito pela empresa" and
  "Aguardando aprovação" while its receipt row shows "Foto 1: Enviando" and no
  "Arquivo guardado"; the form through the FLEET-013 picker keeps save
  disabled until complete, shows the arithmetic warning, passes the exact typed
  values with no receipt, and a failed save says "Nada foi salvo".
- **Regressions** on the same APK pair: `EnterpriseReceiptCapturePresentationDeviceTest`
  5/5, `EnterpriseTripPresentationDeviceTest` 6/6, `EnterpriseContextDeviceTest`
  6/6 (real Hilt graph including `FuelingViewModel` and `ReceiptCaptureViewModel`),
  `EnterpriseSyncContextDeviceTest` 2/2 (it now passes its own fueling and
  receipt owners; the FLEET-013 intermittent did not recur in this run).
- Not run natively: a real camera or photo picker activity, two physical
  devices, a held fueling on the device (host-proven), a receipt refused finally
  by the server (host-proven in OFFL-005).

## Named negatives → tests

| Negative / acceptance | Test(s) |
|---|---|
| Local save without durable queue | `a failed write, an unknown responsible or an inexact value saves nothing` (failed protected write: `saveFailed`, nothing listed, no receipt); `ProtectedFuelingStoreTest` (only what the protected write committed is read back; an identical retry is idempotent); presentation (a failed save says "Nada foi salvo") |
| Wrong account or organization | flow test (another organization's asset choice is refused `STALE`, nothing saved; B lists nothing; the operation is bound to A's account/origin/organization/membership); store test (B reads nothing); native stage 2 (B's flow lists nothing; A's fueling cannot own a receipt in B: `UNKNOWN_OPERATION`), stage 3 (B empty) and fixture (B holds no document); server: `TestFuelingReceiptLinksOnlyTheAuthorsStoredFuelingThroughComposedCompanyHTTP` (another organization resolves the fueling `NOT_FOUND`) |
| Synthetic-scope bypass | every capture through the DOC-004 path (synthetic-only notice, JPEG by content, OQ-04 flag off in normal builds); the builder never produces `unit_price_text`, a trip, supplier, place reference or envelope attachment (shared vectors: retained forms `UNSUPPORTED`); `EnterpriseFuelingProducer` refuses any operation its decoder does not read back |
| Accepted status before acknowledgement | `acceptance follows only the fueling's own receipt` (a held `RETRY` never shows accepted; review "Aguardando aprovação" only after `ACCEPTED`; nothing re-sent after acceptance); `EnterpriseFuelingPresentationTest`; device presentation (offline fueling never "Aceito", send disabled); native stages 2–4 |
| Process death | `a restart keeps the fueling, its amounts and its receipt`; native stages 2→3 (both fuelings with exact quantities, totals and unit price, both sealed receipts with their digests after force-stop) |
| Attachment progress separate from acceptance | flow test (accepted fueling while its receipt is "Enviando"); device presentation; native stage 4 (both fuelings accepted while the truncated receipt transfer is refused 422 and stays "Enviando", then stored) |
| Receipt write fails or is refused | flow tests (`receiptNotSaved` with the fueling kept; `DISABLED`; unknown fueling `UNKNOWN_OPERATION`) |
| Wrong record on the server | composed HTTP test (another member's fueling `FORBIDDEN` with `fuel.fueling.record`, missing fueling and the fueling's own embedded reading `NOT_FOUND`, refusals store nothing, by-target read under the owner's permission) |
| Warnings never block | `warnings inform and never block` (arithmetic mismatch and `READING_BELOW_PREVIOUS`, both still saved); device presentation (warning shown, save enabled) |
| Exact entry | `EnterpriseFuelingPresentationTest` (`FuelingEntry`: decimal comma/point, too many decimals, grouping, signs, exponents, non-ASCII digits refused); flow test (`150,0001` refused, nothing saved) |
| Producer bytes are what the intake parses | `fueling-submit-v1.psv` with both consumers |
| Migration | `TestFuelingLinkPopulatedUpgradeFailureRecoveryAndPreviousImage` (populated 000244 head with an accepted fueling, its reading and expense and an ASSET link; injected failure leaves schema, constraints and data unchanged; recovery once; replay no-op; previous image compatible; FUELING carried only by `fueling_id`, older bindings kept; composite foreign key; forced RLS) |

## Retained (with reasons; not part of this task's acceptance)

- **Trip link and place references** (soft references with `ORPHAN_REFERENCE`
  holds), contracted supplier, internal tank, agreement vouchers (online-only),
  `unit_price_text` for prices with more than three decimals (no rounding
  owner): the intake keeps them `UNSUPPORTED`, the app never produces them.
- **Capacity warning** (`OVER_CAPACITY` needs the tank capacity in the feed),
  previous readings from the device's trips in the METER-003 series, held
  fueling review results on the device (FUEL-003 owns the held form),
  correction/cancel (FUEL-004/015), list retention and dismissal.
- **Receipts**: real receipts (OQ-04), replacement/unlink (DOC-011), a single
  protected write for fueling and receipt (EDR-293), production grants for the
  link principal (`SELECT (org_id,id,author_membership_id) ON org_fuelings`).
- The trip closing conference (EMP-TRIP-020), the panel (FUEL-012), two
  physical devices, a real camera, visual/TalkBack review
  (`VISUAL_VALIDATION_PENDING`), merged-tree verification and any release.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. `docs/enterprise/10-offline-sync.md` §10.3, after the EMP-DOC-004 sentence:
   "Fuelings are recorded offline as signed `fueling.submit` operations read
   back from the partition's queue, and their optional receipt is sealed after
   them and linked to the stored fueling through the `FUELING` link kind
   (EMP-FUEL-011, EDR-294..297)."
2. `docs/enterprise/modules/fuelings.md` §8 Operations, after the EDR-175
   sentence: "EMP-FUEL-011 builds it on Android (shared oracle
   `fueling-submit-v1.psv`) and links receipts to stored fuelings through the
   DOC-003 `FUELING` kind (migration 000245)."
3. `backend/README.md` "Organization document links (EMP-DOC-003)": "Migration
   245 adds the `FUELING` kind (`fueling_id`, composite foreign key to
   `org_fuelings`); the link principal needs `SELECT (org_id,id,author_membership_id)
   ON org_fuelings` and the migrator `REFERENCES ON org_fuelings` and
   ownership of `org_document_links`. Nothing here provisions live grants."
4. `data/src/main/kotlin/com/anpfuel/data/enterprise/README.md`: "EMP-FUEL-011
   registers the `fueling.submit` producer; fuelings are read back from the
   partition's retained queue entries (`ProtectedFuelingStore`), with no
   separate index."
