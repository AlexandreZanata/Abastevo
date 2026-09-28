package com.anpfuel.application.usecase.vehicle

import com.anpfuel.domain.event.VehicleRemoved
import com.anpfuel.domain.repository.DomainEventPublisher
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.DomainId

data class DeleteVehicleResult(
    val vehicleId: DomainId,
    val event: VehicleRemoved,
)

class DeleteVehicleUseCase(
    private val vehicleRepository: VehicleRepository,
    private val eventPublisher: DomainEventPublisher,
) {

    suspend operator fun invoke(id: DomainId): DeleteVehicleResult {
        vehicleRepository.delete(id)

        val event = VehicleRemoved.create(
            payload = VehicleRemoved.Payload(vehicleId = id),
        )
        eventPublisher.publish(event)

        return DeleteVehicleResult(
            vehicleId = id,
            event = event,
        )
    }
}
