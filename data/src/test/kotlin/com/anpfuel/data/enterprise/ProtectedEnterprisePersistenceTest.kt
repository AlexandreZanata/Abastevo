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

    @Test
    fun allPrivateSurfacesStaySealedAcrossRecreationAndPersonalFilesAreUnchanged(): Unit = runBlocking {
        val personal = root.resolve("personal-v37.db")
        val original = byteArrayOf(83,81,76,105,116,101,0,99)
        Files.write(personal, original)
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.store.confirm(p)
        f.store.put(EnterpriseLocalRecord(p, A, payload))
        assertTrue(f.store.enqueue(EnterprisePendingOperation(p, B, A, payload)))
        f.store.putPrivateFile(p, A, payload)
        f.store.putPrivateFile(p, B, payload, cache = true)
        val ciphertext = Files.readAllBytes(f.file(directory))
        f.store.put(EnterpriseLocalRecord(p, A, payload))
        assertFalse(ciphertext.contentEquals(Files.readAllBytes(f.file(directory))))
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
        assertEquals(1, f.keys.creates)
    }

    @Test
    fun scopeAndSessionOwnershipRemainSeparateAndChangedOperationsNeverReplaceHeldWork(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        val partitions = listOf(p, partition(org = B), partition(owner = B), partition(origin = "https://other.example.invalid"))
        partitions.forEachIndexed { i, scope ->
            f.identity = EnterpriseLocalIdentity(scope.ownerAccount, scope.origin, A)
            f.store.confirm(scope)
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
        f.store.confirm(p)
        assertEquals(1, f.store.pending(p, 50).size)
        assertThrows(IllegalArgumentException::class.java) { runBlocking { f.store.pending(p, 0) } }
        assertThrows(IllegalArgumentException::class.java) { runBlocking { f.store.pending(p, 51) } }
    }

    @Test
    fun restoredBytesWithoutOriginalKeysCannotCreateReplacementKeysOrLeakContent(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.store.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        val noKeys = VaultFixture(directory)
        denied { noKeys.store.record(p, A) }
        denied { noKeys.store.confirm(p) }
        assertEquals(0, noKeys.keys.creates)
        assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
    }

    @Test
    fun modifiedWrongScopeWrongKeyAndUnsupportedEnvelopesFailWithStaticErrors(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.store.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        val original = Files.readAllBytes(f.file(directory))
        for (index in listOf(0, 4, original.lastIndex)) {
            val changed = original.copyOf()
            changed[index] = (changed[index].toInt() xor 1).toByte()
            Files.write(f.file(directory), changed)
            val error = assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.record(p, A) } }
            // Coroutine stack recovery may add another copy of this same safe exception.
            generateSequence<Throwable>(error) { it.cause }.forEach {
                assertTrue(it is EnterprisePersistenceUnavailable)
                assertEquals("Enterprise protected storage unavailable", it.message)
            }
            assertFalse(error.toString().contains(directory.toString()))
        }
        Files.write(f.file(directory), original)
        val foreign = partition(org = B)
        f.store.confirm(foreign)
        Files.write(f.file(directory, foreign), original)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.record(foreign, A) } }
        val alias = EnterpriseVaultCodec.alias(EnterpriseVaultCodec.token(p))
        f.keys.values.remove(alias)
        f.keys.create(alias, null)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.record(p, A) } }
    }

    @Test
    fun rebootAndRollbackLockWithoutDroppingWorkButElapsedExpiryShredsTheKey(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory)
        f.store.confirm(p); f.store.enqueue(EnterprisePendingOperation(p, A, B, payload))
        f.now = f.now.copy(boot = "boot-8", elapsedMillis = 0)
        denied { f.store.pending(p, 1) }
        assertEquals(1, f.keys.values.size)
        f.server += 1000
        f.store.confirm(p)
        assertArrayEquals(payload, f.store.pending(p, 1).single().envelope)
        f.now = f.now.copy(wallMillis = 9999)
        denied { f.store.pending(p, 1) }
        assertEquals(1, f.keys.values.size)
        f.now = f.now.copy(wallMillis = 10000, elapsedMillis = EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS)
        denied { f.store.pending(p, 1) }
        assertTrue(f.keys.values.isEmpty())
        assertFalse(Files.exists(f.file(directory)))
        assertFalse(f.erasure.cleanupRequired)
        val reopened = VaultFixture(directory, f.keys)
        denied { reopened.store.pending(p, 1) }
        denied { reopened.store.confirm(p) }
    }

    @Test
    fun trustedContactProvesExpiryAcrossBootChangesAndCannotDowngradeManagedKeys(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"))
        f.unlock = 60
        f.store.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        f.keys.locked = true // Simulate the platform requiring device credential/strong biometric.
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.record(p, A) } }
        f.keys.locked = false
        assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
        f.unlock = null
        denied { f.store.confirm(p) }
        f.unlock = 60
        f.server--
        denied { f.store.confirm(p) }
        f.now = f.now.copy(boot = "boot-8")
        f.server = 10000 + EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS
        denied { f.store.confirm(p) }
        assertTrue(f.keys.values.isEmpty())
    }

    @Test
    fun atomicFailureBeforeOrAfterReplacementHasAnUnambiguousReplayAndNeverExposesPlaintext(): Unit = runBlocking {
        for (after in listOf(false, true)) {
            val directory = root.resolve("vault-" + after)
            var fail = false
            val f = VaultFixture(directory, transform = { delegate -> object : EnterpriseVaultFiles by delegate {
                override fun replace(token: String, ciphertext: ByteArray) {
                    if (fail && !after) error("private write failure")
                    delegate.replace(token, ciphertext)
                    if (fail && after) error("private acknowledgement failure")
                }
            } })
            f.store.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
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
        runBlocking { first.store.confirm(p) }
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
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.confirm(p) } }
        assertEquals(0, f.keys.creates)
        f.denyContact = false
        f.store.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, payload))
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking {
            f.store.putPrivateFile(p, B, ByteArray(EnterpriseVaultCodec.MAX_PLAIN))
        } }
        assertArrayEquals(payload, f.store.record(p, A)!!.envelope)
        val original = Files.readAllBytes(f.file(directory))
        java.io.RandomAccessFile(f.file(directory).toFile(), "rw").use { it.setLength(EnterpriseVaultCodec.MAX_SEALED.toLong() + 1) }
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.record(p, A) } }
        Files.delete(f.file(directory))
        val outside = root.resolve("outside")
        Files.write(outside, original)
        Files.createSymbolicLink(f.file(directory), outside)
        assertThrows(EnterprisePersistenceUnavailable::class.java) { runBlocking { f.store.record(p, A) } }
        assertArrayEquals(original, Files.readAllBytes(outside))
    }
}
