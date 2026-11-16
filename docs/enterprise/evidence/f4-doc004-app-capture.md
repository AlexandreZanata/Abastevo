# EMP-DOC-004 — App: capture and import with offline queue and legibility check

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F3/C1 Android task (module DOC, P0, type APP, size M; prerequisite of
EMP-FUEL-011 → EMP-TRIP-020) on `codex/enterprise-f4-doc004` (worktree
`/data/dev/worktrees/f4-doc004`), based on the F4 head `b4249772` (main
`b0ab7d33` + the F4 continuation incl. EMP-OFFL-005, TRIP-011/014/018/007,
GPS-001; compileSdk/targetSdk 36). The coordinator integrates it into
`codex/enterprise-f4` and owns PROGRESS, the F4 ledger, pushes and the PR.
Synthetic data only: device-encoded synthetic JPEG/PNG pictures, never a camera
photo of a real receipt, person, plate or place; real receipts stay legally
gated (OQ-04, LV-04, LV-11). No migration, SQL, route, permission, role, job
or production Go code; one test-only Go native fixture. The only Android device
used is the owned emulator `emulator-5588` (AVD `enterprise-f2-api36`, API 36),
every native run serialized with `flock /data/dev/worktrees/.f4-emulator.lock`;
the physical phone was never addressed.

Sources read: the EMP-DOC-004 block and the DOC-002/003/005/014 and FUEL-011
blocks ([EP09](../backlog/ep09-documents.md), [EP08](../backlog/ep08-fuelings.md));
[10-offline-sync](../10-offline-sync.md) §§4–6 and 10.3–10.4 (BR-OFFL-002/003/
013/015, UC-OFFL-03, the "Receipt capture and attachment" matrix row);
[11-privacy-security](../11-privacy-security.md) §4 (BR-PRIV-016/020);
[documents](../modules/documents.md) (BR-DOC-001..004, UC-DOC-01, SCR-DOC-01);
[12-ux-app](../12-ux-app.md) §4.9 and §5.1 ("Receipt camera");
REQ-10.1 in [17-traceability](../17-traceability.md); OQ-04 and the
[approved receipt scope](f0-m0-decisions.md#private-receipts); EDR-267..270 and
the latest rows of [16-decisions](../16-decisions.md); the evidence of
EMP-OFFL-005 (sealed `ATTACHMENT` entries and `attachment-book`, capture only
beside a queued operation, the signed session and capability ingress, the
`ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED` flag, its retained list naming DOC-004),
EMP-TRIP-005 (record flows, presentation, native acceptance pattern), FLEET-013
and METER-003; the existing app capture code (`CaptureScreen.kt` system camera
through `TakePicture`, `VehicleScreen.kt` photo picker, `PrivateCaptureFiles`);
the F4 ledger.

IDs used: B-BR-EMP-450..454, BUC-EMP-228..230, EDR-291..293 (all of them;
none left unused). No migration number was needed.

## Observed before any change

- EMP-OFFL-005 left no capture or import screen and no producer: the native
  run called `EnterpriseAttachmentService.capture` directly. `AttachmentLabel`
  was the contract for the screens; no string existed.
- The app already uses two dependency-free platform contracts from
  `androidx.activity`: the system camera (`TakePicture` into a FileProvider
  cache file, `CaptureScreen`) and the Android photo picker
  (`PickVisualMedia(ImageOnly)`, `VehicleScreen`); it also has CameraX for the
  live price board. The manifest declares `CAMERA`, so the system camera intent
  needs the granted permission.
- The server accepts only `image/jpeg` C1 receipts (EMP-DOC-002), so a PNG,
  HEIC or PDF would only be refused later; BR-PRIV-020 decides type by content
  and preserves originals.
- The only C1 producer whose records have a registered link kind is the trip
  (departure and final readings, `METER_READING`).

## Rules frozen before behaviour

B-BR-EMP-450..454 and BUC-EMP-228..230 are the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
section "EMP-DOC-004 — App: capture and import with offline queue and
legibility check (2026-11-15)"; EDR-291..293 are appended to the EDR table of
[16-decisions](../16-decisions.md). Both were written before any code (the
stored-label wording of B-BR-EMP-454 was then tightened to "Arquivo guardado
pela empresa" so that no label can read as a validation of the receipt).
Recommended routine decisions, taken unattended:

- **EDR-291 — capture sources.** System camera (`TakePicture` into a
  FileProvider file under `cache/enterprise-receipts/`, only after `CAMERA` is
  granted) and the photo picker (no storage permission); every selection read
  once, bounded to 5 MiB + 1 byte, judged by content with the OFFL-005 JPEG
  policy, kept unmodified; the camera file deleted right after reading and the
  folder swept when capture opens. No new dependency (no licence/security
  review needed); CameraX, SAF/PDF/XML import and transcoding rejected.
- **EDR-292 — legibility hint.** Pure Kotlin integer heuristic over a ≤512-px
  BT.601 luminance sample: `LOW_RESOLUTION` (shortest original side < 480 px),
  `TOO_DARK` (mean < 60), `TOO_BRIGHT` (mean > 225), `LOW_CONTRAST`
  (standard deviation < 24), `BLURRED` (4-neighbour Laplacian variance < 60);
  `NOT_CHECKED` for an undecodable image; never blocks, never stored or sent,
  always shown with "esta conferência é só uma dica; ela não valida o
  comprovante". No OCR or ML Kit.
- **EDR-293 — ownership.** The record's own flow (C1: `TripFlow`) owns the
  capture: the unsaved photo is memory-only, the save goes through
  `EnterpriseReceipts` (thin wrapper of `EnterpriseAttachmentService.capture`
  mapping local refusals to closed reasons) inside the flow's existing
  account/epoch/partition fencing; operation and photo stay two protected
  writes; rollback flag `ENTERPRISE_RECEIPT_CAPTURE_UI_ENABLED`.

## Delivered surface

- **Domain** `domain/.../enterprise/attachment/ReceiptCapture.kt` (pure):
  `ReceiptSource`, `ReceiptRefusal` (`MISSING`, `TOO_LARGE`, `UNSUPPORTED`),
  `ReceiptOriginal` (exact bytes, copied, redacted), `ReceiptIntake.inspect`
  (OFFL-005 `AttachmentPolicy` as the single judge: size, then JPEG markers;
  `READ_LIMIT` = 5 MiB + 1), `LegibilityIssue`, `LegibilityHint`,
  `LumaSample` (bounded, `fromArgb` BT.601 integer luma) and
  `ReceiptLegibility.assess` (exact integer statistics).
- **Application** `application/.../enterprise/attachment/Receipts.kt`: the
  platform port `ReceiptOriginals` (camera target, bounded read, discard,
  sample), `ReceiptDraft` and `ReceiptCaptureState` (memory only),
  `ReceiptCaptureController` (camera denied/cancelled/missing/unsupported never
  become a draft; a refused new selection keeps the previous photo; the camera
  file is discarded whatever the read gives; the picker's files are never
  deleted), `ReceiptSaveRefused` and `EnterpriseReceipts` (`save`, `byOwner`).
  `TripFlow` gains `attach` (fenced write; a local refusal is
  `attachmentRefusal`, a failed write `saveFailed`, a sealed photo
  `lastAttached`) and reads the partition's attachments beside the trips
  (`attachments`; an unreadable index only sets `attachmentsUnavailable`).
- **Data** `data/.../enterprise/attachment/AndroidReceiptOriginals.kt`
  (FileProvider camera target under `enterprise-receipts/`, content-scheme
  bounded read, deletion only of the app's own camera files, `BitmapFactory`
  sampled decode to a `LumaSample`), DI for it and for `EnterpriseReceipts`
  (enabled with `ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED`), `TripFlow` composed
  with the receipts.
- **App** `app/.../enterprise/attachment/ReceiptCapture.kt` (SCR-DOC-01):
  `ReceiptCaptureViewModel` (Hilt; `enabled` = rollback flag ∧ queue flag),
  `rememberSystemReceiptPickers` (permission → `TakePicture`; `PickVisualMedia`
  images only; the pending camera reference survives recreation),
  `ReceiptCaptureCard` (synthetic-only notice, camera/import buttons, bounded
  preview, the hint with its disclaimer, keep or retake, "Salvar no aparelho"
  only for an accepted original), `AttachmentRows` (per-photo label and
  refusal reason, retake for a missing file). `TripScreen`: start and finish
  records that carry a reading show their photos and "Adicionar foto da
  leitura" (target `METER_READING`, owner the record's operation); the sheet
  closes only on `lastAttached`; feedback for refusals, saved photo and an
  unreadable index. `EnterpriseContextScreen` wires the sheet and drops an
  unsaved photo when trips close. `res/xml/file_paths.xml` adds
  `enterprise_receipts`; build flag `ENTERPRISE_RECEIPT_CAPTURE_UI_ENABLED`
  (default on). 49 strings × 8 locales (`enterprise_receipts.xml`, generated
  from one table by `f4-logs/doc004-scripts/gen_receipt_strings.py`); existing
  white container tokens only, no new colour.
- **Go (test only)** `organization/adapters/orgsync_receipt_native_fixture_test.go`
  (tags `integration nativefixture`): `TestReceiptNativeSourceFixture`, the
  OFFL-005 fixture generalised — the real default runtime with every default
  owner, the ingress fault plan (`truncate`, `flip`) and, at the stop, the
  exact effects the driver names in `expect.json` (documents, `STORED`, link
  kinds, ingress attempts, fuelings, trips; organization B nothing), reusable
  by the receipt producers (EMP-FUEL-011).

## RED → GREEN → REFACTOR

Logs in `/data/dev/worktrees/f4-logs/doc004-*` (XML in `doc004-kotlin-red-xml/`).
Gradle with JDK 17, `--offline --no-daemon --no-parallel --max-workers=2`
under `flock /data/dev/worktrees/.f4-gradle.lock` (script
`doc004-scripts/gradle.sh`). Shape first: the domain types with `TODO()`
bodies (`ReceiptIntake.inspect`, `LumaSample.fromArgb`,
`ReceiptLegibility.assess`), the application port, state, controller and
`EnterpriseReceipts` with `TODO()` bodies, `TripFlow.attach` as `TODO()` and the
new `TripViewState` fields unread.

- **Domain RED** (`doc004-kotlin-domain-app-red.log`): `ReceiptCaptureTest`
  10 tests, 10 failing (nine on `NotImplementedError`, the bounds test on
  `assertThrows` receiving `NotImplementedError` instead of
  `IllegalArgumentException`). The legibility expectations come from the
  independent oracle `doc004-scripts/legibility_oracle.py` over the same
  synthetic grids (sharp text-like stripes, dark, bright, flat, gradient,
  box-blurred text, exact mean/deviation/side boundaries).
- **Application RED** (`doc004-kotlin-application-red.log`; the first run
  failed to compile because a new constructor parameter after `deliver` broke
  the trailing-lambda call sites — `receipts` was moved before `deliver`):
  `EnterpriseReceiptCaptureControllerTest` 6/6 failing on
  `NotImplementedError`, `EnterpriseTripReceiptTest` 9/9 failing (eight on
  `NotImplementedError`, the unreadable-index test on its assertion because
  the stub publish never read attachments).
- **GREEN** on the first implementation run (`doc004-kotlin-domain-app-green-1.log`):
  domain 10/10, controller 6/6, trip receipts 9/9 and the unchanged
  `EnterpriseTripFlowTest` 12/12. The `TripViewModel` forwarding test (current
  gesture only, offline allowed) was written with the wiring, not before it,
  and is not claimed as RED.
- **REFACTOR**: the preview decode moved out of the composable; lint's
  `ProduceStateDoesNotAssignValue` (an error, twice, even with an explicit
  `value =`) led to a `LaunchedEffect` over a remembered state; the
  `ReceiptCaptureViewModel` gained a plain constructor (the Hilt one delegates
  to it) so the Hilt-free `EnterpriseSyncContextDeviceTest` can pass its own
  owner with capture off.

## Validation

```sh
# Kotlin (doc004-kotlin-suites-lint-1.log, doc004-lint-normal-build-2.log, doc004-validation-build-2.log)
gradle.sh --continue :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test --tests 'com.anpfuel.application.enterprise.*' \
  :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.enterprise.*'
    # domain enterprise 4104, application enterprise 143, data enterprise 195, app enterprise 24 tests: 0 failures, errors or skips
gradle.sh --continue :app:assembleDebug :app:lintDebug :data:lintDebug
    # pass; lint 0 errors; app 354 warnings (+2 GradleDependency "newer version available" on unchanged
    # build.gradle.kts dependency lines versus the 352 recorded by OFFL-005), data 17; none in a touched source
gradle.sh -PenterpriseDeviceValidation=true :app:assembleDebug :app:assembleDebugAndroidTest :data:assembleDebugAndroidTest   # pass
# Go (backend/, go1.27.4 toolchain; only a test-only nativefixture file changed)
gofmt -l ./internal/modules/organization/adapters/       # empty (one trailing blank line fixed)
go build ./... && go vet ./... && go vet -tags=integration ./... \
  && go vet -tags='integration nativefixture' ./internal/modules/organization/adapters/   # doc004-go-vet.log: clean
GOTOOLCHAIN=go1.27.1 staticcheck -tags='integration nativefixture' ./internal/modules/organization/adapters/   # no findings
python3 docs/enterprise/tools/check_plan.py              # tasks 348; rules 545; use cases 167; screens 145; problems 0
git diff --cached --check                                # clean
```

- No production Go, SQL, migration, sqlc input, route, permission, OpenAPI or
  job changed, so `sqlc`, the route inventory, the authorization matrix, the
  AUDITOR walk, the race/integration union and the untagged `go test ./...`
  were not affected and not rerun for this task (the coordinator's
  merged-tree verification covers them); the native fixture itself ran
  against real PostGIS (below).
- Secret/PII review of the staged diff: synthetic `7b0e2a54-…`/`7d0c0000-…`/
  `7d000000-…` identifiers, device-encoded synthetic JPEG/PNG pictures and
  byte arrays; the only credential names are the disposable
  `ANPFUEL_TEST_STORAGE_*` variable names. The native fixture's proof, CA and
  session stayed in the mode-0700 `/data/dev/cache/f4-doc004-native`
  directory (empty after the run) and the test package's private storage,
  erased by stage 4's cleanup; fixture and driver logs carry status codes and
  counts only.

## Native acceptance on `emulator-5588`

Acceptance, not RED: written after the services. Every run under
`flock /data/dev/worktrees/.f4-emulator.lock`; the physical phone was never
addressed.

- **Real signed backend** (`doc004-native-source.log`,
  `doc004-native-device.log`, `doc004-native-driver.out`; scripts
  `doc004-scripts/native-source.sh` and `native-driver.sh`):
  `TestReceiptNativeSourceFixture` (tags `integration nativefixture`) over an
  `orgtest` child database of the owned PostGIS `127.0.0.1:32775` and the
  throwaway MinIO project `f4-doc004-s3` (`infra/compose.validation.yml`,
  removed with `down -v` at the end), with `EnterpriseReceiptNativeDeviceTest`
  (data test APK `1e3c28bd…fcdb`) in four processes, the driver force-stopping
  the target between stages. Stage 1 (route present, 4.5 s): custody of both
  organizations and the signed reference pull of A. Route removed. Stage 2
  (new process, 4.4 s): a trip started from the protected cache with the
  184 320 km departure; a 640×960 device-encoded synthetic receipt-like JPEG
  went through the DOC-004 path (content intake, the Android decoder's sample
  — hint "looks sharp" — and `EnterpriseReceipts.save` with target
  `METER_READING`): "Salvo no aparelho", the start "Salvo no aparelho"; the
  dark variant decoded as `TOO_DARK`; a device-encoded PNG refused
  `UNSUPPORTED`; the same record's photo in organization B refused
  `UNKNOWN_OPERATION`; one photo in A, none in B; nothing sent. Stage 3 (new
  process, 0.9 s): the exact original bytes and digest survived the process
  death; B empty; no identifier, digest, target, `attachment-book`,
  `METER_READING` or `JFIF` readable in the partition's generation files;
  nothing sent. Route restored, plan `truncate`. Stage 4 (new process,
  23.4 s): the start accepted (intake 200); the first transfer truncated by the
  fixture answered 422 while the record showed "Aceito pela empresa" and the
  photo "Enviando"; after the backoff the retry stored it, its sealed bytes
  left the device and a further delivery sent nothing; cleanup erased both
  partitions and the keys. Server at stop: intake [200], session ×2 [200],
  ingress truncate [422], clean [200]; organization A: 1 document created by
  the member, 1 `STORED`, 1 `document.upload.received` and 1
  `document.upload.stored` event, 1 `METER_READING` link, 1 trip, 0 fuelings;
  organization B: 0 documents. Fixture PASS (468.1 s); the whole run passed
  the first time.
- **Presentation** `EnterpriseReceiptCapturePresentationDeviceTest` 5/5
  (`doc004-native-app.log`, pt-BR, validation app `3363fc9b…6bac`, test APK
  `cccc5521…9374`; the real `ReceiptCaptureController` over the real
  `AndroidReceiptOriginals` and the app's FileProvider; only the camera and
  picker activities are replaced by stand-ins that write into the app's own
  camera file or return a content reference): the synthetic-only notice;
  save disabled until an accepted original; the camera photo shows its
  preview, "A imagem parece nítida." and the disclaimer, its private camera
  file is gone once read, the saved original equals the camera bytes, and
  "Foto salva no aparelho." and "Foto 1: Salvo no aparelho" appear only when
  the flow reports the protected write; denied camera, PNG import, missing
  file and an empty pick each say so and save nothing (the picker's file is
  never deleted); a dark import shows "A imagem pode estar escura.", may
  still be saved unmodified, and a failed write keeps the photo with "Nada
  foi salvo"; four photos of an accepted start show "Enviando", "Arquivo
  guardado pela empresa", the missing-file text with its retake action and
  "Não aceito" with its reason; no screen text says "válido"; with capture
  switched off no entry point exists.
- **Regressions** on the same APK pair: `EnterpriseTripPresentationDeviceTest`
  6/6, `EnterpriseContextDeviceTest` 6/6 (real Hilt UI graph including the new
  `ReceiptCaptureViewModel` binding), `EnterpriseSyncContextDeviceTest` 3 of 5
  class runs (`doc004-native-app.log`, `doc004-native-synccontext-rep1..4.log`):
  two runs failed `myAssetsShowOnlyTheSelectedOrganizationsProtectedCacheOnlineAndOffline`
  with "My assets not ready at online … error=STALE", the symptom of the
  intermittent retained by FLEET-013 and TRIP-005 (a STALE assets owner after
  the first method); the other three class runs passed both methods. Not
  attributed to this task and not fixed here; retained.
- Not run natively: a real camera or the system photo picker activity (the
  stand-ins exercise the same controller, files and decoder; a TalkBack and
  200 % font pass stays `VISUAL_VALIDATION_PENDING`), two physical devices.

## Named negatives → tests

| Negative / acceptance | Test(s) |
|---|---|
| Missing file | `ReceiptCaptureTest` (null/empty → `MISSING`); controller test (empty camera file, vanished pick, read failure → `MISSING`, no draft); presentation (missing pick says so, nothing saved); `EnterpriseTripReceiptTest` and the service: a sealed original missing at delivery is `FILE_MISSING`, never sent, shown with its text and retake |
| Denied camera/storage permission | controller test (denial saves nothing, import stays available and clears the message); presentation (denied camera message, save disabled); storage needs no permission with the photo picker (EDR-291) and an unreadable picked file reads as missing |
| Process death after local save | `aRestartKeepsTheSealedPhotoWithItsOwnState`; OFFL-005 `ProtectedAttachmentProcessTest` (child JVM halted around the durable replace); native stages 2→3 (exact sealed bytes after force-stop) |
| Wrong organization | `anotherOrganizationNeitherSeesNorOwnsThePhoto` (B lists nothing; A's record refused in B with `UNKNOWN_OPERATION`, no bytes in B); native stage 2 (`UNKNOWN_OPERATION` in B) and stage 3 (B empty), fixture (B holds no document) |
| Unsupported file presented as stored | `theTypeIsDecidedByContentNotByName` (PNG, PDF, HEIC, WebP, text, start-only, too short → `UNSUPPORTED`); `TOO_LARGE` beyond 5 MiB; controller (refusals never become a draft, a refused new selection keeps the previous photo); presentation (PNG refused, no preview, save disabled); native stage 2 (device PNG refused, one photo listed) |
| Saved only after durable persistence | `aReadingPhotoIsSavedOnlyByTheProtectedWriteAndShownUnderItsRecord`; `aFailedWriteSavesNothingAndSaysSo`; presentation (no announcement before `lastAttached`, failed write keeps the photo) |
| Local refusals with reason | `localRefusalsCarryTheirReasonAndSaveNothing` (`UNKNOWN_OPERATION`, `DISABLED`); `receiptsGroupByOwnerAndMapLocalRefusals` |
| Legibility is a hint, never a validation | `ReceiptCaptureTest` legibility tests (each issue, exact boundaries, `NOT_CHECKED` still a photo); presentation (dark photo still saved; disclaimer shown; no "válido"); native stage 2 (Android decoder: sharp and dark) |
| Original kept unmodified | `anAcceptedOriginalIsTheExactBytesAndNeverAliased`; presentation (saved bytes equal the camera/import bytes); native stage 3 (sealed bytes equal the device-encoded original) |
| Photo state independent of its record | `theRecordIsAcceptedWhileItsPhotoIsStillUnverifiedAndAMissingFileIsShown`; presentation (four labels under an accepted start); native stage 4 (record accepted while the truncated transfer stays "Enviando", then stored) |
| Stale gesture or unreadable index | `aStaleGestureSavesNothing`; `anUnreadableAttachmentIndexKeepsTheTripsVisible` |

## Retained (with reasons; not part of this task's acceptance)

- **Producers and kinds.** Fueling receipts (`FUELING`, EMP-FUEL-011, next),
  incident photos on Android (CHECK-003 Android), the closing conference
  (EMP-TRIP-020), checklist photos (CHECK-002 retained a `CHECKLIST_RUN`
  kind). DOC-004 ships the capture path and its first entry point on trip
  readings.
- **Real receipts.** OQ-04/LV-04/LV-11: PDF/XML import, a re-encoding policy
  for camera files above 5 MiB, EXIF/location handling of real camera
  originals, calibration of the legibility thresholds on target devices and
  real receipts, a document-oriented camera frame; the queue flag stays off in
  normal builds, so capture is hidden there.
- **Dismissal and retention** of refused or missing attachments, re-declaration
  after a closed 24 h session (OFFL-005 retained), unlink/replacement
  (DOC-011).
- **Single protected write** for an operation and its photo (needs a new
  queue-transaction surface; EDR-293).
- The `EnterpriseSyncContextDeviceTest` intermittent (2 of 5 class runs here),
  visual/TalkBack review, a real camera and photo picker run, two physical
  devices, merged-tree verification, production grants and any release.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. `docs/enterprise/10-offline-sync.md` §10.3, after the EMP-OFFL-005
   sentence: "Receipt originals come from the system camera or the photo
   picker, are judged by content, kept unmodified, shown with a legibility hint
   that never validates them, and sealed beside their record only after a
   protected write (EMP-DOC-004, EDR-291..293)."
2. `data/src/main/kotlin/com/anpfuel/data/enterprise/README.md`, after the
   EMP-OFFL-005 paragraph: "EMP-DOC-004 adds `AndroidReceiptOriginals` (a
   FileProvider camera file under `cache/enterprise-receipts/`, deleted once
   read; bounded content reads; a decoded luminance sample) and
   `EnterpriseReceipts`; nothing of an unsaved photo is written."
3. `docs/enterprise/modules/documents.md` §11 SCR-DOC-01: "Implemented on trip
   readings by EMP-DOC-004 (capture, import, hint, pending and missing states);
   fueling receipts follow with EMP-FUEL-011."
