# F1 EMP-PRIV-003 protected device store

Status: IN_PROGRESS
Validation: HOST_PASS / NATIVE_PENDING

2026-11-11. Implementation publication targets
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
Host source acceptance and deferred native acceptance are separate. F1/M1
remain open; production persistence/contact bindings remain unavailable.

## Entry and decisions

The owner requested the next eligible task and GitHub delivery. The clean
isolated enterprise worktree entered at published 5d1f2b4a, matching open
dev → main PR202; main ef96cd13 is already an ancestor. Occupied local dev
and unrelated primary-checkout work are preserved. No merge or VPS action
is authorized by this implementation.

Read EMP-PRIV-003, current progress, F0 device decisions, BR-PRIV-016/017/
018/019, UC-PRIV-03 and UC-OFFL-06. B-BR-EMP-33/34/35 and BUC-EMP-18 were
recorded before behavior. The pure offline-lease RED suite had 3/4 failures
before implementation. Focused durable-clear RED tests reproduced two
ignored preference-commit failures; four additional AuthFlow RED tests
reproduced unhandled cleanup failures before the application correction.

The existing account Keystore wrappers do not protect enterprise Room
metadata, journals or arbitrary files. The selected adapter uses one
versioned AES-256-GCM sealed snapshot per partition instead of adding an
unreviewed encrypted SQLite dependency. No new third-party dependency,
migration, personal import or plaintext database builder is introduced.
The prior Room v1 foundation remains synthetic; personal v37 stays intact.

Platform choices follow the official
[Keystore key-custody/authentication contract](https://developer.android.com/privacy-and-security/keystore),
[cryptography guidance](https://developer.android.com/privacy-and-security/cryptography)
and [backup boundary](https://developer.android.com/identity/data/autobackup).
Those documents describe platform behavior, not completed native acceptance.

## Implemented behavior

- Protect record/operation IDs, targets/order, envelopes, attachments, caches
  and lease metadata together. Version/framed partition are AAD; the body
  validates partition, schema, duplicate keys and decoding bounds. Android
  keys are separate non-exportable AES-256 aliases, using provider IVs.
  Reads never generate replacement keys.
- Only scope digests appear in filenames/aliases. Sensitive replacement
  files contain ciphertext under noBackupFilesDir/enterprise-sealed-v1;
  denial markers contain only a fixed version byte and locks no payload.
  Writes fsync, atomically rename and fsync the directory. Same-account OS
  file locks and an overlapping-JVM-lock guard serialize independent
  adapters/processes. Links, truncation, oversized and modified envelopes
  fail closed without raw path/provider contents.
- Allocation budgets are 32 MiB and 10,000 entries per snapshot. Capacity
  failure is explicit, preserves held work and evicts nothing. This is a
  bounded initial adapter, not a commercial quota. Feature capacity/write
  amplification must be measured before activation or a reviewed versioned
  storage extension selected.
- A trusted current-contact port supplies exact partition/session family,
  server receipt, offline validity (default 30 days) and explicit managed
  unlock duration. The default provider denies. Same-boot monotonic time
  enforces expiry. Reboot, decreasing elapsed time or wall time below the
  contact anchor requires fresh contact, preserving pending ciphertext.
  Wall time cannot extend validity. Server receipt proves expiry across boots.
- Managed keys enforce the explicit credential/strong-biometric window,
  with the platform timed-auth policy on the supported APIs. No default
  idle timeout is invented or local unlock treated as a server credential.
  A key-policy change is refused pending explicit migration.
- Session clear/account-family replacement fences old handles; same-family
  token rotation keeps context. MainActivity background invalidation never
  waits for I/O. Epoch checks withhold in-flight/stale replies, including
  same-owner re-login and a late contact result.
- Definitive SESSION_REVOKED/SESSION_REUSE on refresh of the still-current
  requested family triggers erasure for that account/origin. Outage,
  ordinary expiry, foreign proof or a late old-family reply does not erase.
  Future signed organization clients must call partitionRevoked only on
  their verified device-revocation verdict. No new sync client is added.
- Erasure fences, persists denial, deletes keys and attempts file cleanup
  independently. cleanupRequired records failure; no false completion.
  A marker or absent key blocks reopening. Fresh empty initialization after
  erasure needs explicit lost-work acknowledgement, fresh server proof and
  successful cleanup; denial is lifted last. Old work/key never resurrects.
- Secure preference removals now check durable commit. AuthFlow attempts
  both local clears and remote logout, returning secure-storage-unavailable
  rather than throwing or claiming success when cleanup fails. Refresh,
  mismatched identity and deletion paths also report failed cleanup safely.
  No refresh/logout/revocation wire protocol or server behavior changes.

## Verification

Existing Gradle 8.10.2/Kotlin 2.0.21 and Temurin JDK17.0.19, offline and
no-daemon; JAVA_HOME=/home/iiii/.gradle/jdks/eclipse_adoptium-17-amd64-linux.2,
ANDROID_HOME=ANDROID_SDK_ROOT=/data/dev/android/sdk/Sdk. No dependency or
toolchain pin is changed. Commands run from repository root.

- Full §:domain:test§ passed: **1,159 tests**, zero failures/errors/skips.
  New offline classes: **250/255 instructions (98.04%)**; the decision
  method has 100% instruction coverage. Same-boot expiry boundaries,
  reboot/clock rollback, explicit validity, overflow and invalid metadata
  pass. The original cross-language time/money vectors remain included.
- Full §:application:test§ passed: **542 tests**, zero failures/errors/skips.
  Four new durable-clear regressions verify logout, revoked/expired/foreign
  refresh and successful deletion do not hide local failures; both local
  clears and applicable remote revocation are attempted.
- §:data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' --tests 'com.anpfuel.data.local.auth.*'§
  passed: **67 tests**, zero failures/errors/skips. This includes actual
  sealed-file crypto, 24 simultaneous writers across two adapters, four
  independent JVM writers, another JVM read, and abrupt process death
  before/after replacement. Replay remains unambiguous and personal bytes
  unchanged. Missing/wrong keys, ciphertext/scope/version modification,
  oversize/symlink input, pending-operation conflicts, late contact,
  changed identity/family, expiry, fresh empty recovery and independent
  marker/key/file/lock faults pass. A lock outage plus one failed deletion
  still attempts every other scoped key. Background invalidation returns
  without waiting for blocked I/O and withholds the in-flight reply.
- §:data:compileDebugAndroidTestKotlin§ and debug APK/Hilt compilation pass.
  javap confirms both new protected-vault tests and the prior Room tests
  expose void methods; this is compile/discovery evidence, not native runs.
  Final §:data:lintDebug :app:lintDebug :app:assembleDebug§ passed on the
  frozen inputs: app lint 0 errors/347 existing warnings/3 informs; data
  lint 0 errors/16 warnings. No suppression or toolchain change is added.
- §python3 scripts/tests/test-enterprise-protection.py§: **3 passed**.
  Actual noBackup graph, manifest/extraction channels and session/lifecycle
  wiring are guarded. §python3 scripts/tests/test-enterprise-local-sql.py§:
  **5 passed**, retaining the independent real host-SQLite foundation proof.
  No server SQL/migration/auth wire changes require a new PostGIS fixture.
- Enterprise verifier/generator and its **3 regression tests** pass:
  **348 tasks / 545 rules / 167 use cases / 145 screens / 0 problems**.
  These are specification counts, not implemented features. Added relative
  Markdown links and diff whitespace pass. Tracked and staged diff secret
  review/Gitleaks pass, without exposing private fixture material.

The pre-existing aggregate domain gate still fails its 90% target:
**56,871/70,813 instructions = 80.3115%**, with the unchanged configured
exclusions. [Earlier baseline evidence](f1-plat009-exact-values.md) retains
independent reproduction. The current full-test XML was calculated using
the same exclusions; the failing aggregate verification was not repeated
solely for bookkeeping. No threshold, exclusion or test is weakened.

Final behavior/test/backup input manifest: 32 changed .kt/.xml/.py files
relative to 5d1f2b4a, sorted by relative path, each line SHA256, two spaces,
path and newline. Manifest SHA256:
§10a6011d5838f2cf0d5409d610f8838f477e4f37d025da1bad9541443e3a6c8f§.
The implementation and its evidence travel in the same atomic commit.

The tests explicitly use Unit/void returns. Several previously undiscovered
enterprise host tests ended in assertThrows and inferred a non-void Kotlin
return; their signatures are corrected so JUnit actually executes them.
Native Room tests also declare Unit before the deferred device batch.
No assertion, threshold or exclusion is removed.

## Native and activation obligations

The owner explicitly deferred Android device/emulator execution to the final
native batch. EnterpriseProtectedVaultDeviceTest is provided and compiled,
not represented as run. EMP-PRIV-003 stays IN_PROGRESS until native evidence
proves actual non-exportable keys/provider IVs, managed device credential/
strong-biometric inactivity, process/reboot and clock behavior, key loss/
invalidation, backup/transfer/restored-device handling and cleanup failures.
The two native tests do not replace the manual prompt/transfer/failure matrix.

The actual application graph keeps UnavailableEnterprisePersistence and
UnavailableEnterpriseContact. Current AUTH/IDN proof/freshness/replay adapters,
signed device-revocation intake and feature/UI integration remain owed.
The Keystore authorization window is stricter than a sliding idle timeout;
managed prompt/idle UX must be accepted with the actual device. No active
enterprise screen, sync dispatcher or receipt capture path is introduced.

When both denial persistence and physical key disposal fail, durable erasure
cannot be certified. The caller reports cleanupRequired and fences current
access, rather than claiming remote wipe. Independent marker/key/file and
lock outages are tested; the native combined-failure gate remains unaccepted.
A disconnected device cannot receive immediate revocation. No root/forensic
storage wipe or uniform hardware-backed custody is claimed.

The pre-existing aggregate Kotlin domain coverage gate also remains below
90%; no threshold or exclusion is changed. Real-receipt policy, other
[review activation gates](f1-pr202-review.md), G24 and production release
remain separate. EMP-PLAT-012 still awaits EMP-GOV-014.

## Rollback and recovery

Keep the real binding closed. A code rollback must preserve ciphertext and
Keystore aliases for compatible recovery; never fall back to plaintext Room
or import personal data. Unsupported versions/corruption refuse without
overwriting the existing snapshot. Failed replacement preserves the previous
complete snapshot or an idempotently readable new one. Revoked/expired work
cannot be resurrected by restore or re-login; fresh authorized initialization
starts empty only after explicit acknowledgement.

No production credential, receipt, photo, precise GPS or business record
was used. Test subprocess keys are generated software keys under disposable
test directories, never committed, logged or used by production adapters.
