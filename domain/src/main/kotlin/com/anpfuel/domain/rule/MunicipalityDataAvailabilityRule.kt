package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import com.anpfuel.domain.valueobject.MunicipalityLocationKey

/**
 * Resolves [DataAvailability] for a catalog entry at query time (UC-004 A4, BR-010).
 */
object MunicipalityDataAvailabilityRule {

    fun resolve(
        entry: MunicipalityCatalogEntry,
        surveyWeekId: DomainId,
        municipalitiesWithDataThisWeek: Set<MunicipalityLocationKey>,
        municipalitiesEverInAnp: Set<MunicipalityLocationKey>,
    ): DataAvailability {
        return when {
            entry.hasAnpDataForWeek(surveyWeekId, municipalitiesWithDataThisWeek) ->
                DataAvailability.HAS_DATA
            entry.locationKey in municipalitiesEverInAnp ->
                DataAvailability.NO_DATA_THIS_WEEK
            else ->
                DataAvailability.NEVER_IN_ANP
        }
    }
}
