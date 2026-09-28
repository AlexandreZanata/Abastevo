package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import com.anpfuel.domain.valueobject.MunicipalityLocationKey
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class MunicipalityDataAvailabilityRuleTest {

    private val surveyWeekId = DomainId.forSurveyWeek(
        SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
    )

    private val saoPauloEntry = MunicipalityCatalogEntry(
        state = BrazilianState.SAO_PAULO,
        municipality = "SAO PAULO",
    )

    @Test
    fun resolvesHasDataWhenWeekContainsMunicipality() {
        val availability = MunicipalityDataAvailabilityRule.resolve(
            entry = saoPauloEntry,
            surveyWeekId = surveyWeekId,
            municipalitiesWithDataThisWeek = setOf(saoPauloEntry.locationKey),
            municipalitiesEverInAnp = setOf(saoPauloEntry.locationKey),
        )

        assertEquals(DataAvailability.HAS_DATA, availability)
    }

    @Test
    fun resolvesNoDataThisWeekWhenInAnpHistoryButNotCurrentWeek() {
        val availability = MunicipalityDataAvailabilityRule.resolve(
            entry = saoPauloEntry,
            surveyWeekId = surveyWeekId,
            municipalitiesWithDataThisWeek = emptySet(),
            municipalitiesEverInAnp = setOf(saoPauloEntry.locationKey),
        )

        assertEquals(DataAvailability.NO_DATA_THIS_WEEK, availability)
    }

    @Test
    fun resolvesNeverInAnpForIbgeOnlyMunicipality() {
        val catalogOnly = MunicipalityCatalogEntry(
            state = BrazilianState.ACRE,
            municipality = "ACRELANDIA",
        )

        val availability = MunicipalityDataAvailabilityRule.resolve(
            entry = catalogOnly,
            surveyWeekId = surveyWeekId,
            municipalitiesWithDataThisWeek = emptySet(),
            municipalitiesEverInAnp = emptySet(),
        )

        assertEquals(DataAvailability.NEVER_IN_ANP, availability)
    }
}
