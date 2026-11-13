# EMP-PRIV-003 — Protected generations, signed contact and durable maintenance

Date: 2026-11-13.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Continue the [earlier protected-store evidence](f1-priv003-protected-device-store.md)
from tested ORG-014 efd7e886 on owned codex/enterprise-f2. That head and the
preceding F2 checkpoints are backed up at origin/codex/enterprise-f2; cumulative
delivery still targets dev → main PR203 at phase closure. No extra PR/merge or
remote-check wait is introduced. Phase215 retains the full F2 request.
B-BR-EMP-104..107, BUC-EMP-54/55 and EDR-068..070 precede behavior.

## Bound work and retained gates

The continuation implements exact signed contact, native binding/handoff, V2
trusted committed generations and encrypted durable maintenance. At entry, source
had only metadata check, V1 fixed-key snapshots, lazy/memory cleanup and unavailable
bindings. The accepted grant/lease adds membership/enrollment provenance;
a rehire cannot inherit old encrypted content through partition/family alone.
Source ACTIVE is an existing-account custody lease, not business permission.
Current owning feature permission/scope/cache producers remain mandatory. Managed
runtime waits for its actual AUTH/GOV policy; never guess a managed timeout.

The approved offline validity is30days, distinct from15-minute token life and
source family expiry. An exact LOST_OR_COMPROMISED verdict may erase only the
matching owned context; ACCESS_ENDED locks/preserves it without enabling F4 late
intake. Generic errors never erase. Existing confirmed security revocation and
proven privacy expiry discard keys under B-BR-EMP-34/35; ordinary session expiry,
logout, reboot or rollback observations cannot silently become a destructive
server verdict. Empty recovery requires explicit lost-work acknowledgement.

V2's durable ordering is prepare fresh keys → seal/flush data and encrypted
maintenance descriptor → atomic publish/fsync directory → trusted Keystore
commit marker → local durable acknowledgement → verified obsolete cleanup.
The committed head, not a restorable file, selects the generation. Missing or
inconsistent committed data fails closed; uncommitted preparations cannot
replace acknowledged work. Keep memory/entry budgets and ciphertext-only files;
record recovery at every boundary. Cleanup intents survive process death and
scheduled work uses opaque discovery, no raw IDs in WorkManager input.

The user now authorizes an emulator because the physical phone is disconnected.
Use a fresh isolated task AVD, preserving all existing AVDs and normal app data.
Actual emulator Keystore/OS tests are platform evidence for that image, not
manufacturer/physical-device or production certification. Managed unlock may be
exercised only with an explicit synthetic test policy; it does not supply the
missing server policy. Record genuine OS backup/restore limits rather than
calling host file copies a completed platform transfer.

Backend/native ownership and wire were frozen before implementation. Critical
REDs, immediate ordinary PostGIS/source races, host crash/rollback tests and
actual isolated emulator/static acceptance pass as recorded below. This is a
PRIV-003 local checkpoint; full F2, M1/M2/C1 and release acceptance stay open.

## Native acceptance preparation

The task owns the fresh API36 google_apis/x86_64 AVD enterprise-f2-api36, serial
emulator-5588, under a dedicated /tmp AVD/user home. KVM and completed boot are
observed; no existing AVD or physical device is modified. Only explicitly
targeted adb commands may use this AVD. The initial BackupManager is disabled;
local and D2D transports are available. Follow the
[official Android backup/transfer test procedure](https://developer.android.com/identity/data/testingbackup)
with only this task's synthetic isolated validation package and local transports.
No Google account or cloud upload is needed or authorized by this acceptance.

The existing enterprise Hilt UI validation graph deliberately substitutes
synthetic account/network services. It cannot prove actual source contact.
Use an independently constructed actual signed native client, nonexportable
Android identity keys and ordinary PostGIS/HTTP fixture for that boundary.
Only the disposable fixture CA and DNS mapping are supplied to the test client;
production origin/trust configuration is unchanged. Keystore/filesystem and OS
backup acceptance can use a named synthetic authority independently of that
source test. A positive-control public file must really restore before absence
of the vault is accepted as an exclusion result. Uninstall/reinstall applies
only to the isolated enterprisevalidation package, never the normal app.

The image's GMS D2D transport is listed, but actual initialization returns
-1000. Do not treat transport discovery as successful transfer. Use the
[AOSP CTS device-transfer procedure](https://android.googlesource.com/platform/cts/+/dbda408456b311b3bcc9d00a56e5d76bacacfc89%5E2..dbda408456b311b3bcc9d00a56e5d76bacacfc89/):
LocalTransport with is_device_transfer=true runs Android's device-transfer
extraction mode. The
[platform implementation](https://android.googlesource.com/platform/frameworks/base/+/master/packages/LocalTransport/src/com/android/localtransport/LocalTransport.java)
sets the device-transfer flag from that parameter. This is genuine framework
backup/restore in a named transfer simulation, not proof of Google/manufacturer
or two-device physical migration. Keep the GMS failure as an explicit limit.

## Backend contact acceptance

The exact signed contact source is implemented without a migration or command
ledger. Its independent seventeenth special boundary preserves all70 normal
routes and the six existing device routes. The exact terminal lookup retains
account/family/key/contributor proof and owned historical member/enrollment;
ACTIVE additionally reuses current employee/organization/onboarding checks.
Final DB-clock proof is rechecked before commit; a failed COMMIT returns no
lease or verdict. Contact does not change last_sync_at, history or audit.

Actual ordinary-role PostGIS/race acceptance passes26 top-level/54 named runs
in84.516s, including eight new contact cases and18 prior device/source-upgrade/
erasure/HTTP/concurrency cases. This covers exact identities, other accounts/
organizations, actual signatures/nonces/replay, malformed policy requests,
LOST/ENDED distinction, terminal observation without employee/terms projection
grants, family/key/token denial, rehire period separation, observed root-wait
expiry, revoke/member-end races and deferred COMMIT failure/recovery.
Retained log: /tmp/enterprise-priv003-backend-integration-final.log.

Focused OpenAPI/gate/current-router checks pass (14.931s API contract,4.249s
cmd/api; final strict contact gate9.730s). Pinned vet/staticcheck, sqlc vet and
unchanged generation of171 query files, whitespace and secret-surface review
pass. Full affected unit/race packages pass without timeout changes:
membership/application1.076s, membership/adapters/http1.082s, orgapi1.062s,
apicontract384.379s, enterprisesuite1.017s and cmd/api27.305s. Retained log:
/tmp/enterprise-priv003-backend-unit-final.log. The20-file backend/contract source manifest
at /tmp/enterprise-priv003-owned-backend-manifest.sha256 has SHA256
2fa586c71c7d11beed8e29541d94a159ced52b8f897a781deaca88224646675d.

The opt-in integration&&nativefixture Go harness compiles separately and exposes
only actual identity registration/challenge/account binding and enterprise
handlers through an ephemeral TLS origin. Its ordinary runtime is explicitly
verified without superuser/BYPASSRLS. Session fixture/public certificate files
are0600 inside a private0700 /tmp directory; no private TLS key is written and
no test-control endpoint is registered. Actual native execution passes below.

## Storage construction and review

Immediate host RED→GREEN confirms signed client6 cases (1.126s), protected
context flow5 (0.147s), exact owned handoff3 (0.010s) and generation3 (0.086s).
These are host results; actual Keystore/OS/source results are recorded separately
below. DI ports were held unavailable until that proof completed, then bound to
the actual signed client and shared protected-store singleton.

Independent review caught two recovery defects before checkpoint: malformed
committed data/meta files must become unrecoverable (explicit acknowledged
empty recovery), and partial erasure must not delete the highest trusted head
while obsolete payload/key cleanup and both denial channels can fail. The
second counterexample restores an older acknowledged generation after process
death. Keep the trusted head floor until every old/new decryption material and
file is verified gone; only then may heads be removed. The selective-failure
RED was captured; subsequent targeted regressions below prove the corrections.
Earlier successful suites are not used as evidence for later changed inputs.

Maintenance uses the existing WorkManager dependency, authenticated descriptors
and opaque discovery. One invocation should consume cooperative slices of50,
progressing past each attempted token even on failure, until that enumeration
is exhausted or cancellation occurs. A new invocation safely re-enumerates.
This avoids a private persistent cursor and a failing first slice starving
later partitions; failed cleanup remains retryable. Read-time expiry remains
mandatory even when Android delays scheduled execution. This implements the
recorded EDR-070 decision without a new quota or service.

Conservative EDR-069 recovery choice: V2 files/data/maintenance keys without a
trusted head cannot distinguish an interrupted first preparation from loss of
a previously acknowledged head. Treat both as unrecoverable and require fresh
exact proof plus explicit acknowledged empty recovery. This may request
acknowledgement after an unacknowledged first preparation; it never silently
discards possibly acknowledged work. A failed preparation with a retained prior
head continues to preserve that acknowledged generation. Generic provider I/O
failures remain outages, not proof permitting reset.

The consolidated affected host run passes122 cases (domain1/application43/
data78), zero failures/errors/skips, measured40s, retained at
/tmp/enterprise-priv003-final-host.log. It includes the selectively failing
erasure/head floor, head loss, legacy/provenance refusal, data/meta transplant,
account/foreign cleanup, independent sweep handles and sticky rollback lock.
Actual two-thread/Unconfined RED also proved epoch publication lock inversion;
the central recovery publishes monotonic epochs only after the outermost
ownership/fence lock is released, including nested failure and background cases.
The last job-observed RECONFIRM change receives its focused rerun separately;
no second aggregate is scheduled for that small change.

Post-consolidation job-observed RECONFIRM RED→GREEN passes26 affected cases in42s
(persistence14/maintenance6/contact binding6). The final independent review
then proved a second recovery-floor case: even after successful payload/file
disposal, failure of both denial channels must retain a trusted floor so the
next process cannot silently initialize empty without acknowledging lost work.
Its focused RED→GREEN passes27 cases in32s (lifecycle14/maintenance7/binding6),
without skips/errors. Log: /tmp/enterprise-priv003-recovery-floor-green.log,
SHA256bc7dca6e7b812397be0fe0557a6df140a830d912a9e4371c3924af968f0d958e.
The final20-file storage manifest has SHA256
da0bc7c5450f5494116aa3f157d3f178e81f8f9ba33170556e9481e6d58086bc.
The new BoundOfflineLease class measures67/67 instructions,11/11 lines and4/4
branches; this is scoped coverage, not a claim that the global Kotlin gate passed.

## Actual Android execution

Opt-in app/data native APK compilation and the four affected ViewModel tests
pass (JUnit0.870s). The first compile caught the test-only OkHttp Dns interface
construction; an explicit implementation fixed it. No check or source assertion
was removed. Retain /tmp/enterprise-priv003-native-build.log and
/tmp/enterprise-priv003-native-build-recovery.log (successful build1m14s).
Immutable test APK copies/hashes are at /tmp/enterprise-priv003-apks/manifest.json.
The app uses the existing ARM candidates through the image's native bridge;
the data test APK includes x86_64. These are API36 emulator observations.

All three initial native cases pass (reported JUnit0.311s): actual Keystore
nonexportability/policy, sealing/reopen and deleted-key-handle refusal, plus
malformed private JSON/certificate errors with no credential/cause echo.
Retained output: /tmp/enterprise-priv003-native-primitives.log.

Actual Android identity-v1 registration, account binding, nonce/signature,
source enrollment/contact, protected handoff and pending storage pass against
the ordinary PostGIS/HTTPS fixture. A real invalid parent token refuses without
erasure; actual own-device cookie/CSRF revocation produces the exact signed
ERASE verdict, and real scoped decryption keys/files are verified gone before
test-finally cleanup. A reconstructed store also denies. This is one actual
native test, JUnit0.637s / measured wall1.191s, output
/tmp/enterprise-priv003-native-contact.log. The fixture server completed and
cleaned its child DB/role/private proof files; the owned adb reverse was removed.
No production/staging credentials or service changes were involved.

OS backup/transfer, managed real prompt/window and reboot/clock results below
pass. The protection ports stayed closed during acceptance and are now bound to
the verified implementations. This does not certify production readiness.

Actual framework local backup→uninstall→reinstall/restore passes in13.139s;
AOSP LocalTransport device-transfer mode passes separately in11.460s. In each
sequence, the original positive-control public file really restores while
private vault/generation files, Keystore custody and private preferences do
not. Before each backup, all four encrypted surfaces/pending work, a32MiB
capacity refusal preserving the acknowledged generation, and cold-process
reopen also pass. Only the explicitly isolated package is cleared/reinstalled.
Outputs/results are /tmp/enterprise-priv003-os-acceptance.log and
/tmp/enterprise-priv003-os-{local,transfer}-result.json. This proves the named
framework extraction modes, not a cloud upload or physical/vendor migration.

Actual managed key acceptance passes: key use refuses before authentication,
the real Android credential authorizes encryption/decryption, the explicit
synthetic5-second window expires, a second real prompt reauthorizes the key,
and deleting it while that authorization is live invalidates the retained
opaque handle. Authentication refusal cannot masquerade as deletion success.
Reported JUnit9.178s / measured test wall11.369s; no server managed policy is
inferred. The test targets only the verified system credential node, never
globally types the PIN. Initial execution observed a SystemUI unresponsive
dialog; window inspection also identified API36 Settings delegation to the
exact SystemUI credential header/title/input. The test-only adaptation verifies
that delegation and then passes, with the same refusal/expiry/deletion asserts.
Retain the failed/redacted output and recovery in
/tmp/enterprise-priv003-os-acceptance.log; corrected test APK SHA256
9d3baf373774f1f41e630683ca95e8d79607b8f73b8b4fed0b6287e5b2a34eaf.

Actual guest wall-clock rollback also passes: after seeded acknowledged work,
the task-owned emulator's clock moves backward120seconds, access refuses without
changing encrypted data/meta files, the guest clock is restored in a finally,
and a new process still refuses until fresh contact. That fresh synthetic
custody contact preserves the original pending operation/record. Actual boot
identity/wall observations are asserted, not substituted clock ports. Measured
sequence4.948s; /tmp/enterprise-priv003-os-rollback-result.json records its steps.
The actual real-reboot result is recorded next.

Actual reboot acceptance passes after the Android boot identity really changes.
The first external unlock helper stopped before PIN entry because its generic
gesture had not exposed the credential field. Inspection and the actual system
PIN screen completed that guarded step; no PIN entered an unrelated window.
After credential unlock, the new app process refuses the old-boot lease while
preserving encrypted files/keys. Fresh synthetic custody contact then reopens
the original acknowledged record/pending operation. Reported JUnit1.625s;
the separately measured post-boot unlock/test step is17.468s, not total reboot
duration. Retain /tmp/enterprise-priv003-os-reboot-driver.log (stopped helper),
/tmp/enterprise-priv003-os-reboot-recovery.log and its result JSON.

The actual WorkManager worker passes all four native tests with Android
keys/files and the existing work-testing dependency: explicit accelerated
synthetic expiry, scoped provider-failure retry/reconstruction, cancellation
propagation and independent sweeps. JUnit0.514s / measured wall1.744s; no claim
that30real days elapsed. Log: /tmp/enterprise-priv003-worker-native.log.
The added test file has SHA256
88a212ef4e0f1134bee085b65bd4b222835773fbabec7fe57f61da39002b55e9.

## Final binding, validation and retained gates

After native acceptance, contactAuthority binds EnterpriseDeviceContactClient;
persistence/contextProtection share the @Singleton protectedStore. Dependency
review and actual Hilt compilation confirm no store→auth→store cycle. The four
backup/layout/session guards pass with these accepted bindings. Full app/data
lint and opt-in assembly pass in2m25s; normal assembly passes separately in32s.
No test-only scenario/binding/HiltTestApplication class enters the product APK.

The noninstrumented product AnpFuelApplication/MainActivity also starts cold
successfully in the isolated package (measured2.549s). Actual WorkManager SQLite
shows startup SUCCEEDED and periodic ENQUEUED at900,000ms, both with the empty
8-byte input encoding; no private payload/cursor is scheduled. This runs the
real Application/Hilt worker factory, outside the synthetic UI test graph.
Product APK SHA256
62837cfeeb9579e2569e99c5de45852ce17ea385d5e8cbfc21c9f47602cb4925.
Retain /tmp/enterprise-priv003-product-startup.log,
/tmp/enterprise-priv003-product-workmanager.log and
/tmp/enterprise-priv003-final-android-checks.log.

The named emulator matrix has22 successful executions of16 distinct methods;
repeated seed/reopen/restore methods prove the two separate extraction modes.
Diagnostic failures/helpers remain recorded above rather than counted as PASS.
There is no new dependency, applied migration, plaintext private store, real
receipt or production photo/GPS fixture. Ordinary personal storage is unchanged.

Formal M1/global Kotlin coverage, managed AUTH/GOV source policy, legal retention
of real receipts, production provider/grants/staging/backup roles and full
F2/M2/C1/release acceptance remain separate obligations. A terminal wipe may
invalidate its old flow before the loss presentation arrives; the next explicit
selection reads durable denial and offers acknowledged empty recovery. No stale
callback adopts another epoch or automatically replays a request.

This checkpoint is LOCAL_DONE / INTEGRATION_PENDING on the owned F2 branch.
Continue the next dependency-ready task from its committed head. Back up the
task commit to origin/codex/enterprise-f2; final cumulative dev → main PR203
publication/union/Quick belongs at phase closure. No intermediate CI polling,
merge, deployment, wiki publication or real company pilot is authorized here.

Final source/contract/test/guard inputs are pinned in the tracked
[68-file source manifest](f2-priv003-source-manifest.sha256), SHA256
8ad35414e7afff507c77c5e903849869df4eb5caebd0487e10ddc0b9c0c7b069.
Its baseline is tested/published ORG-014 efd7e886; documentation does not hash
itself. Final plan/layout guards, whitespace and staged secret-surface checks
pass before the atomic task commit. No aggregate/CI rerun is inferred
from a documentation-only update.
