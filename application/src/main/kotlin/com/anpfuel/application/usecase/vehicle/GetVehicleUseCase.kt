package com.anpfuel.application.usecase.vehicle

import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.DomainId

class GetVehicleUseCase(
    private val vehicleRepository: VehicleRepository,
) {

    suspend operator fun invoke(id: DomainId): Vehicle? = vehicleRepository.findById(id)
}
