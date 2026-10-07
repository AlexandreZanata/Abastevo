package com.anpfuel.application.usecase.contribution

import com.anpfuel.domain.contribution.ContributionState
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.OwnedContribution
import com.anpfuel.domain.repository.OwnedContributionPhase
import com.anpfuel.domain.repository.QueuedContribution
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P21-T03: private owner status maps durable phases to the frozen P21-T01
 * presentation states; cancellation delegates with blank-id rejection.
 */
class GetOwnedContributionsUseCaseTest {

    private class FakeOutbox(
        val owned: List<OwnedContribution> = emptyList(),
    ) : ContributionOutboxRepository {
        val cancelled = mutableListOf<String>()

        override suspend fun enqueue(
            draft: ContributionDraft,
            payloadJson: String,
        ): QueuedContribution = throw UnsupportedOperationException()

        override suspend fun listDispatchable(nowMillis: Long): List<QueuedContribution> =
            emptyList()

        override suspend fun loadPayload(commandId: String): String? = null

        override suspend fun markDispatched(commandId: String, nonce: String) = Unit

        override suspend fun markAcknowledged(commandId: String, revision: Int) = Unit

        override suspend fun markFailed(commandId: String, nowMillis: Long) = Unit

        override suspend fun cancel(commandId: String) {
            cancelled.add(commandId)
        }

        override suspend fun listOwned(): List<OwnedContribution> = owned
    }

    private fun owned(
        commandId: String,
        phase: OwnedContributionPhase,
        revision: Int = 1,
        attempts: Int = 0,
    ) = OwnedContribution(commandId, revision, attempts, phase)

    @Test
    fun `durable phases map to presentation states`() = runTest {
        val outbox = FakeOutbox(
            owned = listOf(
                owned("cmd-queued", OwnedContributionPhase.QUEUED),
                owned("cmd-flight", OwnedContributionPhase.IN_FLIGHT),
                owned("cmd-failed", OwnedContributionPhase.FAILED, attempts = 2),
                owned("cmd-cancelled", OwnedContributionPhase.CANCELLED),
            ),
        )
        val statuses = GetOwnedContributionsUseCase(outbox).invoke()

        assertEquals(4, statuses.size)
        assertEquals(
            ContributionState.Queued(retryable = false),
            statuses.first { it.commandId == "cmd-queued" }.state,
        )
        assertEquals(
            ContributionState.Queued(retryable = false),
            statuses.first { it.commandId == "cmd-flight" }.state,
        )
        val failed = statuses.first { it.commandId == "cmd-failed" }
        assertEquals(ContributionState.Queued(retryable = true), failed.state)
        assertEquals(2, failed.attempts)
        assertNull(statuses.first { it.commandId == "cmd-cancelled" }.state)
    }

    @Test
    fun `cancel delegates and blank id fails fast`() = runTest {
        val outbox = FakeOutbox()
        val cancel = CancelOwnedContributionUseCase(outbox)

        cancel.invoke("cmd-1")
        assertTrue(outbox.cancelled == listOf("cmd-1"))

        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { cancel.invoke("  ") }
        }
        assertTrue(outbox.cancelled == listOf("cmd-1"))
    }
    @Test fun `owner and environment filtering preserves truthful pending accepted and expired receipts`() = runTest {
        val scope=com.anpfuel.domain.model.ContributionScope("synthetic-owner","https://example.invalid")
        val other=scope.copy(ownerScope="other")
        val outbox=FakeOutbox(listOf(
            owned("pending",OwnedContributionPhase.ACKNOWLEDGED).copy(scope=scope,remoteStatus=com.anpfuel.domain.repository.ContributionRemoteStatus.RECEIVED),
            owned("accepted",OwnedContributionPhase.ACKNOWLEDGED).copy(scope=scope,remoteStatus=com.anpfuel.domain.repository.ContributionRemoteStatus.VALIDATED),
            owned("expired",OwnedContributionPhase.ACKNOWLEDGED).copy(scope=scope,remoteStatus=com.anpfuel.domain.repository.ContributionRemoteStatus.EXPIRED,reason="contribution.photo-expired"),
            owned("foreign",OwnedContributionPhase.QUEUED).copy(scope=other),
            owned("unscoped",OwnedContributionPhase.QUEUED)))
        val provider=com.anpfuel.application.port.ContributionScopeProvider { scope }
        val result=GetOwnedContributionsUseCase(outbox,provider).invoke()
        assertEquals(3,result.size); assertEquals(ContributionState.Pending,result[0].state)
        assertEquals(ContributionState.Accepted,result[1].state); assertEquals(ContributionState.Rejected,result[2].state)
        val cancel=CancelOwnedContributionUseCase(outbox,provider)
        for(id in listOf("foreign","unscoped","accepted")) {
            assertThrows(DomainException::class.java) { kotlinx.coroutines.runBlocking { cancel.invoke(id) } }
        }
        assertTrue(outbox.cancelled.isEmpty())
    }

}
