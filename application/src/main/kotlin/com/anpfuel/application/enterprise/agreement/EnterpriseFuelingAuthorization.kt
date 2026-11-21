package com.anpfuel.application.enterprise.agreement

import com.anpfuel.domain.enterprise.agreement.AgreementRules
import com.anpfuel.domain.enterprise.agreement.AnnouncedPrice
import com.anpfuel.domain.enterprise.agreement.Promotion
import com.anpfuel.domain.enterprise.agreement.Voucher
import java.time.Instant

/**
 * Field-safe Phase F7 slices (EMP-AGREE-004, EMP-STN-003): the driver sees
 * their own bounded fueling authorization metadata — product, limits and
 * expiry with the derived state — and the station's announced commercial
 * source with its labels. The one-time voucher code never enters this
 * layer, and no organization-wide financial or agreement data does either.
 */
object EnterpriseFuelingAuthorizationProducers {
    const val VOUCHER_NOTICE_RECORD_ID = "f07e0b00-0000-4000-8000-0000000000d1"
}

data class FieldVoucherNotice(
    val authorizationId: String,
    val product: String,
    val unit: String,
    val maxQuantityMilli: Long,
    val maxCentavos: Long,
    val validUntilEpochSeconds: Long,
    val storedState: String,
) {
    init {
        require(authorizationId.isNotBlank()) { "authorization identifier required" }
        require(AgreementRules.validProduct(product, unit)) { "authorization product invalid" }
        require(maxQuantityMilli >= 1L && maxCentavos >= 1L) { "authorization limits invalid" }
        require(storedState in setOf("ISSUED", "REDEEMED", "CANCELLED", "LOCKED")) { "authorization state invalid" }
    }

    /** The derived read-time state; expiry applies at read time. */
    fun derivedStateAt(nowEpochSeconds: Long): String {
        val derived = Voucher(
            state = com.anpfuel.domain.enterprise.agreement.VoucherState.valueOf(storedState),
            product = product,
            unit = unit,
            maxQuantityMilli = maxQuantityMilli,
            maxCentavos = maxCentavos,
            validUntilEpochSeconds = validUntilEpochSeconds,
        ).derivedStateAt(nowEpochSeconds)
        return derived.name
    }

    fun eligibleAt(nowEpochSeconds: Long): Boolean = derivedStateAt(nowEpochSeconds) == "ISSUED"

    override fun toString(): String = "FieldVoucherNotice(redacted)"
}

data class FieldStationCommerceSnapshot(
    val announcedPrices: List<AnnouncedPrice> = emptyList(),
    val promotions: List<Promotion> = emptyList(),
) {
    /** Source-labelled reads: only the announced source, never official or community data. */
    fun announcedDisplayStates(nowEpochSeconds: Long): List<String> =
        announcedPrices.map { price ->
            AnnouncedPrice.displayState(
                com.anpfuel.domain.enterprise.agreement.AnnouncedPriceState.ACTIVE.name,
                nowEpochSeconds,
                price.fromEpochSeconds,
                price.untilEpochSeconds,
            )
        }

    fun activePromotionsAt(nowEpochSeconds: Long): List<Promotion> =
        promotions.filter {
            Promotion.displayState(
                com.anpfuel.domain.enterprise.agreement.PromotionState.PUBLISHED.name,
                nowEpochSeconds,
                it.fromEpochSeconds,
                it.untilEpochSeconds,
            ) == com.anpfuel.domain.enterprise.agreement.PromotionState.ACTIVE.name
        }

    override fun toString(): String = "FieldStationCommerceSnapshot(redacted)"
}

data class FuelingAuthorizationAdvisory(
    val notices: List<FieldVoucherNotice> = emptyList(),
    val commerce: FieldStationCommerceSnapshot = FieldStationCommerceSnapshot(),
) {
    /** The nearest usable authorization for the driver, or null. */
    fun usableNoticeAt(nowEpochSeconds: Long): FieldVoucherNotice? =
        notices.filter { it.eligibleAt(nowEpochSeconds) }.minByOrNull { it.validUntilEpochSeconds }

    override fun toString(): String = "FuelingAuthorizationAdvisory(redacted)"
}

/** Builds the advisory from field-safe inputs only. */
fun fuelingAuthorizationAdvisory(
    notices: List<FieldVoucherNotice>,
    commerce: FieldStationCommerceSnapshot,
): FuelingAuthorizationAdvisory = FuelingAuthorizationAdvisory(notices, commerce)

/** Formats the driver's voucher validity instant, UTC, for display. */
fun voucherValidUntilText(notice: FieldVoucherNotice): String =
    Instant.ofEpochSecond(notice.validUntilEpochSeconds).toString()
