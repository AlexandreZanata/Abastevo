# Android enterprise storage foundation

EMP-PLAT-011 provides a separate Room v1 schema, scoped record/queue adapter
and local context boundary. It adds no feature handler, sync dispatcher or UI.
Use EnterpriseLocalContext from the Hilt graph. Select an explicitly resolved
account/server/organization and keep the returned handle only for that context.
Selection itself is not authentication, membership or permission.

Every key/read includes the immutable EnterprisePartition. A pending operation
must carry its creation partition; passing it to another bound store fails.
Operation IDs converge only on an identical target/envelope in that partition.
Snapshots copy envelope bytes. Context switching/lock invalidates old handles
and serializes with local calls; current account/origin is rechecked before
releasing data. Restart starts locked and requires explicit reselection.

The production persistence binding deliberately returns unavailable.
RoomEnterprisePersistence is a synthetic foundation adapter, absent from the
production graph. Do not instantiate a plain database for real organization
data. EMP-PRIV-003 owns encryption selection, Keystore, journals/files/caches,
logout/revocation integration, backup/transfer and device proof. There is no
enabled flag that bypasses this missing composition. The account session's
NeedsRefresh state is only a local ownership hint, not a network permission.

EnterpriseDatabase v1 has no connection to AnpFuelDatabase v37 or personal
outboxes. No destructive migration or personal import is needed. Future
module schemas, cache validity, atomic business changes, operation protocols,
dependencies and dispatch remain with their owning tasks. The current port
accepts opaque envelopes; it neither decodes nor claims to encrypt them.

Run from repository root with the configured JDK17/SDK and matching SDK
environment paths:

```sh
./gradlew :domain:test :application:test --offline --no-daemon
./gradlew :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' --offline --no-daemon
./gradlew :data:kspDebugKotlin --offline --no-daemon
python3 scripts/tests/test-enterprise-local-sql.py
./gradlew :data:compileDebugAndroidTestKotlin :data:lintDebug :app:lintDebug :app:assembleDebug --offline --no-daemon
```

The host SQLite suite uses exported Room schema and KSP-generated production
SQL, real files/connections, twelve concurrent writers and a new interpreter.
It preserves an existing personal v37 file byte for byte. It is not native
Room/device/Keystore evidence. EnterpriseRoomIsolationDeviceTest is compiled;
execution stays in the owner's deferred emulator/device batch. No production
receipt, credential or private fixture is used.

[Architecture](../../../../../../../../docs/enterprise/09-architecture.md#21-implemented-android-partition-foundation)
