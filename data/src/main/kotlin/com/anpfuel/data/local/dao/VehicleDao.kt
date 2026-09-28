package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.VehicleEntity

@Dao
interface VehicleDao {

    @Query("SELECT * FROM vehicle ORDER BY sort_order ASC, display_name COLLATE NOCASE ASC")
    suspend fun listAll(): List<VehicleEntity>

    @Query("SELECT * FROM vehicle WHERE id = :id LIMIT 1")
    suspend fun findById(id: String): VehicleEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(vehicle: VehicleEntity)

    @Query("DELETE FROM vehicle WHERE id = :id")
    suspend fun deleteById(id: String)

    @Query("SELECT COUNT(*) FROM vehicle")
    suspend fun count(): Int
}
