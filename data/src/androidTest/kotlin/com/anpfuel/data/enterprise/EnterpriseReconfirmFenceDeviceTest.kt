package com.anpfuel.data.enterprise

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.Data
import androidx.work.ListenableWorker
import androidx.work.WorkInfo
import androidx.work.WorkManager
import androidx.work.WorkerFactory
import androidx.work.WorkerParameters
import androidx.work.testing.TestListenableWorkerBuilder
import androidx.work.testing.WorkManagerTestInitHelper
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.UnavailableEnterpriseContact
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDispatchResult
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.data.worker.EnterpriseOperationScheduler
import com.anpfuel.data.worker.EnterpriseOperationWorker
import com.anpfuel.domain.enterprise.EnterpriseBoundOfflineLease
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseOfflineLease
import com.anpfuel.domain.enterprise.EnterprisePartition
import java.io.Closeable
import java.io.File
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/**
 * EDR-122 on the actual AndroidKeyStore provider, atomic files and real worker class. The clock is
 * an explicit synthetic port (a changed boot identifier), not an OS reboot.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseReconfirmFenceDeviceTest {
    private val context: Context = ApplicationProvider.getApplicationContext()

    private class Fixture(context: Context) : Closeable {
        val root = File(context.noBackupFilesDir, "enterprise-fence-test-" + UUID.randomUUID())
        private val realKeys = AndroidEnterpriseVaultKeys()
        val files = AtomicEnterpriseVaultFiles(root)
        val account: String = UUID.randomUUID().toString()
        val tag = EnterpriseVaultCodec.accountTag(account, ORIGIN)
        // Narrow results to this synthetic account; the real provider still validates every prefix.
        val keys = object : EnterpriseVaultKeys by realKeys {
            override fun aliases(prefix: String): List<String> = realKeys.aliases(prefix).filter { it.contains(tag) }
        }
        val fence = EnterpriseSessionFence()
        val erasure = EnterpriseVaultErasure(files, keys, fence)
        var now = EnterpriseDeviceClock("synthetic-fence-boot", 1_000, 10_000)
        val identity = EnterpriseLocalIdentity(account, ORIGIN, UUID.randomUUID().toString())
        val store = ProtectedEnterprisePersistence(files, keys, fence, erasure, { identity }, { now }, UnavailableEnterpriseContact)
        val janitor = EnterpriseVaultJanitor(files, keys, fence, erasure, { now })

        fun seed(): EnterprisePartition {
            val partition = EnterprisePartition.of(account, ORIGIN, UUID.randomUUID().toString())
            val time = EnterpriseOfflineLease(identity.family, 10_000, now, EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS)
            val bound = EnterpriseBoundOfflineLease(UUID.randomUUID().toString(), UUID.randomUUID().toString(), 1, time)
            files.locked(tag) { EnterpriseVaultGenerations(files, keys).commit(VaultSnapshot(partition, time, bound = bound)) }
            return partition
        }

        fun lease(): EnterpriseQueueAccountLease {
            val epoch = fence.epoch; val background = fence.backgroundEpoch
            return EnterpriseQueueAccountLease.capture(identity, epoch, background, UUID.randomUUID().toString()) {
                fence.epoch == epoch && fence.backgroundEpoch == background
            }
        }

        val source = object : EnterpriseQueueSource {
            override suspend fun account(): EnterpriseQueueAccountLease = lease()
            override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt =
                account.forEnrollment(store.protectedEnrollment(account, partition))
        }

        fun entries(): List<String> = root.list()?.toList().orEmpty()

        override fun close() {
            try { realKeys.aliases(EnterpriseVaultGenerationNames.PREFIX).filter { it.contains(tag) }.forEach(realKeys::delete) }
            finally { root.deleteRecursively() }
        }
    }

    private fun worker(runner: EnterpriseOperationWorkRunner): EnterpriseOperationWorker =
        TestListenableWorkerBuilder<EnterpriseOperationWorker>(context).setInputData(Data.EMPTY)
            .setWorkerFactory(object : WorkerFactory() {
                override fun createWorker(appContext: Context, workerClassName: String, parameters: WorkerParameters): ListenableWorker? {
                    if (workerClassName != EnterpriseOperationWorker::class.java.name) return null
                    return EnterpriseOperationWorker(appContext, parameters, runner)
                }
            }).build()

    @Test
    fun rebootRequirementFencesOnceAndTheRealWorkerSucceedsWithoutRetry(): Unit = runBlocking {
        Fixture(context).use { f ->
            f.seed()
            f.now = EnterpriseDeviceClock("synthetic-fence-reboot", 5, f.now.wallMillis + 1)
            val before = f.fence.epoch
            assertTrue(f.janitor.runOnce().complete)
            assertEquals("The first durable requirement fences once", before + 1, f.fence.epoch)
            val latched = f.fence.epoch
            repeat(3) { assertTrue(f.janitor.runOnce().complete) }
            assertEquals("A janitor slice advanced the epoch again", latched, f.fence.epoch)
            val runner = EnterpriseOperationWorkRunner(f.source, f.store::discover, { _, _ -> error("Waiting work was dispatched") })
            val result = runner.runOnce()
            assertEquals(0, result.failed); assertEquals(1, result.attention)
            repeat(2) { assertEquals(ListenableWorker.Result.success(), worker(runner).doWork()) }
            assertEquals("Delivery discovery advanced the epoch again", latched, f.fence.epoch)
        }
    }

    @Test
    fun personalOnlyDiscoveryUsesProviderPrefixesCreatesNothingAndRevocationStaysClean(): Unit = runBlocking {
        Fixture(context).use { f ->
            val page = f.store.discover(f.lease())
            assertTrue(page.enrollments.isEmpty()); assertEquals(0, page.unavailable)
            assertEquals("Discovery left a local artifact", emptyList<String>(), f.entries())
            assertTrue(f.erasure.accountRevoked(f.account, ORIGIN))
            assertFalse(f.erasure.cleanupRequired)
            assertEquals(emptyList<String>(), f.entries())
            assertNull(f.keys.existing(EnterpriseVaultGenerationNames.denied(f.tag)))
        }
    }

    @Test
    fun knownDueTimeEnqueuesOneDelayedContinuationWithEmptyInput(): Unit = runBlocking {
        WorkManagerTestInitHelper.initializeTestWorkManager(context)
        val manager = WorkManager.getInstance(context)
        try {
            val partition = EnterprisePartition.of(UUID.randomUUID().toString(), ORIGIN, UUID.randomUUID().toString())
            val identity = EnterpriseLocalIdentity(partition.ownerAccount, ORIGIN, UUID.randomUUID().toString())
            val lease = EnterpriseQueueAccountLease.capture(identity, 0, 0, UUID.randomUUID().toString()) { true }
            val enrollment = com.anpfuel.application.enterprise.EnterpriseEnrollmentBinding(
                com.anpfuel.application.enterprise.EnterpriseDeviceTarget(partition, UUID.randomUUID().toString()), UUID.randomUUID().toString(), 1)
            val source = object : EnterpriseQueueSource {
                override suspend fun account(): EnterpriseQueueAccountLease = lease
                override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition) = account.forEnrollment(enrollment)
            }
            val runner = EnterpriseOperationWorkRunner(source, { _, _, _ -> EnterpriseQueueDiscoveryPage(listOf(enrollment), null, 0) },
                { _, _ -> EnterpriseQueueDispatchResult(retryable = 1, retryAfterMillis = 120_000) })
            assertEquals(ListenableWorker.Result.success(), worker(runner).doWork())
            val infos = manager.getWorkInfosForUniqueWork(EnterpriseOperationScheduler.CONTINUATION).get()
            assertEquals(1, infos.size)
            val info = infos.single()
            assertEquals(WorkInfo.State.ENQUEUED, info.state)
            assertEquals(120_000L, info.initialDelayMillis)
            assertTrue(EnterpriseOperationScheduler.CONTINUATION in info.tags)
        } finally {
            manager.cancelUniqueWork(EnterpriseOperationScheduler.CONTINUATION).result.get()
        }
    }

    private companion object { const val ORIGIN = "https://api.example.invalid" }
}
