package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.VehicleCapabilityLeaseEntity

/**
 * MEX-08 owner/origin capability leases. Reads and writes are partition
 * keyed; clearing one owner never touches another partition.
 */
@Dao
interface VehicleCapabilityLeaseDao {

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(lease: VehicleCapabilityLeaseEntity)

    @Query("SELECT * FROM vehicle_capability_lease WHERE owner_account = :owner AND origin = :origin LIMIT 1")
    suspend fun find(owner: String, origin: String): VehicleCapabilityLeaseEntity?

    @Query("DELETE FROM vehicle_capability_lease WHERE owner_account = :owner")
    suspend fun clearOwner(owner: String)
}
