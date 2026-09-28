package com.anpfuel.domain.rule

/**
 * BR-008 — Station detail is synced by default; on-demand download only when the user opts out
 * and no local station data exists yet.
 */
object StationDetailOptInRule {

    fun requiresOnDemandDownload(
        syncStationDetail: Boolean,
        hasLocalStationData: Boolean,
    ): Boolean = !syncStationDetail && !hasLocalStationData
}
