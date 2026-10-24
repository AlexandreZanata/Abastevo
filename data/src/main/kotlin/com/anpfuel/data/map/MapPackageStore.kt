package com.anpfuel.data.map

import com.anpfuel.data.local.dao.MapPackageDao
import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import com.anpfuel.domain.map.NativeMapManifest

const val PACKAGE_PENDING = "PENDING"
const val PACKAGE_DOWNLOADING = "DOWNLOADING"
const val PACKAGE_READY = "READY"
const val PACKAGE_FAILED = "FAILED"

interface MapPackageStore {
    suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>)
    suspend fun generation(generation: String): MapGenerationEntity?
    suspend fun activeGeneration(): MapGenerationEntity?
    suspend fun packages(generation: String): List<MapPackageEntity>
    suspend fun updatePackage(
        generation: String,
        packageId: String,
        status: String,
        downloadedBytes: Long,
        etag: String?,
        error: String?,
        updatedAtMillis: Long,
    )
    suspend fun activate(generation: String, activatedAtMillis: Long)
    suspend fun deleteInactive(generation: String)
}

class RoomMapPackageStore(private val dao: MapPackageDao) : MapPackageStore {
    override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) {
        dao.insertGeneration(generation)
        dao.insertPackages(packages)
    }
    override suspend fun generation(generation: String) = dao.generation(generation)
    override suspend fun activeGeneration() = dao.activeGeneration()
    override suspend fun packages(generation: String) = dao.packages(generation)
    override suspend fun updatePackage(
        generation: String,
        packageId: String,
        status: String,
        downloadedBytes: Long,
        etag: String?,
        error: String?,
        updatedAtMillis: Long,
    ) = dao.updatePackage(generation, packageId, status, downloadedBytes, etag, error, updatedAtMillis)
    override suspend fun activate(generation: String, activatedAtMillis: Long) =
        dao.activateGeneration(generation, activatedAtMillis)
    override suspend fun deleteInactive(generation: String) = dao.deleteInactiveGeneration(generation)
}

fun NativeMapManifest.packageRows(generationKey: String, now: Long): List<MapPackageEntity> =
    packages.map {
        MapPackageEntity(
            packageId = it.id,
            generation = generationKey,
            role = it.role,
            fileName = it.path,
            bytes = it.bytes,
            sha256 = it.sha256,
            etag = null,
            status = PACKAGE_PENDING,
            downloadedBytes = 0,
            error = null,
            updatedAtMillis = now,
        )
    }
