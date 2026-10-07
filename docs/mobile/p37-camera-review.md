# P37-PC03 camera and review behavior

B-BR-PC01/PC03/PC04 and BUC-PC01/PC03 are defined in the canonical photo plan.
Location permission is requested before camera permission. Every camera launch
obtains a fresh one-shot OS fix and a signed server receipt for the selected
canonical station. Missing/stale/mock/poor fixes, absent station, offline or
server refusal produce instructions and zero camera launches. No precise fix
enters saved state, outbox or logs. Server receipts keep their original deadlines.

Registration/signing uses the exact server-issued nonce, frozen SHA-512 signature
base and Android Keystore. Redirects and automatic mutation retries are disabled.
The account/environment scope is captured with the receipt for later outbox
checks. A test harness in debug sources alone may supply controlled proximity;
it never sends historical evaluation prices to a real backend.

Review runs decoding/encoding/OCR off the UI thread. Generation checks reject
late results; editing/removal during recognition cannot be overwritten. Only
supported detected fuels are prefilled. Explicit add permits manual correction
when labels cannot be read. Blank/X rows are omitted. No payment controls exist;
conditional evidence requires explicit unconditional-price confirmation.

A saved draft stores private media identifiers/rows/capture time, not pixels or
GPS. Native camera files are private and bounded, swept at 24 hours; crops retain
the original capture time and replace obsolete private material. Crop and OCR
are explicit. Account/environment changes invalidate capture proof. No upload
occurs before human confirmation.

Status: LOCAL_DONE
Validation: PASS

Local source/camera/review acceptance only; live submission remains PC04.

## Local evidence

Parent: df77de6 on maintained dev. No deployment/publication occurred.
The private corpus stays ignored; Android instrumentation imports originals
into app-private debug storage rather than fixtures or the gallery.

Passed before final presentation fixes: 21 capture ViewModel regressions,
5 OS-location regressions, 1 pure fix eligibility test, 9 portable photo tests,
2 receipt HTTP tests, 1 exact-body/nonce/redirect proof test, and the existing
anonymous HTTP regressions. POCO API36: three native review tests passed in
7.645s (four genuine OCR rows, edit/X/blank -> two local reviewed rows, outside
151m -> instructions without camera, expiry and frozen JPEG pixel budgets).
The isolated data test package also passed real non-exportable P-256/SHA-512
Keystore signing (one test). This does not delete the main app identity.

ADB operated the real system camera: controlled inside 150m -> camera ->
shutter -> Done -> large retained photo and honest empty-OCR/manual-add review.
The camera test used controlled debug eligibility; no backend authorization,
actual station location or historical observation publication is claimed.

The initial concurrent lint/release run hit a missing generated Hilt Java
source. Serial execution exposed a Compose producer lint diagnostic and
missing translations, including three account labels from the parent.
The decoder state now uses a keyed coroutine and the missing supported-locale
translations are supplied; no lint rule or test was disabled. English/other
locale labels for the legacy GASOLINE_PREMIUM enum now correctly name additive
gasoline, matching GASOLINE_ADDITIVED; true premium stays unsupported by OCR.
Final serial lint/release and native crop/recreation checks passed. The debug evaluation activity now follows app locale and safe insets.

Verified staging TLS fails on host/local Wi-Fi due the untrusted intercepted
chain. A read-only cellular probe on POCO passed TLS and loaded the directory,
then failed the existing profile-read assertion because the selected station
profile was unavailable. Wi-Fi restoration was requested after the probe, then its enabled radio state was explicitly verified. This distinguishes
network interception from a separate deployed-profile integration limitation;
neither failed live test is counted as acceptance. No certificate validation
was bypassed, no account/precise fix/photo/price was sent in these reads.

Next slice after final scoped checks: PC04 signed durable subset/shared-photo
submission. The old submission adapter remains a prototype and is not runtime
acceptance for this new camera flow. PC05 consent/storage/deletion and PC06
expanded benchmark/live acceptance remain required.

Final source/tests fingerprint (32 changed Android/application/data files):
`72b401463c2b1123eff6b02da03c80580080a54e458d08b6c6362ed2a67cd453`.
42 scoped unit tests passed with no skips/failures. Final POCO native four-case
review suite passed (12.331s), then passed with 160% system font (14.1s); original
font scale was restored and verified. The expiry test targets its own synthetic
URI only. Actual production SavedStateHandle receipt/row/age recovery and
late-result/owner/target rejection remain covered by the scoped ViewModel tests;
the debug activity separately proves the shared review UI through recreation.

Commands used sequentially with `--offline --no-daemon --no-parallel
--max-workers=1 -Pkotlin.compiler.execution.strategy=in-process` and the existing
Gradle cache (task-local JVM limits):

- `:application:test --tests com.anpfuel.application.capture.CaptureFixTest --tests com.anpfuel.application.portable.PortablePhotoFlowTest`.
- `:data:testDebugUnitTest --tests com.anpfuel.data.remote.PhotoCaptureHttpClientTest --tests com.anpfuel.data.remote.PhotoProofTransportTest --tests com.anpfuel.data.remote.AnonymousProofHttpClientTest`.
- `:app:testDebugUnitTest --tests com.anpfuel.app.capture.CaptureOcrViewModelTest --tests com.anpfuel.app.location.LocationPermissionHandlerTest`.
- `:app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug :app:verifyReleaseApkSize`.
- After isolating the native expiry test: `:app:assembleDebugAndroidTest :app:lintDebug` passed again; release source stayed unchanged.
- Owner-selected POCO `am instrument -w -r -e class com.anpfuel.app.capture.PhotoReviewDeviceTest -e clearPackageData false` and isolated `AnonymousDeviceKeysDeviceTest`.

Debug lint: zero errors, 189 existing/toolchain warnings. Release compilation,
R8, vital lint and size check passed: 4,191,429 bytes (<15MiB). Binary manifest
and DEX scans exclude the evaluation activity, private corpus path and controlled
distance hooks in both UTF-8/UTF-16. Static/import/fixture/no-background-location,
diff and secret guards passed. No photos, OCR output, device serial or real fixes
are tracked. No final aggregate/remote CI run was repeated for documentation.
