package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import java.math.BigDecimal
import java.math.RoundingMode

/**
 * Tank capacity in liters for a [com.anpfuel.domain.model.Vehicle].
 */
class TankCapacity private constructor(
    val liters: BigDecimal,
) {
    init {
        if (liters.signum() <= 0) {
            throw DomainException("TankCapacity must be greater than zero")
        }
        if (liters.compareTo(BigDecimal.valueOf(MAX_LITERS.toLong())) > 0) {
            throw DomainException("TankCapacity must not exceed $MAX_LITERS liters")
        }
    }

    override fun equals(other: Any?): Boolean =
        other is TankCapacity && liters.compareTo(other.liters) == 0

    override fun hashCode(): Int = liters.hashCode()

    override fun toString(): String = "TankCapacity(liters=$liters)"

    companion object {
        const val MAX_LITERS = 200

        fun of(liters: Double): TankCapacity = of(BigDecimal.valueOf(liters))

        fun of(liters: BigDecimal): TankCapacity =
            TankCapacity(liters.setScale(2, RoundingMode.HALF_UP))
    }
}
