package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Transaction
import com.anpfuel.data.local.entity.DriverDistanceEntity
import com.anpfuel.data.local.entity.DriverDraftEntity
import com.anpfuel.data.local.entity.DriverEarningEntity
import com.anpfuel.data.local.entity.DriverEarningRevisionEntity
import com.anpfuel.data.local.entity.DriverLocalCommandEntity
import com.anpfuel.data.local.entity.DriverShiftEntity
import com.anpfuel.data.local.entity.DriverShiftGapEntity

/**
 * DRV-13 owner/origin-scoped local driver state. There is no unscoped read.
 * Records, immutable revisions and the first local command are written in one
 * Room transaction; a failed insert rolls the whole save back.
 */
@Dao
abstract class DriverLocalDao {

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertShift(shift: DriverShiftEntity)

    @Query("DELETE FROM driver_shift_gap WHERE owner_account=:owner AND origin=:origin AND shift_id=:shiftId")
    abstract suspend fun deleteShiftGaps(owner: String, origin: String, shiftId: String)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    abstract suspend fun insertShiftGaps(gaps: List<DriverShiftGapEntity>)

    @Transaction
    open suspend fun saveShift(shift: DriverShiftEntity, gaps: List<DriverShiftGapEntity>) {
        upsertShift(shift)
        deleteShiftGaps(shift.ownerAccount, shift.origin, shift.shiftId)
        if (gaps.isNotEmpty()) insertShiftGaps(gaps)
    }

    @Query("SELECT * FROM driver_shift WHERE owner_account=:owner AND origin=:origin AND shift_id=:shiftId LIMIT 1")
    abstract suspend fun findShift(owner: String, origin: String, shiftId: String): DriverShiftEntity?

    @Query("SELECT * FROM driver_shift_gap WHERE owner_account=:owner AND origin=:origin AND shift_id=:shiftId ORDER BY created_at_millis, gap_id")
    abstract suspend fun listShiftGaps(owner: String, origin: String, shiftId: String): List<DriverShiftGapEntity>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertEarning(earning: DriverEarningEntity)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    abstract suspend fun insertEarningRevision(revision: DriverEarningRevisionEntity)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertCommand(command: DriverLocalCommandEntity)

    @Transaction
    open suspend fun saveEarning(
        earning: DriverEarningEntity,
        revision: DriverEarningRevisionEntity,
        command: DriverLocalCommandEntity,
    ) {
        upsertEarning(earning)
        insertEarningRevision(revision)
        upsertCommand(command)
    }

    @Query("SELECT * FROM driver_earning WHERE owner_account=:owner AND origin=:origin AND record_id=:recordId LIMIT 1")
    abstract suspend fun findEarning(owner: String, origin: String, recordId: String): DriverEarningEntity?

    @Query("SELECT * FROM driver_earning_revision WHERE owner_account=:owner AND origin=:origin AND record_id=:recordId ORDER BY revision")
    abstract suspend fun listEarningRevisions(owner: String, origin: String, recordId: String): List<DriverEarningRevisionEntity>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertDistance(distance: DriverDistanceEntity)

    @Transaction
    open suspend fun saveDistance(distance: DriverDistanceEntity, command: DriverLocalCommandEntity) {
        upsertDistance(distance)
        upsertCommand(command)
    }

    @Query("SELECT * FROM driver_distance WHERE owner_account=:owner AND origin=:origin AND observation_id=:id LIMIT 1")
    abstract suspend fun findDistance(owner: String, origin: String, id: String): DriverDistanceEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertDraft(draft: DriverDraftEntity)

    @Query("SELECT * FROM driver_draft WHERE owner_account=:owner AND origin=:origin AND draft_id=:draftId LIMIT 1")
    abstract suspend fun findDraft(owner: String, origin: String, draftId: String): DriverDraftEntity?

    @Query("SELECT * FROM driver_draft WHERE owner_account=:owner AND origin=:origin ORDER BY updated_at_millis ASC")
    abstract suspend fun listDrafts(owner: String, origin: String): List<DriverDraftEntity>

    @Query("SELECT * FROM driver_local_command WHERE owner_account=:owner AND origin=:origin AND operation_id=:operationId LIMIT 1")
    abstract suspend fun findCommand(owner: String, origin: String, operationId: String): DriverLocalCommandEntity?
}
