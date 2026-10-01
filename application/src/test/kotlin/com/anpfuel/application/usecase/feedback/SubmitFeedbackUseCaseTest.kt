package com.anpfuel.application.usecase.feedback

import com.anpfuel.application.port.FeedbackFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.CommentWriteReceipt
import com.anpfuel.domain.repository.FeedbackCacheRepository
import com.anpfuel.domain.repository.FeedbackCommentView
import com.anpfuel.domain.repository.FeedbackException
import com.anpfuel.domain.repository.FeedbackGateway
import com.anpfuel.domain.repository.FeedbackOutboxPort
import com.anpfuel.domain.repository.FeedbackPage
import com.anpfuel.domain.repository.FeedbackRejectKind
import com.anpfuel.domain.repository.PendingFeedbackOp
import com.anpfuel.domain.repository.RatingStatsSnapshot
import com.anpfuel.domain.repository.RatingWriteReceipt
import com.anpfuel.domain.repository.VoteTallySnapshot
import com.anpfuel.domain.repository.VoteWriteReceipt
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P17-T01 RED: shared feedback writes (B-BR-F01…F08, BUC-F01…F04).
 *
 * Blank account never touches the network ([Outcome.LoginRequired]);
 * anonymous device proof alone cannot authorize (no such path exists).
 * Local contract violations throw [DomainException] fail-fast (blank
 * target, out-of-range stars, empty/over-280 text, bad vote choice,
 * blank/over-200 report reason). Server refusals surface as
 * [Outcome.Rejected] with the stable kind so callers roll back
 * optimistic state (not-author, stale-revision, self-vote, quota).
 * Transport failures enqueue a bounded outbox op and return
 * [Outcome.Queued] — never a fake success. No payment port exists:
 * any active account may write.
 */
class SubmitFeedbackUseCaseTest {

    private class FakeFlags(val enabled: Boolean) : FeedbackFlagProvider {
        override fun isEnabled(): Boolean = enabled
    }

    private class FakeGateway : FeedbackGateway {
        var rateResult: Result<RatingWriteReceipt> =
            Result.success(RatingWriteReceipt("r-1", true, RatingStatsSnapshot("s-1", "GASOLINE", 1, 5)))
        var commentResult: Result<CommentWriteReceipt> =
            Result.success(CommentWriteReceipt("c-1", 1))
        var voteResult: Result<VoteWriteReceipt> =
            Result.success(VoteWriteReceipt("v-1", true, VoteTallySnapshot("c-1", 1, 1, 0)))
        var calls = 0

        override suspend fun rate(accountId: String, stationId: String, product: String, stars: Int) =
            run { calls++; rateResult.getOrThrow() }

        override suspend fun deleteRating(accountId: String, stationId: String, product: String) {
            calls++
        }

        override suspend fun stats(stationId: String, product: String) =
            RatingStatsSnapshot(stationId, product, 0, 0)

        override suspend fun submitComment(accountId: String, stationId: String, product: String, text: String) =
            run { calls++; commentResult.getOrThrow() }

        override suspend fun reply(
            accountId: String,
            stationId: String,
            product: String,
            parentId: String,
            text: String,
        ) = run { calls++; commentResult.getOrThrow() }

        override suspend fun editComment(accountId: String, commentId: String, text: String, expectedRevision: Int) =
            run { calls++; commentResult.getOrThrow() }

        override suspend fun deleteComment(accountId: String, commentId: String) {
            calls++
        }

        override suspend fun listComments(stationId: String, product: String, cursor: String, limit: Int) =
            FeedbackPage(emptyList(), null)

        override suspend fun listReplies(parentId: String, cursor: String, limit: Int) =
            FeedbackPage(emptyList(), null)

        override suspend fun vote(accountId: String, commentId: String, choice: String) =
            run { calls++; voteResult.getOrThrow() }

        override suspend fun removeVote(accountId: String, commentId: String) {
            calls++
        }

        override suspend fun tally(commentId: String) = VoteTallySnapshot(commentId, 1, 0, 0)

        override suspend fun report(accountId: String, commentId: String, reason: String) {
            calls++
        }
    }

    private class FakeOutbox : FeedbackOutboxPort {
        val ops = mutableListOf<PendingFeedbackOp>()
        override fun enqueue(op: PendingFeedbackOp) {
            ops += op
        }

        override fun pending(): List<PendingFeedbackOp> = ops.toList()
    }

    private fun setup(enabled: Boolean = true) = Triple(FakeFlags(enabled), FakeGateway(), FakeOutbox())

    @Test
    fun `disabled flag never touches gateway`() = runTest {
        val (flags, gateway, outbox) = setup(enabled = false)
        val useCase = SubmitFeedbackUseCase(flags, gateway, outbox)
        val outcome = useCase.rate("acc-1", "station-1", "GASOLINE", 5, "op-1")
        assertTrue(outcome is FeedbackWriteOutcome.Disabled)
        assertEquals(0, gateway.calls)
        assertTrue(outbox.pending().isEmpty())
    }

    @Test
    fun `blank account requires login without network IO`() = runTest {
        val (flags, gateway, outbox) = setup()
        val useCase = SubmitFeedbackUseCase(flags, gateway, outbox)
        assertTrue(useCase.rate("  ", "station-1", "GASOLINE", 5, "op-1") is FeedbackWriteOutcome.LoginRequired)
        assertTrue(useCase.comment("  ", "station-1", "GASOLINE", "good fuel", "op-2") is FeedbackWriteOutcome.LoginRequired)
        assertTrue(useCase.vote("", "c-1", "VALID", "op-3") is FeedbackWriteOutcome.LoginRequired)
        assertTrue(useCase.report("  ", "c-1", "spam", "op-4") is FeedbackWriteOutcome.LoginRequired)
        assertEquals(0, gateway.calls)
    }

    @Test
    fun `local contract violations fail fast`() = runTest {
        val (flags, gateway, outbox) = setup()
        val useCase = SubmitFeedbackUseCase(flags, gateway, outbox)
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.rate("acc-1", "  ", "GASOLINE", 5, "op-1") }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.rate("acc-1", "s-1", "GASOLINE", 0, "op-1") }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.rate("acc-1", "s-1", "GASOLINE", 6, "op-1") }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.comment("acc-1", "s-1", "GASOLINE", "   ", "op-1") }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.comment("acc-1", "s-1", "GASOLINE", "x".repeat(281), "op-1") }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.vote("acc-1", "c-1", "MAYBE", "op-1") }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.report("acc-1", "c-1", "   ", "op-1") }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.report("acc-1", "c-1", "r".repeat(201), "op-1") }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.edit("acc-1", "c-1", "ok", 0, "op-1") }
        }
        assertEquals(0, gateway.calls)
    }

    @Test
    fun `exact 280 scalars accepted, astral emoji counts one`() = runTest {
        val (flags, gateway, outbox) = setup()
        val useCase = SubmitFeedbackUseCase(flags, gateway, outbox)
        val text280 = "é".repeat(279) + "\uD83D\uDE00"
        val outcome = useCase.comment("acc-1", "s-1", "GASOLINE", text280, "op-1")
        assertTrue(outcome is FeedbackWriteOutcome.Commented)
        assertEquals(1, gateway.calls)
    }

    @Test
    fun `server refusals map to rejected kinds for rollback`() = runTest {
        val (flags, gateway, outbox) = setup()
        val useCase = SubmitFeedbackUseCase(flags, gateway, outbox)
        gateway.commentResult = Result.failure(FeedbackException(FeedbackRejectKind.STALE_REVISION, "stale"))
        val stale = useCase.edit("acc-1", "c-1", "new text", 2, "op-1")
        assertTrue(stale is FeedbackWriteOutcome.Rejected)
        assertEquals(FeedbackRejectKind.STALE_REVISION, (stale as FeedbackWriteOutcome.Rejected).kind)

        gateway.commentResult = Result.failure(FeedbackException(FeedbackRejectKind.NOT_AUTHOR, "foreign"))
        val foreign = useCase.edit("acc-1", "c-1", "new text", 2, "op-2")
        assertEquals(FeedbackRejectKind.NOT_AUTHOR, (foreign as FeedbackWriteOutcome.Rejected).kind)

        gateway.voteResult = Result.failure(FeedbackException(FeedbackRejectKind.SELF_VOTE, "own comment"))
        val self = useCase.vote("acc-1", "c-1", "VALID", "op-3")
        assertEquals(FeedbackRejectKind.SELF_VOTE, (self as FeedbackWriteOutcome.Rejected).kind)
    }

    @Test
    fun `transport failure queues op instead of fake success`() = runTest {
        val (flags, gateway, outbox) = setup()
        val useCase = SubmitFeedbackUseCase(flags, gateway, outbox)
        gateway.rateResult = Result.failure(RuntimeException("offline"))
        val outcome = useCase.rate("acc-1", "s-1", "GASOLINE", 4, "op-rate-1")
        assertTrue(outcome is FeedbackWriteOutcome.Queued)
        assertEquals(1, outbox.pending().size)
        assertEquals("op-rate-1", outbox.pending().single().opId)
    }

    @Test
    fun `write outcomes carry server counts for reconciliation`() = runTest {
        val (flags, gateway, outbox) = setup()
        val useCase = SubmitFeedbackUseCase(flags, gateway, outbox)
        val rated = useCase.rate("acc-1", "s-1", "GASOLINE", 5, "op-1") as FeedbackWriteOutcome.Rated
        assertEquals(1L, rated.stats.count)
        assertEquals(5L, rated.stats.sum)
        val voted = useCase.vote("acc-1", "c-1", "VALID", "op-2") as FeedbackWriteOutcome.Voted
        assertEquals(1L, voted.tally.valid)
        assertEquals(0L, voted.tally.invalid)
    }

    @Test
    fun `unused cache port compiles for read path`() {
        val cache: FeedbackCacheRepository? = null
        assertEquals(null, cache)
        val view = FeedbackCommentView("c-1", "alias-1", "s-1", "GASOLINE", "", 0, "text", 1, 1L, 1L)
        assertEquals(0, view.depth)
    }
}
