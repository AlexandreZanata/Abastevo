package com.anpfuel.application.usecase.contribution

import com.anpfuel.domain.repository.*
import com.anpfuel.domain.exception.DomainException
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
        coEvery { outbox.findDispatchable("row", any()) } returns pending
        coEvery { outbox.listDispatchablePage(any(), any()) } returns listOf(pending)
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
        coEvery { outbox.findDispatchable("other", any()) } returns null
        coEvery { outbox.findOwned("other") } returns null
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

    // SEC06-D: server Retry-After overrides the curve (bounded by the
    // ceiling); permanent refusals park distinctly; transient domain
    // refusals keep backoff; every attempt mints a fresh proof nonce
    // under the stable operation id.
    @Test fun `rate limited honors server retry hint`() = runTest {
        ready()
        coEvery { gateway.submit(any(), any(), any(), any()) } throws ContributionRateLimited(120)
        assertEquals(DispatchOutcome.RETRY, dispatcher().invoke("row"))
        coVerify(exactly = 1) { outbox.failDispatch("row", 1, "lease", 1000, 120_000L) }
    }

    @Test fun `permanent refusal parks distinctly while transient backoffs`() = runTest {
        ready()
        coEvery { gateway.submit(any(), any(), any(), any()) } throws DomainException("contribution.owner-or-origin-changed")
        assertEquals(DispatchOutcome.REFUSED, dispatcher().invoke("row"))
        coVerify(exactly = 1) { outbox.refusePermanently("row", 1, "contribution.owner-or-origin-changed") }
        coVerify(exactly = 0) { outbox.failDispatch(any(), any(), any(), any(), any()) }
        coEvery { gateway.submit(any(), any(), any(), any()) } throws DomainException("contribution.flaky-remote")
        assertEquals(DispatchOutcome.REFUSED, dispatcher().invoke("row"))
        coVerify(exactly = 1) { outbox.failDispatch("row", 1, "lease", 1000) }
    }

    @Test fun `attempts mint fresh nonces under the stable operation id`() = runTest {
        ready()
        val nonces = mutableListOf<String>()
        coEvery { gateway.submit(any(), any(), any(), capture(nonces)) } throws java.io.IOException("offline") andThen
            ContributionReceipt("row", 1, ContributionRemoteStatus.VALIDATED, "observation")
        var attempt = 0
        val dispatcher = DispatchContributionUseCase(outbox, gateway, { 1000L }, { "nonce-${++attempt}" })
        coEvery { outbox.claimDispatch("row", 1, any(), any()) } returns true
        assertEquals(DispatchOutcome.RETRY, dispatcher.invoke("row"))
        assertEquals(DispatchOutcome.DONE, dispatcher.invoke("row"))
        assertEquals(listOf("nonce-1", "nonce-2"), nonces)
    }
    @Test fun `unbounded server seconds saturate without negative delay`() = runTest {
        ready()
        coEvery { gateway.submit(any(), any(), any(), any()) } throws ContributionRateLimited(Long.MAX_VALUE)
        assertEquals(DispatchOutcome.RETRY,dispatcher().invoke("row"))
        coVerify {outbox.failDispatch("row",1,"lease",1000,Long.MAX_VALUE)}
    }

}
