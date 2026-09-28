package com.anpfuel.domain.model

import com.anpfuel.domain.valueobject.DomainId

/**
 * Local notification payload for UC-014 price drop alerts.
 */
data class PriceDropAlertNotification(
    val vehicleId: DomainId,
    val vehicleDisplayName: String,
    val currentPriceFormatted: String,
)
