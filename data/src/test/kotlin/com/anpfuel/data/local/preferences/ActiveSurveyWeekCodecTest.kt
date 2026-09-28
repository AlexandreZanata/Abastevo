package com.anpfuel.data.local.preferences

import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class ActiveSurveyWeekCodecTest {

    @Test
    fun encodeDecodeRoundTripPreservesSurveyWeek() {
        val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

        val encoded = ActiveSurveyWeekCodec.encode(week)
        val decoded = ActiveSurveyWeekCodec.decode(encoded?.first, encoded?.second)

        assertEquals(week, decoded)
    }

    @Test
    fun encodeNullReturnsNull() {
        assertNull(ActiveSurveyWeekCodec.encode(null))
    }

    @Test
    fun decodeBlankDatesReturnsNull() {
        assertNull(ActiveSurveyWeekCodec.decode("", "2026-06-13"))
        assertNull(ActiveSurveyWeekCodec.decode("2026-06-07", null))
    }
}
