package com.anpfuel.application.usecase.alert

import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.PriceDropNotificationRepository

/**
 * UC-014 — Applies side effects when a vehicle price drop alert preference changes.
 */
class ConfigurePriceDropAlertUseCase(
    private val priceDropNotificationRepository: PriceDropNotificationRepository,
) {

    suspend operator fun invoke(
        updated: Vehicle,
        previous: Vehicle?,
    ) {
        if (previous?.priceDropAlertEnabled == true && !updated.priceDropAlertEnabled) {
            priceDropNotificationRepository.cancelForVehicle(updated.id)
        }
    }
}
