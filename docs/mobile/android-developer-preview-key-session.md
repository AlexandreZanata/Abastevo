# Temporary Android UI preview and account-key session

Scope: user request 2026-10-07; P37-T01/T02 and P36-T01/T03.
Parent: ce04ba1. Isolated checkout because maintained dev contains other edits.

B-BR-DEV01: an explicit, default-off debug-only UI preview allows camera or
gallery without location. The existing manually selected Home city is reused;
typing a station name searches only that city and requires an explicit canonical
UUID selection. A late normal nearby response cannot clear the manually selected development
station; normal location loading is disabled while preview is active.
Release controls are absent. No GPS is fabricated; import time
is not verified shutter time. Private copies retain bounded size and 24h TTL.

The user subsequently required real local/server submission and explicitly
authorized deployment, private S3 storage and a worker only in abastevo-temp.
The server flag defaults OFF, is forbidden in production and foreign staging
hosts, and expires automatically (configured at deployment for 24 hours).
Signed nonce/account/quota/owner/idempotency checks remain. Receipts persist a
distinct photo-capture-ui-test-v1 provenance; normal policy refuses those
receipts. Turning off debug mode/restoring into normal mode blocks their use.

BUC-DEV01: choose city on Home -> Community/contribute -> enable temporary
developer mode -> search station by name and explicitly select it -> obtain
signed development receipt -> camera/gallery -> existing OCR/crop/fuel review
-> explicit submit -> durable signed outbox -> private S3 -> server status.

Deployment boundary: only abastevo-temp API, worker, storage and owned secret/
migration resources. No other namespace, global prune, shared edge change or
other application's database/image is authorized. Keep previous API image and
take an owned database backup before the append-only upgrade. No GitHub push.

B-BR-KEY01: one successful native credential confirmation authorizes reveal,
copy, save and share for the same account during the app session. Never persist
the grant or key. Cancellation, failed launches and late callbacks do not grant
access. Closing the activity (except configuration recreation), process death,
logout, deletion or changing account invalidates the grant. Backgrounding for
the native credential dialog, file picker or share sheet does not invalidate it.
Secure window, masking and clipboard expiry remain active.

BUC-KEY01: reveal -> authenticate once -> copy/export without another native
prompt -> close/reopen or logout -> a new prompt is required.

B-BR-UI01: signup completion and retained login use the exact account-detail
status/key/action components, spacing, surfaces, typography and buttons. Keep
the signup Continue action and existing login/navigation/session behavior.

Status: LOCAL_DONE
Validation: PASS

Behavior fingerprint (sorted changed/untracked non-doc paths + NUL + bytes +
NUL, SHA-256): `acfbe9362ddc567f0e775c90fba7dcebdb94ac2e3724994d60efb16117bc7ff5`
(48 behavior/test/contract files; parent ce04ba1).

## Local verification

- RED: the new backend flag/policy and Android APIs were absent before the
  change. The additional delayed-nearby test reproduced a lost manual station;
  guarding normal loading/results while preview is active made it GREEN.
- Final debug unit suites: CaptureOcrViewModelTest (27), AuthViewModelTest (23),
  KeyUnlockSessionTest (4); data PhotoCaptureHttpClientTest (3),
  DirectoryStationHttpClientTest (6). All passed, no skips. Debug APK and lint
  passed; Android lint has zero errors.
- Release variants: ReleaseDevelopmentCaptureTest and
  ReleaseDevelopmentPhotoCaptureTest passed (one each). The release transport
  cannot produce a development request even for the owned staging host.
  Final R8/APK/size passed (4,231,129 bytes). The combined invocation
  reported exhausted JVM metaspace before a reflected release Kotlin failure.
  A fresh serial release invocation with 1GiB metaspace passed in 3m 41s,
  without project memory/config changes. Release DEX contains no developer UI
  or evaluation activity class markers. Debug APK SHA-256:
  18370517f0565a338028fccb1a9abb05f12faa5a4c3788baed2f0e7c76c89223.
- Backend: affected config/community application/HTTP/proxy/API tests passed
  with race detection. Production/foreign-host/expired flag, unknown station,
  missing proof, normal location requirement, owner/key/window/provenance,
  stale replay and immutable shared-photo bindings are covered.
- Disposable real PostGIS: development policy persistence, replay/owner/recovery
  and existing capture/shared-photo concurrency passed. Both ordinary media and
  development-capture API/worker process cases passed against real private S3.
  Development process: two reviewed fuel rows reached VALIDATED; foreign owners
  received 404. Synthetic JPEG bytes only.
- Always-active integration proxy test uses an isolated S3 protocol peer to
  check signed host/path/query/MIME/bytes and anonymous refusal without requiring
  a workstation port. Additional real MinIO PUT/GET/DELETE passed before the
  live HTTPS pipeline. No integration test is skipped or weakened.
- `sqlc generate`, `sqlc vet`, affected `go vet`/`staticcheck`,
  `bash scripts/check-mobile.sh --static-only`,
  `bash scripts/check-security.sh --static-only`, `git diff --check` and
  `bash scripts/scan-secrets.sh` passed. OpenAPI vacuum: zero errors,
  62 existing warnings and 41 informs. No SDK/dependency addition, contributor
  GPS, production photo, private key/token or live secret enters Git/logs.
- POCO 2311DRK48G / API 36: updated debug and instrumentation APKs with `adb
  install -r`; no account/data wipe. Raw AndroidJUnitRunner with
  `clearPackageData=false` ran AuthAccountContinuityDeviceTest plus the controlled
  outside-station and synthetic private-age/pixel-bounds methods from
  PhotoReviewDeviceTest: `OK (3 tests)`. Account test uses unique synthetic prefs
  and real isolated Keystore aliases and cleans only those aliases.

Exact Gradle selection (offline, serial, one worker, in-process Kotlin):

```sh
./gradlew --offline --no-daemon --no-parallel --max-workers=1 \
  -Pkotlin.compiler.execution.strategy=in-process \
  :data:testDebugUnitTest --tests '*PhotoCaptureHttpClientTest' \
  --tests '*DirectoryStationHttpClientTest' \
  :app:testDebugUnitTest --tests '*KeyUnlockSessionTest' \
  --tests '*CaptureOcrViewModelTest' --tests '*AuthViewModelTest' \
  :app:assembleDebug :app:lintDebug --console=plain
./gradlew --offline --no-daemon --no-parallel --max-workers=1 \
  -Pkotlin.compiler.execution.strategy=in-process \
  :app:assembleDebugAndroidTest --console=plain
./gradlew --offline --no-daemon --no-parallel --max-workers=1 \
  -Pkotlin.compiler.execution.strategy=in-process \
  :data:testReleaseUnitTest --tests '*ReleaseDevelopmentPhotoCaptureTest' \
  :app:testReleaseUnitTest --tests '*ReleaseDevelopmentCaptureTest' \
  :app:verifyReleaseApkSize --console=plain
# Final release repeat after the combined daemon exhausted metaspace:
./gradlew --offline --no-daemon --no-parallel --max-workers=1 \
  -Dorg.gradle.jvmargs='-Xmx2g -XX:MaxMetaspaceSize=1024m -Dfile.encoding=UTF-8' \
  -Pkotlin.compiler.execution.strategy=in-process \
  :app:testReleaseUnitTest --tests '*ReleaseDevelopmentCaptureTest' \
  :app:verifyReleaseApkSize --console=plain --stacktrace
```

Affected Go selections (inside backend, Go 1.27.1; disposable database/storage
provided through test environment variables, never a live DSN):

```sh
go test -race ./internal/platform/config \
  ./internal/modules/community/application \
  ./internal/modules/community/adapters/http ./internal/platform/httpapi \
  ./cmd/api ./cmd/worker
go test -race -count=1 -tags=integration \
  ./internal/modules/community/adapters -run 'Test(DevelopmentPhotoPreview|PhotoCapture|SharedPhoto)'
go test -race -count=1 -tags=integration \
  ./internal/platform/httpapi -run TestDevelopmentStorageSignedProxyRoundTrip
go test -race -count=1 -tags=integration \
  ./cmd/api -run TestPublicProcessDevelopmentPhotoPreview
```

## Authorized temporary deployment

- Actual namespace: abastevo-temp; existing Deployment abastevo-temp-api now
  owns API + worker + private MinIO sidecars. Database, old API deployment and
  secret were backed up privately before append-only migrations 43–48.
- Owned origin: https://teste.abastevo.com.br. Private bucket
  abastevo-dev-photos is reachable only through signed S3 requests on this
  origin; MinIO binds loopback, console/browser disabled, anonymous denied,
  bucket-only app credentials, one-day object expiry. No edge/NodePort change.
- Automatic capture/submit cutoff: 2026-10-08T23:44:21Z
  (2026-10-08 19:44:21 America/Cuiaba). Defaults remain OFF elsewhere. Deadline
  expiry is a server refusal, not a claim that the sidecars remove themselves.
- Private media is temporary: 5GiB emptyDir, lost on pod replacement; no
  production durability/certification claim. After initial startup OOM, MinIO
  limit is 1GiB with GOMEMLIMIT=384MiB. Rollout maxSurge=0/maxUnavailable=1
  respects the existing namespace CPU quota. Latest pod: 3/3 ready, zero
  restarts across ten hours, HTTPS health ready.
- Images (same tested backend source, revision label ce04ba1-dev-preview-20261007):
  API 80a423f225618ac743bba7a2b3f9763cddb76f7e2065a49d426134a927c23af2;
  worker 43948beaea782e28cd1bf6dd31f1fcb789edfff7454298a36751fce038034583;
  migrate 23f288048834f6b2f71702431eccda16e91edbaf52672443c19bdd6f307e8367;
  storage 359182bc95e9029cbf11886c20944f9f8173a93aed83faa226cb795adf711792.
- Storage uses the existing project MinIO dependency (AGPLv3), cached
  RELEASE.2025-04-22T22-12-26Z because the project-pinned upstream image could
  not be pulled. Original notices preserved; no MinIO source change. Bounded
  nonproduction validation only; no claim of current production suitability.
- Real HTTPS smoke: signed synthetic contributor -> development receipt with
  no GPS -> JPEG PUT -> complete -> worker READY -> GASOLINE_REGULAR + ETHANOL
  VALIDATED. Idempotent retries retained the same facts. Anonymous media denied
  and a normal capture without location still refused. TLS verification stayed
  enabled. No production data was submitted.
- All VPS mutations targeted abastevo-temp and owned /opt/abastevo-temp paths.
  Peer snapshot: 40 non-Abastevo pods retained UID/restart counts; the
  alta-floresta API moved to another ReplicaSet concurrently. No operation in
  this task wrote that namespace, another app's image/config, global quota,
  firewall, shared edge or production database.
- Local-only VPS manifests/rollback notes are under
  .local/abastevo-temp/dev-preview-20261007 in the VPS repository. Secrets,
  database backup and live smoke remain private under the owned VPS path.
  Nothing from that repository is committed or pushed.

## User validation and remaining boundaries

Choose Curitiba on Home, reopen contribution, enable temporary developer
mode, type POSTO PENTEST TEMP, explicitly pick the canonical result, then
choose camera or JPEG/PNG gallery and review/edit/crop the fuel rows before
Send. This is the only station currently present in the temporary server
catalog; typing does not silently create an unverified station. Other cities
need their own actual server catalog entries.

Native device credential entry/biometric success is a human hardware action.
The memory grant, cancellation/late-result/reset boundaries are unit tested;
the actual one-prompt reveal/copy/export sequence remains for owner validation.
Gallery provider selection and a complete Android-to-server visual walkthrough
remain owner QA; local/backend/device results above must not impersonate them.
The temporary staging deployment does not accept PC05/expanded PC06, G09,
production release, public pilot, iOS, merge or wiki gates.

Integration: isolated codex/android-dev-preview-key preserves the occupied dev
checkout and its unrelated UI work. Integrate the owned commit when dev is
available, then use protected dev -> main batch delivery if publication is
subsequently authorized. No GitHub push/PR/merge/wiki publication in this task.
