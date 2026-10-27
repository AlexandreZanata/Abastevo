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
import com.anpfuel.data.local.entity.DriverCostSettingEntity
import com.anpfuel.data.local.entity.DriverGpsObservationEntity
import com.anpfuel.data.local.entity.DriverGoalEntity
import com.anpfuel.data.local.entity.DriverMaintenanceEntity
import com.anpfuel.data.local.entity.DriverReconciliationEntity
import com.anpfuel.data.local.entity.DriverLocalCommandEntity
import com.anpfuel.data.local.entity.DriverPayoutEntity
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

    @Insert(onConflict = OnConflictStrategy.ABORT)
    abstract suspend fun insertShift(shift: DriverShiftEntity)

    @Query("SELECT * FROM driver_shift WHERE owner_account=:owner AND state<>'CLOSED' ORDER BY origin, shift_id")
    abstract suspend fun activeShiftsForOwner(owner: String): List<DriverShiftEntity>

    /** Legacy duplicates or a shift in another origin must never be silently selected. */
    @Transaction
    open suspend fun currentShiftChecked(owner: String, origin: String): DriverShiftEntity? {
        val active = activeShiftsForOwner(owner)
        check(active.size <= 1) { "driver-active-shift-conflict" }
        val current = active.singleOrNull()
        check(current == null || current.origin == origin) { "driver-origin-conflict" }
        return current
    }

    @Transaction
    open suspend fun startShift(shift: DriverShiftEntity) {
        check(activeShiftsForOwner(shift.ownerAccount).isEmpty()) { "driver-active-shift-conflict" }
        insertShift(shift)
    }

    @Transaction
    open suspend fun updateShift(shift: DriverShiftEntity, gaps: List<DriverShiftGapEntity>, expectedVersion: Long) {
        val active = activeShiftsForOwner(shift.ownerAccount)
        check(active.size == 1) { "driver-active-shift-conflict" }
        val current = active.single()
        check(current.origin == shift.origin && current.shiftId == shift.shiftId) { "driver-active-shift-conflict" }
        check(current.version == expectedVersion) { "driver-stale-shift" }
        check(current.vehicleId == shift.vehicleId && current.timezone == shift.timezone &&
            current.createdAtMillis == shift.createdAtMillis) { "driver-shift-identity-conflict" }
        saveShift(shift, gaps)
    }

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

    @Query("SELECT * FROM driver_shift WHERE owner_account=:owner AND origin=:origin AND state<>'CLOSED' ORDER BY updated_at_millis DESC LIMIT 1")
    abstract suspend fun currentShift(owner: String, origin: String): DriverShiftEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertPayout(payout: DriverPayoutEntity)

    @Query("SELECT * FROM driver_payout WHERE owner_account=:owner AND origin=:origin ORDER BY occurred_at_millis DESC")
    abstract suspend fun listPayouts(owner: String, origin: String): List<DriverPayoutEntity>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertCostSetting(setting: DriverCostSettingEntity)

    @Query("SELECT * FROM driver_cost_setting WHERE owner_account=:owner AND origin=:origin AND vehicle_id=:vehicleId LIMIT 1")
    abstract suspend fun findCostSetting(owner: String, origin: String, vehicleId: String): DriverCostSettingEntity?

    @Query("SELECT * FROM driver_earning WHERE owner_account=:owner AND origin=:origin AND vehicle_id=:vehicleId ORDER BY from_millis")
    abstract suspend fun listEarnings(owner: String, origin: String, vehicleId: String): List<DriverEarningEntity>

    @Query("SELECT count(*) FROM driver_earning WHERE owner_account=:owner AND origin=:origin AND vehicle_id=:vehicleId AND platform=:platform AND from_millis<:toMillis AND to_millis>:fromMillis")
    abstract suspend fun countOverlappingEarnings(owner: String, origin: String, vehicleId: String, platform: String, fromMillis: Long, toMillis: Long): Int

    @Query("SELECT * FROM driver_distance WHERE owner_account=:owner AND origin=:origin AND vehicle_id=:vehicleId ORDER BY from_millis")
    abstract suspend fun listDistances(owner: String, origin: String, vehicleId: String): List<DriverDistanceEntity>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertGpsObservation(observation: DriverGpsObservationEntity)

    @Query("SELECT * FROM driver_gps_observation WHERE owner_account=:owner AND origin=:origin AND shift_id=:shiftId ORDER BY created_at_millis DESC LIMIT 1")
    abstract suspend fun latestGpsObservation(owner: String, origin: String, shiftId: String): DriverGpsObservationEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertReconciliation(row: DriverReconciliationEntity)

    @Query("SELECT * FROM driver_reconciliation WHERE owner_account=:owner AND origin=:origin AND vehicle_id=:vehicleId ORDER BY day DESC")
    abstract suspend fun listReconciliations(owner: String, origin: String, vehicleId: String): List<DriverReconciliationEntity>

    @Query("SELECT * FROM driver_reconciliation WHERE owner_account=:owner AND origin=:origin AND vehicle_id=:vehicleId AND day=:day LIMIT 1")
    abstract suspend fun findReconciliation(owner: String, origin: String, vehicleId: String, day: String): DriverReconciliationEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertGoal(goal: DriverGoalEntity)

    @Query("SELECT * FROM driver_goal WHERE owner_account=:owner AND origin=:origin AND vehicle_id=:vehicleId ORDER BY period")
    abstract suspend fun listGoals(owner: String, origin: String, vehicleId: String): List<DriverGoalEntity>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun upsertMaintenance(item: DriverMaintenanceEntity)

    @Query("SELECT * FROM driver_maintenance WHERE owner_account=:owner AND origin=:origin AND vehicle_id=:vehicleId ORDER BY kind")
    abstract suspend fun listMaintenance(owner: String, origin: String, vehicleId: String): List<DriverMaintenanceEntity>

    @Query("SELECT * FROM driver_local_command WHERE owner_account=:owner AND origin=:origin AND operation_id=:operationId LIMIT 1")
    abstract suspend fun findCommand(owner: String, origin: String, operationId: String): DriverLocalCommandEntity?

    @Query(
        "SELECT * FROM driver_local_command WHERE owner_account=:owner AND origin=:origin " +
            "AND operation_id=:operationId AND state='PENDING' AND next_eligible_millis<=:now " +
            "AND lease_until_millis<=:now LIMIT 1",
    )
    abstract suspend fun findClaimable(owner: String, origin: String, operationId: String, now: Long): DriverLocalCommandEntity?

    @Query(
        "UPDATE driver_local_command SET lease_until_millis=:leaseUntil, updated_at_millis=:now " +
            "WHERE owner_account=:owner AND origin=:origin AND operation_id=:operationId " +
            "AND state='PENDING' AND next_eligible_millis<=:now AND lease_until_millis<=:now",
    )
    abstract suspend fun claim(owner: String, origin: String, operationId: String, now: Long, leaseUntil: Long): Int

    @Query(
        "UPDATE driver_local_command SET state=:state, lease_until_millis=0, last_error=:error, " +
            "updated_at_millis=:now WHERE owner_account=:owner AND origin=:origin AND operation_id=:operationId",
    )
    abstract suspend fun settle(owner: String, origin: String, operationId: String, state: String, error: String?, now: Long): Int

    @Query(
        "UPDATE driver_local_command SET state=:state, attempts=:attempts, next_eligible_millis=:nextEligible, " +
            "lease_until_millis=0, last_error=:error, updated_at_millis=:now " +
            "WHERE owner_account=:owner AND origin=:origin AND operation_id=:operationId",
    )
    abstract suspend fun retry(owner: String, origin: String, operationId: String, state: String, attempts: Int, nextEligible: Long, error: String, now: Long): Int
}
