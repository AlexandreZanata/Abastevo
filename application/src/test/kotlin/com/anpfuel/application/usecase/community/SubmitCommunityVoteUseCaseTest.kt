package com.anpfuel.application.usecase.community

import com.anpfuel.application.port.CommunityVoteFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.CommunityDisputeReasons
import com.anpfuel.domain.repository.CommunityVoteException
import com.anpfuel.domain.repository.CommunityVoteGateway
import com.anpfuel.domain.repository.CommunityVoteReceipt
import com.anpfuel.domain.repository.CommunityVoteRejectKind
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T07: confirm/dispute flows (BUC-005, B-BR-006/007/011/012).
 *
 * Disabled flag never touches the network; self-confirm is denied;
 * the same stable client submission id retries without amplifying the
 * vote; a changed price references a distinct replacement observation
 * (the new observation itself travels via the contribution outbox);
 * private dispute detail never leaks into error messages.
 */
class SubmitCommunityVoteUseCaseTest {

    private val observationId = "11111111-1111-4111-8111-111111111111"
    private val replacementId = "22222222-2222-4222-8222-222222222222"

    private class FakeFlags(val enabled: Boolean) : CommunityVoteFlagProvider {
        override fun isEnabled(): Boolean = enabled
    }

    private class FakeGateway(
        var confirmResult: Result<CommunityVoteReceipt> =
            Result.success(CommunityVoteReceipt("vote-1", "obs", false)),
        var disputeResult: Result<CommunityVoteReceipt> =
            Result.success(CommunityVoteReceipt("dispute-1", "obs", false)),
    ) : CommunityVoteGateway {
        data class ConfirmCall(val observationId: String, val clientSubmissionId: String)
        data class DisputeCall(
            val targetObservationId: String,
            val clientSubmissionId: String,
            val reasonWire: String,
            val detail: String?,
            val replacementObservationId: String?,
        )

        val confirms = mutableListOf<ConfirmCall>()
        val disputes = mutableListOf<DisputeCall>()

        override suspend fun submitConfirmation(
            observationId: String,
            clientSubmissionId: String,
        ): CommunityVoteReceipt {
            confirms += ConfirmCall(observationId, clientSubmissionId)
            return confirmResult.getOrThrow()
        }

        override suspend fun submitDispute(
            targetObservationId: String,
            clientSubmissionId: String,
            reasonWire: String,
            detail: String?,
            replacementObservationId: String?,
        ): CommunityVoteReceipt {
            disputes += DisputeCall(
                targetObservationId,
                clientSubmissionId,
                reasonWire,
                detail,
                replacementObservationId,
            )
            return disputeResult.getOrThrow()
        }
    }

    private fun confirmRequest(
        submissionId: String = "cmd-confirm-1",
    ) = SubmitCommunityVoteUseCase.ConfirmRequest(
        observationId = observationId,
        clientSubmissionId = submissionId,
        shownConditionKind = "STANDARD",
        shownUnit = "BRL/L",
    )

    private fun disputeRequest(
        reason: String = CommunityDisputeReasons.WRONG_PRODUCT,
        detail: String? = null,
        replacement: String? = null,
    ) = SubmitCommunityVoteUseCase.DisputeRequest(
        targetObservationId = observationId,
        clientSubmissionId = "cmd-dispute-1",
        reasonWire = reason,
        detail = detail,
        replacementObservationId = replacement,
        shownConditionKind = "STANDARD",
    )

    @Test
    fun `disabled flag never touches gateway`() = runTest {
        val gateway = FakeGateway()
        val useCase = SubmitCommunityVoteUseCase(FakeFlags(false), gateway)

        val confirm = useCase.confirm(confirmRequest())
        val dispute = useCase.dispute(disputeRequest())

        assertTrue(confirm is CommunityVoteOutcome.Disabled)
        assertTrue(dispute is CommunityVoteOutcome.Disabled)
        assertTrue(gateway.confirms.isEmpty())
        assertTrue(gateway.disputes.isEmpty())
    }

    @Test
    fun `confirm success returns receipt`() = runTest {
        val gateway = FakeGateway(
            confirmResult = Result.success(
                CommunityVoteReceipt("vote-9", observationId, false),
            ),
        )
        val useCase = SubmitCommunityVoteUseCase(FakeFlags(true), gateway)

        val outcome = useCase.confirm(confirmRequest())

        assertTrue(outcome is CommunityVoteOutcome.Confirmed)
        val confirmed = outcome as CommunityVoteOutcome.Confirmed
        assertEquals("vote-9", confirmed.receipt.voteId)
        assertEquals(1, gateway.confirms.size)
        assertEquals("cmd-confirm-1", gateway.confirms.single().clientSubmissionId)
    }

    @Test
    fun `self-confirm denial maps to rejected without amplification`() = runTest {
        val gateway = FakeGateway(
            confirmResult = Result.failure(
                CommunityVoteException(
                    CommunityVoteRejectKind.SELF_CONFIRMATION,
                    "contributors cannot confirm their own observations",
                ),
            ),
        )
        val useCase = SubmitCommunityVoteUseCase(FakeFlags(true), gateway)

        val outcome = useCase.confirm(confirmRequest())

        assertTrue(outcome is CommunityVoteOutcome.Rejected)
        val rejected = outcome as CommunityVoteOutcome.Rejected
        assertEquals(CommunityVoteRejectKind.SELF_CONFIRMATION, rejected.kind)
        assertEquals(1, gateway.confirms.size)
    }

    @Test
    fun `identical retry keeps stable id and surfaces replayed`() = runTest {
        val gateway = FakeGateway(
            confirmResult = Result.success(
                CommunityVoteReceipt("vote-1", observationId, true),
            ),
        )
        val useCase = SubmitCommunityVoteUseCase(FakeFlags(true), gateway)

        val first = useCase.confirm(confirmRequest())
        val second = useCase.confirm(confirmRequest())

        assertTrue(first is CommunityVoteOutcome.Confirmed)
        assertTrue(second is CommunityVoteOutcome.Confirmed)
        assertEquals(2, gateway.confirms.size)
        assertEquals(
            gateway.confirms[0].clientSubmissionId,
            gateway.confirms[1].clientSubmissionId,
        )
        assertTrue((second as CommunityVoteOutcome.Confirmed).replayed)
    }

    @Test
    fun `dispute price-changed carries distinct replacement`() = runTest {
        val gateway = FakeGateway()
        val useCase = SubmitCommunityVoteUseCase(FakeFlags(true), gateway)

        val outcome = useCase.dispute(
            disputeRequest(
                reason = CommunityDisputeReasons.PRICE_CHANGED,
                replacement = replacementId,
            ),
        )

        assertTrue(outcome is CommunityVoteOutcome.Disputed)
        val call = gateway.disputes.single()
        assertEquals(CommunityDisputeReasons.PRICE_CHANGED, call.reasonWire)
        assertEquals(replacementId, call.replacementObservationId)
    }

    @Test
    fun `replacement equal to target fails fast`() = runTest {
        val gateway = FakeGateway()
        val useCase = SubmitCommunityVoteUseCase(FakeFlags(true), gateway)

        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking {
                useCase.dispute(
                    disputeRequest(
                        reason = CommunityDisputeReasons.PRICE_CHANGED,
                        replacement = observationId,
                    ),
                )
            }
        }
        assertTrue(gateway.disputes.isEmpty())
    }

    @Test
    fun `unknown reason and overlong detail fail fast and stay private`() = runTest {
        val gateway = FakeGateway(
            disputeResult = Result.failure(
                CommunityVoteException(
                    CommunityVoteRejectKind.INELIGIBLE_TARGET,
                    "target not eligible for this command",
                ),
            ),
        )
        val useCase = SubmitCommunityVoteUseCase(FakeFlags(true), gateway)
        val secretDetail = "secret-detail-abc-123"

        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.dispute(disputeRequest(reason = "MADE_UP")) }
        }

        val longOutcome = try {
            kotlinx.coroutines.runBlocking {
                useCase.dispute(disputeRequest(detail = "x".repeat(501)))
            }
            null
        } catch (_: DomainException) {
            "thrown"
        }
        assertEquals("thrown", longOutcome)

        val rejected = useCase.dispute(disputeRequest(detail = secretDetail))
        assertTrue(rejected is CommunityVoteOutcome.Rejected)
        val message = (rejected as CommunityVoteOutcome.Rejected).message
        assertTrue(!message.contains(secretDetail))
        assertTrue(gateway.disputes.size == 1)
    }

    @Test
    fun `blank ids fail fast`() = runTest {
        val useCase = SubmitCommunityVoteUseCase(FakeFlags(true), FakeGateway())

        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking {
                useCase.confirm(confirmRequest().copy(observationId = "  "))
            }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking {
                useCase.dispute(disputeRequest().copy(clientSubmissionId = ""))
            }
        }
    }
}
