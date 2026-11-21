package com.anpfuel.domain.enterprise.route

/*
 * Driving-mode interaction safety policy, heavy-vehicle CAR routing disclosure,
 * and degraded guidance modes (EMP-ROUTE-005, EMP-ROUTE-007, BR-ROUTE-008..012,
 * BR-ROUTE-018).
 */

/** Actions available during navigation; restricted while speed >= 1.5 m/s (BR-ROUTE-018). */
enum class DrivingQuickAction {
    CONFIRM_ARRIVAL, MUTED_VOICE_TOGGLE, EXTERNAL_NAV_HANDOFF, NEXT_STOP_OVERVIEW
}

enum class DetailedInteractionKind {
    FREE_TEXT_INPUT, COMPLEX_LIST_BROWSE, WAYPOINT_REORDER
}

/** Pure driving-mode rules enforced whenever speed is at least 1.5 m/s (150 cm/s). */
object DrivingModePolicy {
    const val MOVING_THRESHOLD_CM_S = 150
    const val MAX_QUICK_ACTIONS = 4

    fun isDriving(speedCmPerSecond: Int?): Boolean =
        speedCmPerSecond != null && speedCmPerSecond >= MOVING_THRESHOLD_CM_S

    fun isDetailedInteractionAllowed(speedCmPerSecond: Int?, kind: DetailedInteractionKind): Boolean =
        !isDriving(speedCmPerSecond)

    fun quickActionsFor(speedCmPerSecond: Int?, requested: List<DrivingQuickAction>): List<DrivingQuickAction> {
        val distinct = requested.distinct()
        return if (isDriving(speedCmPerSecond)) distinct.take(MAX_QUICK_ACTIONS) else distinct
    }
}

/** Asset profile used to surface explicit CAR routing limitation notice (BR-ROUTE-008). */
enum class EnterpriseVehicleProfile {
    LIGHT_PASSENGER, HEAVY_TRUCK, BUS, MACHINERY
}

data class HeavyVehicleWarning(
    val required: Boolean,
    val profileUsed: String = EnterpriseRouteComputation.PROFILE_CAR,
    val doesNotEnforceRestrictions: Boolean = true,
) {
    companion object {
        fun forProfile(profile: EnterpriseVehicleProfile, noticeFlag: Boolean): HeavyVehicleWarning =
            HeavyVehicleWarning(
                required = noticeFlag || profile != EnterpriseVehicleProfile.LIGHT_PASSENGER,
                profileUsed = EnterpriseRouteComputation.PROFILE_CAR,
                doesNotEnforceRestrictions = true,
            )
    }
}

/** Degraded states that keep the operational trip active and allow manual/external fallback (BR-ROUTE-012). */
enum class GuidanceDegradedState(
    val keepsTripActive: Boolean = true,
    val allowsExternalHandoff: Boolean = true,
) {
    MAP_REGION_MISSING,
    ROUTE_NOT_FOUND,
    NO_USABLE_FIX,
    VOICE_UNAVAILABLE_VISUAL_ONLY,
}
