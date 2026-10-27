package com.anpfuel.domain.premium

import java.security.MessageDigest

enum class OdometerEvidence { UNVERIFIED, VERIFIED_METRES }
enum class OdometerReviewChoice { AS_METRES, AS_KILOMETRES }

data class OdometerReviewPreview(
    val originalValue: Long,
    val correctedMetres: Long,
    val baseBodySha256: String,
    val choice: OdometerReviewChoice,
)

object LegacyOdometerReview {
    fun value(draft: LedgerDraft): Long? = when (draft) {
        is RefuelDraft -> draft.odometerMetres
        is ExpenseDraft -> draft.odometerMetres
        is ChargeDraft -> draft.odometerMetres
    }

    fun fingerprint(draft: LedgerDraft): String = MessageDigest.getInstance("SHA-256")
        .digest(draft.bodyJson().toByteArray(Charsets.UTF_8)).joinToString("") { "%02x".format(it) }

    fun preview(draft: LedgerDraft, choice: OdometerReviewChoice): OdometerReviewPreview {
        val original = value(draft) ?: throw PremiumValidationException("odometer-reading-missing")
        val corrected = try {
            if (choice == OdometerReviewChoice.AS_KILOMETRES) Math.multiplyExact(original, 1000L) else original
        } catch (_: ArithmeticException) {
            throw PremiumValidationException("odometer-review-out-of-bounds")
        }
        corrected(draft, corrected) // Reuse the dataset's existing exact bounds, including charge.
        return OdometerReviewPreview(original, corrected, fingerprint(draft), choice)
    }

    fun corrected(draft: LedgerDraft, metres: Long): LedgerDraft = when (draft) {
        is RefuelDraft -> draft.copy(odometerMetres = metres)
        is ExpenseDraft -> draft.copy(odometerMetres = metres)
        is ChargeDraft -> draft.copy(odometerMetres = metres)
    }
}
