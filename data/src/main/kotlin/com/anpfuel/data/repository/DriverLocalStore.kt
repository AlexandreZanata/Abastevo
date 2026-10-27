package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.DriverLocalDao
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
import javax.inject.Inject
import javax.inject.Singleton

/**
 * DRV-13 durable, owner-scoped driver input. Every method requires the exact
 * owner/origin pair; process restart re-reads only that partition. Drafts keep
 * integer metres/centavos/seconds and have no location or GPS-start field.
 */
@Singleton
class DriverLocalStore @Inject constructor(
    private val dao: DriverLocalDao,
) {
    suspend fun saveDraft(draft: DriverDraftEntity): DriverDraftEntity {
        require(draft.unitVersion.isNotBlank())
        dao.upsertDraft(draft)
        return draft
    }

    suspend fun findDraft(owner: String, origin: String, draftId: String): DriverDraftEntity? =
        dao.findDraft(owner, origin, draftId)

    suspend fun listDrafts(owner: String, origin: String): List<DriverDraftEntity> =
        dao.listDrafts(owner, origin)

    suspend fun saveEarning(
        earning: DriverEarningEntity,
        revision: DriverEarningRevisionEntity,
        command: DriverLocalCommandEntity,
    ) {
        require(earning.ownerAccount == revision.ownerAccount && earning.origin == revision.origin && earning.recordId == revision.recordId)
        require(earning.ownerAccount == command.ownerAccount && earning.origin == command.origin)
        dao.saveEarning(earning, revision, command)
    }

    suspend fun findEarning(owner: String, origin: String, recordId: String): DriverEarningEntity? =
        dao.findEarning(owner, origin, recordId)

    suspend fun listEarningRevisions(owner: String, origin: String, recordId: String): List<DriverEarningRevisionEntity> =
        dao.listEarningRevisions(owner, origin, recordId)

    suspend fun saveShift(shift: DriverShiftEntity, gaps: List<DriverShiftGapEntity>) {
        require(shift.ownerAccount == gaps.firstOrNull()?.ownerAccount ?: shift.ownerAccount)
        require(gaps.all { it.ownerAccount == shift.ownerAccount && it.origin == shift.origin && it.shiftId == shift.shiftId })
        dao.saveShift(shift, gaps)
    }

    /** Atomic owner-wide admission; unlike legacy saveShift this never replaces a row. */
    suspend fun startShift(shift: DriverShiftEntity) {
        require(shift.ownerAccount.isNotBlank() && shift.origin.isNotBlank() && shift.shiftId.isNotBlank())
        require(shift.state == "RUNNING" && shift.version == 1L)
        dao.startShift(shift)
    }

    /** Compare the persisted version and publish the shift and its gaps in one Room transaction. */
    suspend fun updateShift(shift: DriverShiftEntity, gaps: List<DriverShiftGapEntity>, expectedVersion: Long) {
        require(expectedVersion >= 1 && shift.version == Math.addExact(expectedVersion, 1))
        require(gaps.all { it.ownerAccount == shift.ownerAccount && it.origin == shift.origin && it.shiftId == shift.shiftId })
        dao.updateShift(shift, gaps, expectedVersion)
    }

    suspend fun currentShiftChecked(owner: String, origin: String): DriverShiftEntity? =
        dao.currentShiftChecked(owner, origin)

    suspend fun findShift(owner: String, origin: String, shiftId: String): DriverShiftEntity? =
        dao.findShift(owner, origin, shiftId)

    suspend fun listShiftGaps(owner: String, origin: String, shiftId: String): List<DriverShiftGapEntity> =
        dao.listShiftGaps(owner, origin, shiftId)

    suspend fun saveDistance(distance: DriverDistanceEntity, command: DriverLocalCommandEntity) {
        require(distance.ownerAccount == command.ownerAccount && distance.origin == command.origin)
        dao.saveDistance(distance, command)
    }

    suspend fun findDistance(owner: String, origin: String, id: String): DriverDistanceEntity? =
        dao.findDistance(owner, origin, id)

    suspend fun findCommand(owner: String, origin: String, operationId: String): DriverLocalCommandEntity? =
        dao.findCommand(owner, origin, operationId)

    suspend fun currentShift(owner: String, origin: String): DriverShiftEntity? = dao.currentShift(owner, origin)

    suspend fun savePayout(payout: DriverPayoutEntity) { dao.upsertPayout(payout) }
    suspend fun listPayouts(owner: String, origin: String): List<DriverPayoutEntity> = dao.listPayouts(owner, origin)

    suspend fun saveCostSetting(setting: DriverCostSettingEntity) { dao.upsertCostSetting(setting) }
    suspend fun findCostSetting(owner: String, origin: String, vehicleId: String): DriverCostSettingEntity? =
        dao.findCostSetting(owner, origin, vehicleId)

    suspend fun listEarnings(owner: String, origin: String, vehicleId: String): List<DriverEarningEntity> =
        dao.listEarnings(owner, origin, vehicleId)
    suspend fun overlapCount(owner: String, origin: String, vehicleId: String, platform: String, from: Long, to: Long): Int =
        dao.countOverlappingEarnings(owner, origin, vehicleId, platform, from, to)
    suspend fun listDistances(owner: String, origin: String, vehicleId: String): List<DriverDistanceEntity> =
        dao.listDistances(owner, origin, vehicleId)
    suspend fun saveGpsObservation(observation: DriverGpsObservationEntity) { dao.upsertGpsObservation(observation) }
    suspend fun latestGpsObservation(owner: String, origin: String, shiftId: String): DriverGpsObservationEntity? =
        dao.latestGpsObservation(owner, origin, shiftId)
    suspend fun saveReconciliation(row: DriverReconciliationEntity) { dao.upsertReconciliation(row) }
    suspend fun listReconciliations(owner: String, origin: String, vehicleId: String): List<DriverReconciliationEntity> =
        dao.listReconciliations(owner, origin, vehicleId)
    suspend fun saveGoal(goal: DriverGoalEntity) { dao.upsertGoal(goal) }
    suspend fun listGoals(owner: String, origin: String, vehicleId: String): List<DriverGoalEntity> =
        dao.listGoals(owner, origin, vehicleId)
    suspend fun saveMaintenance(item: DriverMaintenanceEntity) { dao.upsertMaintenance(item) }
    suspend fun listMaintenance(owner: String, origin: String, vehicleId: String): List<DriverMaintenanceEntity> =
        dao.listMaintenance(owner, origin, vehicleId)
}
