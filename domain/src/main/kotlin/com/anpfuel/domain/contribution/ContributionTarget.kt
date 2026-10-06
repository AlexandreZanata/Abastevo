package com.anpfuel.domain.contribution

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.BackendPriceGroup

/**
 * P37-T01 contextual capture target.
 *
 * Binds one capture/review/submit journey to a canonical Directory UUID
 * plus a wire fuel product — never a legacy full-CNPJ row and never an
 * unvalidated manual id. The contributor still picks/confirms price and
 * condition explicitly; the target only fixes *where* the observation
 * lands. Account proof travels separately at submit time (P37-T02).
 */
class ContributionTarget private constructor(
    val stationId: String,
    val fuelProductWire: String,
) {
    companion object {
        fun create(stationId: String, fuelProductWire: String): ContributionTarget {
            if (!BackendPriceGroup.isUuid(stationId.trim())) {
                throw DomainException("contribution target must be a canonical station UUID")
            }
            if (fuelProductWire !in BackendPriceGroup.WIRE_PRODUCTS) {
                throw DomainException("unknown wire fuel product: $fuelProductWire")
            }
            return ContributionTarget(
                stationId = stationId.trim().lowercase(),
                fuelProductWire = fuelProductWire,
            )
        }
    }
}
