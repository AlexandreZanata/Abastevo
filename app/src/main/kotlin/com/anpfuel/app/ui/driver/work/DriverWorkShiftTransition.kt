package com.anpfuel.app.ui.driver.work

import com.anpfuel.data.local.entity.DriverShiftEntity
import com.anpfuel.data.local.entity.DriverShiftGapEntity
import com.anpfuel.domain.driver.DriverClock
import com.anpfuel.domain.driver.DriverClockPolicy
import com.anpfuel.domain.driver.DriverShift
import com.anpfuel.domain.driver.DriverShiftAction
import com.anpfuel.domain.driver.DriverShiftCommand
import com.anpfuel.domain.driver.DriverShiftGap
import com.anpfuel.domain.driver.DriverShiftState

/** Provisional local policy; field and battery validation remain outstanding. */
internal object DriverWorkShiftTransition {
    private val policy = DriverClockPolicy(60_000, 1_000)

    fun apply(
        row: DriverShiftEntity,
        gaps: List<DriverShiftGapEntity>,
        action: DriverShiftAction,
        clock: DriverClock,
        operationId: String,
        gapId: String? = null,
        reviewedWorkMillis: Long? = null,
    ): Pair<DriverShiftEntity, List<DriverShiftGapEntity>> {
        val last = DriverClock(requireNotNull(row.lastBootId), requireNotNull(row.lastElapsedMillis), requireNotNull(row.lastUtcMillis))
        val shift = DriverShift(
            id = row.shiftId, owner = row.ownerAccount, origin = row.origin, vehicle = row.vehicleId,
            timezone = row.timezone, state = DriverShiftState.valueOf(row.state), version = row.version,
            last = last, activeMillis = row.activeMillis, manuallyReviewedMillis = row.manuallyReviewedMillis,
            gaps = gaps.map {
                DriverShiftGap(it.gapId, it.reason,
                    DriverClock(it.previousBootId, it.previousElapsedMillis, it.previousUtcMillis),
                    DriverClock(it.nextBootId, it.nextElapsedMillis, it.nextUtcMillis), it.reviewedWorkMillis)
            },
        )
        val next = shift.apply(DriverShiftCommand(operationId, action, row.version, clock, gapId = gapId, reviewedWorkMillis = reviewedWorkMillis), policy).shift
        val mapped = next.gaps.map { gap ->
            val original = gaps.firstOrNull { it.gapId == gap.id }
            DriverShiftGapEntity(
                row.ownerAccount, row.origin, row.shiftId, gap.id, gap.reason,
                gap.previous.bootId, gap.previous.elapsedMillis, gap.previous.utcMillis,
                gap.next.bootId, gap.next.elapsedMillis, gap.next.utcMillis,
                (original?.reviewed == true && original.reviewedWorkMillis != null) ||
                    (action == DriverShiftAction.REVIEW_GAP && gap.id == gapId && reviewedWorkMillis != null),
                gap.reviewedWorkMillis, original?.createdAtMillis ?: clock.utcMillis,
            )
        }
        return row.copy(
            state = next.state.name, version = next.version, lastBootId = next.last?.bootId,
            lastElapsedMillis = next.last?.elapsedMillis, lastUtcMillis = next.last?.utcMillis,
            activeMillis = next.activeMillis, manuallyReviewedMillis = next.manuallyReviewedMillis,
            updatedAtMillis = clock.utcMillis,
        ) to mapped
    }
}
