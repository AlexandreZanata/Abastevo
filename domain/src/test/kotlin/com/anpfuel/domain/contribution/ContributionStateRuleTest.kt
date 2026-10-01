package com.anpfuel.domain.contribution

import com.anpfuel.domain.portable.PortablePhoto
import com.anpfuel.domain.repository.ContributionRemoteStatus
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ContributionStateRuleTest {

    @Test
    fun `never-sent draft is queued without retry`() {
        val state = ContributionStateRule.resolve(
            local = ContributionLocalPhase.NOT_SENT,
            remote = null,
            review = ContributionReview.NONE,
        )

        assertEquals(ContributionState.Queued(retryable = false), state)
    }

    @Test
    fun `transport failure stays queued with retry`() {
        val state = ContributionStateRule.resolve(
            local = ContributionLocalPhase.TRANSPORT_FAILED,
            remote = null,
            review = ContributionReview.NONE,
        )

        assertEquals(ContributionState.Queued(retryable = true), state)
    }

    @Test
    fun `in-flight dispatch counts as queued`() {
        val state = ContributionStateRule.resolve(
            local = ContributionLocalPhase.IN_FLIGHT,
            remote = null,
            review = ContributionReview.NONE,
        )

        assertEquals(ContributionState.Queued(retryable = false), state)
    }

    @Test
    fun `received receipt is pending`() {
        val state = ContributionStateRule.resolve(
            local = ContributionLocalPhase.ACKNOWLEDGED,
            remote = ContributionRemoteStatus.RECEIVED,
            review = ContributionReview.NONE,
        )

        assertEquals(ContributionState.Pending, state)
    }

    @Test
    fun `validated receipt is accepted`() {
        val state = ContributionStateRule.resolve(
            local = ContributionLocalPhase.ACKNOWLEDGED,
            remote = ContributionRemoteStatus.VALIDATED,
            review = ContributionReview.NONE,
        )

        assertEquals(ContributionState.Accepted, state)
    }

    @Test
    fun `rejected review overrides validated receipt`() {
        val state = ContributionStateRule.resolve(
            local = ContributionLocalPhase.ACKNOWLEDGED,
            remote = ContributionRemoteStatus.VALIDATED,
            review = ContributionReview.REJECTED,
        )

        assertEquals(ContributionState.Rejected, state)
    }

    @Test
    fun `disputed review overrides pending receipt`() {
        val state = ContributionStateRule.resolve(
            local = ContributionLocalPhase.ACKNOWLEDGED,
            remote = ContributionRemoteStatus.RECEIVED,
            review = ContributionReview.DISPUTED,
        )

        assertEquals(ContributionState.Disputed, state)
    }

    @Test
    fun `cancelled draft has no status even when validated`() {
        val state = ContributionStateRule.resolve(
            local = ContributionLocalPhase.CANCELLED,
            remote = ContributionRemoteStatus.VALIDATED,
            review = ContributionReview.ACCEPTED,
        )

        assertNull(state)
    }

    @Test
    fun `evidence lives until the 24h transient boundary`() {
        val capturedAt = 1_000_000L

        assertTrue(ContributionRetention.isEvidenceLive(capturedAt, capturedAt + PortablePhoto.TRANSIENT_TTL_MILLIS - 1))
        assertFalse(ContributionRetention.isEvidenceLive(capturedAt, capturedAt + PortablePhoto.TRANSIENT_TTL_MILLIS))
    }
}
