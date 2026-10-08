# Temporary Android UI preview and account-key session

Scope: user request 2026-10-07; P37-T01/T02 and P36-T01/T03.
Opened at ce04ba1 in an isolated checkout because maintained dev was occupied.
Combined source preserves the existing UI commits through 4fa7298, rollout
plan 523be92, upload-origin fix abfc9a6 and dependency/auth-test fix 60a3fd7;
no edits were overwritten.

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

## Current combined checkpoint

Behavior merge e26d546 retains maintained dev 60a3fd7 and the previously
validated UI/session/preview changes. A further negative upload-origin test
refuses a configured origin that differs from the signed proof before network
I/O. Current behavior/test fingerprint: 71 files,
`9c7c4460d48c2fde18fb81091a6990934fa55cd2914181d2360f746f0773c82b`.
Debug APK SHA-256:
`cbda350bdcb0042151e1775201756b2ac7ba05c8a60ebd04c1aa23e9ec880a9f`.

Affected origin build: debug APK/instrumentation APK and lint passed. The first
release invocation had a wrong test-class filter; correcting it to
ReleaseDevelopmentPhotoCaptureTest passed. Eight ContributionUploadHttpClient
cases, release transport refusal, R8/size passed (BUILD SUCCESSFUL in 2m 43s,
126 tasks; 4,224,869 bytes). The combined focused evidence now covers 73
Android unit cases, with prior unchanged selections retained rather than
repeated. Release DEX still excludes developer controls/evaluation activities.
POCO reconnected with a new ADB transport: final APK installed with -r and the
correct fully qualified AuthAccountContinuityDeviceTest passed (one test),
without clearing user data. Native credential entry remains owner QA.

Backend dependency refresh: affected race suites and vet passed; key HTTP
suite passed 20 repetitions and real PostGIS key login/parallel signup passed.
Updated govulncheck: no vulnerabilities found. Mobile/security static checks,
secret scan and diff whitespace checks passed. No aggregate or remote gate
acceptance is inferred.

Actual commands (same serial/offline/JVM arguments recorded below):

```sh
./gradlew --offline --no-daemon --no-parallel --max-workers=1 \
  -Dorg.gradle.jvmargs='-Xmx2g -XX:MaxMetaspaceSize=1024m -Dfile.encoding=UTF-8' \
  -Pkotlin.compiler.execution.strategy=in-process \
  :data:testDebugUnitTest --tests '*ContributionUploadHttpClientTest' \
  :data:testReleaseUnitTest --tests '*ReleaseDevelopmentPhotoCaptureTest' \
  :app:verifyReleaseApkSize --console=plain
# backend; ANPFUEL_TEST_DATABASE_URL names the disposable local PostGIS only
go test -race -count=20 ./internal/modules/account/adapters/http
go test -race -count=1 -tags=integration ./internal/modules/account/adapters \
  -run 'TestPG(KeyAccountRoundTrip|ParallelKeySignupAdmitsExactlyOne)'
govulncheck ./...
go vet ./...
```

## Prior combined UI checkpoint

Owned feature commit: 16f3a07. UI-preserving merge b2cad57 was tested after
combining maintained dev 4fa7298; later 523be92 changes only the rollout plan.
The behavior diff after retaining that plan is empty. Final Android invocation:
serial/offline Gradle, 1GiB metaspace, selected 54 debug unit tests, debug APK,
instrumentation APK, lint (zero errors), release availability test, R8 and size:
BUILD SUCCESSFUL in 5m 40s (233 tasks), release 4,224,869 bytes. Unchanged data
and backend evidence below remains applicable. Release DEX excludes developer
UI and evaluation activities. Combined existing-behavior-file fingerprint:
4908b7475c5f15e93f98861d5b5820b8a001ad1e9a8b4ea930b12832385f3f7e
(64 files; the removed placeholder card is recorded by b2cad57).
Final debug APK SHA-256: 0d6dd80be65ccb0e2504c9058e7d7d721a0a88850771762a76d383f07a01c2a9.

Combined debug APK installed with `adb install -r`. Repeated only the affected
AuthAccountContinuityDeviceTest on POCO/API36: OK (1 test); no data/account wipe.
The earlier two synthetic media cases below are unchanged. Local disposable
PostGIS/MinIO fixtures and temporary local credential files were cleaned up;
authorized VPS sidecars remain available until operator teardown. The preview
flag still expires automatically at the recorded deadline.

## Initial isolated verification

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
  respects the existing namespace CPU quota. Prior pod stayed ready for ten hours. Current dependency-refreshed pod:
  abastevo-temp-api-64d8ddbd44-xcn4k, 3/3 ready, zero restarts, HTTPS health ready.
- Initial images (revision label ce04ba1-dev-preview-20261007):
  API 80a423f225618ac743bba7a2b3f9763cddb76f7e2065a49d426134a927c23af2;
  worker 43948beaea782e28cd1bf6dd31f1fcb789edfff7454298a36751fce038034583;
  migrate 23f288048834f6b2f71702431eccda16e91edbaf52672443c19bdd6f307e8367;
  storage 359182bc95e9029cbf11886c20944f9f8173a93aed83faa226cb795adf711792.
- Current API/worker tags: dev-preview-20261008, revision
  e26d546-dev-preview-20261008, with corrected x/text/x/sync dependencies.
  API 4f1e525fb970c18b18c3de5abc0c09c813af9b6147d51160582fbb67a746dfdb;
  worker 455e96dbe1fb2779b424ad683a61b9365c1704f6479719e798ba02cba22398e3.
  Migration/storage images remain the above owned images. Before the rollout,
  the existing private object was copied into a mode-0700 owned backup; after
  rollout its bytes/hash/MIME were verified and the transient copy removed.
- Repeated smoke initially reused a content-addressed image already bound to
  a different session, triggering the existing duplicate-key refusal. Fresh
  random synthetic pixels avoided that fixture collision. Repeated signed
  status polling exhausted the public peer challenge quota (60/hour), which
  was preserved: the observed window renews 2026-10-08T12:00:00Z (08:00 Cuiaba).
  The updated runtime passed via an owned loopback API port-forward plus
  verified public HTTPS private media: READY, two fuel rows VALIDATED,
  idempotency, anonymous denial and normal location requirement. Public HTTPS
  health is ready; full public HTTPS pipeline evidence is the earlier run.
  No quota reset, edge/auth weakening or duplicate-policy change was made.
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

Integration: maintained dev was clean at 60a3fd7. Guarded local fast-forward
completed to d95b189, preserving every existing UI/plan/upload/security commit.
The checkout is locally integrated; main acceptance remains pending.
Main integration remains INTEGRATION_PENDING. Subsequent publication must use
protected dev -> main batch delivery. No GitHub push/PR/merge/wiki in this task.
