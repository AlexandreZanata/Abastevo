package com.anpfuel.application.usecase.vehicle

import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.VehicleRepository

class ListVehiclesUseCase(
    private val vehicleRepository: VehicleRepository,
) {

    suspend operator fun invoke(): List<Vehicle> = vehicleRepository.listAll()
}
