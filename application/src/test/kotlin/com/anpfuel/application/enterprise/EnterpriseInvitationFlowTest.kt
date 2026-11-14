package com.anpfuel.application.enterprise

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
class EnterpriseInvitationFlowTest {
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val b = "7b0e2a54-0000-4000-8000-000000000002"
    private val binding = EnterpriseAccessBinding(a, "https://api.example.invalid", a, 0)
    private val preview = EnterpriseInvitationPreview("Synthetic company", false, listOf(EnterpriseInvitationGrant("DRIVER", "SELF")), true, "a".repeat(64))
    private fun ack(membership: String = "INVITED", state: String = "ACCEPTED") = EnterpriseInvitationAcknowledgement(a, a, b, state, membership, false)
    private open inner class Gateway : EnterpriseInvitationGateway {
        var selected: EnterpriseAccessBinding? = binding
        val commands = mutableListOf<Pair<EnterpriseInvitationDecision, String>>()
        var previews = 0
        var accepted: EnterpriseAccessResult<EnterpriseInvitationAcknowledgement> = EnterpriseAccessResult.Ok(ack())
        override fun currentBinding() = selected
        override suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding> = selected?.let { EnterpriseAccessResult.Ok(it) } ?: EnterpriseAccessResult.Err(EnterpriseAccessError.SIGN_IN_REQUIRED)
        override suspend fun preview(binding: EnterpriseAccessBinding, code: String): EnterpriseAccessResult<EnterpriseInvitationPreview> {
            previews++; return EnterpriseAccessResult.Ok(this@EnterpriseInvitationFlowTest.preview)
        }
        override suspend fun accept(binding: EnterpriseAccessBinding, operationId: String, code: String, consentDigest: String): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement> {
            commands += EnterpriseInvitationDecision.ACCEPT to operationId
            assertEquals("a".repeat(64), consentDigest)
            return accepted
        }
        override suspend fun decline(binding: EnterpriseAccessBinding, operationId: String, code: String): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement> {
            commands += EnterpriseInvitationDecision.DECLINE to operationId
            return EnterpriseAccessResult.Ok(ack("ENDED", "DECLINED"))
        }
        override fun invalidate() = Unit
    }

    @Test fun `explicit consent waits for server and preserves confirmation pending`(): Unit = runTest {
        val gateway = Gateway()
        val flow = EnterpriseInvitationFlow(gateway) { a }
        assertTrue(flow.state.value.signedIn)
        flow.preview("synthetic-code")
        assertEquals(EnterpriseInvitationStage.PREVIEW, flow.state.value.stage)
        flow.accept(false)
        assertTrue(gateway.commands.isEmpty())
        assertNull(flow.state.value.acknowledgement)
        flow.accept(true)
        assertTrue(flow.state.value.acknowledgement!!.awaitingConfirmation)
        assertFalse(flow.state.value.acknowledgement!!.active)
        assertEquals(EnterpriseInvitationStage.RESULT, flow.state.value.stage)
        assertNull(flow.state.value.preview)
        flow.restart()
        flow.preview("another-code")
        flow.decline()
        assertEquals("DECLINED", flow.state.value.acknowledgement!!.state)
    }

    @Test fun `lost decision retries only immutable original intent and operation`(): Unit = runTest {
        val gateway = Gateway().apply { accepted = EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
        var ids = 0
        val flow = EnterpriseInvitationFlow(gateway) { ids++; a }
        flow.preview("synthetic-code")
        flow.accept(true)
        assertNull(flow.state.value.acknowledgement)
        assertEquals(EnterpriseInvitationDecision.ACCEPT, flow.state.value.retryDecision)
        flow.decline()
        flow.preview("new-code")
        assertEquals(1, gateway.previews)
        assertEquals(1, gateway.commands.size)
        gateway.accepted = EnterpriseAccessResult.Ok(ack())
        flow.accept(true)
        assertEquals(1, ids)
        assertEquals(listOf(a, a), gateway.commands.map { it.second })
        assertTrue(flow.state.value.acknowledgement!!.awaitingConfirmation)
    }

    @Test fun `account family origin background and logout invalidate displayed private state immediately`(): Unit = runTest {
        for (changed in listOf(binding.copy(accountId = b), binding.copy(familyId = b), binding.copy(origin = "https://other.example.invalid"), binding.copy(epoch = 1), null)) {
            val gateway = Gateway()
            val flow = EnterpriseInvitationFlow(gateway) { a }
            flow.preview("private-code")
            gateway.selected = changed
            flow.invalidateIfStale()
            assertNull(flow.state.value.preview)
            assertNull(flow.state.value.acknowledgement)
            flow.accept(true)
            assertTrue(gateway.commands.isEmpty())
            assertFalse(flow.state.value.toString().contains("private-code"))
        }
        val loggedOut = Gateway().apply { selected = null }
        val flow = EnterpriseInvitationFlow(loggedOut) { a }
        assertFalse(flow.state.value.signedIn)
        flow.preview("code")
        assertEquals(EnterpriseAccessError.SIGN_IN_REQUIRED, flow.state.value.error)
        loggedOut.selected = binding
        flow.invalidateIfStale()
        assertTrue(flow.state.value.signedIn)
    }

    @Test fun `late preview and receipt cannot resurrect old account or restarted ceremony`(): Unit = runTest {
        for (receipt in listOf(false, true)) {
            val entered = CompletableDeferred<Unit>()
            val release = CompletableDeferred<Unit>()
            val gateway = object : Gateway() {
                override suspend fun preview(binding: EnterpriseAccessBinding, code: String): EnterpriseAccessResult<EnterpriseInvitationPreview> {
                    if (!receipt) { entered.complete(Unit); release.await() }
                    return super.preview(binding, code)
                }
                override suspend fun accept(binding: EnterpriseAccessBinding, operationId: String, code: String, consentDigest: String): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement> {
                    entered.complete(Unit); release.await()
                    return super.accept(binding, operationId, code, consentDigest)
                }
            }
            val flow = EnterpriseInvitationFlow(gateway) { a }
            if (receipt) flow.preview("code")
            val request = async { if (receipt) flow.accept(true) else flow.preview("code") }
            entered.await()
            gateway.selected = binding.copy(epoch = 1)
            flow.invalidateIfStale()
            release.complete(Unit)
            request.await()
            assertNull(flow.state.value.preview)
            assertNull(flow.state.value.acknowledgement)
            assertFalse(flow.state.value.busy)
        }
    }

    @Test fun `same account family renewal requires a new ceremony before preview`(): Unit = runTest {
        lateinit var flow: EnterpriseInvitationFlow
        val gateway = object : Gateway() {
            override suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding> {
                selected = binding.copy(familyId = b, epoch = 1)
                flow.invalidateIfStale()
                return EnterpriseAccessResult.Ok(selected!!)
            }
        }
        flow = EnterpriseInvitationFlow(gateway) { a }
        flow.preview("code")
        assertEquals(EnterpriseAccessError.STALE, flow.state.value.error)
        assertNull(flow.state.value.preview)
        flow.preview("code")
        assertEquals(EnterpriseInvitationStage.PREVIEW, flow.state.value.stage)
    }

    @Test fun `uniform refusals clear preview and never retain raw exceptions`(): Unit = runTest {
        for (error in listOf(EnterpriseAccessError.INVALID_CODE, EnterpriseAccessError.CONFLICT, EnterpriseAccessError.ALREADY_MEMBER, EnterpriseAccessError.MANAGED_RESTRICTED, EnterpriseAccessError.FRESH_AUTH_REQUIRED)) {
            val gateway = Gateway().apply { accepted = EnterpriseAccessResult.Err(error) }
            val flow = EnterpriseInvitationFlow(gateway) { a }
            flow.preview("private-code")
            flow.accept(true)
            assertEquals(error, flow.state.value.error)
            assertNull(flow.state.value.preview)
            assertNull(flow.state.value.retryDecision)
        }
        val gateway = object : Gateway() {
            override suspend fun preview(binding: EnterpriseAccessBinding, code: String): EnterpriseAccessResult<EnterpriseInvitationPreview> = error("PRIVATE-BODY")
        }
        val flow = EnterpriseInvitationFlow(gateway) { a }
        flow.preview("private-code")
        assertEquals(EnterpriseAccessError.UNAVAILABLE, flow.state.value.error)
        assertFalse(flow.state.value.toString().contains("PRIVATE"))
        val contradictory = Gateway().apply { accepted = EnterpriseAccessResult.Ok(ack("ACTIVE")) }
        val guarded = EnterpriseInvitationFlow(contradictory) { a }
        guarded.preview("code")
        guarded.accept(true)
        assertEquals(EnterpriseAccessError.UNAVAILABLE, guarded.state.value.error)
        assertNull(guarded.state.value.acknowledgement)
    }

    @Test fun `cancellation resets busy state and concurrent clicks submit once`(): Unit = runTest {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val gateway = object : Gateway() {
            override suspend fun accept(binding: EnterpriseAccessBinding, operationId: String, code: String, consentDigest: String): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement> {
                commands += EnterpriseInvitationDecision.ACCEPT to operationId
                entered.complete(Unit); release.await()
                return EnterpriseAccessResult.Ok(ack())
            }
        }
        val flow = EnterpriseInvitationFlow(gateway) { a }
        flow.preview("code")
        val first = launch { flow.accept(true) }
        entered.await()
        flow.accept(true); flow.decline()
        assertEquals(1, gateway.commands.size)
        first.cancelAndJoin()
        assertFalse(flow.state.value.busy)
        assertNull(flow.state.value.preview)
    }

    @Test fun `fence publishes monotonic epochs without waiting for local IO`(): Unit = runTest {
        val fence = EnterpriseSessionFence()
        val seen = mutableListOf<Long>()
        backgroundScope.launch(UnconfinedTestDispatcher(testScheduler)) { fence.epochs.toList(seen) }
        fence.lock(); fence.markBackground(); fence.lock()
        assertEquals(3L, fence.epoch)
        assertEquals(1L, fence.backgroundEpoch)
        assertEquals(listOf(0L, 1L, 2L, 3L), seen)
        val threads = List(8) { Thread { repeat(100) { fence.markBackground() } } }
        threads.forEach(Thread::start); threads.forEach(Thread::join)
        assertEquals(803L, fence.epochs.value)
        assertEquals(fence.epoch, fence.epochs.value)
    }
}
