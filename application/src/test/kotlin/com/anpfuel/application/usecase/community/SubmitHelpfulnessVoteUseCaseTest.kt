package com.anpfuel.application.usecase.community

import com.anpfuel.application.port.CommunityHelpfulnessFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.CommunityHelpfulnessChoices
import com.anpfuel.domain.repository.CommunityHelpfulnessException
import com.anpfuel.domain.repository.CommunityHelpfulnessGateway
import com.anpfuel.domain.repository.CommunityHelpfulnessReceipt
import com.anpfuel.domain.repository.CommunityHelpfulnessRejectKind
import com.anpfuel.domain.repository.CommunityHelpfulnessTally
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * CS08A: helpful/unhelpful flows (B-BR-CS01, BUC-CS02).
 *
 * Disabled flag never touches the network; frozen choices only;
 * self-votes and stale revisions surface as rejection kinds (never as
 * truth claims); the same stable client submission id retries without
 * amplifying the vote; removals converge as idempotent success.
 */
class SubmitHelpfulnessVoteUseCaseTest {

    private val observationId = "11111111-1111-4111-8111-111111111111"

    private class FakeFlags(val enabled: Boolean) : CommunityHelpfulnessFlagProvider {
        override fun isEnabled(): Boolean = enabled
    }

    private class FakeGateway(
        var castResult: Result<CommunityHelpfulnessReceipt> =
            Result.success(CommunityHelpfulnessReceipt("vote-1", "obs", 1, 0, 1, 10000, "HELPFUL", false)),
        var removeResult: Result<CommunityHelpfulnessTally> =
            Result.success(CommunityHelpfulnessTally("obs", 2, 0, 0, 0, null, null)),
        var tallyResult: Result<CommunityHelpfulnessTally> =
            Result.success(CommunityHelpfulnessTally("obs", 2, 1, 0, 1, 10000, "HELPFUL")),
    ) : CommunityHelpfulnessGateway {
        data class CastCall(val observationId: String, val clientSubmissionId: String, val choice: String, val sequence: Long)

        val casts = mutableListOf<CastCall>()
        var removes = 0
        var tallies = 0

        override suspend fun castHelpfulness(
            observationId: String,
            clientSubmissionId: String,
            choiceWire: String,
            decisionSequence: Long,
        ): CommunityHelpfulnessReceipt {
            casts += CastCall(observationId, clientSubmissionId, choiceWire, decisionSequence)
            return castResult.getOrThrow()
        }

        override suspend fun removeHelpfulness(
            observationId: String,
            decisionSequence: Long,
        ): CommunityHelpfulnessTally {
            removes++
            return removeResult.getOrThrow()
        }

        override suspend fun tallyFor(
            observationId: String,
            decisionSequence: Long,
        ): CommunityHelpfulnessTally {
            tallies++
            return tallyResult.getOrThrow()
        }
    }

    private fun useCase(enabled: Boolean, gateway: FakeGateway) =
        SubmitHelpfulnessVoteUseCase(FakeFlags(enabled), gateway)

    @Test
    fun `disabled flag never calls gateway`() = runTest {
        val gateway = FakeGateway()
        val outcome = useCase(false, gateway).cast(
            SubmitHelpfulnessVoteUseCase.CastRequest(observationId, "sub-1", "HELPFUL", 2),
        )
        assertTrue(outcome is HelpfulnessVoteOutcome.Disabled)
        assertTrue(gateway.casts.isEmpty())
        assertEquals(0, gateway.removes)
        assertEquals(0, gateway.tallies)
    }

    @Test
    fun `cast helpful records with exact tally`() = runTest {
        val gateway = FakeGateway()
        val outcome = useCase(true, gateway).cast(
            SubmitHelpfulnessVoteUseCase.CastRequest(observationId, "sub-1", "HELPFUL", 2),
        )
        assertTrue(outcome is HelpfulnessVoteOutcome.Voted)
        val voted = outcome as HelpfulnessVoteOutcome.Voted
        assertEquals("vote-1", voted.receipt.voteId)
        assertEquals(1, voted.receipt.helpful)
        assertEquals("HELPFUL", voted.receipt.myChoice)
        assertEquals(
            FakeGateway.CastCall(observationId, "sub-1", "HELPFUL", 2),
            gateway.casts.single(),
        )
    }

    @Test
    fun `cast refuses blank fields unknown choice and bad sequence`() = runTest {
        val gateway = FakeGateway()
        val cases = listOf(
            SubmitHelpfulnessVoteUseCase.CastRequest("", "sub-1", "HELPFUL", 2),
            SubmitHelpfulnessVoteUseCase.CastRequest(observationId, "", "HELPFUL", 2),
            SubmitHelpfulnessVoteUseCase.CastRequest(observationId, "sub-1", "VALID", 2),
            SubmitHelpfulnessVoteUseCase.CastRequest(observationId, "sub-1", "helpful", 2),
            SubmitHelpfulnessVoteUseCase.CastRequest(observationId, "sub-1", "HELPFUL", 0),
        )
        for (request in cases) {
            assertThrows(DomainException::class.java) {
                kotlinx.coroutines.runBlocking {
                    useCase(true, gateway).cast(request)
                }
            }
        }
        assertTrue(gateway.casts.isEmpty())
        assertTrue(CommunityHelpfulnessChoices.ALL.containsAll(listOf("HELPFUL", "NOT_HELPFUL")))
    }

    @Test
    fun `self vote and stale revision map to rejection kinds`() = runTest {
        val gateway = FakeGateway()
        gateway.castResult = Result.failure(
            CommunityHelpfulnessException(CommunityHelpfulnessRejectKind.SELF_VOTE, "self"),
        )
        val self = useCase(true, gateway).cast(
            SubmitHelpfulnessVoteUseCase.CastRequest(observationId, "sub-1", "HELPFUL", 2),
        )
        assertTrue(self is HelpfulnessVoteOutcome.Rejected)
        assertEquals(CommunityHelpfulnessRejectKind.SELF_VOTE, (self as HelpfulnessVoteOutcome.Rejected).kind)

        gateway.castResult = Result.failure(
            CommunityHelpfulnessException(CommunityHelpfulnessRejectKind.STALE_REVISION, "stale"),
        )
        val stale = useCase(true, gateway).cast(
            SubmitHelpfulnessVoteUseCase.CastRequest(observationId, "sub-2", "HELPFUL", 1),
        )
        assertEquals(CommunityHelpfulnessRejectKind.STALE_REVISION, (stale as HelpfulnessVoteOutcome.Rejected).kind)
    }

    @Test
    fun `transport failure never claims a vote`() = runTest {
        val gateway = FakeGateway()
        gateway.castResult = Result.failure(
            CommunityHelpfulnessException(CommunityHelpfulnessRejectKind.TRANSPORT, "down"),
        )
        val outcome = useCase(true, gateway).cast(
            SubmitHelpfulnessVoteUseCase.CastRequest(observationId, "sub-1", "HELPFUL", 2),
        )
        assertEquals(CommunityHelpfulnessRejectKind.TRANSPORT, (outcome as HelpfulnessVoteOutcome.Rejected).kind)
    }

    @Test
    fun `remove converges and tally serves counts`() = runTest {
        val gateway = FakeGateway()
        val removed = useCase(true, gateway).remove(
            SubmitHelpfulnessVoteUseCase.RemoveRequest(observationId, 2),
        )
        assertTrue(removed is HelpfulnessVoteOutcome.Removed)
        assertEquals(1, gateway.removes)

        val tally = useCase(true, gateway).tally(
            SubmitHelpfulnessVoteUseCase.TallyRequest(observationId, 2),
        )
        assertTrue(tally is HelpfulnessVoteOutcome.Tallied)
        assertEquals(1, (tally as HelpfulnessVoteOutcome.Tallied).tally.helpful)
        assertEquals(1, gateway.tallies)
    }
}
