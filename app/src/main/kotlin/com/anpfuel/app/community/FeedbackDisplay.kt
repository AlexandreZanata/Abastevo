package com.anpfuel.app.community

import com.anpfuel.domain.portable.PortableFeedback
import com.anpfuel.domain.portable.PortableText
import com.anpfuel.domain.repository.FeedbackRejectKind
import com.anpfuel.domain.repository.VoteTallySnapshot

/**
 * P17-T02 feedback presentation (B-BR-F03/F06, BUC-F02/F03/F05);
 * P22-T02 adds the personal-star aggregate (B-BR-F02/B-BR-C04).
 *
 * Pure (no `android.*`): agreement renders from exact counts with
 * floor truncation and one decimal (2 valid / 1 invalid → 66.6%);
 * zero votes render "No votes", never 0% certainty. Ratings render
 * from the exact count/sum the same way (21/5 → 4.2 ★ · 5 ratings;
 * zero renders "No ratings", never an invented score) and always
 * travel apart from price confidence — community stars never certify
 * a pump price. Text travels verbatim and callers must use plain
 * `Text` — HTML is never rendered. Rejection labels are fixed short
 * codes; private report reasons and dispute detail never enter copy.
 */
object FeedbackDisplay {

    /** Scalar budget left for the 280 rule; negative means over. */
    fun charsRemaining(text: String): Int {
        val normalized = PortableText.normalize(text)
        return PortableText.MAX_COMMENT_SCALARS - PortableText.countScalars(normalized)
    }

    /** One-line agreement with valid/invalid/total counts. */
    fun agreementLine(tally: VoteTallySnapshot): String {
        val total = tally.valid + tally.invalid
        if (total <= 0L) return "No votes"
        val basisPoints = PortableFeedback.agreementBasisPoints(tally.valid, tally.invalid)
            ?: return "No votes"
        val whole = basisPoints / 100L
        val tenth = (basisPoints % 100L) / 10L
        return "$whole.$tenth% · ${tally.valid} valid / ${tally.invalid} invalid"
    }

    /**
     * One-line personal-star aggregate with the exact count. Truncated
     * (floor) one-decimal mean from the integer sum, kept apart from
     * price confidence: stars describe experience, never pump truth.
     */
    fun ratingLine(count: Long, sum: Long): String {
        require(count >= 0 && sum >= 0) {
            "negative rating aggregate"
        }
        if (count <= 0L) return "No ratings"
        val tenth = 10L * sum / count
        val amount = "${tenth / 10L}.${tenth % 10L} ★"
        val denominator = if (count == 1L) "1 rating" else "$count ratings"
        return "$amount · $denominator"
    }

    fun rejectKindLabel(kind: FeedbackRejectKind): String =
        when (kind) {
            FeedbackRejectKind.TARGET_INVALID -> "TARGET"
            FeedbackRejectKind.RATING_OUT_OF_RANGE -> "STARS"
            FeedbackRejectKind.TEXT_EMPTY -> "EMPTY"
            FeedbackRejectKind.TEXT_TOO_LONG -> "TOO_LONG"
            FeedbackRejectKind.NOT_AUTHOR -> "NOT_AUTHOR"
            FeedbackRejectKind.STALE_REVISION -> "STALE"
            FeedbackRejectKind.COMMENT_NOT_FOUND -> "NOT_FOUND"
            FeedbackRejectKind.RATING_NOT_FOUND -> "NOT_FOUND"
            FeedbackRejectKind.SELF_VOTE -> "SELF_VOTE"
            FeedbackRejectKind.VOTE_CHOICE_INVALID -> "CHOICE"
            FeedbackRejectKind.REPORT_INVALID -> "REPORT"
            FeedbackRejectKind.QUOTA_EXCEEDED -> "QUOTA"
            FeedbackRejectKind.TRANSPORT -> "TRANSPORT"
            FeedbackRejectKind.GATE_REQUIRED -> "SIGN_IN"
        }
}
