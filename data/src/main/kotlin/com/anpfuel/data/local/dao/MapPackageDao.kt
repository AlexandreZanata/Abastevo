package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity

@Dao
interface MapPackageDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun insertGeneration(generation: MapGenerationEntity)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun insertPackages(packages: List<MapPackageEntity>)

    @Query("SELECT * FROM map_generation WHERE generation = :generation")
    suspend fun generation(generation: String): MapGenerationEntity?

    @Query("SELECT * FROM map_generation WHERE active = 1 LIMIT 1")
    suspend fun activeGeneration(): MapGenerationEntity?

    @Query("SELECT * FROM map_package WHERE generation = :generation ORDER BY package_id")
    suspend fun packages(generation: String): List<MapPackageEntity>

    @Query("SELECT * FROM map_package WHERE generation = :generation AND package_id = :packageId")
    suspend fun packageById(generation: String, packageId: String): MapPackageEntity?

    @Query("UPDATE map_package SET status = :status, downloaded_bytes = :downloadedBytes, etag = :etag, error = :error, updated_at_millis = :updatedAtMillis WHERE generation = :generation AND package_id = :packageId")
    suspend fun updatePackage(
        generation: String,
        packageId: String,
        status: String,
        downloadedBytes: Long,
        etag: String?,
        error: String?,
        updatedAtMillis: Long,
    )

    @Query("UPDATE map_generation SET active = 0 WHERE active = 1")
    suspend fun deactivateAll()

    @Query("UPDATE map_generation SET active = 1, activated_at_millis = :activatedAtMillis WHERE generation = :generation")
    suspend fun activate(generation: String, activatedAtMillis: Long)

    @Query("DELETE FROM map_generation WHERE generation = :generation AND active = 0")
    suspend fun deleteInactiveGeneration(generation: String)

    @Query("SELECT generation FROM map_generation WHERE active = 0 ORDER BY generation")
    suspend fun inactiveGenerations(): List<String>

    @Transaction
    suspend fun activateGeneration(generation: String, activatedAtMillis: Long) {
        deactivateAll()
        activate(generation, activatedAtMillis)
    }
}
