package com.anpfuel.application.usecase.contribution

import com.anpfuel.application.port.ContributionOutboxFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.OwnedContribution
import com.anpfuel.domain.repository.QueuedContribution
import com.anpfuel.domain.valueobject.FuelProduct
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T05: disabled never persists, process-death duplicate keeps one
 * command with bumped revision, old drafts stay historical.
 */
class EnqueueContributionUseCaseTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    private class FakeFlags(val enabled: Boolean) : ContributionOutboxFlagProvider {
        override fun isEnabled(): Boolean = enabled
    }

    private class InMemoryOutbox : ContributionOutboxRepository {
        val payloads = mutableMapOf<String, String>()
        val revisions = mutableMapOf<String, Int>()
        var calls = 0

        override suspend fun enqueue(
            draft: ContributionDraft,
            payloadJson: String,
        ): QueuedContribution {
            calls += 1
            val nextRevision = (revisions[draft.clientSubmissionId] ?: 0) + 1
            revisions[draft.clientSubmissionId] = nextRevision
            payloads[draft.clientSubmissionId] = payloadJson
            return QueuedContribution(
                draft.clientSubmissionId,
                nextRevision,
                payloadJson,
                payloadJson.contains("\"freshness\":\"historical\""),
            )
        }

        override suspend fun listDispatchable(nowMillis: Long): List<QueuedContribution> =
            payloads.map { (id, payload) ->
                QueuedContribution(id, revisions.getValue(id), payload, payload.contains("historical"))
            }

        override suspend fun loadPayload(commandId: String): String? = payloads[commandId]

        override suspend fun markDispatched(commandId: String, nonce: String) = Unit

        override suspend fun markAcknowledged(commandId: String, revision: Int) {
            if (revisions[commandId] == revision) {
                payloads.remove(commandId)
                revisions.remove(commandId)
            }
        }

        override suspend fun markFailed(commandId: String, nowMillis: Long) = Unit

        override suspend fun cancel(commandId: String) = Unit

        override suspend fun listOwned(): List<OwnedContribution> = emptyList()
    }

    private fun request(
        commandId: String = "cmd-1",
        capturedAt: Long = 1_000_000L,
    ) = EnqueueContributionUseCase.Request(
        clientSubmissionId = commandId,
        stationId = stationId,
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        amountMilliBrl = 5890L,
        conditionKind = "STANDARD",
        capturedAtMillis = capturedAt,
    )

    @Test
    fun `disabled flag never touches outbox`() = runTest {
        val outbox = InMemoryOutbox()
        val useCase = EnqueueContributionUseCase(FakeFlags(false), outbox)

        val outcome = useCase.invoke(request())

        assertTrue(outcome is EnqueueContributionOutcome.Disabled)
        assertEquals(0, outbox.calls)
    }

    @Test
    fun `fresh capture queues fresh with stable id`() = runTest {
        val outbox = InMemoryOutbox()
        val useCase = EnqueueContributionUseCase(FakeFlags(true), outbox, nowMillis = { 2_000_000L })

        val outcome = useCase.invoke(request(capturedAt = 1_900_000L))

        assertTrue(outcome is EnqueueContributionOutcome.Queued)
        val queued = outcome as EnqueueContributionOutcome.Queued
        assertEquals("cmd-1", queued.command.commandId)
        assertEquals(1, queued.command.revision)
        assertEquals(false, queued.historical)
        assertTrue(queued.command.payloadJson.contains("\"client_submission_id\":\"cmd-1\""))
    }

    @Test
    fun `old capture queues historical never relabelled fresh`() = runTest {
        val outbox = InMemoryOutbox()
        val now = 100_000_000_000L
        val useCase = EnqueueContributionUseCase(FakeFlags(true), outbox, nowMillis = { now })

        val outcome = useCase.invoke(request(capturedAt = 1_000_000L))

        assertTrue(outcome is EnqueueContributionOutcome.Queued)
        val queued = outcome as EnqueueContributionOutcome.Queued
        assertTrue(queued.historical)
        assertTrue(queued.command.payloadJson.contains("\"freshness\":\"historical\""))
    }

    @Test
    fun `duplicate send bumps revision keeping one command`() = runTest {
        val outbox = InMemoryOutbox()
        val useCase = EnqueueContributionUseCase(FakeFlags(true), outbox, nowMillis = { 2_000_000L })

        useCase.invoke(request(capturedAt = 1_900_000L))
        val second = useCase.invoke(request(capturedAt = 1_900_000L))

        assertTrue(second is EnqueueContributionOutcome.Queued)
        assertEquals(2, (second as EnqueueContributionOutcome.Queued).command.revision)
        assertEquals(1, outbox.payloads.size)
    }

    @Test
    fun `blank command id fails fast`() = runTest {
        val useCase = EnqueueContributionUseCase(FakeFlags(true), InMemoryOutbox())

        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.invoke(request(commandId = "  ")) }
        }
    }

    @Test
    fun `payload carries no location contributor or metadata keys`() = runTest {
        val outbox = InMemoryOutbox()
        val useCase = EnqueueContributionUseCase(FakeFlags(true), outbox, nowMillis = { 2_000_000L })

        val outcome = useCase.invoke(request(capturedAt = 1_900_000L))

        assertTrue(outcome is EnqueueContributionOutcome.Queued)
        val payload = (outcome as EnqueueContributionOutcome.Queued).command.payloadJson.lowercase()
        for (key in listOf("lat", "lng", "lon", "gps", "location", "exif", "contributor", "signed_url", "mock", "simulated")) {
            assertTrue(!payload.contains("\"$key\""), "payload leaks $key")
        }
    }
}
