package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.VehicleCapabilityLeaseDao
import com.anpfuel.data.local.dao.VehicleDao
import com.anpfuel.data.local.entity.VehicleCapabilityLeaseEntity
import com.anpfuel.data.mapper.VehicleMapper
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.VehicleAdmissionResult
import com.anpfuel.domain.premium.VehicleAdmissionRule
import com.anpfuel.domain.premium.VehicleCapabilityLease
import com.anpfuel.domain.repository.VehicleAdmissionRepository
import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class VehicleAdmissionRepositoryImpl @Inject constructor(
    private val vehicleDao: VehicleDao,
    private val leaseDao: VehicleCapabilityLeaseDao,
) : VehicleAdmissionRepository {

    override suspend fun countVehicles(): Int = vehicleDao.count()

    override suspend fun findVehicle(id: DomainId): Vehicle? =
        vehicleDao.findById(id.value)?.let(VehicleMapper::toDomain)

    override suspend fun admit(vehicle: Vehicle, hasPaidCapability: Boolean): VehicleAdmissionResult =
        vehicleDao.admit(
            VehicleMapper.toEntity(vehicle),
            VehicleAdmissionRule.FREE_LIMIT,
            VehicleAdmissionRule.TECHNICAL_CEILING,
            hasPaidCapability,
        )

    override suspend fun loadLease(accountId: String, origin: String): VehicleCapabilityLease? =
        leaseDao.find(accountId, origin)?.toDomain()

    override suspend fun saveLease(lease: VehicleCapabilityLease) {
        leaseDao.upsert(
            VehicleCapabilityLeaseEntity(
                ownerAccount = lease.accountId,
                origin = lease.origin,
                policyVersion = lease.version,
                capability = lease.capability,
                verifiedAtMillis = lease.verifiedAt.toEpochMilli(),
                paidThroughMillis = lease.paidThrough.toEpochMilli(),
                revoked = lease.revoked,
                savedAtMillis = Instant.now().toEpochMilli(),
            ),
        )
    }

    override suspend fun clearLeases(accountId: String) {
        leaseDao.clearOwner(accountId)
    }

    private fun VehicleCapabilityLeaseEntity.toDomain() = VehicleCapabilityLease(
        version = policyVersion,
        accountId = ownerAccount,
        origin = origin,
        capability = capability,
        verifiedAt = Instant.ofEpochMilli(verifiedAtMillis),
        paidThrough = Instant.ofEpochMilli(paidThroughMillis),
        revoked = revoked,
    )
}
