package com.anpfuel.application.usecase.feedback

import com.anpfuel.application.port.FeedbackFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.portable.PortableFeedback
import com.anpfuel.domain.portable.PortableText
import com.anpfuel.domain.repository.CommentWriteReceipt
import com.anpfuel.domain.repository.FeedbackException
import com.anpfuel.domain.repository.FeedbackGateway
import com.anpfuel.domain.repository.FeedbackOutboxPort
import com.anpfuel.domain.repository.FeedbackRejectKind
import com.anpfuel.domain.repository.PendingFeedbackOp
import com.anpfuel.domain.repository.RatingStatsSnapshot
import com.anpfuel.domain.repository.RatingWriteReceipt
import com.anpfuel.domain.repository.VoteTallySnapshot
import com.anpfuel.domain.repository.VoteWriteReceipt

/**
 * P17-T01 — Shared feedback writes (B-BR-F01…F08, BUC-F01…F04).
 *
 * Flag disabled: [FeedbackWriteOutcome.Disabled], no network IO.
 * Blank account: [FeedbackWriteOutcome.LoginRequired], no network IO —
 * social writes require an active account session (F01/F08);
 * anonymous device proof alone never authorizes.
 *
 * Local contract violations throw [DomainException] fail-fast (blank
 * target, out-of-range stars, empty/over-280 text, bad vote choice,
 * blank/over-200 report reason, non-positive revision). Server
 * refusals surface as [FeedbackWriteOutcome.Rejected] with the stable
 * kind so callers roll back optimistic state (not-author,
 * stale-revision, self-vote, quota). Transport failures enqueue a
 * bounded outbox op and return [FeedbackWriteOutcome.Queued] — never
 * a fake success. No payment check exists on any path: free accounts
 * participate fully and paid plans never gate feedback.
 */
sealed interface FeedbackWriteOutcome {
    data object Disabled : FeedbackWriteOutcome
    data object LoginRequired : FeedbackWriteOutcome
    data class Rated(val receipt: RatingWriteReceipt, val stats: RatingStatsSnapshot) : FeedbackWriteOutcome
    data object RatingDeleted : FeedbackWriteOutcome
    data class Commented(val receipt: CommentWriteReceipt) : FeedbackWriteOutcome
    data class Replied(val receipt: CommentWriteReceipt) : FeedbackWriteOutcome
    data class Edited(val receipt: CommentWriteReceipt) : FeedbackWriteOutcome
    data object CommentDeleted : FeedbackWriteOutcome
    data class Voted(val receipt: VoteWriteReceipt, val tally: VoteTallySnapshot) : FeedbackWriteOutcome
    data object VoteRemoved : FeedbackWriteOutcome
    data object Reported : FeedbackWriteOutcome
    data class Queued(val op: PendingFeedbackOp) : FeedbackWriteOutcome
    data class Rejected(val kind: FeedbackRejectKind, val message: String) : FeedbackWriteOutcome
}

class SubmitFeedbackUseCase(
    private val flagProvider: FeedbackFlagProvider,
    private val gateway: FeedbackGateway,
    private val outbox: FeedbackOutboxPort,
) {
    suspend fun rate(
        accountId: String,
        stationId: String,
        product: String,
        stars: Int,
        opId: String,
    ): FeedbackWriteOutcome {
        if (!flagProvider.isEnabled()) return FeedbackWriteOutcome.Disabled
        if (accountId.isBlank()) return FeedbackWriteOutcome.LoginRequired
        requireTarget(stationId, product)
        if (!PortableFeedback.isValidRating(stars)) throw DomainException("stars outside 1-5")
        requireOpId(opId)
        return try {
            val receipt = gateway.rate(accountId.trim(), stationId.trim(), product.trim(), stars)
            FeedbackWriteOutcome.Rated(receipt, receipt.stats)
        } catch (refused: FeedbackException) {
            FeedbackWriteOutcome.Rejected(refused.kind, refused.message ?: refused.kind.name)
        } catch (transport: Exception) {
            queue(opId, OP_RATE, "$stationId|$product|$stars")
        }
    }

    suspend fun deleteRating(
        accountId: String,
        stationId: String,
        product: String,
        opId: String,
    ): FeedbackWriteOutcome {
        if (!flagProvider.isEnabled()) return FeedbackWriteOutcome.Disabled
        if (accountId.isBlank()) return FeedbackWriteOutcome.LoginRequired
        requireTarget(stationId, product)
        requireOpId(opId)
        return try {
            gateway.deleteRating(accountId.trim(), stationId.trim(), product.trim())
            FeedbackWriteOutcome.RatingDeleted
        } catch (refused: FeedbackException) {
            FeedbackWriteOutcome.Rejected(refused.kind, refused.message ?: refused.kind.name)
        } catch (transport: Exception) {
            queue(opId, OP_DELETE_RATING, "$stationId|$product")
        }
    }

    suspend fun comment(
        accountId: String,
        stationId: String,
        product: String,
        text: String,
        opId: String,
    ): FeedbackWriteOutcome {
        if (!flagProvider.isEnabled()) return FeedbackWriteOutcome.Disabled
        if (accountId.isBlank()) return FeedbackWriteOutcome.LoginRequired
        requireTarget(stationId, product)
        val clean = requireCommentText(text)
        requireOpId(opId)
        return try {
            val receipt = gateway.submitComment(accountId.trim(), stationId.trim(), product.trim(), clean)
            FeedbackWriteOutcome.Commented(receipt)
        } catch (refused: FeedbackException) {
            FeedbackWriteOutcome.Rejected(refused.kind, refused.message ?: refused.kind.name)
        } catch (transport: Exception) {
            queue(opId, OP_COMMENT, "$stationId|$product")
        }
    }

    suspend fun reply(
        accountId: String,
        stationId: String,
        product: String,
        parentId: String,
        text: String,
        opId: String,
    ): FeedbackWriteOutcome {
        if (!flagProvider.isEnabled()) return FeedbackWriteOutcome.Disabled
        if (accountId.isBlank()) return FeedbackWriteOutcome.LoginRequired
        requireTarget(stationId, product)
        if (parentId.isBlank()) throw DomainException("parent_id is blank")
        val clean = requireCommentText(text)
        requireOpId(opId)
        return try {
            val receipt = gateway.reply(accountId.trim(), stationId.trim(), product.trim(), parentId.trim(), clean)
            FeedbackWriteOutcome.Replied(receipt)
        } catch (refused: FeedbackException) {
            FeedbackWriteOutcome.Rejected(refused.kind, refused.message ?: refused.kind.name)
        } catch (transport: Exception) {
            queue(opId, OP_REPLY, "$stationId|$product|$parentId")
        }
    }

    suspend fun edit(
        accountId: String,
        commentId: String,
        text: String,
        expectedRevision: Int,
        opId: String,
    ): FeedbackWriteOutcome {
        if (!flagProvider.isEnabled()) return FeedbackWriteOutcome.Disabled
        if (accountId.isBlank()) return FeedbackWriteOutcome.LoginRequired
        if (commentId.isBlank()) throw DomainException("comment_id is blank")
        if (expectedRevision <= 0) throw DomainException("expected revision must be positive")
        val clean = requireCommentText(text)
        requireOpId(opId)
        return try {
            val receipt = gateway.editComment(accountId.trim(), commentId.trim(), clean, expectedRevision)
            FeedbackWriteOutcome.Edited(receipt)
        } catch (refused: FeedbackException) {
            FeedbackWriteOutcome.Rejected(refused.kind, refused.message ?: refused.kind.name)
        } catch (transport: Exception) {
            queue(opId, OP_EDIT, "$commentId|$expectedRevision")
        }
    }

    suspend fun deleteComment(
        accountId: String,
        commentId: String,
        opId: String,
    ): FeedbackWriteOutcome {
        if (!flagProvider.isEnabled()) return FeedbackWriteOutcome.Disabled
        if (accountId.isBlank()) return FeedbackWriteOutcome.LoginRequired
        if (commentId.isBlank()) throw DomainException("comment_id is blank")
        requireOpId(opId)
        return try {
            gateway.deleteComment(accountId.trim(), commentId.trim())
            FeedbackWriteOutcome.CommentDeleted
        } catch (refused: FeedbackException) {
            FeedbackWriteOutcome.Rejected(refused.kind, refused.message ?: refused.kind.name)
        } catch (transport: Exception) {
            queue(opId, OP_DELETE_COMMENT, commentId)
        }
    }

    suspend fun vote(
        accountId: String,
        commentId: String,
        choice: String,
        opId: String,
    ): FeedbackWriteOutcome {
        if (!flagProvider.isEnabled()) return FeedbackWriteOutcome.Disabled
        if (accountId.isBlank()) return FeedbackWriteOutcome.LoginRequired
        if (commentId.isBlank()) throw DomainException("comment_id is blank")
        if (choice != VOTE_VALID && choice != VOTE_INVALID) throw DomainException("vote must be VALID or INVALID")
        requireOpId(opId)
        return try {
            val receipt = gateway.vote(accountId.trim(), commentId.trim(), choice)
            FeedbackWriteOutcome.Voted(receipt, receipt.tally)
        } catch (refused: FeedbackException) {
            FeedbackWriteOutcome.Rejected(refused.kind, refused.message ?: refused.kind.name)
        } catch (transport: Exception) {
            queue(opId, OP_VOTE, "$commentId|$choice")
        }
    }

    suspend fun removeVote(
        accountId: String,
        commentId: String,
        opId: String,
    ): FeedbackWriteOutcome {
        if (!flagProvider.isEnabled()) return FeedbackWriteOutcome.Disabled
        if (accountId.isBlank()) return FeedbackWriteOutcome.LoginRequired
        if (commentId.isBlank()) throw DomainException("comment_id is blank")
        requireOpId(opId)
        return try {
            gateway.removeVote(accountId.trim(), commentId.trim())
            FeedbackWriteOutcome.VoteRemoved
        } catch (refused: FeedbackException) {
            FeedbackWriteOutcome.Rejected(refused.kind, refused.message ?: refused.kind.name)
        } catch (transport: Exception) {
            queue(opId, OP_REMOVE_VOTE, commentId)
        }
    }

    suspend fun report(
        accountId: String,
        commentId: String,
        reason: String,
        opId: String,
    ): FeedbackWriteOutcome {
        if (!flagProvider.isEnabled()) return FeedbackWriteOutcome.Disabled
        if (accountId.isBlank()) return FeedbackWriteOutcome.LoginRequired
        if (commentId.isBlank()) throw DomainException("comment_id is blank")
        val clean = reason.trim()
        if (clean.isEmpty()) throw DomainException("report reason is required")
        if (clean.codePointCount(0, clean.length) > MAX_REPORT_REASON_SCALARS) {
            throw DomainException("report reason exceeds $MAX_REPORT_REASON_SCALARS chars")
        }
        requireOpId(opId)
        return try {
            gateway.report(accountId.trim(), commentId.trim(), clean)
            FeedbackWriteOutcome.Reported
        } catch (refused: FeedbackException) {
            FeedbackWriteOutcome.Rejected(refused.kind, refused.message ?: refused.kind.name)
        } catch (transport: Exception) {
            queue(opId, OP_REPORT, commentId)
        }
    }

    private fun queue(opId: String, kind: String, payload: String): FeedbackWriteOutcome {
        val op = PendingFeedbackOp(opId, kind, payload)
        outbox.enqueue(op)
        return FeedbackWriteOutcome.Queued(op)
    }

    private fun requireTarget(stationId: String, product: String) {
        if (stationId.isBlank() || product.isBlank()) throw DomainException("station/product target is blank")
    }

    private fun requireCommentText(text: String): String {
        val normalized = PortableText.normalize(text)
        if (normalized.isEmpty()) throw DomainException("comment text is empty")
        if (PortableText.countScalars(normalized) > PortableText.MAX_COMMENT_SCALARS) {
            throw DomainException("comment text exceeds ${PortableText.MAX_COMMENT_SCALARS} scalars")
        }
        return normalized
    }

    private fun requireOpId(opId: String) {
        if (opId.isBlank()) throw DomainException("op_id is blank")
    }

    companion object {
        const val VOTE_VALID = "VALID"
        const val VOTE_INVALID = "INVALID"

        /** Backend report reason bound (moderation.go `ReportMaxReasonChars`). */
        const val MAX_REPORT_REASON_SCALARS = 200

        const val OP_RATE = "rate"
        const val OP_DELETE_RATING = "delete-rating"
        const val OP_COMMENT = "comment"
        const val OP_REPLY = "reply"
        const val OP_EDIT = "edit"
        const val OP_DELETE_COMMENT = "delete-comment"
        const val OP_VOTE = "vote"
        const val OP_REMOVE_VOTE = "remove-vote"
        const val OP_REPORT = "report"
    }
}
