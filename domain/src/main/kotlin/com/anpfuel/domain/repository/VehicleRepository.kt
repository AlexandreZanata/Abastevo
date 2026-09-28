package com.anpfuel.domain.repository

import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.DomainId

/**
 * Port for local vehicle profiles (UC-010).
 */
interface VehicleRepository {

    suspend fun listAll(): List<Vehicle>

    suspend fun findById(id: DomainId): Vehicle?

    suspend fun save(vehicle: Vehicle)

    suspend fun delete(id: DomainId)

    suspend fun count(): Int
}
