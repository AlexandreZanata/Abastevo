package com.anpfuel.domain.repository

import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.VehicleAdmissionResult
import com.anpfuel.domain.premium.VehicleCapabilityLease
import com.anpfuel.domain.valueobject.DomainId

/**
 * MEX-08 vehicle admission port: atomic slot admission plus the
 * owner/origin capability lease store. Leases are partition keyed and
 * never merged across owners.
 */
interface VehicleAdmissionRepository {

    suspend fun countVehicles(): Int

    suspend fun findVehicle(id: DomainId): Vehicle?

    suspend fun admit(vehicle: Vehicle, hasPaidCapability: Boolean): VehicleAdmissionResult

    suspend fun loadLease(accountId: String, origin: String): VehicleCapabilityLease?

    suspend fun saveLease(lease: VehicleCapabilityLease)

    suspend fun clearLeases(accountId: String)
}
