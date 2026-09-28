package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.VehicleDao
import com.anpfuel.data.mapper.VehicleMapper
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.DomainId
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class VehicleRepositoryImpl @Inject constructor(
    private val vehicleDao: VehicleDao,
) : VehicleRepository {

    override suspend fun listAll(): List<Vehicle> =
        vehicleDao.listAll().map(VehicleMapper::toDomain)

    override suspend fun findById(id: DomainId): Vehicle? =
        vehicleDao.findById(id.value)?.let(VehicleMapper::toDomain)

    override suspend fun save(vehicle: Vehicle) {
        vehicleDao.upsert(VehicleMapper.toEntity(vehicle))
    }

    override suspend fun delete(id: DomainId) {
        vehicleDao.deleteById(id.value)
    }

    override suspend fun count(): Int = vehicleDao.count()
}
