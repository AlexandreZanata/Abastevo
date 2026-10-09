package com.anpfuel.application.usecase.contribution

import com.anpfuel.domain.repository.*
import com.anpfuel.domain.exception.ContributionPhotoExpired
import io.mockk.*
import java.io.IOException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class DispatchContributionUseCaseTest {
    private val outbox = mockk<ContributionOutboxRepository>(relaxed = true)
    private val gateway = mockk<ContributionSubmissionGateway>()
    private val pending = QueuedContribution("row", 1, "frozen", false)
    private fun dispatcher() = DispatchContributionUseCase(outbox, gateway, { 1000L }, { "lease" })
    private fun ready(receipt: ContributionReceipt? = null) {
        coEvery { outbox.listDispatchable(any()) } returns listOf(pending)
        coEvery { outbox.claimDispatch("row", 1, "lease", 1000) } returns true
        coEvery { outbox.receipt("row") } returns receipt
    }
    @Test fun `received receipt stays durable pending and later refresh validates without reupload`() = runTest {
        ready()
        val received = ContributionReceipt("row", 1, ContributionRemoteStatus.RECEIVED, "observation")
        coEvery { gateway.submit("row", 1, "frozen", "lease") } returns received
        assertEquals(DispatchOutcome.RETRY, dispatcher().invoke("row"))
        coVerify(exactly = 1) { outbox.recordReceipt(received, 1000) }
        coVerify(exactly = 0) { outbox.markAcknowledged(any(), any()) }
        ready(received)
        val validated = received.copy(status = ContributionRemoteStatus.VALIDATED)
        coEvery { gateway.refresh(received, "frozen") } returns validated
        assertEquals(DispatchOutcome.DONE, dispatcher().invoke("row"))
        coVerify(exactly = 1) { gateway.submit(any(), any(), any(), any()) }
        coVerify(exactly = 1) { outbox.recordReceipt(validated, 1000) }
    }
    @Test fun `lost claim never sends and never substitutes another requested row`() = runTest {
        ready()
        coEvery { outbox.claimDispatch(any(), any(), any(), any()) } returns false
        assertEquals(DispatchOutcome.RETRY, dispatcher().invoke("row"))
        assertEquals(DispatchOutcome.DONE, dispatcher().invoke("other"))
        coVerify(exactly = 0) { gateway.submit(any(), any(), any(), any()) }
    }
    @Test fun `transport failure preserves command and expiration gets truthful terminal receipt`() = runTest {
        ready()
        coEvery { gateway.submit(any(), any(), any(), any()) } throws IOException("offline")
        assertEquals(DispatchOutcome.RETRY, dispatcher().invoke("row"))
        coVerify(exactly = 1) { outbox.failDispatch("row", 1, "lease", 1000) }
        coEvery { gateway.submit(any(), any(), any(), any()) } throws ContributionPhotoExpired()
        assertEquals(DispatchOutcome.DONE, dispatcher().invoke("row"))
        coVerify { outbox.recordReceipt(match { it.status == ContributionRemoteStatus.EXPIRED && it.observationId == null }, 1000) }
    }
    @Test fun `coroutine cancellation leaves recoverable lease and no failure or acknowledgement`() = runTest {
        ready()
        coEvery { gateway.submit(any(), any(), any(), any()) } throws CancellationException()
        try { dispatcher().invoke("row"); fail<Unit>("cancellation swallowed") } catch (_: CancellationException) { }
        coVerify(exactly = 0) { outbox.failDispatch(any(), any(), any(), any()) }
        coVerify(exactly = 0) { outbox.recordReceipt(any(), any()) }
    }
}
