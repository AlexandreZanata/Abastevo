package com.anpfuel.domain.repository

/**
 * P17-T01 — Shared feedback ports (B-BR-F01…F08).
 *
 * Mirrors the frozen backend contracts
 * (`backend/internal/modules/feedback/domain` + `application`):
 * integer 1–5 stars with one current rating per account/target,
 * 280-scalar comment/reply text, one-level replies, VALID/INVALID
 * votes scoped to the comment revision, and moderator visibility.
 * The server enforces every bound; client validation is fail-fast
 * UX, never proof. Counts (never bare percentages) travel with
 * every write outcome so state reconciles against the backend.
 * Payment never appears here: any active account may write.
 */

/** Rebuildable rating aggregate for one station/product. */
data class RatingStatsSnapshot(
    val stationId: String,
    val product: String,
    val count: Long,
    val sum: Long,
)

/** Exact per-revision vote count snapshot for one comment. */
data class VoteTallySnapshot(
    val commentId: String,
    val revision: Int,
    val valid: Long,
    val invalid: Long,
)

/** Public read shape: opaque alias, never emails, GPS or media URLs. */
data class FeedbackCommentView(
    val id: String,
    val alias: String,
    val stationId: String,
    val product: String,
    val parentId: String,
    val depth: Int,
    val text: String,
    val revision: Int,
    val createdAt: Long,
    val updatedAt: Long,
)

/** One bounded keyset page; null [nextCursor] ends pagination. */
data class FeedbackPage(
    val items: List<FeedbackCommentView>,
    val nextCursor: String?,
)

/** Server-confirmed rating write with refreshed aggregate. */
data class RatingWriteReceipt(
    val ratingId: String,
    val created: Boolean,
    val stats: RatingStatsSnapshot,
)

/** Server-confirmed comment/reply write at its new revision. */
data class CommentWriteReceipt(
    val commentId: String,
    val revision: Int,
)

/** Server-confirmed vote with refreshed revision tally. */
data class VoteWriteReceipt(
    val voteId: String,
    val created: Boolean,
    val tally: VoteTallySnapshot,
)

/** One queued write for bounded outbox retry (never a fake success). */
data class PendingFeedbackOp(
    val opId: String,
    val kind: String,
    val payload: String,
)

/** Stable server refusal codes (mirrors `VerdictCode` on the backend). */
enum class FeedbackRejectKind {
    TARGET_INVALID,
    RATING_OUT_OF_RANGE,
    TEXT_EMPTY,
    TEXT_TOO_LONG,
    NOT_AUTHOR,
    STALE_REVISION,
    COMMENT_NOT_FOUND,
    RATING_NOT_FOUND,
    SELF_VOTE,
    VOTE_CHOICE_INVALID,
    REPORT_INVALID,
    QUOTA_EXCEEDED,
    TRANSPORT,
    GATE_REQUIRED,
}

/** Typed server refusal so callers roll back optimistic state by kind. */
class FeedbackException(
    val kind: FeedbackRejectKind,
    message: String?,
) : Exception(message)

/** Network + persistence boundary for feedback reads and writes. */
interface FeedbackGateway {
    suspend fun rate(accountId: String, stationId: String, product: String, stars: Int): RatingWriteReceipt
    suspend fun deleteRating(accountId: String, stationId: String, product: String)
    suspend fun stats(stationId: String, product: String): RatingStatsSnapshot
    suspend fun submitComment(accountId: String, stationId: String, product: String, text: String): CommentWriteReceipt
    suspend fun reply(
        accountId: String,
        stationId: String,
        product: String,
        parentId: String,
        text: String,
    ): CommentWriteReceipt
    suspend fun editComment(
        accountId: String,
        commentId: String,
        text: String,
        expectedRevision: Int,
    ): CommentWriteReceipt
    suspend fun deleteComment(accountId: String, commentId: String)
    suspend fun listComments(stationId: String, product: String, cursor: String, limit: Int): FeedbackPage
    suspend fun listReplies(parentId: String, cursor: String, limit: Int): FeedbackPage
    suspend fun vote(accountId: String, commentId: String, choice: String): VoteWriteReceipt
    suspend fun removeVote(accountId: String, commentId: String)
    suspend fun tally(commentId: String): VoteTallySnapshot
    suspend fun report(accountId: String, commentId: String, reason: String)
}

/** First-page cache for offline read fallback (never presented as fresh). */
interface FeedbackCacheRepository {
    fun loadComments(stationId: String, product: String): FeedbackPage?
    fun saveComments(stationId: String, product: String, page: FeedbackPage)
}

/** Bounded outbox for transport-failed writes. */
interface FeedbackOutboxPort {
    fun enqueue(op: PendingFeedbackOp)
    fun pending(): List<PendingFeedbackOp>
}
