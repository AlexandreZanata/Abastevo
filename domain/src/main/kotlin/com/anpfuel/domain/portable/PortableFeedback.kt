package com.anpfuel.domain.portable

/**
 * Portable station/fuel feedback values (P14-T01, B-BR-F02…F06).
 *
 * Pure Kotlin with zero `java.*` imports so this file moves unchanged into a
 * future `commonMain` source set. Mirrors the frozen backend rules
 * (`backend/internal/modules/feedback/domain`): integer 1–5 stars and
 * floor basis-point agreement with null for zero votes. Text counting
 * stays in [PortableText] (280 scalars, shared vectors); the server
 * enforces every bound.
 */
object PortableFeedback {

    /** Personal-experience stars, integers 1–5 (F02). */
    const val RATING_MIN: Int = 1
    const val RATING_MAX: Int = 5

    /** Agreement scale: floor(10000 * valid / total) basis points (F06). */
    const val AGREEMENT_SCALE: Long = 10000L

    /** True for an integer inside the frozen star range. */
    fun isValidRating(stars: Int): Boolean =
        stars in RATING_MIN..RATING_MAX

    /**
     * Floor agreement in basis points, or null when no vote exists (F06:
     * null "No votes", never 0% certainty). Negative counts throw:
     * denominators come from constrained stores, never client math.
     */
    fun agreementBasisPoints(valid: Long, invalid: Long): Long? {
        require(valid >= 0 && invalid >= 0) {
            "negative vote count"
        }
        val total = valid + invalid
        if (total == 0L) return null
        return AGREEMENT_SCALE * valid / total
    }
}
