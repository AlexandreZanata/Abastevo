# Android enterprise storage and protection

EMP-PLAT-011 supplies immutable creation scope, separate synthetic Room v1
and EnterpriseLocalContext. EMP-PRIV-003 now supplies a protected adapter;
its native/server acceptance is IN_PROGRESS. Production
EnterprisePersistence still binds UnavailableEnterprisePersistence.
EnterpriseContactAuthority still binds UnavailableEnterpriseContact.

## Protected implementation

ProtectedEnterprisePersistence stores one AES-256-GCM authenticated snapshot
per account/canonical HTTPS origin/organization. AndroidEnterpriseVaultKeys
uses separate AES-256 AndroidKeyStore aliases. Provider-generated IVs satisfy
randomized-encryption policy; no raw key is persisted or exported. Version
and framed scope are authenticated, with schema/scope checked inside the
encrypted body. Alias/filename digests contain no plaintext IDs or origin.

Records, pending operations, attachments, caches and offline lease metadata
are encrypted together, including IDs/targets/order. AtomicEnterpriseVaultFiles
writes ciphertext-only temporary files under noBackupFilesDir, fsyncs and
atomically renames them. There is no plaintext DB/WAL/journal/file fallback.
The old Room v1 foundation remains synthetic, and personal v37 tables,
outboxes and files are not imported or migrated.

Allocation/decoder budgets are 32 MiB and 10,000 entries per snapshot.
Over-budget storage refuses without eviction or rewriting held work.
This is a bounded initial store, not a commercial plan quota. Owning
features must measure write amplification/capacity before activation.
No encrypted SQLite dependency is added without a concrete need/review.
Sealing encrypts one presized plaintext in place, so a write holds about
three budgets (entries, plaintext, ciphertext) instead of five. An
OutOfMemoryError while opening or sealing is reported as
EnterprisePersistenceUnavailable: writes are atomic replacements and locks
are released, so nothing partial remains. Device memory budgets are still
measured in the native batch.

The backup rules also exclude root no_backup/enterprise-sealed-v1/, the
actual vault path, besides the platform's own no_backup exclusion.

## Authority, session and lifecycle

Before selecting a local context, an owning flow calls confirm(partition).
Only its trusted EnterpriseContactAuthority may issue the current partition,
session family, server contact time, offline validity and managed unlock
duration. It must verify current account/device/membership on the server,
including freshness/replay protection. Local caller input is not proof.
The default provider refuses; no client route is invented by this task.

A lease lasts 30 days by default, using same-boot elapsed time. Restart in
the same boot can reopen after explicit context selection under the same
session. Reboot, decreasing elapsed time or wall time below the contact
anchor requires fresh server contact, keeping pending ciphertext. Expiry
discards keys; server time proves expiry across boots. Without a comparable
monotonic clock (after a reboot), device wall time at or past the contact
anchor plus validity also discards the key on the next access or contact:
a clock moved forward can only erase early, a clock set back still only
requires contact. Reads never create replacement keys. A restored blob
without the original key cannot reopen.

Expiry is enforced lazily, on access or contact. A scheduled sweep is an
activation gate: a partition is sealed under AAD bound to account, origin
and organization, so a sweep needs the owner's partition list (from the
membership snapshot) or per-contact re-keying with a Keystore validity end.
No WorkManager job is scheduled while the production binding stays closed.

Managed policies require a positive explicit device-unlock duration, enforced
by the AndroidKeyStore key. The device credential/strong biometric ceremony
belongs before access; a hardware authorization window is not a server
credential or a sliding-idle clock. No managed timeout default is guessed.

Outcomes stay typed. UserNotAuthenticatedException at key use becomes
EnterpriseUnlockRequired: the caller asks for the device unlock and
retries; it is never a reason to erase. An invalidated key
(KeyPermanentlyInvalidatedException), a different key under the alias,
failed authentication, a malformed or truncated file, or a key whose policy
differs from the snapshot becomes EnterpriseVaultUnrecoverable. So does a
changed managed-unlock policy in a fresh grant, because no re-keying
migration exists. Unrecoverable work continues only through
confirm(acknowledgeErasedWork = true) after fresh contact, which erases
and starts empty. Provider outages stay EnterprisePersistenceUnavailable.

Use EnterpriseLocalContext handles only in their selected scope. Session
clear, changed account/family and app backgrounding advance the shared fence.
Old handles cannot revive on same-owner re-login. Ordinary same-family token
rotation preserves them. Clearing secure preferences must report durable
write failure; it cannot claim logout from a memory-only removal.

Definitive current-family SESSION_REVOKED/SESSION_REUSE on account refresh
erases that account/origin, while preserving unrelated accounts/origins.
Outages, normal expiry, foreign credentials or late replies do not erase.
This runs in the personal auth graph, so an account with no vault file,
lock file or key alias gets no denial marker and no file at all: a later
enterprise start never asks to acknowledge work that never existed. An
owner/origin that cannot form a partition likewise has nothing to erase.
Future signed organization clients invoke partitionRevoked only on their
verified server revocation verdict; this adapter sends no sync operation.

Erasure persists denial and discards keys, attempting independent steps.
cleanupRequired reports any failure and clears after a later successful
revocation of the same scope. It is in-memory and has no production reader
yet; persisting and retrying it is an activation gate. A persistent marker
or missing key blocks reopening, including after process recreation. No
durable-erasure claim is possible if both marker persistence and key
deletion fail. Retain the activation gate and retry cleanup; do not
acknowledge success.

Residual risk, accepted until native acceptance: the AAD binds scope, not
a generation, so someone able to write this app's private files (root, or
run-as on a debuggable build) can swap in an older ciphertext sealed under
the same key. Deleted records and already-sent pending operations would
reappear; resent operations keep their original IDs. Closing
it needs per-generation re-keying or a monotonic counter outside the app
sandbox, both of which belong with the native batch.

Starting empty after erasure requires acknowledgeErasedWork=true plus a fresh
verified grant and successful cleanup. Denial is lifted last, after a new
empty encrypted snapshot/key are durable. The old work/key is never recovered.
A same-account login without fresh contact cannot see or send held work.

## Validation and remaining acceptance

Host JVM tests use actual crypto/files, independent JVMs and abrupt death
before/after replacement, concurrent writers, malformed/tampered/oversize
input, restored ciphertext without keys, changed scopes, clock/family errors
and independent cleanup faults. They are not Android Keystore/backup proof.
EnterpriseProtectedVaultDeviceTest compiles; execution and the manual managed
unlock/restore/transfer matrix remain in the owner's deferred native batch.

Run with the configured JDK17/SDK and matching SDK environment paths:

```sh
./gradlew :domain:test :application:test --offline --no-daemon
./gradlew :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' --tests 'com.anpfuel.data.local.auth.*' --offline --no-daemon
./gradlew :app:testDebugUnitTest --tests 'com.anpfuel.app.security.*' --tests 'com.anpfuel.app.ui.auth.*' --offline --no-daemon
python3 scripts/tests/test-enterprise-protection.py
./gradlew :data:compileDebugAndroidTestKotlin :data:lintDebug :app:lintDebug :app:assembleDebug --offline --no-daemon
```

Retain native evidence for real non-exportable keys/provider IVs, device
credential/strong biometric inactivity, hardware/key-loss failure, restart/
reboot and clock rollback, actual backup/transfer/restored-device behavior,
and cleanup outages. Current AUTH/IDN integration is also owed before opening
the production binding. No real receipt, production credential, GPS trace or
unreviewed storage dependency is used.

[Architecture](../../../../../../../../docs/enterprise/09-architecture.md#22-implemented-protected-vault-source-native-acceptance-pending)
