package com.anpfuel.application.usecase.feedback

import com.anpfuel.application.port.FeedbackFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.CommentWriteReceipt
import com.anpfuel.domain.repository.FeedbackCacheRepository
import com.anpfuel.domain.repository.FeedbackCommentView
import com.anpfuel.domain.repository.FeedbackException
import com.anpfuel.domain.repository.FeedbackGateway
import com.anpfuel.domain.repository.FeedbackPage
import com.anpfuel.domain.repository.FeedbackRejectKind
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
 * P17-T01 RED: shared feedback reads (B-BR-F06/F07, BUC-F05).
 *
 * Public reads need no login: anonymous browsing stays available.
 * Fresh gateway pages are cached; transport falls back to the cached
 * first page ([Outcome.StaleCache] explicit, never fresh); no cache
 * means [Outcome.Unavailable]. Limits clamp to 1…100 (default 20),
 * mirroring the backend. Corrupt cursors fail closed as unavailable,
 * never as a crash.
 */
class GetFeedbackPageUseCaseTest {

    private class FakeFlags(val enabled: Boolean) : FeedbackFlagProvider {
        override fun isEnabled(): Boolean = enabled
    }

    private class FakeGateway(
        var page: Result<FeedbackPage> = Result.success(
            FeedbackPage(
                listOf(FeedbackCommentView("c-1", "alias-1", "s-1", "GASOLINE", "", 0, "good", 1, 10L, 10L)),
                "next-1",
            ),
        ),
    ) : FeedbackGateway {
        var calls = 0

        override suspend fun rate(
            accountId: String,
            stationId: String,
            product: String,
            stars: Int,
        ): RatingWriteReceipt = throw UnsupportedOperationException()

        override suspend fun deleteRating(accountId: String, stationId: String, product: String) {
            throw UnsupportedOperationException()
        }

        override suspend fun stats(stationId: String, product: String): RatingStatsSnapshot =
            throw UnsupportedOperationException()

        override suspend fun submitComment(
            accountId: String,
            stationId: String,
            product: String,
            text: String,
        ): CommentWriteReceipt = throw UnsupportedOperationException()

        override suspend fun reply(
            accountId: String,
            stationId: String,
            product: String,
            parentId: String,
            text: String,
        ): CommentWriteReceipt = throw UnsupportedOperationException()

        override suspend fun editComment(
            accountId: String,
            commentId: String,
            text: String,
            expectedRevision: Int,
        ): CommentWriteReceipt = throw UnsupportedOperationException()

        override suspend fun deleteComment(accountId: String, commentId: String) {
            throw UnsupportedOperationException()
        }
        override suspend fun listComments(stationId: String, product: String, cursor: String, limit: Int): FeedbackPage {
            calls++
            return page.getOrThrow()
        }

        override suspend fun listReplies(parentId: String, cursor: String, limit: Int): FeedbackPage {
            calls++
            return page.getOrThrow()
        }

        override suspend fun vote(accountId: String, commentId: String, choice: String): VoteWriteReceipt =
            throw UnsupportedOperationException()

        override suspend fun removeVote(accountId: String, commentId: String) {
            throw UnsupportedOperationException()
        }

        override suspend fun tally(commentId: String): VoteTallySnapshot =
            throw UnsupportedOperationException()

        override suspend fun report(accountId: String, commentId: String, reason: String) {
            throw UnsupportedOperationException()
        }
    }

    private class FakeCache(var stored: FeedbackPage? = null) : FeedbackCacheRepository {
        override fun loadComments(stationId: String, product: String): FeedbackPage? = stored

        override fun saveComments(stationId: String, product: String, page: FeedbackPage) {
            stored = page
        }
    }

    @Test
    fun `disabled flag serves nothing`() = runTest {
        val gateway = FakeGateway()
        val useCase = GetFeedbackPageUseCase(FakeFlags(false), gateway, FakeCache())
        assertTrue(useCase.comments("s-1", "GASOLINE", "", 20) is FeedbackPageOutcome.Disabled)
        assertEquals(0, gateway.calls)
    }

    @Test
    fun `fresh page caches without login`() = runTest {
        val gateway = FakeGateway()
        val cache = FakeCache()
        val useCase = GetFeedbackPageUseCase(FakeFlags(true), gateway, cache)
        val outcome = useCase.comments("s-1", "GASOLINE", "", 20)
        assertTrue(outcome is FeedbackPageOutcome.Fresh)
        assertEquals(1, (outcome as FeedbackPageOutcome.Fresh).page.items.size)
        assertEquals(1, cache.stored?.items?.size)
    }

    @Test
    fun `offline falls back to cache, else unavailable`() = runTest {
        val gateway = FakeGateway(page = Result.failure(RuntimeException("offline")))
        val cached = FeedbackPage(
            listOf(FeedbackCommentView("c-0", "alias-0", "s-1", "GASOLINE", "", 0, "cached", 1, 5L, 5L)),
            null,
        )
        val withCache = GetFeedbackPageUseCase(FakeFlags(true), gateway, FakeCache(cached))
        val stale = withCache.comments("s-1", "GASOLINE", "", 20)
        assertTrue(stale is FeedbackPageOutcome.StaleCache)

        val withoutCache = GetFeedbackPageUseCase(FakeFlags(true), gateway, FakeCache(null))
        val empty = withoutCache.comments("s-1", "GASOLINE", "", 20)
        assertTrue(empty is FeedbackPageOutcome.Unavailable)
    }

    @Test
    fun `blank target fails fast`() = runTest {
        val useCase = GetFeedbackPageUseCase(FakeFlags(true), FakeGateway(), FakeCache())
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.comments("  ", "GASOLINE", "", 20) }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase.replies("  ", "", 20) }
        }
    }

    @Test
    fun `corrupt cursor fails closed as unavailable`() = runTest {
        val gateway = FakeGateway(
            page = Result.failure(FeedbackException(FeedbackRejectKind.TARGET_INVALID, "bad cursor")),
        )
        val useCase = GetFeedbackPageUseCase(FakeFlags(true), gateway, FakeCache(null))
        val outcome = useCase.comments("s-1", "GASOLINE", "!!!not-a-cursor!!!", 20)
        assertTrue(outcome is FeedbackPageOutcome.Unavailable)
    }
}
