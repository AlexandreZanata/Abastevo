package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import java.nio.file.Files
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class EnterpriseVaultMaintenanceTest {
    @TempDir lateinit var root: Path
    @Test fun erasureCannotLowerTrustedFloorWhenBothIntentChannelsAndOldCleanupFail(): Unit = runBlocking {
        var refuseIntent = false; var oldGeneration = 0L
        val f = VaultFixture(root, transform = { real -> object : EnterpriseVaultFiles by real {
            override fun deny(token: String) { if (refuseIntent) error("private marker fault"); real.deny(token) }
            override fun eraseGeneration(token: String, generation: Long) {
                if (generation == oldGeneration) error("private old-file fault")
                real.eraseGeneration(token, generation)
            }
        } })
        val p = partition(); val token = EnterpriseVaultCodec.token(p)
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        oldGeneration = EnterpriseVaultGenerations(f.files, f.keys).head(token)!!
        val oldAliases = f.keys.values.keys.toSet(); f.keys.failAliases = oldAliases
        f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(8)))
        val committedHead = EnterpriseVaultGenerations(f.files, f.keys).head(token)!!
        refuseIntent = true; f.keys.failCreate = true
        assertFalse(f.erasure.partitionRevoked(p))
        assertTrue(f.erasure.cleanupRequired)
        val recreated = VaultFixture(root, f.keys)
        assertEquals(committedHead, EnterpriseVaultGenerations(recreated.files, recreated.keys).head(token))
        assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { recreated.store.record(p, A) } }
        f.keys.failCreate = false; f.keys.failAliases = emptySet(); refuseIntent = false; oldGeneration = 0
        assertTrue(f.erasure.partitionRevoked(p))
        assertTrue(f.contentKeys().isEmpty())
    }
    @Test fun privacyExpiryMaintenanceNeedsNoPayloadUnlockAndRetriesDurableFailures(): Unit = runBlocking {
        val f = VaultFixture(root); val p = partition()
        f.unlock = 60; f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        f.keys.locked = true; f.now = f.now.copy(elapsedMillis = f.now.elapsedMillis + EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS)
        f.keys.failDelete = true
        val first = EnterpriseVaultJanitor(f.files, f.keys, f.fence, f.erasure, { f.now }).runOnce()
        assertTrue(first.failed > 0); assertTrue(f.erasure.cleanupRequired)
        val recreated = VaultFixture(root, f.keys); recreated.now = f.now
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { recreated.store.record(p, A) } }
        f.keys.failDelete = false
        val next = EnterpriseVaultJanitor(recreated.files, f.keys, recreated.fence, recreated.erasure, { f.now }).runOnce()
        assertTrue(next.complete); assertTrue(recreated.contentKeys().isEmpty()); assertFalse(recreated.erasure.cleanupRequired)
    }
    @Test fun malformedCommittedCiphertextAllowsOnlyAcknowledgedEmptySourceRecovery(): Unit = runBlocking {
        for (descriptor in listOf(false, true)) {
            val directory = root.resolve(descriptor.toString()); val f = VaultFixture(directory); val p = partition()
            f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
            val data = f.file(directory)
            val path = if (descriptor) data.resolveSibling(data.fileName.toString().removeSuffix(".data") + ".meta") else data
            Files.write(path, byteArrayOf(1))
            assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { f.confirm(p) } }
            f.confirm(p, acknowledgeErasedWork = true)
            assertNull(f.store.record(p, A)); assertFalse(f.erasure.cleanupRequired)
        }
    }
    @Test fun cancelledSweepAndOverlappingInvocationsHaveIndependentBoundedProgress(): Unit = runBlocking {
        val seen = mutableListOf<String>()
        val f = VaultFixture(root, transform = { real -> object : EnterpriseVaultFiles by real {
            override fun readGeneration(token: String, generation: Long, descriptor: Boolean): ByteArray? {
                if (descriptor) seen += token
                return real.readGeneration(token, generation, descriptor)
            }
        } })
        listOf(A, B, VaultFixture.C).forEach { f.confirm(partition(org = it)) }
        val janitor = EnterpriseVaultJanitor(f.files, f.keys, f.fence, f.erasure, { f.now })
        val cancelled = janitor.newSweep(); seen.clear()
        val first = janitor.runSlice(cancelled, 1); val firstToken = seen.single()
        assertTrue(first.hasMore); assertEquals(1, first.checked)
        seen.clear()
        val replacement = janitor.newSweep()
        assertEquals(1, janitor.runSlice(replacement, 1).checked)
        assertEquals(firstToken, seen.single(), "A new worker begins at the first scope")
        seen.clear()
        assertEquals(1, janitor.runSlice(cancelled, 1).checked)
        assertNotEquals(firstToken, seen.single(), "Independent startup/periodic handles do not share a cursor")
        assertThrows(IllegalArgumentException::class.java) { janitor.runOnce(0) }
        assertThrows(IllegalArgumentException::class.java) { janitor.runOnce(51) }
    }
    @Test fun accountIntentAndKeystoreOnlyResidueConvergeWithinSameMaintenanceInstance(): Unit = runBlocking {
        var lockFault = false
        val f = VaultFixture(root, transform = { real -> object : EnterpriseVaultFiles by real {
            override fun <T> locked(accountTag: String, work: () -> T): T {
                if (lockFault) error("private lock fault")
                return real.locked(accountTag, work)
            }
        } })
        val token = EnterpriseVaultCodec.token(partition())
        f.keys.create(EnterpriseVaultGenerationNames.retained(token), null)
        val foreign = EnterpriseVaultGenerationNames.retained(EnterpriseVaultCodec.token(partition(owner = B)))
        f.keys.create(foreign, null)
        lockFault = true
        assertFalse(f.erasure.accountRevoked(A, VaultFixture.ORIGIN))
        assertTrue(f.erasure.cleanupRequired)
        lockFault = false
        val janitor = EnterpriseVaultJanitor(f.files, f.keys, f.fence, f.erasure, { f.now })
        janitor.runOnce(); janitor.runOnce()
        assertFalse(f.erasure.cleanupRequired)
        assertTrue(f.contentKeys().isEmpty())
        assertNull(f.keys.existing(EnterpriseVaultGenerationNames.retained(token)))
        assertNotNull(f.keys.existing(foreign), "Another account's lock marker is preserved")
    }

    @Test fun maintenanceObservesRollbackDurablyWithoutErasingOrOpeningManagedPayload(): Unit = runBlocking {
        val f = VaultFixture(root); val p = partition()
        f.unlock = 60; f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        f.keys.locked = true; f.now = f.now.copy(wallMillis = f.now.wallMillis - 1)
        val result = EnterpriseVaultJanitor(f.files, f.keys, f.fence, f.erasure, { f.now }).runOnce()
        assertTrue(result.complete); assertEquals(0, result.erased)
        f.keys.locked = false; f.now = f.now.copy(wallMillis = f.now.wallMillis + 1)
        val recreated = VaultFixture(root, f.keys); recreated.now = f.now; recreated.unlock = 60
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { recreated.store.record(p, A) } }
        recreated.server++
        recreated.confirm(p)
        assertArrayEquals(byteArrayOf(7), recreated.store.record(p, A)!!.envelope)
    }

    @Test fun disposalWithoutAnyDurableDenialKeepsRecoveryFloorUntilAcknowledgedFreshStart(): Unit = runBlocking {
        var denyFault = false
        val f = VaultFixture(root, transform = { real -> object : EnterpriseVaultFiles by real {
            override fun deny(token: String) { if (denyFault) error("private intent fault"); real.deny(token) }
        } })
        val p = partition(); val token = EnterpriseVaultCodec.token(p)
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        val head = EnterpriseVaultGenerations(f.files, f.keys).head(token)!!
        denyFault = true; f.keys.failCreate = true
        assertFalse(f.erasure.partitionRevoked(p))
        assertTrue(f.files.generations(token).isEmpty(), "Payload files were successfully disposed")
        assertTrue(f.keys.values.keys.none { it.endsWith("_d") || it.endsWith("_m") }, "Payload/maintenance keys were disposed")
        assertFalse(f.files.denied(token)); assertNull(f.keys.existing(EnterpriseVaultGenerationNames.denied(token)))
        assertEquals(head, EnterpriseVaultGenerations(f.files, f.keys).head(token), "A recovery floor remains when no denial was durable")
        f.keys.failCreate = false
        val recreated = VaultFixture(root, f.keys)
        assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { recreated.confirm(p) } }
        assertEquals(0, recreated.contacts)
        recreated.confirm(p, acknowledgeErasedWork = true)
        assertNull(recreated.store.record(p, A)); assertFalse(recreated.erasure.cleanupRequired)
    }

}
