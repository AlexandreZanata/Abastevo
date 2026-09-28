package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class SurveyWeekSelectedTest {

    @Test
    fun createStoresSurveyWeekAndSelectionMode() {
        val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
        val event = SurveyWeekSelected.create(
            payload = SurveyWeekSelected.Payload(
                surveyWeek = week,
                selectionMode = SurveyWeekSelectionMode.LATEST,
            ),
        )

        assertEquals(week, event.payload.surveyWeek)
        assertEquals(SurveyWeekSelectionMode.LATEST, event.payload.selectionMode)
    }
}
