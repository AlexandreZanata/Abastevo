package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import java.math.BigDecimal
import java.math.RoundingMode

/**
 * Non-negative monetary amount with at most two decimal places.
 */
class PriceAmount private constructor(
    val value: BigDecimal,
) {
    init {
        if (value.signum() < 0) {
            throw DomainException("PriceAmount must be non-negative")
        }
    }

    override fun equals(other: Any?): Boolean =
        other is PriceAmount && value.compareTo(other.value) == 0

    override fun hashCode(): Int = value.hashCode()

    override fun toString(): String = "PriceAmount(value=$value)"

    companion object {
        private const val SCALE = 2

        fun of(value: BigDecimal): PriceAmount =
            PriceAmount(value.setScale(SCALE, RoundingMode.HALF_UP))

        fun of(value: Double): PriceAmount = of(BigDecimal.valueOf(value))

        fun of(value: String): PriceAmount = of(BigDecimal(value))
    }
}
