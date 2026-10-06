package com.anpfuel.domain.feedback

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.BackendPriceGroup

/**
 * P36-T02 canonical social target.
 *
 * Ratings, comments/replies, validity votes and reports address a
 * canonical Directory UUID plus a wire fuel product — never a legacy
 * full-CNPJ row and never a manual station-ID textbox. A blank account
 * is an honest guest (reads only); writes require a signed account and
 * are enforced again server-side. Vote agreement stays separate from
 * price confidence; no trust bonus or moderator power is granted here.
 */
class FeedbackTarget private constructor(
    val stationId: String,
    val fuelProductWire: String,
    val accountId: String,
) {
    val isSignedIn: Boolean get() = accountId.isNotEmpty()

    companion object {
        fun create(
            stationId: String,
            fuelProductWire: String,
            accountId: String,
        ): FeedbackTarget {
            if (!BackendPriceGroup.isUuid(stationId.trim())) {
                throw DomainException("social target must be a canonical station UUID")
            }
            if (fuelProductWire !in BackendPriceGroup.WIRE_PRODUCTS) {
                throw DomainException("unknown wire fuel product: $fuelProductWire")
            }
            return FeedbackTarget(
                stationId = stationId.trim().lowercase(),
                fuelProductWire = fuelProductWire,
                accountId = accountId.trim(),
            )
        }
    }
}
