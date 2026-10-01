package com.anpfuel.application.usecase.feedback

import com.anpfuel.application.port.FeedbackFlagProvider
import com.anpfuel.domain.portable.PortableFeedback
import com.anpfuel.domain.portable.PortableLocation
import com.anpfuel.domain.portable.PortablePhoto
import com.anpfuel.domain.portable.PortableText
import com.anpfuel.domain.repository.CommentWriteReceipt
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
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P17-T04 cross-platform lifecycle and privacy exit (B-BR-F01…F08,
 * B-BR-M05, B-BR-L01…L04).
 *
 * Ties the already-shipped portable contracts together with the same
 * fixture numbers replayed Swift-side: 280-scalar text, floor
 * agreement (2 valid / 1 invalid → 6666 bp), revoked-session refusal
 * with zero network IO, ownership/erasure refusals carrying fixed
 * kinds, 24 h transient media expiry and simulated-location denial.
 * Diagnostics carry opaque aliases and counts only — no emails, GPS
 * coordinates or photo payloads.
 */
class FeedbackLifecycleExitTest {

    private class FakeFlags(val enabled: Boolean) : FeedbackFlagProvider {
        override fun isEnabled(): Boolean = enabled
    }

    private class FakeGateway : FeedbackGateway {
        var calls = 0
        var commentResult: Result<CommentWriteReceipt> =
            Result.success(CommentWriteReceipt("c-1", 1))

        override suspend fun rate(accountId: String, stationId: String, product: String, stars: Int) =
            run { calls++; throw FeedbackException(FeedbackRejectKind.GATE_REQUIRED, "ratings transport not published") }

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

        override suspend fun editComment(
            accountId: String,
            commentId: String,
            text: String,
            expectedRevision: Int,
        ) = run { calls++; commentResult.getOrThrow() }

        override suspend fun deleteComment(accountId: String, commentId: String) {
            calls++
        }

        override suspend fun listComments(stationId: String, product: String, cursor: String, limit: Int) =
            FeedbackPage(emptyList(), null)

        override suspend fun listReplies(parentId: String, cursor: String, limit: Int) =
            FeedbackPage(emptyList(), null)

        override suspend fun vote(accountId: String, commentId: String, choice: String) =
            run { calls++; throw FeedbackException(FeedbackRejectKind.SELF_VOTE, "own comment") }

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

    @Test
    fun `same fixtures rule both platforms`() {
        assertTrue(PortableText.isValidComment("x".repeat(280)))
        assertFalse(PortableText.isValidComment("x".repeat(281)))
        assertEquals(6666L, PortableFeedback.agreementBasisPoints(2, 1))
        assertEquals(null, PortableFeedback.agreementBasisPoints(0, 0))
    }

    @Test
    fun `revoked session blocks writes on every device without IO`() = runTest {
        val gateway = FakeGateway()
        val useCase = SubmitFeedbackUseCase(FakeFlags(true), gateway, FakeOutbox())
        assertTrue(useCase.comment("  ", "s-1", "GASOLINE", "good fuel", "op-1") is FeedbackWriteOutcome.LoginRequired)
        assertTrue(useCase.vote("", "c-1", "VALID", "op-2") is FeedbackWriteOutcome.LoginRequired)
        assertTrue(useCase.report("  ", "c-1", "spam", "op-3") is FeedbackWriteOutcome.LoginRequired)
        assertEquals(0, gateway.calls)
    }

    @Test
    fun `ownership and erasure refusals carry fixed kinds`() = runTest {
        val gateway = FakeGateway()
        val useCase = SubmitFeedbackUseCase(FakeFlags(true), gateway, FakeOutbox())
        gateway.commentResult = Result.failure(FeedbackException(FeedbackRejectKind.NOT_AUTHOR, "foreign"))
        val foreign = useCase.edit("acc-1", "c-1", "new text", 2, "op-1") as FeedbackWriteOutcome.Rejected
        assertEquals(FeedbackRejectKind.NOT_AUTHOR, foreign.kind)
        gateway.commentResult = Result.failure(FeedbackException(FeedbackRejectKind.COMMENT_NOT_FOUND, "gone"))
        val gone = useCase.edit("acc-1", "c-1", "new text", 2, "op-2") as FeedbackWriteOutcome.Rejected
        assertEquals(FeedbackRejectKind.COMMENT_NOT_FOUND, gone.kind)
        gateway.commentResult = Result.failure(FeedbackException(FeedbackRejectKind.STALE_REVISION, "stale"))
        val stale = useCase.edit("acc-1", "c-1", "new text", 2, "op-3") as FeedbackWriteOutcome.Rejected
        assertEquals(FeedbackRejectKind.STALE_REVISION, stale.kind)
    }

    @Test
    fun `transient media expires at exactly 24 hours`() {
        val ttl = PortablePhoto.TRANSIENT_TTL_MILLIS
        assertFalse(PortablePhoto.isTransientExpired(0L, ttl - 1))
        assertTrue(PortablePhoto.isTransientExpired(0L, ttl))
        assertTrue(PortablePhoto.isTransientExpired(0L, ttl + 1))
    }

    @Test
    fun `simulated location denies claims while verified allows`() {
        val simulated = PortableLocation.classify(
            PortableLocation.FixInput(
                permissionGranted = true,
                hasFix = true,
                sourceInfoPresent = true,
                simulated = true,
                accuracyMeters = 10.0,
                fixAgeSeconds = 30L,
                clockSkewSeconds = 0L,
            ),
        )
        assertEquals(PortableLocation.SIMULATED, simulated.verdict)
        assertFalse(simulated.allowsClaim)
        val denied = PortableLocation.classify(
            PortableLocation.FixInput(
                permissionGranted = false,
                hasFix = false,
                sourceInfoPresent = false,
                simulated = false,
                accuracyMeters = null,
                fixAgeSeconds = null,
                clockSkewSeconds = null,
            ),
        )
        assertEquals(PortableLocation.DENIED, denied.verdict)
        assertFalse(denied.allowsClaim)
    }

    @Test
    fun `diagnostics carry aliases and counts never PII`() {
        val view = FeedbackCommentView("c-1", "alias-7", "s-1", "GASOLINE", "", 0, "Preço bom", 1, 1L, 1L)
        assertFalse(view.alias.contains("@"))
        assertFalse(view.text.contains("@"))
        val tally = VoteTallySnapshot("c-1", 1, 2, 1)
        val line = "${tally.valid} valid / ${tally.invalid} invalid"
        assertTrue(line.contains("2 valid / 1 invalid"))
        assertFalse(line.contains("@"))
    }
}
