package com.anpfuel.domain.enterprise.agreement

/**
 * EMP-STN-002..004 (BR-STN-001/002, VAL-33): the station's own attributed
 * commercial source on the device — announced prices and labelled promotions
 * keep source, conditions and age, never enter official or community
 * consensus and expiry applies at read time.
 */
object StationCommerceRules {
    const val METHOD_VERSION = "STATION_COMMERCE_V1"

    val PRODUCTS = setOf(
        "ETHANOL", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED",
        "DIESEL_S500", "DIESEL_S10", "CNG", "LPG_P13",
    )
    val UNITS = setOf("L", "M3", "KG_13")

    fun validProduct(product: String, unit: String): Boolean =
        product in PRODUCTS && unit in UNITS
}

enum class CommerceError {
    ANNOUNCED_WINDOW_INVALID,
    ANNOUNCED_PRICE_INVALID,
    ANNOUNCED_PRODUCT_INVALID,
    ANNOUNCED_CONDITIONS_INVALID,
    PROMOTION_WINDOW_INVALID,
    PROMOTION_TEXT_INVALID,
}

/** Stored announced-price states; PENDING and EXPIRED derive at read time. */
enum class AnnouncedPriceState { ACTIVE, WITHDRAWN, SUPERSEDED, PENDING, EXPIRED }

data class AnnouncedPrice(
    val product: String,
    val unit: String,
    val priceMilliBrl: Long,
    val conditions: String = "",
    val fromEpochSeconds: Long,
    val untilEpochSeconds: Long,
) {
    init {
        if (!StationCommerceRules.validProduct(product, unit)) {
            throw IllegalStateException(CommerceError.ANNOUNCED_PRODUCT_INVALID.name)
        }
        if (priceMilliBrl !in 1..1_000_000L) {
            throw IllegalStateException(CommerceError.ANNOUNCED_PRICE_INVALID.name)
        }
        if (untilEpochSeconds <= fromEpochSeconds) {
            throw IllegalStateException(CommerceError.ANNOUNCED_WINDOW_INVALID.name)
        }
        if (conditions.length > 120) {
            throw IllegalStateException(CommerceError.ANNOUNCED_CONDITIONS_INVALID.name)
        }
    }

    companion object {
        fun displayState(state: String, nowEpochSeconds: Long, fromEpochSeconds: Long, untilEpochSeconds: Long): String =
            when (state) {
                AnnouncedPriceState.WITHDRAWN.name, AnnouncedPriceState.SUPERSEDED.name -> state
                else -> when {
                    nowEpochSeconds < fromEpochSeconds -> AnnouncedPriceState.PENDING.name
                    nowEpochSeconds > untilEpochSeconds -> AnnouncedPriceState.EXPIRED.name
                    else -> AnnouncedPriceState.ACTIVE.name
                }
            }
    }
}



/** Stored promotion states; SCHEDULED, ACTIVE and ENDED derive at read time. */
enum class PromotionState { PUBLISHED, WITHDRAWN, SCHEDULED, ACTIVE, ENDED }

data class Promotion(
    val label: String,
    val description: String,
    val product: String? = null,
    val fromEpochSeconds: Long,
    val untilEpochSeconds: Long,
) {
    init {
        if (label.isEmpty() || label.length > 60 || description.isEmpty() || description.length > 280) {
            throw IllegalStateException(CommerceError.PROMOTION_TEXT_INVALID.name)
        }
        if (product != null && product !in StationCommerceRules.PRODUCTS) {
            throw IllegalStateException(CommerceError.ANNOUNCED_PRODUCT_INVALID.name)
        }
        if (untilEpochSeconds <= fromEpochSeconds) {
            throw IllegalStateException(CommerceError.PROMOTION_WINDOW_INVALID.name)
        }
    }

    companion object {
        fun displayState(state: String, nowEpochSeconds: Long, fromEpochSeconds: Long, untilEpochSeconds: Long): String =
            when (state) {
                PromotionState.WITHDRAWN.name -> state
                else -> when {
                    nowEpochSeconds < fromEpochSeconds -> PromotionState.SCHEDULED.name
                    nowEpochSeconds > untilEpochSeconds -> PromotionState.ENDED.name
                    else -> PromotionState.ACTIVE.name
                }
            }
    }
}
