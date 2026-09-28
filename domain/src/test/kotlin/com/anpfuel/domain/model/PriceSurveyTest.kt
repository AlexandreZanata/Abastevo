package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.time.Instant

class PriceSurveyTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    @Test
    fun createUsesDeterministicIdFromSurveyWeek() {
        val survey = PriceSurvey.create(surveyWeek)

        assertEquals(DomainId.forSurveyWeek(surveyWeek), survey.id)
        assertEquals(surveyWeek, survey.surveyWeek)
        assertFalse(survey.hasSummaryData)
        assertFalse(survey.hasStationData)
    }

    @Test
    fun markSummaryImportedRecordsTimestamp() {
        val survey = PriceSurvey.create(surveyWeek)
        val importedAt = Instant.parse("2026-06-14T10:00:00Z")

        survey.markSummaryImported(importedAt)

        assertTrue(survey.hasSummaryData)
        assertEquals(importedAt, survey.summaryImportedAt)
        assertTrue(survey.isReadyFor(PriceTableType.WEEKLY_SUMMARY))
    }

    @Test
    fun markStationImportedRequiresSummaryFirst() {
        val survey = PriceSurvey.create(surveyWeek)

        assertThrows(DomainException::class.java) {
            survey.markStationImported(Instant.parse("2026-06-14T11:00:00Z"))
        }
    }

    @Test
    fun markStationImportedAfterSummaryRecordsTimestamp() {
        val survey = PriceSurvey.create(surveyWeek)
        val summaryAt = Instant.parse("2026-06-14T10:00:00Z")
        val stationAt = Instant.parse("2026-06-14T11:00:00Z")

        survey.markSummaryImported(summaryAt)
        survey.markStationImported(stationAt)

        assertTrue(survey.hasStationData)
        assertEquals(stationAt, survey.stationImportedAt)
        assertTrue(survey.isReadyFor(PriceTableType.STATION_DETAIL))
    }

    @Test
    fun reimportUpdatesSummaryTimestampForImmutableHistoryFlow() {
        val survey = PriceSurvey.create(surveyWeek)
        val firstImport = Instant.parse("2026-06-14T10:00:00Z")
        val secondImport = Instant.parse("2026-06-15T10:00:00Z")

        survey.markSummaryImported(firstImport)
        survey.markSummaryImported(secondImport)

        assertEquals(secondImport, survey.summaryImportedAt)
    }
}
