package com.anpfuel.data.enterprise

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.Data
import androidx.work.ListenableWorker
import androidx.work.WorkerFactory
import androidx.work.WorkerParameters
import androidx.work.testing.TestListenableWorkerBuilder
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.data.worker.EnterpriseVaultMaintenanceWorker
import com.anpfuel.domain.enterprise.EnterpriseBoundOfflineLease
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseOfflineLease
import com.anpfuel.domain.enterprise.EnterprisePartition
import java.io.Closeable
import java.io.File
import java.io.IOException
import java.util.UUID
import java.util.concurrent.ConcurrentLinkedQueue
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Real worker, AndroidKeyStore and atomic files; accelerated clocks/policies are explicitly synthetic. */
@RunWith(AndroidJUnit4::class)
class EnterpriseVaultMaintenanceWorkerDeviceTest {
    private val context: Context = ApplicationProvider.getApplicationContext()

    private data class Seed(val partition: EnterprisePartition, val token: String, val record: String, val payloadAlias: String)

    private class Fixture(private val context: Context) : Closeable {
        private val root = File(context.noBackupFilesDir, "enterprise-worker-test-" + UUID.randomUUID())
        private val realKeys = AndroidEnterpriseVaultKeys()
        private val ownedTokens = linkedSetOf<String>()
        private val atomicFiles = AtomicEnterpriseVaultFiles(root)
        val seenDescriptors = ConcurrentLinkedQueue<String>()
        val payloadReads = AtomicInteger()
        val factoryCalls = AtomicInteger()
        val deletionFaults = AtomicInteger()
        @Volatile var refuseDeletion: String? = null
        @Volatile var beforeRead: ((String, Boolean) -> Unit)? = null
        // This is a deterministic port clock, not an OS clock or a 30-day elapsed-time acceptance.
        var now = EnterpriseDeviceClock("synthetic-worker-boot", 1_000, 10_000)
        val fence = EnterpriseSessionFence()

        private fun owned(alias: String): Boolean = ownedTokens.any { token ->
            alias == EnterpriseVaultCodec.alias(token) || alias.startsWith(EnterpriseVaultGenerationNames.prefix(token)) ||
                alias == EnterpriseVaultGenerationNames.denied(token) || alias == EnterpriseVaultGenerationNames.retained(token) ||
                alias == EnterpriseVaultGenerationNames.denied(token.substringBefore('-'))
        }

        // Narrow discovery to this fixture so unrelated validation contexts are never inspected or removed.
        val keys = object : EnterpriseVaultKeys by realKeys {
            override fun aliases(prefix: String): List<String> = realKeys.aliases(prefix).filter(::owned)
            override fun delete(alias: String) {
                if (alias == refuseDeletion) {
                    deletionFaults.incrementAndGet()
                    throw IOException("Synthetic native-provider disposal outage")
                }
                realKeys.delete(alias)
            }
        }
        val files = object : EnterpriseVaultFiles by atomicFiles {
            override fun readGeneration(token: String, generation: Long, descriptor: Boolean): ByteArray? {
                if (descriptor) seenDescriptors.add(token) else payloadReads.incrementAndGet()
                beforeRead?.invoke(token, descriptor)
                return atomicFiles.readGeneration(token, generation, descriptor)
            }
        }
        val erasure = EnterpriseVaultErasure(files, keys, fence)
        val janitor = EnterpriseVaultJanitor(files, keys, fence, erasure, { now })

        fun seed(validityMillis: Long = EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS): Seed {
            val partition = EnterprisePartition.of(UUID.randomUUID().toString(), "https://api.example.invalid", UUID.randomUUID().toString())
            val token = EnterpriseVaultCodec.token(partition)
            ownedTokens += token
            val record = UUID.randomUUID().toString()
            val time = EnterpriseOfflineLease(UUID.randomUUID().toString(), 10_000, now, validityMillis)
            val bound = EnterpriseBoundOfflineLease(UUID.randomUUID().toString(), UUID.randomUUID().toString(), 1, time)
            val snapshot = VaultSnapshot(partition, time, bound = bound)
            VaultKind.entries.forEach { kind ->
                val id = if (kind == VaultKind.RECORD) record else UUID.randomUUID().toString()
                snapshot.entries[kind.name + ":" + id] = VaultEntry(kind, id,
                    if (kind == VaultKind.OPERATION) record else "", PAYLOAD.copyOf())
            }
            files.locked(token.substringBefore('-')) { EnterpriseVaultGenerations(files, keys).commit(snapshot) }
            keys.aliases(EnterpriseVaultGenerationNames.prefix(token)).forEach { alias ->
                assertNull("Real Android keys are not exportable", keys.existing(alias)!!.secret.encoded)
            }
            return Seed(partition, token, record, EnterpriseVaultGenerationNames.data(token, 1))
        }

        fun worker(janitor: EnterpriseVaultJanitor = this.janitor): EnterpriseVaultMaintenanceWorker =
            TestListenableWorkerBuilder<EnterpriseVaultMaintenanceWorker>(context)
                .setInputData(Data.EMPTY)
                .setWorkerFactory(object : WorkerFactory() {
                    override fun createWorker(appContext: Context, workerClassName: String, workerParameters: WorkerParameters): ListenableWorker? {
                        if (workerClassName != EnterpriseVaultMaintenanceWorker::class.java.name) return null
                        factoryCalls.incrementAndGet()
                        return EnterpriseVaultMaintenanceWorker(appContext, workerParameters, janitor)
                    }
                }).build()

        fun assertDisposed(seed: Seed) {
            assertTrue(files.generations(seed.token).isEmpty())
            assertTrue(keys.aliases(EnterpriseVaultGenerationNames.prefix(seed.token)).isEmpty())
            assertNull(keys.existing(seed.payloadAlias))
            assertTrue(files.denied(seed.token))
            assertNotNull(keys.existing(EnterpriseVaultGenerationNames.denied(seed.token)))
        }

        fun assertHeld(seed: Seed) {
            files.locked(seed.token.substringBefore('-')) {
                val snapshot = EnterpriseVaultGenerations(files, keys).load(seed.partition)!!
                assertArrayEquals(PAYLOAD, snapshot.entries["RECORD:" + seed.record]!!.bytes)
                assertEquals(VaultKind.entries.size, snapshot.entries.size)
            }
        }

        override fun close() {
            refuseDeletion = null
            beforeRead = null
            try { keys.aliases(EnterpriseVaultGenerationNames.PREFIX).forEach(realKeys::delete) }
            finally { root.deleteRecursively() }
        }

        companion object {
            private val PAYLOAD = "SYNTHETIC-WORKER-PRIVATE-INTENT".toByteArray()
        }
    }

    @Test
    fun actualWorkerErasesOnlyAcceleratedExpiredScopeAndPreservesLiveEncryptedWork(): Unit = runBlocking {
        Fixture(context).use { f ->
            val expired = f.seed(validityMillis = 1_000) // Named accelerated synthetic policy.
            val live = f.seed()
            val liveBytes = f.files.readGeneration(live.token, 1, false)!!
            f.payloadReads.set(0)
            f.now = f.now.copy(elapsedMillis = 2_000)
            val worker = f.worker()
            assertEquals(Data.EMPTY, worker.inputData)
            assertEquals(ListenableWorker.Result.success(), worker.doWork())
            assertEquals(1, f.factoryCalls.get())
            assertEquals("Maintenance never decrypts payload files", 0, f.payloadReads.get())
            f.assertDisposed(expired)
            assertFalse(f.erasure.cleanupRequired)
            assertArrayEquals(liveBytes, f.files.readGeneration(live.token, 1, false))
            f.assertHeld(live)
        }
    }

    @Test
    fun nativeKeyDisposalPortFailureReturnsRetryAndNewWorkerConvergesFromDurableIntent(): Unit = runBlocking {
        Fixture(context).use { f ->
            val expired = f.seed(validityMillis = 1_000)
            val live = f.seed()
            f.now = f.now.copy(elapsedMillis = 2_000)
            f.refuseDeletion = expired.payloadAlias
            assertEquals(ListenableWorker.Result.retry(), f.worker().doWork())
            assertTrue("The injected provider-port failure was reached", f.deletionFaults.get() > 0)
            assertNotNull(f.keys.existing(expired.payloadAlias))
            assertTrue(f.files.denied(expired.token))
            assertNotNull(f.keys.existing(EnterpriseVaultGenerationNames.denied(expired.token)))
            assertTrue(f.erasure.cleanupRequired)
            f.assertHeld(live)

            f.refuseDeletion = null
            val nextFence = EnterpriseSessionFence()
            val recreatedErasure = EnterpriseVaultErasure(f.files, f.keys, nextFence)
            val recreatedJanitor = EnterpriseVaultJanitor(f.files, f.keys, nextFence, recreatedErasure, { f.now })
            assertEquals(ListenableWorker.Result.success(), f.worker(recreatedJanitor).doWork())
            assertEquals(2, f.factoryCalls.get())
            f.assertDisposed(expired)
            assertFalse(recreatedErasure.cleanupRequired)
            f.assertHeld(live)
        }
    }

    @Test
    fun cancellationDuringRealDescriptorReadPropagatesWithoutReturningRetryOrSuccess(): Unit = runBlocking {
        Fixture(context).use { f ->
            val held = f.seed()
            val entered = CountDownLatch(1)
            val release = CountDownLatch(1)
            val first = AtomicBoolean(true)
            val completed = AtomicReference<ListenableWorker.Result?>()
            f.beforeRead = { _, descriptor ->
                if (descriptor && first.compareAndSet(true, false)) {
                    entered.countDown()
                    check(release.await(10, TimeUnit.SECONDS)) { "Synthetic worker read release timed out" }
                }
            }
            val pending = async(Dispatchers.Default) { f.worker().doWork().also(completed::set) }
            try {
                assertTrue("Actual worker reached native-backed descriptor read", entered.await(10, TimeUnit.SECONDS))
                pending.cancel(CancellationException("Synthetic maintenance cancellation"))
                release.countDown()
                try { pending.await(); fail("Cancelled maintenance returned a result") }
                catch (_: CancellationException) { assertTrue(pending.isCancelled) }
                assertNull(completed.get())
            } finally {
                release.countDown()
                withContext(NonCancellable) { pending.cancelAndJoin() }
                f.beforeRead = null
            }
            assertEquals(ListenableWorker.Result.success(), f.worker().doWork())
            f.assertHeld(held)
        }
    }

    @Test
    fun partialIndependentSlicesCannotMakeANewWorkerSkipEarlierScopes(): Unit = runBlocking {
        Fixture(context).use { f ->
            val firstSeed = f.seed()
            val secondSeed = f.seed()
            val expected = listOf(firstSeed.token, secondSeed.token).sorted()
            f.seenDescriptors.clear()
            val outgoing = f.janitor.newSweep()
            val overlapping = f.janitor.newSweep()
            assertTrue(f.janitor.runSlice(outgoing, 1).hasMore)
            assertTrue(f.janitor.runSlice(overlapping, 1).hasMore)
            assertEquals(listOf(expected.first(), expected.first()), f.seenDescriptors.toList())
            f.seenDescriptors.clear()
            assertEquals(ListenableWorker.Result.success(), f.worker().doWork())
            assertEquals(expected, f.seenDescriptors.toList())
            f.assertHeld(firstSeed)
            f.assertHeld(secondSeed)
        }
    }
}
