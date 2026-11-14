package com.anpfuel.application.enterprise

import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseHandoffPublicationTest {
    @Test fun `terminal and reconfirm locks coalesce through nested failure before observer publication`() {
        val fence = EnterpriseSessionFence()
        val observed = mutableListOf<Long>()
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        scope.launch(start = CoroutineStart.UNDISPATCHED) { fence.epochs.collect { observed += it } }
        try {
            assertThrows(IllegalStateException::class.java) {
                fence.access {
                    fence.lock()
                    fence.access { fence.lock(); throw IllegalStateException("Synthetic nested failure") }
                }
            }
            assertEquals(2L, fence.epoch)
            assertEquals(listOf(0L, 2L), observed)
        } finally { scope.cancel() }
    }

    @Test fun `background remains independent of a held fence and later flush never publishes an older epoch`() {
        val fence = EnterpriseSessionFence()
        val held = CountDownLatch(1); val release = CountDownLatch(1)
        val backgroundFinished = CountDownLatch(1); val ownerFinished = CountDownLatch(1)
        val observed = java.util.Collections.synchronizedList(mutableListOf<Long>())
        val failures = java.util.Collections.synchronizedList(mutableListOf<Throwable>())
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        scope.launch(start = CoroutineStart.UNDISPATCHED) { fence.epochs.collect { observed += it } }
        val owner = Thread({
            try {
                fence.access {
                    fence.lock(); held.countDown()
                    check(release.await(5, TimeUnit.SECONDS))
                    fence.lock()
                }
            } catch (failure: Throwable) { failures += failure }
            finally { ownerFinished.countDown() }
        }, "enterprise-held-fence").apply { isDaemon = true }
        val background = Thread({
            try { fence.markBackground() }
            catch (failure: Throwable) { failures += failure }
            finally { backgroundFinished.countDown() }
        }, "enterprise-background-fence").apply { isDaemon = true }
        try {
            owner.start()
            assertTrue(held.await(3, TimeUnit.SECONDS))
            background.start()
            assertTrue(backgroundFinished.await(3, TimeUnit.SECONDS), "Background invalidation waited for held file work")
            assertEquals(2L, fence.epochs.value)
            assertEquals(1L, fence.backgroundEpoch)
            release.countDown()
            assertTrue(ownerFinished.await(3, TimeUnit.SECONDS))
            assertEquals(listOf(0L, 2L, 3L), observed)
            assertTrue(failures.isEmpty())
        } finally { release.countDown(); scope.cancel() }
    }

    @Test fun `nested contact publication resumes observers after the enclosing fence access unlocks`() {
        val account = "7b0e2a54-0000-4000-8000-000000000001"
        val origin = "https://api.example.invalid"
        val fence = EnterpriseSessionFence()
        fun binding() = EnterpriseAccessBinding(account, origin, account, fence.epoch)
        val local = EnterpriseLocalContext({ account }, { origin }, UnavailableEnterprisePersistence, { fence.epoch })
        val owner = local.reserveContact(local.acquireOwner(), binding(), fence, ::binding)
        val accessible = AtomicBoolean()
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        scope.launch(start = CoroutineStart.UNDISPATCHED) {
            fence.epochs.collect { epoch -> if (epoch == 1L) {
                val probeEntered = CountDownLatch(1)
                Thread({ fence.access { probeEntered.countDown() } }, "enterprise-handoff-fence-probe").apply { isDaemon = true; start() }
                accessible.set(probeEntered.await(3, TimeUnit.SECONDS))
            } }
        }
        try {
            val handoff = fence.access { fence.commitHandoff(owner, 0) }
            assertTrue(accessible.get(), "An observer resumed while the enclosing fence access still held its lock")
            assertTrue(fence.handoffIsCurrent(owner, handoff, binding()))
        } finally { scope.cancel() }
    }

    @Test fun `epoch observer publication never holds local ownership against withdrawing flow monitor`() {
        val account = "7b0e2a54-0000-4000-8000-000000000001"
        val origin = "https://api.example.invalid"
        val fence = EnterpriseSessionFence()
        val publishing = CountDownLatch(1)
        val monitorHeld = CountDownLatch(1)
        val continueWithdrawal = CountDownLatch(1)
        val finished = CountDownLatch(2)
        val failures = java.util.Collections.synchronizedList(mutableListOf<Throwable>())
        fun binding() = EnterpriseAccessBinding(account, origin, account, fence.epoch)
        val local = EnterpriseLocalContext({ account }, { origin }, UnavailableEnterprisePersistence, { fence.epoch })
        val gateway = object : EnterpriseContextGateway {
            override fun currentBinding(): EnterpriseAccessBinding {
                if (Thread.currentThread().name == "enterprise-handoff-withdrawal") {
                    monitorHeld.countDown()
                    check(continueWithdrawal.await(5, TimeUnit.SECONDS))
                }
                return binding()
            }
            override suspend fun open() = EnterpriseAccessResult.Ok(binding())
            override suspend fun list(binding: EnterpriseAccessBinding, afterOrganizationId: String?) = EnterpriseAccessResult.Ok(EnterpriseContextPage(emptyList()))
            override suspend fun read(binding: EnterpriseAccessBinding, context: EnterpriseContextSummary) = EnterpriseAccessResult.Ok(context)
            override fun invalidate() = Unit
        }
        val flow = EnterpriseContextFlow(gateway, local)
        val owner = local.reserveContact(local.acquireOwner(), binding(), fence, ::binding)
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined)
        scope.launch(start = CoroutineStart.UNDISPATCHED) {
            fence.epochs.collect { epoch -> if (epoch == 1L) {
                publishing.countDown()
                check(monitorHeld.await(5, TimeUnit.SECONDS))
                continueWithdrawal.countDown()
                flow.invalidateIfStale()
            } }
        }
        // A failing RED owns only daemon threads and isolated locks. It cannot
        // leave a stuck non-daemon worker or block the whole test process.
        val commit = Thread({
            try { fence.commitHandoff(owner, 0) }
            catch (failure: Throwable) { failures += failure }
            finally { finished.countDown() }
        }, "enterprise-handoff-commit").apply { isDaemon = true }
        val withdrawal = Thread({
            try {
                check(publishing.await(5, TimeUnit.SECONDS))
                flow.unconfirm()
            } catch (failure: Throwable) { failures += failure }
            finally { finished.countDown() }
        }, "enterprise-handoff-withdrawal").apply { isDaemon = true }
        try {
            withdrawal.start(); commit.start()
            assertTrue(finished.await(7, TimeUnit.SECONDS), "Unconfined epoch publication inverted the flow monitor and local ownership locks")
            assertTrue(failures.isEmpty(), "Concurrent publication/withdrawal failed")
        } finally { scope.cancel() }
    }
}
