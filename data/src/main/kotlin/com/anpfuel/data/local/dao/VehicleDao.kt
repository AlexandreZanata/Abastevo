package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import com.anpfuel.data.local.entity.VehicleEntity
import com.anpfuel.domain.premium.AdmissionOutcome
import com.anpfuel.domain.premium.VehicleAdmissionRefused
import com.anpfuel.domain.premium.VehicleAdmissionResult

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

    /**
     * MEX-08 transaction-safe admission. Editing an existing ID replays
     * without consuming a slot; new IDs are admitted only within the
     * technical ceiling and, past the free limit, only with a paid
     * capability. The recount happens inside the transaction, so two
     * racing inserts cannot both slip past the ceiling (no bare
     * count-then-save).
     */
    @Transaction
    suspend fun admit(
        vehicle: VehicleEntity,
        freeLimit: Int,
        technicalCeiling: Int,
        hasPaidCapability: Boolean,
    ): VehicleAdmissionResult {
        if (findById(vehicle.id) != null) {
            upsert(vehicle)
            return VehicleAdmissionResult.Replayed
        }
        val count = count()
        if (count >= technicalCeiling) {
            throw VehicleAdmissionRefused(AdmissionOutcome.CAPACITY_EXCEEDED)
        }
        if (count >= freeLimit && !hasPaidCapability) {
            throw VehicleAdmissionRefused(AdmissionOutcome.PREMIUM_REQUIRED)
        }
        upsert(vehicle)
        return VehicleAdmissionResult.AdmittedAsNew
    }
}
