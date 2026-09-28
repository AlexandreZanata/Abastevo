package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MunicipalityCatalogEntryTest {

    @Test
    fun validEntryExposesLocationKey() {
        val entry = MunicipalityCatalogEntry(
            state = BrazilianState.SAO_PAULO,
            municipality = "SAO PAULO",
            ibgeCode = "3550308",
        )

        assertEquals(
            MunicipalityLocationKey(BrazilianState.SAO_PAULO, "SAO PAULO"),
            entry.locationKey,
        )
    }

    @Test
    fun blankMunicipalityThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            MunicipalityCatalogEntry(
                state = BrazilianState.SAO_PAULO,
                municipality = "   ",
            )
        }
    }

    @Test
    fun invalidIbgeCodeThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            MunicipalityCatalogEntry(
                state = BrazilianState.SAO_PAULO,
                municipality = "SAO PAULO",
                ibgeCode = "35503",
            )
        }
    }

    @Test
    fun hasAnpDataForWeekReturnsTrueWhenLocationKeyPresent() {
        val entry = MunicipalityCatalogEntry(
            state = BrazilianState.SAO_PAULO,
            municipality = "SAO PAULO",
        )
        val surveyWeekId = DomainId.forSurveyWeek(
            SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
        )
        val weekData = setOf(entry.locationKey)

        assertTrue(entry.hasAnpDataForWeek(surveyWeekId, weekData))
    }

    @Test
    fun hasAnpDataForWeekReturnsFalseWhenLocationKeyAbsent() {
        val entry = MunicipalityCatalogEntry(
            state = BrazilianState.SAO_PAULO,
            municipality = "SAO PAULO",
        )
        val surveyWeekId = DomainId.forSurveyWeek(
            SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
        )

        assertFalse(entry.hasAnpDataForWeek(surveyWeekId, emptySet()))
    }
}
