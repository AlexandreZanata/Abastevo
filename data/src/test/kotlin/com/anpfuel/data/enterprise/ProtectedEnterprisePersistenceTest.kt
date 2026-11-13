package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.Callable
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class ProtectedEnterprisePersistenceTest {
    @TempDir lateinit var root: Path
    private val p get() = partition()
    private val payload = ("PRIVATE-SYNTHETIC-ENTERPRISE-INTENT-" + A).toByteArray()
    private fun denied(work: suspend () -> Unit) =
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { work() } }
    private fun unrecoverable(work: suspend () -> Unit) =
        assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { work() } }

    @Test
    fun allPrivateSurfacesStaySealedAcrossRecreationAndPersonalFilesAreUnchanged(): Unit = runBlocking {
        val personal = root.resolve("personal-v37.db")
        val original = byteArrayOf(83,81,76,105,116,101,0,99)
        Files.write(personal, original)
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.confirm(p)
        f.store.put(EnterpriseLocalRecord(p, A, payload))
        assertTrue(f.store.enqueue(EnterprisePendingOperation(p, B, A, payload)))
        f.store.putPrivateFile(p, A, payload)
        f.store.putPrivateFile(p, B, payload, cache = true)
        val ciphertext = Files.readAllBytes(f.file(directory))
        f.store.put(EnterpriseLocalRecord(p, A, payload))
        assertFalse(ciphertext.contentEquals(Files.readAllBytes(f.file(directory))))
        val createsBeforeRead = f.keys.creates
        val reopened = VaultFixture(directory, f.keys)
        assertArrayEquals(payload, reopened.store.record(p, A)!!.envelope)
        assertArrayEquals(payload, reopened.store.pending(p, 50).single().envelope)
        assertArrayEquals(payload, reopened.store.privateFile(p, A))
        assertArrayEquals(payload, reopened.store.privateFile(p, B, true))
        assertNull(reopened.store.privateFile(p, C))
        assertNull(reopened.store.record(p, C))
        assertArrayEquals(original, Files.readAllBytes(personal))
        Files.walk(directory).use { paths ->
            paths.filter(Files::isRegularFile).forEach { file ->
                val bytes = Files.readAllBytes(file)
                val text = String(bytes, Charsets.ISO_8859_1)
                for (secret in listOf(A, B, ORIGIN, String(payload))) {
                    assertFalse(text.contains(secret), file.fileName.toString())
                    assertFalse(file.fileName.toString().contains(secret))
                }
                assertFalse(file.fileName.toString().endsWith(".db"))
                assertFalse(file.fileName.toString().endsWith("-wal"))
                assertFalse(file.fileName.toString().endsWith(".new"))
            }
        }
        assertEquals(createsBeforeRead, f.keys.creates)
        assertEquals(3, f.contentKeys().size)
    }

    @Test
    fun scopeAndSessionOwnershipRemainSeparateAndChangedOperationsNeverReplaceHeldWork(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        val partitions = listOf(p, partition(org = B), partition(owner = B), partition(origin = "https://other.example.invalid"))
        partitions.forEachIndexed { i, scope ->
            f.identity = EnterpriseLocalIdentity(scope.ownerAccount, scope.origin, A)
            f.confirm(scope)
            f.store.put(EnterpriseLocalRecord(scope, A, byteArrayOf(i.toByte())))
            assertTrue(f.store.enqueue(EnterprisePendingOperation(scope, B, A, byteArrayOf(i.toByte()))))
            assertFalse(f.store.enqueue(EnterprisePendingOperation(scope, B, A, byteArrayOf(i.toByte()))))
            assertThrows(EnterpriseOperationReused::class.java) { runBlocking {
                f.store.enqueue(EnterprisePendingOperation(scope, B, C, byteArrayOf(i.toByte())))
            } }
        }
        partitions.forEachIndexed { i, scope ->
            f.identity = EnterpriseLocalIdentity(scope.ownerAccount, scope.origin, A)
            assertArrayEquals(byteArrayOf(i.toByte()), f.store.record(scope, A)!!.envelope)
            assertEquals(A, f.store.pending(scope, 1).single().targetId)
        }
        f.identity = EnterpriseLocalIdentity(B, ORIGIN, A)
        denied { f.store.record(p, A) }
        f.identity = EnterpriseLocalIdentity(A, ORIGIN, B)
        denied { f.store.pending(p, 1) }
        f.server++
        f.confirm(p)
        assertEquals(1, f.store.pending(p, 50).size)
        assertThrows(IllegalArgumentException::class.java) { runBlocking { f.store.pending(p, 0) } }
        assertThrows(IllegalArgumentException::class.java) { runBlocking { f.store.pending(p, 51) } }
    }

    @Test
    fun restoredBytesWithoutOriginalKeysCannotCreateReplacementKeysOrLeakContent(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        val noKeys = VaultFixture(directory)
        unrecoverable { noKeys.store.record(p, A) }
        unrecoverable { noKeys.confirm(p) }
        assertEquals(0, noKeys.keys.creates)
        assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
    }

    @Test
    fun modifiedWrongScopeWrongKeyAndUnsupportedEnvelopesFailWithStaticErrors(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        val original = Files.readAllBytes(f.file(directory))
        for (index in listOf(0, 4, original.lastIndex)) {
            val changed = original.copyOf()
            changed[index] = (changed[index].toInt() xor 1).toByte()
            Files.write(f.file(directory), changed)
            val error = assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { f.store.record(p, A) } }
            // Coroutine stack recovery may add another copy of this same safe exception.
            generateSequence<Throwable>(error) { it.cause }.forEach {
                assertTrue(it is EnterpriseVaultUnrecoverable)
                assertEquals("Enterprise protected storage unrecoverable", it.message)
            }
            assertFalse(error.toString().contains(directory.toString()))
        }
        Files.write(f.file(directory), original)
        val foreign = partition(org = B)
        f.confirm(foreign)
        Files.write(f.file(directory, foreign), original)
        unrecoverable { f.store.record(foreign, A) }
        val alias = f.payloadAlias(p)
        f.keys.values.remove(alias)
        f.keys.create(alias, null)
        unrecoverable { f.store.record(p, A) }
        // A different key under the alias can never reopen the work: only an
        // explicitly acknowledged fresh start, after fresh contact, recovers.
        f.server++
        unrecoverable { f.confirm(p) }
        assertTrue(Files.exists(f.file(directory)))
        val creates = f.keys.creates
        f.confirm(p, acknowledgeErasedWork = true)
        assertNull(f.store.record(p, A))
        assertTrue(f.store.pending(p, 50).isEmpty())
        assertEquals(creates + 4, f.keys.creates) // One durable denial marker and three fresh generation keys.
        assertFalse(f.erasure.cleanupRequired)
    }

    @Test
    fun invalidatedKeyIsUnrecoverableUntilAcknowledgedFreshStart(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        f.keys.invalidated = true // e.g. the device lock screen was removed.
        unrecoverable { f.store.record(p, A) }
        f.server++
        unrecoverable { f.confirm(p) }
        f.keys.invalidated = false
        assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
        f.keys.invalidated = true
        f.confirm(p, acknowledgeErasedWork = true)
        f.keys.invalidated = false
        assertNull(f.store.record(p, A))
        assertEquals(3, f.contentKeys().size)
    }

    @Test
    fun corruptedOrTruncatedCiphertextIsUnrecoverableUntilAcknowledgedFreshStart(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        Files.write(f.file(directory), ByteArray(31))
        unrecoverable { f.store.record(p, A) }
        f.server++
        unrecoverable { f.confirm(p) }
        f.confirm(p, acknowledgeErasedWork = true)
        assertNull(f.store.record(p, A))
    }

    @Test
    fun rebootAndRollbackLockWithoutDroppingWorkButElapsedExpiryShredsTheKey(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.confirm(p); f.store.enqueue(EnterprisePendingOperation(p, A, B, payload))
        f.now = f.now.copy(boot = "boot-8", elapsedMillis = 0)
        denied { f.store.pending(p, 1) }
        assertEquals(3, f.contentKeys().size)
        f.server += 1000
        f.confirm(p)
        assertArrayEquals(payload, f.store.pending(p, 1).single().envelope)
        f.now = f.now.copy(wallMillis = 9999)
        denied { f.store.pending(p, 1) }
        assertEquals(3, f.contentKeys().size)
        f.now = f.now.copy(wallMillis = 10000, elapsedMillis = EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS)
        denied { f.store.pending(p, 1) }
        assertTrue(f.contentKeys().isEmpty())
        assertTrue(f.files.generations(EnterpriseVaultCodec.token(p)).isEmpty())
        assertFalse(f.erasure.cleanupRequired)
        val reopened = VaultFixture(directory, f.keys)
        denied { reopened.store.pending(p, 1) }
        unrecoverable { reopened.confirm(p) }
    }

    @Test
    fun trustedContactProvesExpiryAcrossBootChangesAndCannotDowngradeManagedKeys(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        f.unlock = 60
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        f.keys.locked = true // The platform requires device credential/strong biometric at key use.
        assertThrows(EnterpriseUnlockRequired::class.java) { runBlocking { f.store.record(p, A) } }
        // Unlock-required is never a reason to erase, even when loss is acknowledged.
        assertThrows(EnterpriseUnlockRequired::class.java) { runBlocking { f.confirm(p, acknowledgeErasedWork = true) } }
        f.keys.locked = false
        assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
        f.unlock = null
        unrecoverable { f.confirm(p) }
        assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
        f.unlock = 60
        f.server--
        denied { f.confirm(p) }
        f.now = f.now.copy(boot = "boot-8")
        f.server = 10000 + EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS
        unrecoverable { f.confirm(p) }
        assertTrue(f.contentKeys().isEmpty())
    }

    @Test
    fun changedManagedUnlockPolicyStartsFreshOnlyAfterAcknowledgedLoss(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        f.unlock = 60
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        f.unlock = 300
        f.server++
        unrecoverable { f.confirm(p) }
        assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
        f.confirm(p, acknowledgeErasedWork = true)
        assertNull(f.store.record(p, A))
        assertEquals(300, f.payloadKey().managedUnlockSeconds)
    }

    @Test
    fun rebootPastWallValidityShredsTheKeyWithoutServerContact(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.confirm(p); f.store.enqueue(EnterprisePendingOperation(p, A, B, payload))
        f.now = EnterpriseDeviceClock("boot-8", 0, f.now.wallMillis + EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS - 1)
        denied { f.store.pending(p, 1) }
        assertEquals(3, f.contentKeys().size)
        f.now = f.now.copy(wallMillis = f.now.wallMillis + 1)
        val contacts = f.contacts
        denied { f.store.pending(p, 1) }
        assertEquals(contacts, f.contacts)
        assertTrue(f.contentKeys().isEmpty())
        assertTrue(f.files.generations(EnterpriseVaultCodec.token(p)).isEmpty())
    }

    @Test
    fun allocationFailureIsAnOutageAndLeavesHeldWorkReadable(): Unit = runBlocking {
        var exhaust = false
        val f = VaultFixture(root.resolve("vault"), transform = { delegate -> object : EnterpriseVaultFiles by delegate {
            override fun readGeneration(token: String, generation: Long, descriptor: Boolean): ByteArray? {
                if (exhaust) throw OutOfMemoryError("private allocation diagnostic")
                return delegate.readGeneration(token, generation, descriptor)
            }
        } })
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        exhaust = true
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.record(p, A) } }
        exhaust = false
        assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
    }

    @Test
    fun atomicFailureBeforeOrAfterReplacementHasAnUnambiguousReplayAndNeverExposesPlaintext(): Unit = runBlocking {
        for (after in listOf(false, true)) {
            val directory = root.resolve("vault-" + after)
            var fail = false
            val f = VaultFixture(directory, transform = { delegate -> object : EnterpriseVaultFiles by delegate {
                override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                    if (fail && !after && !descriptor) error("private write failure")
                    delegate.replaceGeneration(token, generation, descriptor, ciphertext)
                }
            } })
            f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
            f.keys.afterCreate = { alias -> if (fail && after && alias.endsWith("_h")) error("private acknowledgement failure") }
            fail = true
            assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
                f.store.enqueue(EnterprisePendingOperation(p, B, A, payload))
            } }
            fail = false
            assertEquals(!after, f.store.enqueue(EnterprisePendingOperation(p, B, A, payload)))
            assertEquals(1, f.store.pending(p, 50).size)
            assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
        }
    }

    @Test
    fun simultaneousWritersThroughIndependentAdaptersPreserveEveryOperation() {
        val directory = root.resolve("vault")
        val first = VaultFixture(directory)
        runBlocking { first.confirm(p) }
        val second = VaultFixture(directory, first.keys)
        val start = CountDownLatch(1)
        val executor = Executors.newFixedThreadPool(8)
        try {
            val jobs = (1..24).map { index ->
                executor.submit(Callable {
                    start.await()
                    val id = "7b0e2a54-0000-4000-8000-" + index.toString().padStart(12, '0')
                    runBlocking {
                        (if (index % 2 == 0) first else second).store.enqueue(EnterprisePendingOperation(p, id, A, payload))
                    }
                })
            }
            start.countDown()
            jobs.forEach { assertTrue(it.get(20, java.util.concurrent.TimeUnit.SECONDS)) }
            val pending = runBlocking { first.store.pending(p, 50) }
            assertEquals(24, pending.size)
            assertEquals(24, pending.map { it.id }.toSet().size)
        } finally { executor.shutdownNow() }
    }

    @Test
    fun unavailableAuthorityAndOversizedOrLinkedFilesFailWithoutChangingHeldWork(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.denyContact = true
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.confirm(p) } }
        assertEquals(0, f.keys.creates)
        f.denyContact = false
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
            f.store.putPrivateFile(p, B, ByteArray(EnterpriseVaultCodec.MAX_PLAIN))
        } }
        assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
        val original = Files.readAllBytes(f.file(directory))
        java.io.RandomAccessFile(f.file(directory).toFile(), "rw").use { it.setLength(EnterpriseVaultCodec.MAX_SEALED.toLong() + 1) }
        unrecoverable { f.store.record(p, A) }
        Files.delete(f.file(directory))
        val outside = root.resolve("outside")
        Files.write(outside, original)
        Files.createSymbolicLink(f.file(directory), outside)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.record(p, A) } }
        assertArrayEquals(original, Files.readAllBytes(outside))
    }
}
