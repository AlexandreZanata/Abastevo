package com.anpfuel.application.usecase.vehicle

import com.anpfuel.domain.event.DomainEvent
import com.anpfuel.domain.event.PriceDropAlertConfigured
import com.anpfuel.domain.event.VehicleRegistered
import com.anpfuel.domain.event.VehicleUpdated
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.rule.MaxRegisteredVehiclesRule

data class SaveVehicleResult(
    val vehicle: Vehicle,
    val events: List<DomainEvent>,
)

class SaveVehicleUseCase(
    private val vehicleRepository: VehicleRepository,
    private val eventPublisher: DomainEventPublisher,
) {

    suspend operator fun invoke(vehicle: Vehicle): SaveVehicleResult {
        val existing = vehicleRepository.findById(vehicle.id)
        if (existing == null) {
            MaxRegisteredVehiclesRule.requireCanRegister(vehicleRepository.count())
        }

        vehicleRepository.save(vehicle)

        val events = buildList {
            if (existing == null) {
                add(
                    VehicleRegistered.create(
                        payload = VehicleRegistered.Payload(
                            vehicleId = vehicle.id,
                            displayName = vehicle.displayName,
                            fuelProduct = vehicle.fuelProduct,
                        ),
                    ),
                )
            } else {
                add(
                    VehicleUpdated.create(
                        payload = VehicleUpdated.Payload(
                            vehicleId = vehicle.id,
                            displayName = vehicle.displayName,
                            fuelProduct = vehicle.fuelProduct,
                        ),
                    ),
                )
            }

            if (shouldPublishPriceDropAlertConfigured(existing, vehicle)) {
                add(
                    PriceDropAlertConfigured.create(
                        payload = PriceDropAlertConfigured.Payload(
                            vehicleId = vehicle.id,
                            enabled = vehicle.priceDropAlertEnabled,
                            alertPriceSource = vehicle.priceSource,
                        ),
                    ),
                )
            }
        }

        events.forEach { event -> eventPublisher.publish(event) }

        return SaveVehicleResult(
            vehicle = vehicle,
            events = events,
        )
    }

    internal fun shouldPublishPriceDropAlertConfigured(
        existing: Vehicle?,
        updated: Vehicle,
    ): Boolean {
        if (existing == null) {
            return updated.priceDropAlertEnabled
        }
        return existing.priceDropAlertEnabled != updated.priceDropAlertEnabled ||
            existing.priceSource != updated.priceSource
    }
}
