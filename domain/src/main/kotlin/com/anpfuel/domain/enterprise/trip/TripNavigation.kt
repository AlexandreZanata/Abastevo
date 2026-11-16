package com.anpfuel.domain.enterprise.trip

/*
 * EMP-ROUTE-004 — navigation phase 1 (B-BR-EMP-532..535, EDR-345..347): which destination of the member's own mission or
 * trip may be handed to the app's own route preview. Pure: it opens nothing, records nothing and sends nothing.
 */

/** Why "Navegar" is not offered. */
enum class TripNavigationUnsupported {
    /** The trip or mission is not in the visible partition's own records. */
    NOT_OWN,
    /** The trip executes no mission. */
    NO_MISSION,
    /** The trip's mission is no longer in the missions copy. */
    MISSION_NOT_IN_COPY,
    /** The mission has no plan from the company. */
    NO_PLAN,
    /** The plan names a company unit, whose name and location the device does not hold. */
    UNIT_DESTINATION,
}

/** The hand-off: a destination text the driver confirms in the in-app search, or why there is none. */
sealed class TripNavigationTarget {
    class Search(val query: String) : TripNavigationTarget() {
        override fun toString(): String = "EnterpriseTripNavigationSearch(redacted)"
    }
    class Unsupported(val reason: TripNavigationUnsupported) : TripNavigationTarget() {
        override fun toString(): String = "EnterpriseTripNavigationUnsupported(${reason.name})"
    }
}

object TripNavigation {
    private fun ofItem(item: MissionInboxItem?): TripNavigationTarget {
        val plan = item?.plan ?: return TripNavigationTarget.Unsupported(TripNavigationUnsupported.NO_PLAN)
        val text = plan.destinationText ?: return TripNavigationTarget.Unsupported(TripNavigationUnsupported.UNIT_DESTINATION)
        return TripNavigationTarget.Search(text)
    }

    /** The destination of [missionId] in the partition's own copy (B-BR-EMP-532/533). */
    fun forMission(inbox: MissionInboxView?, missionId: String): TripNavigationTarget {
        val mission = inbox?.missions?.firstOrNull { it.missionId == missionId } ?: return TripNavigationTarget.Unsupported(TripNavigationUnsupported.NOT_OWN)
        if (mission.item == null) return TripNavigationTarget.Unsupported(TripNavigationUnsupported.MISSION_NOT_IN_COPY)
        return ofItem(mission.item)
    }

    /** The destination of the mission that [tripId] of the partition's own book executes, from its start's own reference. */
    fun forTrip(trips: List<TripView>, inbox: MissionInboxView?, tripId: String): TripNavigationTarget {
        val trip = trips.firstOrNull { it.trip.tripId == tripId } ?: return TripNavigationTarget.Unsupported(TripNavigationUnsupported.NOT_OWN)
        val mission = trip.missionId ?: return TripNavigationTarget.Unsupported(TripNavigationUnsupported.NO_MISSION)
        val item = inbox?.missions?.firstOrNull { it.missionId == mission }?.item
            ?: return TripNavigationTarget.Unsupported(TripNavigationUnsupported.MISSION_NOT_IN_COPY)
        return ofItem(item)
    }
}
