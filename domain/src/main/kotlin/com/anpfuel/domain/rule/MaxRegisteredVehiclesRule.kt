package com.anpfuel.domain.rule

import com.anpfuel.domain.exception.DomainException

/**
 * BR-027 — Limit registered vehicles for home layout stability.
 */
object MaxRegisteredVehiclesRule {

    const val MAX_VEHICLES = 3

    fun canRegister(currentCount: Int): Boolean = currentCount < MAX_VEHICLES

    fun requireCanRegister(currentCount: Int) {
        if (!canRegister(currentCount)) {
            throw DomainException("Cannot register more than $MAX_VEHICLES vehicles")
        }
    }
}
