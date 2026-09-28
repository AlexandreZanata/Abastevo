package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test
import java.time.LocalDate

class SurveyWeekCatalogEntryTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    @Test
    fun createStoresUrlsAndOptionalMetadata() {
        val entry = SurveyWeekCatalogEntry.create(
            surveyWeek = surveyWeek,
            summaryUrl = "https://example.com/resumo.xlsx",
            stationUrl = "https://example.com/revendas.xlsx",
            publishedAt = LocalDate.parse("2026-06-12"),
            operationalNote = "Belo Horizonte data gap",
        )

        assertEquals(surveyWeek, entry.surveyWeek)
        assertEquals(LocalDate.parse("2026-06-12"), entry.publishedAt)
        assertEquals("Belo Horizonte data gap", entry.operationalNote)
    }

    @Test
    fun blankSummaryUrlFailsAtCreation() {
        assertThrows(IllegalArgumentException::class.java) {
            SurveyWeekCatalogEntry.create(
                surveyWeek = surveyWeek,
                summaryUrl = " ",
                stationUrl = "https://example.com/revendas.xlsx",
            )
        }
    }

    @Test
    fun blankOperationalNoteFailsAtCreation() {
        assertThrows(DomainException::class.java) {
            SurveyWeekCatalogEntry.create(
                surveyWeek = surveyWeek,
                summaryUrl = "https://example.com/resumo.xlsx",
                stationUrl = "https://example.com/revendas.xlsx",
                operationalNote = "   ",
            )
        }
    }
}

class UserPreferencesActiveSurveyWeekTest {

    @Test
    fun activeSurveyWeekDefaultsToNull() {
        val preferences = UserPreferences()

        assertNull(preferences.activeSurveyWeek)
    }

    @Test
    fun activeSurveyWeekPersistsInCopy() {
        val week = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
        val preferences = UserPreferences(activeSurveyWeek = week)

        assertEquals(week, preferences.activeSurveyWeek)
    }
}
