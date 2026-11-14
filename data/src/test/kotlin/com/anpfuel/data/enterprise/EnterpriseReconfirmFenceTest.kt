package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.domain.enterprise.*
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/**
 * A durable reconfirmation requirement fences live contexts exactly once. Background maintenance,
 * delivery discovery and storage reads that later observe the same requirement must not advance
 * the shared epoch again (that invalidated their own leases and every open company flow).
 */
class EnterpriseReconfirmFenceTest {
    @TempDir lateinit var root: Path

    /** Another boot identifier with wall time still inside validity: the reboot RECONFIRM state. */
    private fun reboot(f: VaultFixture) { f.now = EnterpriseDeviceClock("boot-8", 5, f.now.wallMillis + 1) }

    private fun janitor(f: VaultFixture) = EnterpriseVaultJanitor(f.files, f.keys, f.fence, f.erasure, { f.now })

    private fun source(f: VaultFixture) = object : EnterpriseQueueSource {
        override suspend fun account(): EnterpriseQueueAccountLease = ProtectedQueueFixture.account(f)
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt =
            account.forEnrollment(f.store.protectedEnrollment(account, partition))
    }

    @Test fun existingContactRequirementIsAQuietWaitOnEveryBackgroundAndStoragePath(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); val p = partition()
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        reboot(f)
        val maintenance = janitor(f)
        val before = f.fence.epoch
        assertTrue(maintenance.runOnce().complete)
        assertEquals(before + 1, f.fence.epoch, "The first durable requirement fences live contexts once")
        assertNotNull(f.keys.existing(EnterpriseVaultGenerationNames.retained(EnterpriseVaultCodec.token(p))))
        val latched = f.fence.epoch
        repeat(3) { assertTrue(maintenance.runOnce().complete) }
        assertEquals(latched, f.fence.epoch, "Every janitor slice advanced the epoch again")
        val page = f.store.discover(ProtectedQueueFixture.account(f))
        assertTrue(page.enrollments.isEmpty()); assertEquals(0, page.unavailable)
        assertEquals(latched, f.fence.epoch, "Delivery discovery advanced the epoch again")
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.protectedEnrollment(ProtectedQueueFixture.account(f), p) } }
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.record(p, A) } }
        assertEquals(latched, f.fence.epoch, "A denied storage read advanced the epoch again")
        assertEquals(latched, f.fence.epochs.value, "An observer saw another invalidation")
    }

    @Test fun rebootReconfirmIsWorkerAttentionAndNeverARetryLoop(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); val p = partition()
        f.confirm(p)
        reboot(f)
        var dispatched = 0
        val runner = EnterpriseOperationWorkRunner(source(f), f.store::discover, { _, _ -> dispatched++; EnterpriseQueueDispatchResult() })
        val before = f.fence.epoch
        runner.runOnce() // The one-time durable transition may fence this first invocation.
        val latched = f.fence.epoch
        assertEquals(before + 1, latched, "Only the durable transition may fence")
        repeat(3) {
            val result = runner.runOnce()
            assertEquals(0, result.failed, "Waiting for reconfirmation was counted as a worker failure")
            assertEquals(1, result.attention, "Waiting for reconfirmation is an attention state")
            assertFalse(result.needsRetry, "A partition waiting for the person must not retry the worker")
            assertEquals(latched, f.fence.epoch)
        }
        assertEquals(0, dispatched, "Work waiting for reconfirmation was dispatched")
    }

    @Test fun inFlightReconfirmationSurvivesBackgroundWorkOnALatchedPartition(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault")); val p = partition()
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        reboot(f)
        val maintenance = janitor(f)
        maintenance.runOnce()
        val latched = f.fence.epoch
        val runner = EnterpriseOperationWorkRunner(source(f), f.store::discover, { _, _ -> error("Latched work was dispatched") })
        var background = 0
        val authority = EnterpriseContactAuthority { request ->
            // The person's reconfirmation waits on the network while startup/periodic work runs.
            maintenance.runOnce(); runner.runOnce(); maintenance.runOnce(); background++
            testGrant(request.target.partition, f.identity!!.family, f.server + 1, null, request.target.membershipId)
        }
        val store = ProtectedEnterprisePersistence(f.files, f.keys, f.fence, f.erasure, { f.identity }, { f.now }, authority)
        f.confirm(p, store = store)
        assertEquals(1, background)
        assertEquals(latched + 1, f.fence.epoch, "Only the reconfirmation's own handoff publishes")
        assertNull(f.keys.existing(EnterpriseVaultGenerationNames.retained(EnterpriseVaultCodec.token(p))))
        assertArrayEquals(byteArrayOf(7), f.store.record(p, A)!!.envelope)
    }
}
